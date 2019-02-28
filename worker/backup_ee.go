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
	"time"

	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"golang.org/x/net/context"
)

// NOTE: This file is a self-contained source of callers of 'ee/backup' pkg.

func backupProcess(ctx context.Context, req *pb.BackupRequest) error {
	glog.Infof("Backup request: group %d at %d", req.GroupId, req.ReadTs)
	if err := ctx.Err(); err != nil {
		glog.Errorf("Context error during backup: %v\n", err)
		return err
	}
	// sanity, make sure this is our group.
	if groups().groupId() != req.GroupId {
		return x.Errorf("Backup request group mismatch. Mine: %d. Requested: %d\n",
			groups().groupId(), req.GroupId)
	}
	// wait for this node to catch-up.
	if err := posting.Oracle().WaitForTs(ctx, req.ReadTs); err != nil {
		return err
	}
	// create backup request and process it.
	br := &backup.Request{DB: pstore, Backup: req}
	// calculate estimated upload size
	for _, t := range groups().tablets {
		if t.GroupId == req.GroupId {
			br.Sizex += uint64(float64(t.Space) * 1.2)
		}
	}
	return br.Process(ctx)
}

// Backup handles a request coming from another node.
func (w *grpcWorker) Backup(ctx context.Context, req *pb.BackupRequest) (*pb.Status, error) {
	var resp pb.Status
	glog.V(2).Infof("Received backup request via Grpc: %+v", req)
	if err := backupProcess(ctx, req); err != nil {
		resp.Code = -1
		resp.Msg = err.Error()
		return &resp, err
	}
	return &resp, nil
}

func backupGroup(ctx context.Context, in pb.BackupRequest) error {
	glog.V(2).Infof("Sending backup request: %+v\n", in)
	// this node is part of the group, process backup.
	if groups().groupId() == in.GroupId {
		return backupProcess(ctx, &in)
	}

	// send request to any node in the group.
	pl := groups().AnyServer(in.GroupId)
	if pl == nil {
		return x.Errorf("Couldn't find a server in group %d", in.GroupId)
	}
	status, err := pb.NewWorkerClient(pl.Get()).Backup(ctx, &in)
	if err != nil {
		glog.Errorf("Backup error group %d: %s", in.GroupId, err)
		return err
	}
	if status.Code != 0 {
		err := x.Errorf("Backup error group %d: %s", in.GroupId, status.Msg)
		glog.Errorln(err)
		return err
	}
	glog.V(2).Infof("Backup request to gid=%d. OK\n", in.GroupId)
	return nil
}

// BackupOverNetwork handles a request coming from an HTTP client.
func BackupOverNetwork(pctx context.Context, dst string) error {
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

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

	gids := groups().KnownGroups()
	req := pb.BackupRequest{
		ReadTs:   ts.ReadOnly,
		Location: dst,
		UnixTs:   time.Now().UTC().Format("20060102.1504"),
	}
	glog.Infof("Created backup request: %+v. Groups=%v\n", req, gids)

	// This will dispatch the request to all groups and wait for their response.
	// If we receive any failures, we cancel the process.
	errCh := make(chan error, len(gids))
	for _, gid := range gids {
		req.GroupId = gid
		go func() {
			errCh <- backupGroup(ctx, req)
		}()
	}

	for i := 0; i < len(gids); i++ {
		err := <-errCh
		if err != nil {
			glog.Errorf("Error received during backup: %v", err)
			return err
		}
	}
	req.GroupId = 0
	glog.Infof("Backup completed OK.")
	return nil
}
