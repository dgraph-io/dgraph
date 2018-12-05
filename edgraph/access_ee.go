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
	"github.com/dgrijalva/jwt-go"
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

	user, err := s.authenticate(ctx, request)
	if err != nil {
		errMsg := fmt.Sprintf("authentication from address %s failed: %v", addr, err)
		glog.Errorf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	resp := &api.Response{}
	accessJwt, err := getAccessJwt(request.Userid, user.Groups)
	if err != nil {
		errMsg := fmt.Sprintf("unable to get access jwt (userid=%s,addr=%s):%v",
			request.Userid, addr, err)
		glog.Errorf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	refreshJwt, err := getRefreshJwt(request.Userid)
	if err != nil {
		errMsg := fmt.Sprintf("unable to get refresh jwt (userid=%s,addr=%s):%v",
			request.Userid, addr, err)
		glog.Errorf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	loginJwt := api.Jwt{
		AccessJwt:  accessJwt,
		RefreshJwt: refreshJwt,
	}

	jwtBytes, err := loginJwt.Marshal()
	if err != nil {
		errMsg := fmt.Sprintf("unable to marshal jwt (userid=%s,addr=%s):%v",
			request.Userid, addr, err)
		glog.Errorf(errMsg)
		return nil, fmt.Errorf(errMsg)
	}
	resp.Json = jwtBytes
	return resp, nil
}

func (s *Server) authenticate(ctx context.Context, request *api.LogInRequest) (*acl.User, error) {
	if err := validateLoginRequest(request); err != nil {
		return nil, fmt.Errorf("invalid login request: %v", err)
	}

	var user *acl.User
	if len(request.RefreshToken) > 0 {
		userId, err := authenticateRefreshToken(request.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("unable to authenticate the refresh token %v: %v",
				request.RefreshToken, err)
		}

		user, err = s.queryUser(ctx, userId, "")
		if err != nil {
			return nil, fmt.Errorf("error while querying user with id: %v",
				request.Userid)
		}

		if user == nil {
			return nil, fmt.Errorf("user not found for id %v", request.Userid)
		}
	} else {
		var err error
		user, err = s.queryUser(ctx, request.Userid, request.Password)
		if err != nil {
			return nil, fmt.Errorf("error while querying user with id: %v",
				request.Userid)
		}

		if user == nil {
			return nil, fmt.Errorf("user not found for id %v", request.Userid)
		}
		if !user.PasswordMatch {
			return nil, fmt.Errorf("password mismatch for user: %v", request.Userid)
		}
	}

	return user, nil
}

func authenticateRefreshToken(refreshToken string) (string, error) {
	token, err := jwt.Parse(refreshToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return Config.HmacSecret, nil
	})

	if err != nil {
		return "", fmt.Errorf("unable to parse refresh token:%v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if (!ok) || (!token.Valid) {
		return "", fmt.Errorf("claims in refresh token is not map claims:%v", refreshToken)
	}

	// by default, the MapClaims.Valid will return true if the exp field is not set
	// here we enforce the checking to make sure that the refresh token has not expired
	now := time.Now().Unix()
	if claims.VerifyExpiresAt(now, true) == false {
		return "", fmt.Errorf("refresh token has expired: %v", refreshToken)
	}

	userId, ok := claims["userid"].(string)
	if !ok {
		return "", fmt.Errorf("userid in claims is not a string:%v", userId)
	}
	return userId, nil
}

func validateLoginRequest(request *api.LogInRequest) error {
	if request == nil {
		return fmt.Errorf("the request should not be nil")
	}
	// we will use the refresh token for authentication if it's set
	if len(request.RefreshToken) > 0 {
		return nil
	}

	// otherwise make sure both userid and password are set
	if len(request.Userid) == 0 {
		return fmt.Errorf("the userid should not be empty")
	}
	if len(request.Password) == 0 {
		return fmt.Errorf("the password should not be empty")
	}
	return nil
}

func getAccessJwt(userId string, groups []acl.Group) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userid": userId,
		"groups": acl.GetGroupIDs(groups),
		// set the jwt exp according to the ttl
		"exp": json.Number(
			strconv.FormatInt(time.Now().Add(Config.AccessJwtTtl).Unix(), 10)),
	})

	jwtString, err := token.SignedString(Config.HmacSecret)
	if err != nil {
		return "", fmt.Errorf("unable to encode jwt to string: %v", err)
	}
	return jwtString, nil
}

func getRefreshJwt(userId string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userid": userId,
		// set the jwt exp according to the ttl
		"exp": json.Number(
			strconv.FormatInt(time.Now().Add(Config.RefreshJwtTtl).Unix(), 10)),
	})

	jwtString, err := token.SignedString(Config.HmacSecret)
	if err != nil {
		return "", fmt.Errorf("unable to encode jwt to string: %v", err)
	}
	return jwtString, nil
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
