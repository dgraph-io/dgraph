//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"

	"github.com/golang/glog"

	"github.com/hypermodeinc/dgraph/v24/protos/pb"
	"github.com/hypermodeinc/dgraph/v24/x"
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

func ProcessListBackups(ctx context.Context, location string, creds *x.MinioCredentials) ([]*Manifest, error) {
	return nil, x.ErrNotSupported
}
