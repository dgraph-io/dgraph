//go:build oss
// +build oss

/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"sync"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
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

func handleRestoreProposal(ctx context.Context, req *pb.RestoreRequest) error {
	return nil
}
