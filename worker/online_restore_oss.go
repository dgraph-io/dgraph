//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"
	"sync"

	"github.com/golang/glog"

	"github.com/hypermodeinc/dgraph/v24/protos/pb"
	"github.com/hypermodeinc/dgraph/v24/x"
)

func ProcessRestoreRequest(ctx context.Context, req *pb.RestoreRequest, wg *sync.WaitGroup) error {
	glog.Warningf("Restore failed: %v", x.ErrNotSupported)
	return x.ErrNotSupported
}

// Restore implements the Worker interface.
func (w *grpcWorker) Restore(ctx context.Context, req *pb.RestoreRequest) (*pb.Status, error) {
	glog.Warningf("Restore failed: %v", x.ErrNotSupported)
	return &pb.Status{}, x.ErrNotSupported
}

func handleRestoreProposal(ctx context.Context, req *pb.RestoreRequest, pidx uint64) error {
	return nil
}
