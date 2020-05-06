// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package worker

import (
	"context"
	"net/url"
	"sort"
	"time"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// Backup handles a request coming from another node.
func (w *grpcWorker) Backup(ctx context.Context, req *pb.BackupRequest) (*pb.Status, error) {
	glog.V(2).Infof("Received backup request via Grpc: %+v", req)
	return backupCurrentGroup(ctx, req)
}

func backupCurrentGroup(ctx context.Context, req *pb.BackupRequest) (*pb.Status, error) {
	glog.Infof("Backup request: group %d at %d", req.GroupId, req.ReadTs)
	if err := ctx.Err(); err != nil {
		glog.Errorf("Context error during backup: %v\n", err)
		return nil, err
	}

	g := groups()
	if g.groupId() != req.GroupId {
		return nil, errors.Errorf("Backup request group mismatch. Mine: %d. Requested: %d\n",
			g.groupId(), req.GroupId)
	}

	if err := posting.Oracle().WaitForTs(ctx, req.ReadTs); err != nil {
		return nil, err
	}

	bp := &BackupProcessor{DB: pstore, Request: req}
	return bp.WriteBackup(ctx)
}

// BackupGroup backs up the group specified in the backup request.
func BackupGroup(ctx context.Context, in *pb.BackupRequest) (*pb.Status, error) {
	glog.V(2).Infof("Sending backup request: %+v\n", in)
	if groups().groupId() == in.GroupId {
		return backupCurrentGroup(ctx, in)
	}

	// This node is not part of the requested group, send the request over the network.
	pl := groups().AnyServer(in.GroupId)
	if pl == nil {
		return nil, errors.Errorf("Couldn't find a server in group %d", in.GroupId)
	}
	res, err := pb.NewWorkerClient(pl.Get()).Backup(ctx, in)
	if err != nil {
		glog.Errorf("Backup error group %d: %s", in.GroupId, err)
		return nil, err
	}

	return res, nil
}

func ProcessBackupRequest(ctx context.Context, req *pb.BackupRequest, forceFull bool) error {
	if !EnterpriseEnabled() {
		return errors.New("you must enable enterprise features first. " +
			"Supply the appropriate license file to Dgraph Zero using the HTTP endpoint.")
	}

	if req.Destination == "" {
		return errors.Errorf("you must specify a 'destination' value")
	}

	if err := x.HealthCheck(); err != nil {
		glog.Errorf("Backup canceled, not ready to accept requests: %s", err)
		return err
	}

	ts, err := Timestamps(ctx, &pb.Num{ReadOnly: true})
	if err != nil {
		glog.Errorf("Unable to retrieve readonly timestamp for backup: %s", err)
		return err
	}

	req.ReadTs = ts.ReadOnly
	req.UnixTs = time.Now().UTC().Format("20060102.150405.000")

	// Read the manifests to get the right timestamp from which to start the backup.
	uri, err := url.Parse(req.Destination)
	if err != nil {
		return err
	}
	handler, err := NewUriHandler(uri, GetCredentialsFromRequest(req))
	if err != nil {
		return err
	}
	latestManifest, err := handler.GetLatestManifest(uri)
	if err != nil {
		return err
	}

	req.SinceTs = latestManifest.Since
	if forceFull {
		req.SinceTs = 0
	} else {
		if Config.BadgerKeyFile != "" {
			// If encryption turned on, latest backup should be encrypted.
			if latestManifest.Type != "" && !latestManifest.Encrypted {
				err = errors.Errorf("latest manifest indicates the last backup was not encrypted " +
					"but this instance has encryption turned on. Try \"forceFull\" flag.")
				return err
			}
		} else {
			// If encryption turned off, latest backup should be unencrypted.
			if latestManifest.Type != "" && latestManifest.Encrypted {
				err = errors.Errorf("latest manifest indicates the last backup was encrypted " +
					"but this instance has encryption turned off. Try \"forceFull\" flag.")
				return err
			}
		}
	}

	// Update the membership state to get the latest mapping of groups to predicates.
	if err := UpdateMembershipState(ctx); err != nil {
		return err
	}

	// Get the current membership state and parse it for easier processing.
	state := GetMembershipState()
	var groups []uint32
	predMap := make(map[uint32][]string)
	for gid, group := range state.Groups {
		groups = append(groups, gid)
		predMap[gid] = make([]string, 0)
		for pred := range group.Tablets {
			predMap[gid] = append(predMap[gid], pred)
		}
	}

	glog.Infof("Created backup request: %s. Groups=%v\n", req, groups)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(state.Groups))
	for _, gid := range groups {
		br := proto.Clone(req).(*pb.BackupRequest)
		br.GroupId = gid
		br.Predicates = predMap[gid]
		go func(req *pb.BackupRequest) {
			_, err := BackupGroup(ctx, req)
			errCh <- err
		}(br)
	}

	for range groups {
		if err := <-errCh; err != nil {
			glog.Errorf("Error received during backup: %v", err)
			return err
		}
	}

	m := Manifest{Since: req.ReadTs, Groups: predMap}
	if req.SinceTs == 0 {
		m.Type = "full"
		m.BackupId = x.GetRandomName(1)
		m.BackupNum = 1
	} else {
		m.Type = "incremental"
		m.BackupId = latestManifest.BackupId
		m.BackupNum = latestManifest.BackupNum + 1
	}
	if Config.BadgerKeyFile != "" {
		m.Encrypted = true
	}

	bp := &BackupProcessor{Request: req}
	return bp.CompleteBackup(ctx, &m)
}

func ProcessListBackups(ctx context.Context, location string, creds *Credentials) (
	[]*Manifest, error) {

	manifests, err := ListBackupManifests(location, creds)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read manfiests at location %s", location)
	}

	res := make([]*Manifest, 0)
	for _, m := range manifests {
		res = append(res, m)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Path < res[j].Path })
	return res, nil
}
