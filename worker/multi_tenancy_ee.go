//go:build !oss
// +build !oss

/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
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
	// Update the membership state to get the latest mapping of groups to predicates.
	if err := UpdateMembershipState(ctx); err != nil {
		return errors.Wrapf(err, "Failed to update membership state while deleting namesapce")
	}

	state := GetMembershipState()
	g := new(errgroup.Group)

	for gid := range state.Groups {
		req := &pb.DeleteNsRequest{Namespace: ns, GroupId: gid}
		g.Go(func() error {
			return x.RetryUntilSuccess(10, 100*time.Millisecond, func() error {
				return proposeDeleteOrSend(ctx, req)
			})
		})
	}

	if err := g.Wait(); err != nil {
		return errors.Wrap(err, "Failed to process delete request")
	}

	// Now propose the change to zero.
	return x.RetryUntilSuccess(10, 100*time.Millisecond, func() error {
		return sendDeleteToZero(ctx, ns)
	})
}

func sendDeleteToZero(ctx context.Context, ns uint64) error {
	gr := groups()
	pl := gr.connToZeroLeader()
	if pl == nil {
		return conn.ErrNoConnection
	}
	zc := pb.NewZeroClient(pl.Get())
	_, err := zc.DeleteNamespace(gr.Ctx(), &pb.DeleteNsRequest{Namespace: ns})
	return err
}

func proposeDeleteOrSend(ctx context.Context, req *pb.DeleteNsRequest) error {
	glog.V(2).Infof("Sending delete namespace request: %+v", req)
	if groups().ServesGroup(req.GetGroupId()) && groups().Node.AmLeader() {
		_, err := (&grpcWorker{}).DeleteNamespace(ctx, req)
		return err
	}

	pl := groups().Leader(req.GetGroupId())
	if pl == nil {
		return conn.ErrNoConnection
	}
	c := pb.NewWorkerClient(pl.Get())
	_, err := c.DeleteNamespace(ctx, req)
	return err
}
