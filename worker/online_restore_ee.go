// +build !oss

/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package worker

import (
	"compress/gzip"
	"context"
	"io"
	"net/url"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// ProcessRestoreRequest verifies the backup data and sends a restore proposal to each group.
func ProcessRestoreRequest(ctx context.Context, req *pb.RestoreRequest) error {
	if req == nil {
		return errors.Errorf("restore request cannot be nil")
	}

	if err := UpdateMembershipState(ctx); err != nil {
		return errors.Wrapf(err, "cannot update membership state before restore")
	}
	memState := GetMembershipState()

	currentGroups := make([]uint32, 0)
	for gid := range memState.GetGroups() {
		currentGroups = append(currentGroups, gid)
	}

	creds := Credentials{
		AccessKey:    req.AccessKey,
		SecretKey:    req.SecretKey,
		SessionToken: req.SessionToken,
		Anonymous:    req.Anonymous,
	}
	if err := VerifyBackup(req.Location, req.BackupId, &creds, currentGroups); err != nil {
		return errors.Wrapf(err, "failed to verify backup")
	}

	if err := FillRestoreCredentials(req.Location, req); err != nil {
		return errors.Wrapf(err, "cannot fill restore proposal with the right credentials")
	}
	req.RestoreTs = State.GetTimestamp(false)

	// TODO: prevent partial restores when proposeRestoreOrSend only sends the restore
	// request to a subset of groups.
	for _, gid := range currentGroups {
		reqCopy := proto.Clone(req).(*pb.RestoreRequest)
		reqCopy.GroupId = gid
		if err := proposeRestoreOrSend(ctx, reqCopy); err != nil {
			// In case of an error, return but don't cancel the context.
			// After the timeout expires, the Done channel will be closed.
			// If the channel is closed due to a deadline issue, we can
			// ignore the requests for the groups that did not error out.
			return err
		}
	}

	return nil
}

func proposeRestoreOrSend(ctx context.Context, req *pb.RestoreRequest) error {
	if groups().ServesGroup(req.GetGroupId()) {
		_, err := (&grpcWorker{}).Restore(ctx, req)
		return err
	}

	pl := groups().Leader(req.GetGroupId())
	if pl == nil {
		return conn.ErrNoConnection
	}
	con := pl.Get()
	c := pb.NewWorkerClient(con)

	_, err := c.Restore(ctx, req)
	return err
}

// Restore implements the Worker interface.
func (w *grpcWorker) Restore(ctx context.Context, req *pb.RestoreRequest) (*pb.Status, error) {
	var emptyRes pb.Status
	if !groups().ServesGroup(req.GroupId) {
		return &emptyRes, errors.Errorf("this server doesn't serve group id: %v", req.GroupId)
	}

	// We should wait to ensure that we have seen all the updates until the StartTs
	// of this restore transaction.
	if err := posting.Oracle().WaitForTs(ctx, req.RestoreTs); err != nil {
		return nil, errors.Wrapf(err, "cannot wait for restore ts %d", req.RestoreTs)
	}

	err := groups().Node.proposeAndWait(ctx, &pb.Proposal{Restore: req})
	if err != nil {
		return &emptyRes, errors.Wrapf(err, "cannot propose restore request")
	}

	return &emptyRes, nil
}

// TODO(DGRAPH-1220): Online restores support passing the backup id.
// TODO(DGRAPH-1232): Ensure all groups receive the restore proposal.
func handleRestoreProposal(ctx context.Context, req *pb.RestoreRequest) error {
	if req == nil {
		return errors.Errorf("nil restore request")
	}

	// Drop all the current data. This also cancels all existing transactions.
	dropProposal := pb.Proposal{
		Mutations: &pb.Mutations{
			GroupId: req.GroupId,
			StartTs: req.RestoreTs,
			DropOp:  pb.Mutations_ALL,
		},
	}
	if err := groups().Node.applyMutations(ctx, &dropProposal); err != nil {
		return err
	}

	// TODO: after the drop, the tablets for the predicates stored in this group's
	// backup could be in a different group. The tablets need to be moved.

	// Reset tablets and set correct tablets to match the restored backup.
	creds := &Credentials{
		AccessKey:    req.AccessKey,
		SecretKey:    req.SecretKey,
		SessionToken: req.SessionToken,
		Anonymous:    req.Anonymous,
	}
	uri, err := url.Parse(req.Location)
	if err != nil {
		return errors.Wrapf(err, "cannot parse backup location")
	}
	handler, err := NewUriHandler(uri, creds)
	if err != nil {
		return errors.Wrapf(err, "cannot create backup handler")
	}

	manifests, err := handler.GetManifests(uri, req.BackupId)
	if err != nil {
		return errors.Wrapf(err, "cannot get backup manifests")
	}
	if len(manifests) == 0 {
		return errors.Errorf("no backup manifests found at location %s", req.Location)
	}
	lastManifest := manifests[len(manifests)-1]
	preds, ok := lastManifest.Groups[req.GroupId]
	if !ok {
		return errors.Errorf("backup manifest does not contain information for group ID %d",
			req.GroupId)
	}
	for _, pred := range preds {
		if tablet, err := groups().Tablet(pred); err != nil {
			return errors.Wrapf(err, "cannot create tablet for restored predicate %s", pred)
		} else if tablet.GetGroupId() != req.GroupId {
			return errors.Errorf("cannot assign tablet for pred %s to group %d", pred, req.GroupId)
		}
	}

	// Write restored values to disk and update the UID lease.
	if err := writeBackup(ctx, req); err != nil {
		return errors.Wrapf(err, "cannot write backup")
	}

	// Load schema back.
	if err := schema.LoadFromDb(); err != nil {
		return errors.Wrapf(err, "cannot load schema after restore")
	}

	// Propose a snapshot immediately after all the work is done to prevent the restore
	// from being replayed.
	if err := groups().Node.proposeSnapshot(1); err != nil {
		return errors.Wrapf(err, "cannot propose snapshot after processing restore proposal")
	}

	// Update the membership state to re-compute the group checksums.
	if err := UpdateMembershipState(ctx); err != nil {
		return errors.Wrapf(err, "cannot update membership state after restore")
	}
	return nil
}

func writeBackup(ctx context.Context, req *pb.RestoreRequest) error {
	res := LoadBackup(req.Location, req.BackupId,
		func(r io.Reader, groupId int, preds predicateSet) (uint64, error) {
			r, err := enc.GetReader(req.GetKeyFile(), r)
			if err != nil {
				return 0, errors.Wrapf(err, "cannot get encrypted reader")
			}
			gzReader, err := gzip.NewReader(r)
			if err != nil {
				return 0, errors.Wrapf(err, "cannot create gzip reader")
			}

			maxUid, err := loadFromBackup(pstore, gzReader, req.RestoreTs, preds)
			if err != nil {
				return 0, errors.Wrapf(err, "cannot write backup")
			}

			if maxUid == 0 {
				// No need to update the lease, return here.
				return 0, nil
			}

			// Use the value of maxUid to update the uid lease.
			pl := groups().connToZeroLeader()
			if pl == nil {
				return 0, errors.Errorf(
					"cannot update uid lease due to no connection to zero leader")
			}
			zc := pb.NewZeroClient(pl.Get())
			if _, err = zc.AssignUids(ctx, &pb.Num{Val: maxUid}); err != nil {
				return 0, errors.Wrapf(err, "cannot update max uid lease after restore.")
			}

			// We return the maxUid to enforce the signature of the method but it will
			// be ignored as the uid lease was updated above.
			return maxUid, nil
		})
	if res.Err != nil {
		return errors.Wrapf(res.Err, "cannot write backup")
	}
	return nil
}
