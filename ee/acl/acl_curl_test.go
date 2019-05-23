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

package acl

import (
	"fmt"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/z"
	"github.com/golang/glog"
	"github.com/stretchr/testify/require"
)

var loginEndpoint = "http://" + z.SockAddrHttp + "/login"

func TestCurlAuthorization(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping because -short=true")
	}

	glog.Infof("testing with port %s", z.SockAddr)
	dg := z.DgraphClientWithGroot(z.SockAddr)
	createAccountAndData(t, dg)

	// test query through curl
	accessJwt, refreshJwt, err := z.HttpLogin(&z.LoginParams{
		Endpoint: loginEndpoint,
		UserID:   userid,
		Passwd:   userpassword,
	})
	require.NoError(t, err, "login failed")

	// test fail open with the accessJwt
	queryArgs := func(jwt string) []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessToken:%s", jwt),
			"-H", "Content-Type: application/graphqlpm",
			"-d", query, curlQueryEndpoint}
	}
	z.VerifyCurlCmd(t, queryArgs(accessJwt), &z.FailureConfig{
		ShouldFail: false,
	})

	mutateArgs := func(jwt string) []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessToken:%s", jwt),
			"-H", "Content-Type: application/rdf",
			"-d", fmt.Sprintf(`{ set {
	   _:a <%s>  "string" .
	   }}`, predicateToWrite), curlMutateEndpoint}

	}

	z.VerifyCurlCmd(t, mutateArgs(accessJwt), &z.FailureConfig{
		ShouldFail: false,
	})

	alterArgs := func(jwt string) []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessToken:%s", jwt),
			"-d", fmt.Sprintf(`%s: int .`, predicateToAlter), curlAlterEndpoint}
	}
	z.VerifyCurlCmd(t, alterArgs(accessJwt), &z.FailureConfig{
		ShouldFail: false,
	})

	// sleep long enough (longer than 10s, the access JWT TTL defined in the docker-compose.yml
	// in this directory) for the accessJwt to expire, in order to test auto login through refresh
	// JWT
	glog.Infof("Sleeping for 4 seconds for accessJwt to expire")
	time.Sleep(4 * time.Second)
	z.VerifyCurlCmd(t, queryArgs(accessJwt), &z.FailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	z.VerifyCurlCmd(t, mutateArgs(accessJwt), &z.FailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	z.VerifyCurlCmd(t, alterArgs(accessJwt), &z.FailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	// login again using the refreshJwt
	accessJwt, refreshJwt, err = z.HttpLogin(&z.LoginParams{
		Endpoint:   loginEndpoint,
		RefreshJwt: refreshJwt,
	})
	require.NoError(t, err, fmt.Sprintf("login through refresh token failed: %v", err))
	// verify that the query works again with the new access jwt
	z.VerifyCurlCmd(t, queryArgs(accessJwt), &z.FailureConfig{
		ShouldFail: false,
	})

	createGroupAndAcls(t, unusedGroup, false)
	// wait for 6 seconds to ensure the new acl have reached all acl caches
	glog.Infof("Sleeping for 6 seconds for acl caches to be refreshed")
	time.Sleep(6 * time.Second)
	z.VerifyCurlCmd(t, queryArgs(accessJwt), &z.FailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	// refresh the jwts again
	accessJwt, refreshJwt, err = z.HttpLogin(&z.LoginParams{
		Endpoint:   loginEndpoint,
		RefreshJwt: refreshJwt,
	})
	require.NoError(t, err, fmt.Sprintf("login through refresh token failed: %v", err))
	// verify that with an ACL rule defined, all the operations should be denied when the acsess JWT
	// does not have the required permissions
	z.VerifyCurlCmd(t, queryArgs(accessJwt), &z.FailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})
	z.VerifyCurlCmd(t, mutateArgs(accessJwt), &z.FailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})
	z.VerifyCurlCmd(t, alterArgs(accessJwt), &z.FailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})

	createGroupAndAcls(t, devGroup, true)
	glog.Infof("Sleeping for 6 seconds for acl caches to be refreshed")
	time.Sleep(6 * time.Second)
	// refresh the jwts again
	accessJwt, _, err = z.HttpLogin(&z.LoginParams{
		Endpoint:   loginEndpoint,
		RefreshJwt: refreshJwt,
	})
	require.NoError(t, err, fmt.Sprintf("login through refresh token failed: %v", err))
	// verify that the operations should be allowed again through the dev group
	z.VerifyCurlCmd(t, queryArgs(accessJwt), &z.FailureConfig{
		ShouldFail: false,
	})
	z.VerifyCurlCmd(t, mutateArgs(accessJwt), &z.FailureConfig{
		ShouldFail: false,
	})
	z.VerifyCurlCmd(t, alterArgs(accessJwt), &z.FailureConfig{
		ShouldFail: false,
	})
}

const (
	curlLoginEndpoint  = "localhost:8180/login"
	curlQueryEndpoint  = "localhost:8180/query"
	curlMutateEndpoint = "localhost:8180/mutate"
	curlAlterEndpoint  = "localhost:8180/alter"
)
