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
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/stretchr/testify/require"
)

func TestCurlAuthorization(t *testing.T) {
	glog.Infof("testing with port 9180")
	dg, cancel := x.GetDgraphClientOnPort(9180)
	defer cancel()
	createAccountAndData(t, dg)

	// test query through curl
	accessJwt, refreshJwt := curlLogin(t, "")

	// test fail open with the accessJwt
	queryArgs := func() []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessJWT:%s", accessJwt),
			"-d", query, curlQueryEndpoint}
	}

	verifyCurlCmd(t, queryArgs(), &FailureConfig{
		shouldFail: false,
	})

	mutateArgs := func() []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessJWT:%s", accessJwt),
			"-d", fmt.Sprintf(`{ set {
	   _:a <%s>  "string" .
	   }}`, predicateToWrite), curlMutateEndpoint}

	}
	verifyCurlCmd(t, mutateArgs(), &FailureConfig{
		shouldFail: false,
	})

	alterArgs := func() []string {
		return []string{"-H", fmt.Sprintf("X-Dgraph-AccessJWT:%s", accessJwt),
			"-d", fmt.Sprintf(`%s: int .`, predicateToAlter), curlAlterEndpoint}
	}

	verifyCurlCmd(t, alterArgs(), &FailureConfig{
		shouldFail: false,
	})

	// sleep long enough (10s per the docker-compose.yml in this directory)
	// for the accessJwt to expire in order to test auto login through refresh jwt
	log.Println("Sleeping for 12 seconds for accessJwt to expire")
	time.Sleep(12 * time.Second)
	verifyCurlCmd(t, queryArgs(), &FailureConfig{
		shouldFail: true,
		failMsg:    "Token is expired",
	})
	verifyCurlCmd(t, mutateArgs(), &FailureConfig{
		shouldFail: true,
		failMsg:    "Token is expired",
	})
	verifyCurlCmd(t, alterArgs(), &FailureConfig{
		shouldFail: true,
		failMsg:    "Token is expired",
	})
	// login again using the refreshJwt
	accessJwt, refreshJwt = curlLogin(t, refreshJwt)
	// verify that the query works again with the new access jwt
	verifyCurlCmd(t, queryArgs(), &FailureConfig{
		shouldFail: false,
	})

	createGroupAndAcls(t, unusedGroup, false)
	// wait for 35 seconds to ensure the new acl have reached all acl caches
	log.Println("Sleeping for 35 seconds for acl caches to be refreshed")
	time.Sleep(35 * time.Second)
	verifyCurlCmd(t, queryArgs(), &FailureConfig{
		shouldFail: true,
		failMsg:    "Token is expired",
	})
	// refresh the jwts again
	accessJwt, refreshJwt = curlLogin(t, refreshJwt)
	verifyCurlCmd(t, queryArgs(), &FailureConfig{
		shouldFail: true,
		failMsg:    "PermissionDenied",
	})
	verifyCurlCmd(t, mutateArgs(), &FailureConfig{
		shouldFail: true,
		failMsg:    "PermissionDenied",
	})
	verifyCurlCmd(t, alterArgs(), &FailureConfig{
		shouldFail: true,
		failMsg:    "PermissionDenied",
	})

	createGroupAndAcls(t, devGroup, true)
	// wait for 35 seconds to ensure the new acl have reached all acl caches
	log.Println("Sleeping for 35 seconds for acl caches to be refreshed")
	time.Sleep(35 * time.Second)
	// refresh the jwts again
	accessJwt, refreshJwt = curlLogin(t, refreshJwt)
	// through the dev group, the operations should be allowed again
	verifyCurlCmd(t, queryArgs(), &FailureConfig{
		shouldFail: false,
	})
	verifyCurlCmd(t, mutateArgs(), &FailureConfig{
		shouldFail: false,
	})
	verifyCurlCmd(t, alterArgs(), &FailureConfig{
		shouldFail: false,
	})
}

var curlLoginEndpoint = "localhost:8180/login"
var curlQueryEndpoint = "localhost:8180/query"
var curlMutateEndpoint = "localhost:8180/mutate"
var curlAlterEndpoint = "localhost:8180/alter"

// curlLogin sends a curl request to the curlLoginEndpoint
// and returns the access jwt and refresh jwt extracted from
// the curl command output
func curlLogin(t *testing.T, refreshJwt string) (string, string) {
	// login with alice's account using curl
	args := []string{"-X", "POST",
		curlLoginEndpoint}

	if len(refreshJwt) > 0 {
		args = append(args,
			"-H", fmt.Sprintf(`X-Dgraph-RefreshJWT:%s`, refreshJwt))
	} else {
		args = append(args,
			"-H", fmt.Sprintf(`X-Dgraph-User:%s`, userid),
			"-H", fmt.Sprintf(`X-Dgraph-Password:%s`, userpassword))
	}

	userLoginCmd := exec.Command("curl", args...)
	//t.Logf("curl %s\n", strings.Join(args, " "))
	out, err := userLoginCmd.Output()
	require.NoError(t, err, "the login should have succeeded")

	outputLines := strings.Split(string(out), "\n")
	var newAccessJwt string
	var newRefreshJwt string
	for idx := 0; idx < len(outputLines); idx++ {
		line := outputLines[idx]
		if line == "ACCESS JWT:" {
			idx++
			require.True(t, idx < len(outputLines),
				"no line found after ACCESS JWT")
			newAccessJwt = outputLines[idx]
		} else if line == "REFRESH JWT:" {
			idx++
			require.True(t, idx < len(outputLines),
				"no line found after REFRESH JWT")
			newRefreshJwt = outputLines[idx]
		}
	}
	require.True(t, len(newAccessJwt) > 0, "no access jwt received")
	require.True(t, len(newRefreshJwt) > 0, "no refresh jwt received")

	return newAccessJwt, newRefreshJwt
}

type ErrorEntry struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Output struct {
	Data   map[string]interface{} `json:"data"`
	Errors []ErrorEntry           `json:"errors"`
}

type FailureConfig struct {
	shouldFail bool
	failMsg    string
}

func verifyOutput(t *testing.T, bytes []byte, failureConfig *FailureConfig) {
	output := Output{}
	require.NoError(t, json.Unmarshal(bytes, &output),
		"unable to unmarshal the curl output")

	if failureConfig.shouldFail {
		require.True(t, len(output.Errors) > 0, "no error entry found")
		if len(failureConfig.failMsg) > 0 {
			errorEntry := output.Errors[0]
			require.True(t, strings.Contains(errorEntry.Message, failureConfig.failMsg),
				fmt.Sprintf("the failure msg\n%s\nis not part of the curl error output:%s\n",
					failureConfig.failMsg, errorEntry.Message))
		}
	} else {
		require.True(t, len(output.Data) > 0,
			fmt.Sprintf("no data entry found in the output:%+v", output))
	}
}

func verifyCurlCmd(t *testing.T, args []string,
	failureConfig *FailureConfig) {
	//t.Logf("curl %s\n", strings.Join(args, " "))
	queryCmd := exec.Command("curl", args...)

	output, err := queryCmd.Output()
	// the curl command should always succeed
	require.NoError(t, err, "the curl command should have succeeded")
	verifyOutput(t, output, failureConfig)
}
