//go:build oss
// +build oss

/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

func (w *grpcWorker) DeleteNamespace(ctx context.Context,
	req *pb.DeleteNsRequest) (*pb.Status, error) {
	return nil, x.ErrNotSupported
}

func ProcessDeleteNsRequest(ctx context.Context, ns uint64) error {
	return x.ErrNotSupported
}

func proposeDeleteOrSend(ctx context.Context, req *pb.DeleteNsRequest) error {
	return nil
}
