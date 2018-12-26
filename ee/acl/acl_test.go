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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	userid         = "alice"
	userpassword   = "simplepassword"
	dgraphEndpoint = "localhost:9180"
)

/*func TestAcl(t *testing.T) {
	t.Run("create user", CreateAndDeleteUsers)
	t.Run("login", LogIn)
}*/

func checkOutput(t *testing.T, cmd *exec.Cmd, shouldFail bool) string {
	out, err := cmd.CombinedOutput()
	if (!shouldFail && err != nil) || (shouldFail && err == nil) {
		t.Errorf("Error output from command:%v", string(out))
		t.Fatal(err)
	}

	return string(out)
}

func TestCreateAndDeleteUsers(t *testing.T) {
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
func TestLogIn(t *testing.T) {
	// delete and recreate the user to ensure a clean state
	deleteUserCmd := exec.Command("dgraph", "acl", "userdel", "-d", dgraphEndpoint, "-u", userid)
	checkOutput(t, deleteUserCmd, false)
	createUserCmd := exec.Command("dgraph", "acl", "useradd", "-d", dgraphEndpoint, "-u", userid,
		"-p", userpassword)
	checkOutput(t, createUserCmd, false)

	// now try to login with the wrong password

	loginWithCorrectPassword(t)
	loginWithWrongPassword(t)
}

func loginWithCorrectPassword(t *testing.T) {
	loginCmd := exec.Command("dgraph", "acl", "login", "-d", dgraphEndpoint, "-u", userid,
		"-p", userpassword)
	loginOutput := checkOutput(t, loginCmd, false)

	// parse the output to extract the accessJwt and refreshJwt
	lines := strings.Split(loginOutput, "\n")
	var accessJwt string
	var refreshJwt string
	for idx := 0; idx < len(lines); idx++ {
		line := lines[idx]
		if line == "access jwt:" {
			accessJwt = lines[idx+1]
			idx++ // skip the next line
		} else if line == "refresh jwt:" {
			refreshJwt = lines[idx+1]
			idx++
		}
	}
	require.True(t, len(accessJwt) > 0, "The accessJwt should not be empty")
	require.True(t, len(refreshJwt) > 0, "The refreshJwt should not be empty")
}

func loginWithWrongPassword(t *testing.T) {
	loginCmd := exec.Command("dgraph", "acl", "login", "-d", dgraphEndpoint, "-u", userid,
		"-p", userpassword+"123")
	checkOutput(t, loginCmd, true)
}
