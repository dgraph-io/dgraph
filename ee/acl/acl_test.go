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
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/test"
	"github.com/stretchr/testify/require"
)

const (
	userid         = "alice"
	userpassword   = "simplepassword"
	dgraphEndpoint = "localhost:9180"
)

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

func TestAuthorization(t *testing.T) {
	dg, close := test.GetDgraphClient()
	defer close()

	createAccountAndData(t, dg)
	queryPredicateWithUserAccount(t, dg, true)
	createGroupAndAcls(t)
	// wait for 35 seconds to ensure the new acl have reached all acl caches
	// on all alpha servers
	log.Println("Sleeping for 35 seconds for acl to catch up")
	time.Sleep(35 * time.Second)
	queryPredicateWithUserAccount(t, dg, false)
}

var user = "alice"
var password = "password123"
var predicate = "city_name"
var group = "dev"
var rootDir = filepath.Join(os.TempDir(), "acl_test")

func queryPredicateWithUserAccount(t *testing.T, dg *dgo.Dgraph, shouldFail bool) {
	// try to query the user whose name is alice
	ctx := context.Background()
	if err := dg.Login(ctx, user, password); err != nil {
		t.Fatalf("unable to login using the account %v", user)
	}

	ctxWithUserJwt := dg.GetContext(ctx)
	txn := dg.NewTxn()
	query := fmt.Sprintf(`
	{
		q(func: eq(%s, "SF")) {
			name
		}
	}`, predicate)
	txn = dg.NewTxn()
	_, err := txn.Query(ctxWithUserJwt, query)

	if shouldFail {
		require.Error(t, err, "the query should have failed")
	} else {
		require.NoError(t, err, "the query should have succeeded")
	}
}

func createAccountAndData(t *testing.T, dg *dgo.Dgraph) {
	// use the admin account to clean the database
	ctx := context.Background()
	if err := dg.Login(ctx, "admin", "password"); err != nil {
		t.Fatalf("unable to login using the admin account")
	}
	ctxWithAdminJwt := dg.GetContext(ctx)
	op := api.Operation{
		DropAll: true,
	}
	if err := dg.Alter(ctxWithAdminJwt, &op); err != nil {
		t.Fatalf("Unable to cleanup db:%v", err)
	}

	// use commands to create users and groups
	createUserCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "useradd",
		"-d", "localhost:9180",
		"-u", user, "-p", password, "--adminPassword", "password")
	if err := createUserCmd.Run(); err != nil {
		t.Fatalf("Unable to create user:%v", err)
	}

	// create some data, e.g. user with name alice
	require.NoError(t, dg.Alter(ctxWithAdminJwt, &api.Operation{
		Schema: `city_name: string @index(exact) .`,
	}))

	txn := dg.NewTxn()
	_, err := txn.Mutate(ctxWithAdminJwt, &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("_:a <%s> \"SF\" .", predicate)),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))
}

func createGroupAndAcls(t *testing.T) {
	// use commands to create users and groups
	createGroupCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "groupadd",
		"-d", "localhost:9180",
		"-g", group, "--adminPassword", "password")
	if err := createGroupCmd.Run(); err != nil {
		t.Fatalf("Unable to create group:%v", err)
	}

	addPermCmd1 := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "chmod",
		"-d", "localhost:9180",
		"-g", group, "-p", predicate, "-P", strconv.Itoa(int(Read)), "--adminPassword",
		"password")
	if err := addPermCmd1.Run(); err != nil {
		t.Fatalf("Unable to add permission to group %s:%v", group, err)
	}

	addPermCmd2 := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "chmod",
		"-d", "localhost:9180",
		"-g", group, "-p", "name", "-P", strconv.Itoa(int(Read)), "--adminPassword",
		"password")
	if err := addPermCmd2.Run(); err != nil {
		t.Fatalf("Unable to add permission to group %s:%v", group, err)
	}


	addUserToGroupCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "usermod",
		"-d", "localhost:9180",
		"-u", user, "-g", group, "--adminPassword", "password")
	if err := addUserToGroupCmd.Run(); err != nil {
		t.Fatalf("Unable to add user %s to group %s:%v", user, group, err)
	}
}
