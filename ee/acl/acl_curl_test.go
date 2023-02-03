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

package acl

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

var adminEndpoint string

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
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    userid,
		Passwd:    userpassword,
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	// No ACL rules are specified, so query should return empty response,
	// alter and mutate should fail.
	queryArgs := func(jwt string) []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessToken:%s", jwt),
			"-H", "Content-Type: application/dql",
			"-d", query, testutil.SockAddrHttp + "/query"}
	}
	testutil.VerifyCurlCmd(t, queryArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail: false,
	})

	mutateArgs := func(jwt string) []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessToken:%s", jwt),
			"-H", "Content-Type: application/rdf",
			"-d", fmt.Sprintf(`{ set {
	   _:a <%s>  "string" .
	   }}`, predicateToWrite), testutil.SockAddrHttp + "/mutate"}

	}

	testutil.VerifyCurlCmd(t, mutateArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})

	alterArgs := func(jwt string) []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessToken:%s", jwt),
			"-d", fmt.Sprintf(`%s: int .`, predicateToAlter), testutil.SockAddrHttp + "/alter"}
	}
	testutil.VerifyCurlCmd(t, alterArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})

	// sleep long enough (longer than 10s, the access JWT TTL defined in the docker-compose.yml
	// in this directory) for the accessJwt to expire, in order to test auto login through refresh
	// JWT
	glog.Infof("Sleeping for accessJwt to expire")
	time.Sleep(expireJwtSleep)
	testutil.VerifyCurlCmd(t, queryArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	testutil.VerifyCurlCmd(t, mutateArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	testutil.VerifyCurlCmd(t, alterArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	// login again using the refreshJwt
	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:   adminEndpoint,
		RefreshJwt: token.RefreshToken,
		Namespace:  x.GalaxyNamespace,
	})
	require.NoError(t, err, fmt.Sprintf("login through refresh httpToken failed: %v", err))

	createGroupAndAcls(t, unusedGroup, false)
	time.Sleep(expireJwtSleep)
	testutil.VerifyCurlCmd(t, queryArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "Token is expired",
	})
	// refresh the jwts again
	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:   adminEndpoint,
		RefreshJwt: token.RefreshToken,
	})
	require.NoError(t, err, fmt.Sprintf("login through refresh httpToken failed: %v", err))
	// verify that with an ACL rule defined, all the operations except query should
	// does not have the required permissions be denied when the acsess JWT
	testutil.VerifyCurlCmd(t, queryArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail: false,
	})
	testutil.VerifyCurlCmd(t, mutateArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})
	testutil.VerifyCurlCmd(t, alterArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail:   true,
		DgraphErrMsg: "PermissionDenied",
	})

	createGroupAndAcls(t, devGroup, true)
	time.Sleep(defaultTimeToSleep)
	// refresh the jwts again
	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:   adminEndpoint,
		RefreshJwt: token.RefreshToken,
	})
	require.NoError(t, err, fmt.Sprintf("login through refresh httpToken failed: %v", err))
	// verify that the operations should be allowed again through the dev group
	testutil.VerifyCurlCmd(t, queryArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail: false,
	})
	testutil.VerifyCurlCmd(t, mutateArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail: false,
	})
	testutil.VerifyCurlCmd(t, alterArgs(token.AccessJwt), &testutil.CurlFailureConfig{
		ShouldFail: false,
	})
}
