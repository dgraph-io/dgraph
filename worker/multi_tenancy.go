//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"

	"github.com/hypermodeinc/dgraph/v24/protos/pb"
	"github.com/hypermodeinc/dgraph/v24/x"
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
