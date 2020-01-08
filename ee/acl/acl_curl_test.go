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

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/golang/glog"
	"github.com/stretchr/testify/require"
)

var loginEndpoint = "http://" + testutil.SockAddrHttp + "/login"

func TestCurlAuthorization(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping because -short=true")
	}

	glog.Infof("testing with port %s", testutil.SockAddr)
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	createAccountAndData(t, dg)

	// test query through curl
	accessJwt, refreshJwt, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: loginEndpoint,
		UserID:   userid,
		Passwd:   userpassword,
	})
	require.NoError(t, err, "login failed")

	// No ACL rules are specified, so everything should fail.
	queryArgs := func(jwt string) []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessToken:%s", jwt),
			"-H", "Content-Type: application/graphql+-",
			"-d", query, curlQueryEndpoint}
	}
	testutil.VerifyCurlCmd(t, queryArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})

	mutateArgs := func(jwt string) []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessToken:%s", jwt),
			"-H", "Content-Type: application/rdf",
			"-d", fmt.Sprintf(`{ set {
	   _:a <%s>  "string" .
	   }}`, predicateToWrite), curlMutateEndpoint}

	}

	testutil.VerifyCurlCmd(t, mutateArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})

	alterArgs := func(jwt string) []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessToken:%s", jwt),
			"-d", fmt.Sprintf(`%s: int .`, predicateToAlter), curlAlterEndpoint}
	}
	testutil.VerifyCurlCmd(t, alterArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})

	// sleep long enough (longer than 10s, the access JWT TTL defined in the docker-compose.yml
	// in this directory) for the accessJwt to expire, in order to test auto login through refresh
	// JWT
	glog.Infof("Sleeping for 4 seconds for accessJwt to expire")
	time.Sleep(4 * time.Second)
	testutil.VerifyCurlCmd(t, queryArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	testutil.VerifyCurlCmd(t, mutateArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	testutil.VerifyCurlCmd(t, alterArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	// login again using the refreshJwt
	accessJwt, refreshJwt, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:   loginEndpoint,
		RefreshJwt: refreshJwt,
	})
	require.NoError(t, err, fmt.Sprintf("login through refresh token failed: %v", err))

	createGroupAndAcls(t, unusedGroup, false)
	// wait for 6 seconds to ensure the new acl have reached all acl caches
	glog.Infof("Sleeping for 6 seconds for acl caches to be refreshed")
	time.Sleep(6 * time.Second)
	testutil.VerifyCurlCmd(t, queryArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	// refresh the jwts again
	accessJwt, refreshJwt, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:   loginEndpoint,
		RefreshJwt: refreshJwt,
	})
	require.NoError(t, err, fmt.Sprintf("login through refresh token failed: %v", err))
	// verify that with an ACL rule defined, all the operations should be denied when the acsess JWT
	// does not have the required permissions
	testutil.VerifyCurlCmd(t, queryArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})
	testutil.VerifyCurlCmd(t, mutateArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})
	testutil.VerifyCurlCmd(t, alterArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})

	createGroupAndAcls(t, devGroup, true)
	glog.Infof("Sleeping for 6 seconds for acl caches to be refreshed")
	time.Sleep(6 * time.Second)
	// refresh the jwts again
	accessJwt, _, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:   loginEndpoint,
		RefreshJwt: refreshJwt,
	})
	require.NoError(t, err, fmt.Sprintf("login through refresh token failed: %v", err))
	// verify that the operations should be allowed again through the dev group
	testutil.VerifyCurlCmd(t, queryArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail: false,
	})
	testutil.VerifyCurlCmd(t, mutateArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail: false,
	})
	testutil.VerifyCurlCmd(t, alterArgs(accessJwt), &testutil.CurlFailureConfig{
		ShouldFail: false,
	})
}

const (
	curlLoginEndpoint  = "localhost:8180/login"
	curlQueryEndpoint  = "localhost:8180/query"
	curlMutateEndpoint = "localhost:8180/mutate"
	curlAlterEndpoint  = "localhost:8180/alter"
)
