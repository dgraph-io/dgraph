// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
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
	"fmt"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/golang/glog"
	"google.golang.org/grpc/peer"
	"time"

	otrace "go.opencensus.io/trace"
)

func (s *Server) Login(ctx context.Context,
	request *api.LogInRequest) (*api.LogInResponse, error) {
	ctx, span := otrace.StartSpan(ctx, "server.LogIn")
	defer span.End()

	// record the client ip for this login request
	clientPeer, ok := peer.FromContext(ctx)
	if ok {
		span.Annotate([]otrace.Attribute{
			otrace.StringAttribute("client_ip", clientPeer.Addr.String()),
		}, "client ip for login")
	}

	if err := acl.ValidateLoginRequest(request); err != nil {
		return nil, err
	}

	resp := &api.LogInResponse{
		Context: &api.TxnContext{}, // context needed in order to set the jwt below
		Code:    api.AclResponseCode_UNAUTHENTICATED,
	}

	user, err := s.queryUser(ctx, request.Userid, request.Password)
	if err != nil {
		glog.Warningf("Unable to login user with user id: %v", request.Userid)
		return nil, err
	}
	if user == nil {
		errMsg := fmt.Sprintf("User not found for user id %v", request.Userid)
		glog.Warningf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	if !user.PasswordMatch {
		errMsg := fmt.Sprintf("Password mismatch for user: %v", request.Userid)
		glog.Warningf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	jwt := &acl.Jwt{
		Header: acl.StdJwtHeader,
		Payload: acl.JwtPayload{
			Userid: request.Userid,
			Groups: acl.ToJwtGroups(user.Groups),
			// TODO add the token refresh mechanism
			Exp: time.Now().Add(Config.JwtTtl).Unix(), // set the jwt valid for 30 days
		},
	}

	resp.Context.Jwt, err = jwt.EncodeToString(Config.HmacSecret)
	if err != nil {
		glog.Errorf("Unable to encode jwt to string: %v", err)
		return nil, err
	}

	resp.Code = api.AclResponseCode_OK
	return resp, nil
}

const queryUser = `
    query search($userid: string, $password: string){
      user(func: eq(dgraph.xid, $userid)) {
	    uid
        password_match: checkpwd(dgraph.password, $password)
        dgraph.user.group {
          uid
          dgraph.xid
        }
      }
    }`

func (s *Server) queryUser(ctx context.Context, userid string, password string) (user *acl.User,
	err error) {
	queryVars := make(map[string]string)
	queryVars["$userid"] = userid
	queryVars["$password"] = password
	queryRequest := api.Request{
		Query: queryUser,
		Vars:  queryVars,
	}

	queryResp, err := s.Query(ctx, &queryRequest)
	if err != nil {
		glog.Errorf("Error while query user with id %s: %v", userid, err)
		return nil, err
	}
	user, err = acl.UnmarshallUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return user, nil
}