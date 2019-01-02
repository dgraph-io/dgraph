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

	"google.golang.org/grpc/metadata"

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

func resetUser(t *testing.T) {
	// delete and recreate the user to ensure a clean state
	deleteUserCmd := exec.Command("dgraph", "acl", "userdel", "-d", dgraphEndpoint,
		"-u", userid, "--adminPassword", "password")
	deleteUserCmd.Run()

	createUserCmd := exec.Command("dgraph", "acl", "useradd", "-d", dgraphEndpoint, "-u",
		userid, "-p", userpassword, "--adminPassword", "password")
	checkOutput(t, createUserCmd, false)
}

func TestLogIn(t *testing.T) {
	resetUser(t)

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

// test dgo client login with the refresh jwt
func TestClientLoginWithRefreshJwt(t *testing.T) {
	resetUser(t)

	dg, close := test.GetDgraphClient()
	defer close()

	ctx := context.Background()
	if err := dg.Login(ctx, userid, userpassword); err != nil {
		t.Fatalf("unable to login using the account %v: %v", userid, err)
	}

	firstCtxWithUserJwt := dg.GetContext(ctx)
	firstMd, ok := metadata.FromOutgoingContext(firstCtxWithUserJwt)
	require.True(t, ok, "unable to get metadata from outgoing context")
	firstAccessJwt := firstMd.Get("accessJwt")
	if len(firstAccessJwt) == 0 {
		t.Fatalf("unable to get accessJwt from outgoing metadata")
	}

	// now login with the refresh jwt instead of password
	if err := dg.LoginWithRefreshJwt(ctx); err != nil {
		t.Fatalf("unable to login using the refresh jwt: %v", err)
	}

	secondCtxWithUserJwt := dg.GetContext(ctx)
	secondMd, ok := metadata.FromOutgoingContext(secondCtxWithUserJwt)
	require.True(t, ok, "unable to get metadata from outgoing context")
	secondAccessJwt := secondMd.Get("accessJwt")
	if len(secondAccessJwt) == 0 {
		t.Fatalf("unable to get accessJwt from outgoing metadata")
	}

	require.NotEqual(t, firstAccessJwt, secondAccessJwt,
		"the 2nd access jwt should be different from the first one")
}

func TestAuthorization(t *testing.T) {
	dg, close := test.GetDgraphClient()
	defer close()

	createAccountAndData(t, dg)
	queryPredicateWithUserAccount(t, dg, true)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)
	createGroupAndAcls(t)
	// wait for 35 seconds to ensure the new acl have reached all acl caches
	// on all alpha servers
	log.Println("Sleeping for 35 seconds for acl to catch up")
	time.Sleep(35 * time.Second)
	queryPredicateWithUserAccount(t, dg, false)
	mutatePredicateWithUserAccount(t, dg, false)
	alterPredicateWithUserAccount(t, dg, false)
}

var predicateToRead = "predicate_to_read"
var queryAttr = "name"
var predicateToWrite = "predicate_to_write"
var predicateToAlter = "predicate_to_alter"
var group = "dev"
var rootDir = filepath.Join(os.TempDir(), "acl_test")

func queryPredicateWithUserAccount(t *testing.T, dg *dgo.Dgraph, shouldFail bool) {
	// login with alice's account
	ctx := context.Background()
	if err := dg.Login(ctx, userid, userpassword); err != nil {
		t.Fatalf("unable to login using the account %v", userid)
	}

	ctxWithUserJwt := dg.GetContext(ctx)
	txn := dg.NewTxn()
	query := fmt.Sprintf(`
	{
		q(func: eq(%s, "SF")) {
			%s
		}
	}`, predicateToRead, queryAttr)
	txn = dg.NewTxn()
	_, err := txn.Query(ctxWithUserJwt, query)

	if shouldFail {
		require.Error(t, err, "the query should have failed")
	} else {
		require.NoError(t, err, "the query should have succeeded")
	}
}

