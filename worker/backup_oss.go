//go:build oss
// +build oss

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
	"context"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

// Backup implements the Worker interface.
func (w *grpcWorker) Backup(ctx context.Context, req *pb.BackupRequest) (*pb.BackupResponse, error) {
	glog.Warningf("Backup failed: %v", x.ErrNotSupported)
	return nil, x.ErrNotSupported
}

func ProcessBackupRequest(ctx context.Context, req *pb.BackupRequest) error {
	glog.Warningf("Backup failed: %v", x.ErrNotSupported)
	return x.ErrNotSupported
}

func ProcessListBackups(ctx context.Context, location string, creds *x.MinioCredentials) (
	[]*Manifest, error) {

	return nil, x.ErrNotSupported
}
