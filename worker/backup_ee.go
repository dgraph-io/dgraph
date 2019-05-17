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
	"net/http"
	"time"

	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// NOTE: This file is a self-contained source of callers of 'ee/backup' pkg.

func backupProcess(ctx context.Context, req *pb.BackupRequest) error {
	glog.Infof("Backup request: group %d at %d", req.GroupId, req.ReadTs)
	if err := ctx.Err(); err != nil {
		glog.Errorf("Context error during backup: %v\n", err)
		return err
	}
	g := groups()
	// sanity, make sure this is our group.
	if g.groupId() != req.GroupId {
		return errors.Errorf("Backup request group mismatch. Mine: %d. Requested: %d\n",
			g.groupId(), req.GroupId)
	}
	// wait for this node to catch-up.
	if err := posting.Oracle().WaitForTs(ctx, req.ReadTs); err != nil {
		return err
	}
	// Get snapshot to fill any gaps.
	snap, err := g.Node.Snapshot()
	if err != nil {
		return err
	}
	glog.V(3).Infof("Backup group %d snapshot: %+v", req.GroupId, snap)
	// Attach snapshot readTs to request to compare with any previous version.
	req.SnapshotTs = snap.ReadTs
	// create backup request and process it.
	br := &backup.Request{DB: pstore, Backup: req}
	return br.Process(ctx)
}

// Backup handles a request coming from another node.
func (w *grpcWorker) Backup(ctx context.Context, req *pb.BackupRequest) (*pb.Num, error) {
	var num pb.Num
	glog.V(2).Infof("Received backup request via Grpc: %+v", req)
	if err := backupProcess(ctx, req); err != nil {
		return nil, err
	}
	// num.Val is the max version value from `Backup()`
	num.Val = req.Since
	num.ReadOnly = true
	return &num, nil
}

func backupGroup(ctx context.Context, in *pb.BackupRequest) error {
	glog.V(2).Infof("Sending backup request: %+v\n", in)
	// this node is part of the group, process backup.
	if groups().groupId() == in.GroupId {
		return backupProcess(ctx, in)
	}

	// send request to any node in the group.
	pl := groups().AnyServer(in.GroupId)
	if pl == nil {
		return errors.Errorf("Couldn't find a server in group %d", in.GroupId)
	}
	res, err := pb.NewWorkerClient(pl.Get()).Backup(ctx, in)
	if err != nil {
		glog.Errorf("Backup error group %d: %s", in.GroupId, err)
		return err
	}
	// Attach distributed max version value
	in.Since = res.Val
	glog.V(2).Infof("Backup request to gid=%d, since=%d. OK\n", in.GroupId, in.Since)
	return nil
}

// BackupOverNetwork handles a request coming from an HTTP client.
func BackupOverNetwork(ctx context.Context, r *http.Request) error {
	destination := r.FormValue("destination")
	if destination == "" {
		return errors.Errorf("You must specify a 'destination' value")
	}

	accessKey := r.FormValue("access_key")
	secretKey := r.FormValue("secret_key")
	sessionToken := r.FormValue("session_token")
	anonymous := r.FormValue("anonymous") == "true"

	// Check that this node can accept requests.
	if err := x.HealthCheck(); err != nil {
		glog.Errorf("Backup canceled, not ready to accept requests: %s", err)
		return err
	}

	// Get ReadTs from zero and wait for stream to catch up.
	ts, err := Timestamps(ctx, &pb.Num{ReadOnly: true})
	if err != nil {
		glog.Errorf("Unable to retrieve readonly timestamp for backup: %s", err)
		return err
	}

	req := pb.BackupRequest{
		ReadTs:       ts.ReadOnly,
		Location:     destination,
		UnixTs:       time.Now().UTC().Format("20060102.150405"),
		AccessKey:    accessKey,
		SecretKey:    secretKey,
		SessionToken: sessionToken,
		Anonymous:    anonymous,
	}
	m := backup.Manifest{Groups: groups().KnownGroups()}
	glog.Infof("Created backup request: %s. Groups=%v\n", &req, m.Groups)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// This will dispatch the request to all groups and wait for their response.
	// If we receive any failures, we cancel the process.
	errCh := make(chan error, len(m.Groups))
	for _, gid := range m.Groups {
		req := req
		req.GroupId = gid
		go func(req *pb.BackupRequest) {
			errCh <- backupGroup(ctx, req)
			// req.Since is the max version result from Backup().
			m.Lock()
			if req.Since > m.Version {
				m.Version = req.Since
			}
			m.Unlock()
		}(&req)
	}

	for range m.Groups {
		if err := <-errCh; err != nil {
			// No changes, nothing was done.
			if err == backup.ErrBackupNoChanges {
				return nil
			}
			glog.Errorf("Error received during backup: %v", err)
			return err
		}
	}

	// The manifest completes the backup.
	br := &backup.Request{Backup: &req, Manifest: &m}
	return br.Complete(ctx)
}