func mutatePredicateWithUserAccount(t *testing.T, dg *dgo.Dgraph, shouldFail bool) {
	ctx := context.Background()
	if err := dg.Login(ctx, userid, userpassword); err != nil {
		t.Fatalf("unable to login using the account %v", userid)
	}

	ctxWithUserJwt := dg.GetContext(ctx)
	txn := dg.NewTxn()
	_, err := txn.Mutate(ctxWithUserJwt, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`_:a <%s>  "string" .`, predicateToWrite)),
	})

	if shouldFail {
		require.Error(t, err, "the mutation should have failed")
	} else {
		require.NoError(t, err, "the mutation should have succeeded")
	}
}

func alterPredicateWithUserAccount(t *testing.T, dg *dgo.Dgraph, shouldFail bool) {
	ctx := context.Background()
	if err := dg.Login(ctx, userid, userpassword); err != nil {
		t.Fatalf("unable to login using the account %v", userid)
	}

	ctxWithUserJwt := dg.GetContext(ctx)

	err := dg.Alter(ctxWithUserJwt, &api.Operation{
		Schema: fmt.Sprintf(`%s: int .`, predicateToAlter),
	})
	if shouldFail {
		require.Error(t, err, "the alter should have failed")
	} else {
		require.NoError(t, err, "the alter should have succeeded")
	}
}

func createAccountAndData(t *testing.T, dg *dgo.Dgraph) {
	// use the admin account to clean the database
	ctx := context.Background()
	if err := dg.Login(ctx, "admin", "password"); err != nil {
		t.Fatalf("unable to login using the admin account:%v", err)
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
		"-u", userid, "-p", userpassword, "--adminPassword", "password")
	if err := createUserCmd.Run(); err != nil {
		t.Fatalf("Unable to create user:%v", err)
	}

	// create some data, e.g. user with name alice
	require.NoError(t, dg.Alter(ctxWithAdminJwt, &api.Operation{
		Schema: fmt.Sprintf(`%s: string @index(exact) .`, predicateToRead),
	}))

	txn := dg.NewTxn()
	_, err := txn.Mutate(ctxWithAdminJwt, &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("_:a <%s> \"SF\" .", predicateToRead)),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))
}

func createGroupAndAcls(t *testing.T) {
	// create a new group
	createGroupCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "groupadd",
		"-d", "localhost:9180",
		"-g", group, "--adminPassword", "password")
	if err := createGroupCmd.Run(); err != nil {
		t.Fatalf("Unable to create group:%v", err)
	}

	// add the user to the group
	addUserToGroupCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "usermod",
		"-d", "localhost:9180",
		"-u", userid, "-g", group, "--adminPassword", "password")
	if err := addUserToGroupCmd.Run(); err != nil {
		t.Fatalf("Unable to add user %s to group %s:%v", userid, group, err)
	}

	// add READ permission on the predicateToRead to the group
	addReadPermCmd1 := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "chmod",
		"-d", "localhost:9180",
		"-g", group, "-p", predicateToRead, "-P", strconv.Itoa(int(Read)), "--adminPassword",
		"password")
	if err := addReadPermCmd1.Run(); err != nil {
		t.Fatalf("Unable to add READ permission on %s to group %s:%v",
			predicateToRead, group, err)
	}

	// also add read permission to the attribute queryAttr, which is used inside the query block
	addReadPermCmd2 := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "chmod",
		"-d", "localhost:9180",
		"-g", group, "-p", queryAttr, "-P", strconv.Itoa(int(Read)), "--adminPassword",
		"password")
	if err := addReadPermCmd2.Run(); err != nil {
		t.Fatalf("Unable to add READ permission on %s to group %s:%v", queryAttr, group, err)
	}

	// add WRITE permission on the predicateToWrite
	addWritePermCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "chmod",
		"-d", "localhost:9180",
		"-g", group, "-p", predicateToWrite, "-P", strconv.Itoa(int(Write)), "--adminPassword",
		"password")
	if err := addWritePermCmd.Run(); err != nil {
		t.Fatalf("Unable to add permission on %s to group %s:%v", predicateToWrite, group, err)
	}

	// add MODIFY permission on the predicateToAlter
	addModifyPermCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "chmod",
		"-d", "localhost:9180",
		"-g", group, "-p", predicateToAlter, "-P", strconv.Itoa(int(Modify)), "--adminPassword",
		"password")
	if err := addModifyPermCmd.Run(); err != nil {
		t.Fatalf("Unable to add permission on %s to group %s:%v", predicateToAlter, group, err)
	}
}
