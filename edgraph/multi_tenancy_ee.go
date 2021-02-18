// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package edgraph

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type ResetPasswordInput struct {
	UserID    string
	Password  string
	Namespace uint64
}

func (s *Server) ResetPassword(ctx context.Context, inp *ResetPasswordInput) error {
	query := fmt.Sprintf(`{
			x as updateUser(func: eq(dgraph.xid, "%s")) @filter(type(dgraph.type.User)) {
				uid
			}
		}`, inp.UserID)

	userNQuads := []*api.NQuad{
		{
			Subject:     "uid(x)",
			Predicate:   "dgraph.password",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: inp.Password}},
		},
	}
	req := &Request{
		req: &api.Request{
			CommitNow: true,
			Query:     query,
			Mutations: []*api.Mutation{
				{
					Set:  userNQuads,
					Cond: "@if(gt(len(x), 0))",
				},
			},
		},
		doAuth: NoAuthorize,
	}
	ctx = x.AttachNamespace(ctx, inp.Namespace)
	resp, err := (&Server{}).doQuery(ctx, req)
	if err != nil {
		return errors.Wrapf(err, "Reset password for user %s in namespace %d, got error:",
			inp.UserID, inp.Namespace)
	}

	type userNode struct {
		Uid string `json:"uid"`
	}

	type userQryResp struct {
		User []userNode `json:"updateUser"`
	}
	var userResp userQryResp
	if err := json.Unmarshal(resp.GetJson(), &userResp); err != nil {
		return errors.Wrap(err, "Reset password failed with error")
	}

	if len(userResp.User) == 0 {
		return errors.New("Failed to reset password, user doesn't exist")
	}
	return nil
}

// CreateNamespace creates a new namespace. Only guardian of galaxy is authorized to do so.
// Authorization is handled by middlewares.
func (s *Server) CreateNamespace(ctx context.Context, passwd string) (uint64, error) {
	glog.V(2).Info("Got create namespace request.")

	num := &pb.Num{Val: 1, Type: pb.Num_NS_ID}
	ids, err := worker.AssignNsIdsOverNetwork(ctx, num)
	if err != nil {
		return 0, errors.Wrapf(err, "Creating namespace, got error:")
	}

	ns := ids.StartId
	glog.V(2).Infof("Got a lease for NsID: %d", ns)

	// Attach the newly leased NsID in the context in order to create guardians/groot for it.
	ctx = x.AttachNamespace(ctx, ns)
	m := &pb.Mutations{StartTs: worker.State.GetTimestamp(false)}
	m.Schema = schema.InitialSchema(ns)
	m.Types = schema.InitialTypes(ns)
	_, err = query.ApplyMutations(ctx, m)
	if err != nil {
		return 0, err
	}

	if err = worker.WaitForIndexing(ctx, true); err != nil {
		return 0, errors.Wrap(err, "Creating namespace, got error: ")
	}
	if err := createGuardianAndGroot(ctx, ids.StartId, passwd); err != nil {
		return 0, errors.Wrapf(err, "Failed to create guardian and groot: ")
	}
	glog.V(2).Infof("Created namespace: %d", ns)
	return ns, nil
}

// This function is used while creating new namespace. New namespace creation is only allowed
// by the guardians of the galaxy group.
func createGuardianAndGroot(ctx context.Context, namespace uint64, passwd string) error {
	if err := upsertGuardian(ctx); err != nil {
		return errors.Wrap(err, "While creating Guardian")
	}
	if err := upsertGroot(ctx, passwd); err != nil {
		return errors.Wrap(err, "While creating Groot")
	}
	return nil
}

// DeleteNamespace deletes a new namespace. Only guardian of galaxy is authorized to do so.
// Authorization is handled by middlewares.
func (s *Server) DeleteNamespace(ctx context.Context, namespace uint64) error {
	glog.Info("Deleting namespace", namespace)
	return worker.ProcessDeleteNsRequest(ctx, namespace)
}
