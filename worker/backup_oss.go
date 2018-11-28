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
	"errors"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

var errNotSupported = errors.New("Feature available only in Dgraph Enterprise Edition.")

// Backup implements the Worker interface.
func (w *grpcWorker) Backup(ctx context.Context, req *pb.BackupRequest) (*pb.Status, error) {
	glog.Infof("Backup failed: %s", errNotSupported)
	return &pb.Status{}, nil
}

// BackupOverNetwork handles a request coming from an HTTP client.
func BackupOverNetwork(pctx context.Context, target string) error {
	return x.Errorf("Backup failed: %s", errNotSupported)
}
