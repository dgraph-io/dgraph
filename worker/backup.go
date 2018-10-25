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
	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

// Backup handles a request coming from another node.
func (w *grpcWorker) Backup(ctx context.Context, req *pb.BackupRequest) (*pb.BackupResponse, error) {
	var resp pb.BackupResponse
	bw := &backup.Worker{
		ReadTs: req.StartTs,
		DB:     pstore,
	}
	if err := bw.Process(ctx); err != nil {
		resp.Status = pb.BackupResponse_FAILED
		return &resp, err
	}
	resp.Status = pb.BackupResponse_SUCCESS
	return &resp, nil
}

// BackupOverNetwork handles a request coming from an HTTP client.
func BackupOverNetwork(ctx context.Context) error {
	// Get ReadTs from zero and wait for stream to catch up.
	ts, err := Timestamps(ctx, &pb.Num{ReadOnly: true})
	if err != nil {
		glog.Errorf("Unable to retrieve readonly ts for export: %v\n", err)
		return err
	}
	readTs := ts.ReadOnly
	glog.Infof("Got readonly ts from Zero: %d\n", readTs)
	posting.Oracle().WaitForTs(ctx, readTs)

	// Let's first collect all groups.
	gids := groups().KnownGroups()
	glog.Infof("Requesting export for groups: %v\n", gids)

	return nil
}
