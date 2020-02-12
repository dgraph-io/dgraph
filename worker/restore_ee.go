// +build !oss

/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package worker

import (
	"context"

	"github.com/dgraph-io/dgraph/protos/pb"
)

// Restore implements the Worker interface.
func (w *grpcWorker) Restore(ctx context.Context, req *pb.Restore) (*pb.Status, error) {
	return nil, nil
}
