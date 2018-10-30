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

func backupProcess(ctx context.Context, in *pb.BackupRequest) (*pb.BackupResponse, error) {
	resp := &pb.BackupResponse{Status: pb.BackupResponse_FAILED}
	if err := ctx.Err(); err != nil {
		glog.Errorf("Context error during backup: %v\n", err)
		resp.Message = err.Error()
		return resp, err
	}
	// sanity, make sure this is our group.
	if groups().groupId() != in.GroupId {
		err := x.Errorf("Backup request group mismatch. Mine: %d. Requested: %d\n",
			groups().groupId(), in.GroupId)
		resp.Message = err.Error()
		return resp, err
	}
	glog.Infof("Export requested at %d.", in.StartTs)

	// wait for this node to catch-up.
	if err := posting.Oracle().WaitForTs(ctx, in.StartTs); err != nil {
		resp.Message = err.Error()
		return resp, err
	}
	glog.Infof("Running export for group %d at timestamp %d.", in.GroupId, in.StartTs)

	// process this request
	w := &backup.Worker{
		ReadTs:  in.StartTs,
		GroupId: in.GroupId,
		SeqTs:   fmt.Sprint(time.Now().UTC().UnixNano()),
		DB:      pstore,
	}
	if err := w.Process(ctx); err != nil {
		resp.Message = err.Error()
		return resp, err
	}
	resp.Status = pb.BackupResponse_SUCCESS
	return resp, nil
}

// Backup handles a request coming from another node.
func (w *grpcWorker) Backup(ctx context.Context, req *pb.BackupRequest) (*pb.BackupResponse, error) {
	glog.Infof("Received backup request via Grpc: %+v\n", req)
	return backupProcess(ctx, req)
}

// TODO: add stop to all goroutines to cancel on failure.
func backupDispatch(ctx context.Context, readTs uint64, gids ...uint32) chan *pb.BackupResponse {
	out := make(chan *pb.BackupResponse)
	go func() {
		glog.Infof("Dispatching backup requests...")
		for _, gid := range gids {
			in := &pb.BackupRequest{StartTs: readTs, GroupId: gid}
			// this node is part of the group, process backup.
			if groups().groupId() == gid {
				resp, err := backupProcess(ctx, in)
				if err != nil {
					glog.Errorf("Error while running backup: %v", err)
				}
				out <- resp
				continue
			}
			// send request to any node in the group.
			pl := groups().AnyServer(gid)
			c := pb.NewWorkerClient(pl.Get())
			resp, err := c.Backup(ctx, in)
			if err != nil {
				glog.Errorf("Backup error received from group: %d. Error: %v\n", gid, err)
			}
			out <- resp
		}
		close(out)
	}()
	return out
}

// BackupOverNetwork handles a request coming from an HTTP client.
func BackupOverNetwork(ctx context.Context) error {
	// Check that this node can accept requests.
	if err := x.HealthCheck(); err != nil {
		glog.Errorf("Backup canceled, not ready to accept requests: %v\n", err)
		return err
	}
	// Get ReadTs from zero and wait for stream to catch up.
	ts, err := Timestamps(ctx, &pb.Num{ReadOnly: true})
	if err != nil {
		glog.Errorf("Unable to retrieve readonly ts for backup: %v\n", err)
		return err
	}
	readTs := ts.ReadOnly
	glog.Infof("Got readonly ts from Zero: %d\n", readTs)
	posting.Oracle().WaitForTs(ctx, readTs)

	// Let's first collect all groups.
	gids := groups().KnownGroups()
	glog.Infof("Requesting backup for groups: %v\n", gids)

	// This will dispatch the request to all groups and wait for their response.
	// If we receive any failures, we cancel the process.
	for resp := range backupDispatch(ctx, readTs, gids...) {
		if resp.Status == pb.BackupResponse_FAILED {
			return x.Errorf("Backup error: %s", resp.Message)
		}
	}
	return nil
}
