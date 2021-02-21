// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/pkg/errors"
)

func (w *grpcWorker) DeleteNamespace(ctx context.Context,
	req *pb.DeleteNsRequest) (*pb.Status, error) {
	var emptyRes pb.Status
	if !groups().ServesGroup(req.GroupId) {
		return &emptyRes, errors.Errorf("The server doesn't serve group id: %v", req.GroupId)
	}
	if err := groups().Node.proposeAndWait(ctx, &pb.Proposal{DeleteNs: req}); err != nil {
		return &emptyRes, errors.Wrapf(err, "Delete namespace failed for namespace %d on group %d",
			req.Namespace, req.GroupId)
	}
	return &emptyRes, nil
}

func ProcessDeleteNsRequest(ctx context.Context, ns uint64) error {
	gr := groups()
	pl := gr.connToZeroLeader()
	if pl == nil {
		return conn.ErrNoConnection
	}
	zc := pb.NewZeroClient(pl.Get())
	if _, err := zc.DeleteNamespace(gr.Ctx(), &pb.DeleteNsRequest{Namespace: ns}); err != nil {
		return err
	}
	return nil
}
