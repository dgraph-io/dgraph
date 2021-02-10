// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

	if err := groups().Node.proposeAndWait(ctx, &pb.Proposal{Delete: req}); err != nil {
		return &emptyRes, errors.Wrapf(err, "Delete namespace error: ")
	}
	return &emptyRes, nil
}

func ProcessDeleteNsRequest(ctx context.Context, ns uint64) error {
	// Update the membership state to get the latest mapping of groups to predicates.
	if err := UpdateMembershipState(ctx); err != nil {
		return errors.Wrapf(err, "Failed to update membership state while deleting namesapce")
	}

	state := GetMembershipState()

	errCh := make(chan error, len(state.Groups))
	for gid, _ := range state.Groups {
		req := &pb.DeleteNsRequest{Namespace: ns, GroupId: gid}
		go func(req *pb.DeleteNsRequest) {
			errCh <- tryDeleteProposal(ctx, req)
		}(req)
	}

	for range state.Groups {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

// TODO - Currently there is no way by which we can prevent partial banning. It is possible
// that if a group is behind a network partition then we might not be able to propose to it.
func tryDeleteProposal(ctx context.Context, req *pb.DeleteNsRequest) error {
	var err error
	for i := 0; i < 10; i++ {
		err = proposeDeleteOrSend(ctx, req)
		if err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return err
}

func proposeDeleteOrSend(ctx context.Context, req *pb.DeleteNsRequest) error {
	if groups().ServesGroup(req.GetGroupId()) && groups().Node.AmLeader() {
		_, err := (&grpcWorker{}).DeleteNamespace(ctx, req)
		return err
	}

	pl := groups().Leader(req.GetGroupId())
	if pl == nil {
		return conn.ErrNoConnection
	}
	con := pl.Get()
	c := pb.NewWorkerClient(con)

	_, err := c.DeleteNamespace(ctx, req)
	return err
}
