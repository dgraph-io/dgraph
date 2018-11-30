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
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/golang/glog"
	"google.golang.org/grpc/peer"

	otrace "go.opencensus.io/trace"
)

func (s *Server) Login(ctx context.Context,
	request *api.LogInRequest) (*api.Response, error) {
	ctx, span := otrace.StartSpan(ctx, "server.LogIn")
	defer span.End()

	// record the client ip for this login request
	var addr string
	if ip, ok := peer.FromContext(ctx); ok {
		addr = ip.Addr.String()
		glog.Infof("Login request from: %s", addr)
		span.Annotate([]otrace.Attribute{
			otrace.StringAttribute("client_ip", addr),
		}, "client ip for login")
	}

	if err := acl.ValidateLoginRequest(request); err != nil {
		glog.Warningf("Invalid login from: %s", addr)
		return nil, err
	}

	resp := &api.Response{}

	user, err := s.queryUser(ctx, request.Userid, request.Password)
	if err != nil {
		glog.Warningf("Unable to login user id: %v. Addr: %s", request.Userid, addr)
		return nil, err
	}
	if user == nil {
		errMsg := fmt.Sprintf("User not found for user id %v. Addr: %s", request.Userid, addr)
		glog.Warningf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	if !user.PasswordMatch {
		errMsg := fmt.Sprintf("Password mismatch for user: %v. Addr: %s", request.Userid, addr)
		glog.Warningf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userid": request.Userid,
		"groups": acl.ToJwtGroups(user.Groups),
		// set the jwt exp according to the ttl
		"exp": json.Number(
			strconv.FormatInt(time.Now().Add(Config.JwtTtl).Unix(), 10)),
	})

	jwtString, err := token.SignedString(Config.HmacSecret)
	if err != nil {
		glog.Errorf("Unable to encode jwt to string: %v", err)
		return nil, err
	}

	resp.Json = []byte(jwtString)
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
	queryVars := map[string]string{
		"$userid":   userid,
		"$password": password,
	}
	queryRequest := api.Request{
		Query: queryUser,
		Vars:  queryVars,
	}

	queryResp, err := s.Query(ctx, &queryRequest)
	if err != nil {
		glog.Errorf("Error while query user with id %s: %v", userid, err)
		return nil, err
	}
	user, err = acl.UnmarshalUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return user, nil
}
