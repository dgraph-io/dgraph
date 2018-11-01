/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"fmt"
	"time"

	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

func backupProcess(ctx context.Context, req *pb.BackupRequest) error {
	glog.Infof("Backup request: group %d at %d", req.GroupId, req.ReadTs)
	if err := ctx.Err(); err != nil {
		glog.Errorf("Context error during backup: %v\n", err)
		return err
	}
	// sanity, make sure this is our group.
	if groups().groupId() != req.GroupId {
		err := x.Errorf("Backup request group mismatch. Mine: %d. Requested: %d\n",
			groups().groupId(), req.GroupId)
		return err
	}
	// wait for this node to catch-up.
	if err := posting.Oracle().WaitForTs(ctx, req.ReadTs); err != nil {
		return err
	}
	// create backup request and process it.
	br := &backup.Request{
		ReadTs:    req.ReadTs,
		GroupId:   req.GroupId,
		UnixTs:    fmt.Sprint(time.Now().UTC().UnixNano()),
		TargetURI: req.Target,
		DB:        pstore,
	}
	if err := br.Process(ctx); err != nil {
		return err
	}
	return nil
}

// Backup handles a request coming from another node.
func (w *grpcWorker) Backup(ctx context.Context, req *pb.BackupRequest) (*pb.Status, error) {
	var resp pb.Status
	glog.V(3).Infof("Received backup request via Grpc: %+v", req)
	if err := backupProcess(ctx, req); err != nil {
		resp.Code = -1
		resp.Msg = err.Error()
		return &resp, err
	}
	return &resp, nil
}

// TODO: add stop to all goroutines to cancel on failure.
func backupDispatch(ctx context.Context, in *pb.BackupRequest, gids []uint32) chan error {
	statusCh := make(chan error)
	go func() {
		glog.Infof("Dispatching backup requests...")
		for _, gid := range gids {
			glog.V(3).Infof("dispatching to group %d snapshot at %d", gid, in.ReadTs)
			in := in
			in.GroupId = gid
			// this node is part of the group, process backup.
			if groups().groupId() == gid {
				statusCh <- backupProcess(ctx, in)
				continue
			}
			// send request to any node in the group.
			pl := groups().AnyServer(gid)
			if pl == nil {
				statusCh <- x.Errorf("Couldn't find a server in group %d", gid)
				continue
			}
			_, err := pb.NewWorkerClient(pl.Get()).Backup(ctx, in)
			if err != nil {
				glog.Errorf("Backup error group %d: %s", gid, err)
			}
			statusCh <- err
		}
		close(statusCh)
	}()
	return statusCh
}

// BackupOverNetwork handles a request coming from an HTTP client.
func BackupOverNetwork(ctx context.Context, target string) error {
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
	readTs := ts.ReadOnly
	glog.Infof("Got readonly ts from Zero: %d", readTs)
	if err := posting.Oracle().WaitForTs(ctx, readTs); err != nil {
		glog.Errorf("Error while waiting for ts: %s", err)
		return err
	}

	// Let's first collect all groups.
	gids := groups().KnownGroups()
	glog.Infof("Requesting backup for groups: %v", gids)

	// This will dispatch the request to all groups and wait for their response.
	// If we receive any failures, we cancel the process.
	req := &pb.BackupRequest{ReadTs: readTs, Target: target}
	for err := range backupDispatch(ctx, req, gids) {
		if err != nil {
			glog.Errorf("Error while running backup: %s", err)
			return err
		}
	}
	glog.Infof("Backup done.")
	return nil
}
