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
	"os/exec"
	"testing"
)

const (
	userid         = "alice"
	userpassword   = "simplepassword"
	dgraphEndpoint = "localhost:9180"
)

func TestAcl(t *testing.T) {
	t.Run("create user", CreateAndDeleteUsers)
	// t.Run("login", LogIn)
}

func checkOutput(t *testing.T, cmd *exec.Cmd, shouldFail bool) string {
	out, err := cmd.CombinedOutput()
	if (!shouldFail && err != nil) || (shouldFail && err == nil) {
		t.Errorf("Error output from command:%v", string(out))
		t.Fatal(err)
	}

	return string(out)
}

func CreateAndDeleteUsers(t *testing.T) {
	createUserCmd1 := exec.Command("dgraph", "acl", "useradd", "-d", dgraphEndpoint, "-u", userid,
		"-p", userpassword)
	createUserOutput1 := checkOutput(t, createUserCmd1, false)
	t.Logf("Got output when creating user:%v", createUserOutput1)

	createUserCmd2 := exec.Command("dgraph", "acl", "useradd", "-d", dgraphEndpoint, "-u", userid,
		"-p", userpassword)

	// create the user again should fail
	createUserOutput2 := checkOutput(t, createUserCmd2, true)
	t.Logf("Got output when creating user:%v", createUserOutput2)

	// delete the user
	deleteUserCmd := exec.Command("dgraph", "acl", "userdel", "-d", dgraphEndpoint, "-u", userid)
	deleteUserOutput := checkOutput(t, deleteUserCmd, false)
	t.Logf("Got output when deleting user:%v", deleteUserOutput)

	// now we should be able to create the user again
	createUserCmd3 := exec.Command("dgraph", "acl", "useradd", "-d", dgraphEndpoint, "-u", userid,
		"-p", userpassword)
	createUserOutput3 := checkOutput(t, createUserCmd3, false)
	t.Logf("Got output when creating user:%v", createUserOutput3)
}

// TODO(gitlw): Finish this later.
// func LogIn(t *testing.T) {
// delete and recreate the user to ensure a clean state
/*
	deleteUserCmd := exec.Command("dgraph", "acl", "userdel", "-d", dgraphEndpoint, "-u", "lucas")
	deleteUserOutput := checkOutput(t, deleteUserCmd, false)
	createUserCmd := exec.Command("dgraph", "acl", "useradd", "-d", dgraphEndpoint, "-u", "lucas",
		"-p", "haha")
	createUserOutput := checkOutput(t, createUserCmd, false)
*/

// now try to login with the wrong password

//loginWithWrongPassword(t, ctx, adminClient)
//loginWithCorrectPassword(t, ctx, adminClient)
// }

/*
func loginWithCorrectPassword(t *testing.T, ctx context.Context,
	adminClient api.DgraphAccessClient) {
	loginRequest := api.LogInRequest{
		Userid:   userid,
		Password: userpassword,
	}
	response2, err := adminClient.LogIn(ctx, &loginRequest)
	require.NoError(t, err)
	if response2.Code != api.AclResponseCode_OK {
		t.Errorf("Login with the correct password should result in the code %v",
			api.AclResponseCode_OK)
	}
	jwt := acl.Jwt{}
	jwt.DecodeString(response2.Context.Jwt, false, nil)
	if jwt.Payload.Userid != userid {
		t.Errorf("the jwt token should have the user id encoded")
	}
	jwtTime := time.Unix(jwt.Payload.Exp, 0)
	jwtValidDays := jwtTime.Sub(time.Now()).Round(time.Hour).Hours() / 24
	if jwtValidDays != 30.0 {
		t.Errorf("The jwt token should be valid for 30 days, received %v days", jwtValidDays)
	}
}

func loginWithWrongPassword(t *testing.T, ctx context.Context,
	adminClient api.DgraphAccessClient) {
	loginRequestWithWrongPassword := api.LogInRequest{
		Userid:   userid,
		Password: userpassword + "123",
	}

	response, err := adminClient.LogIn(ctx, &loginRequestWithWrongPassword)
	require.NoError(t, err)
	if response.Code != api.AclResponseCode_UNAUTHENTICATED {
		t.Errorf("Login with the wrong password should result in the code %v", api.AclResponseCode_UNAUTHENTICATED)
	}
}

*/
