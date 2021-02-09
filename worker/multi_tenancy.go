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

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

// deleteNamespace handles a request coming from another node.
func (w *grpcWorker) DeleteNamespace(ctx context.Context,
	req *pb.DeleteNsRequest) (*pb.Status, error) {
	glog.V(2).Infof("Received delete namespace request via Grpc: %+v", req)
	return deleteNsCurrentGroup(ctx, req)
}

func ProcessDeleteNsRequest(ctx context.Context, ns uint64) error {
	if !EnterpriseEnabled() {
		return errors.New("you must enable enterprise features first. " +
			"Supply the appropriate license file to Dgraph Zero using the HTTP endpoint.")
	}

	if err := x.HealthCheck(); err != nil {
		glog.Errorf("Failed to delete Namespace, Server not ready to accept requests: %s", err)
		return err
	}

	// Update the membership state to get the latest mapping of groups to predicates.
	if err := UpdateMembershipState(ctx); err != nil {
		return err
	}

	// Get the current membership state and parse it for easier processing.
	state := GetMembershipState()

	for gid, _ := range state.Groups {
		req := &pb.DeleteNsRequest{
			Namespace: ns,
			GroupId:   gid,
		}
		deleteNamespace(ctx, req)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	return nil

}
func deleteNsCurrentGroup(ctx context.Context, req *pb.DeleteNsRequest) (*pb.Status, error) {
	glog.Infof("Delete namespace request: group %d for namespace %d", req.GroupId, req.Namespace)
	if err := ctx.Err(); err != nil {
		glog.Errorf("Context error during delete namespace: %v\n", err)
		return nil, err
	}

	g := groups()
	if g.groupId() != req.GroupId {
		return nil, errors.Errorf("Delete namespace request invalid, group Id: %d. Requested: %d\n",
			g.groupId(), req.GroupId)
	}

	closer, err := g.Node.startTask(opDeleteNS)
	if err != nil {
		return nil, errors.Wrapf(err, "Cannot perform delete namespace operation")
	}
	defer closer.Done()

	ps := State.Pstore
	if err := ps.BanNamespace(req.Namespace); err != nil {
		return &pb.Status{Code: 1, Msg: "Failed to delete the namespace"}, err
	}
	return &pb.Status{Code: 0, Msg: "Successfully deleted namespace"}, nil
}

func deleteNamespace(ctx context.Context, req *pb.DeleteNsRequest) (*pb.Status, error) {
	glog.V(2).Infof("Sending delete namespace request: %+v\n", req)
	if groups().groupId() == req.GroupId {
		return deleteNsCurrentGroup(ctx, req)
	}

	// This node is not part of the requested group, send the request over the network.
	pl := groups().AnyServer(req.GroupId)
	if pl == nil {
		return nil, errors.Errorf("Couldn't find a server in group %d", req.GroupId)
	}
	res, err := pb.NewWorkerClient(pl.Get()).DeleteNamespace(ctx, req)
	if err != nil {
		glog.Errorf("Delete namespace error, group %d: %s", req.GroupId, err)
		return nil, err
	}
	return res, nil
}
