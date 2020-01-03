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
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/stretchr/testify/require"
)

var (
	userid         = "alice"
	userpassword   = "simplepassword"
	dgraphEndpoint = testutil.SockAddr
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
	// clean up the user to allow repeated running of this test
	cleanUserCmd := exec.Command("dgraph", "acl", "del", "-a", dgraphEndpoint,
		"-u", userid, "-x", "password")
	cleanUserCmd.Run()
	glog.Infof("cleaned up db user state")

	createUserCmd1 := exec.Command("dgraph", "acl", "add", "-a", dgraphEndpoint, "-u", userid,
		"-p", userpassword, "-x", "password")
	checkOutput(t, createUserCmd1, false)

	createUserCmd2 := exec.Command("dgraph", "acl", "add", "-a", dgraphEndpoint, "-u", userid,
		"-p", userpassword, "-x", "password")
	// create the user again should fail
	checkOutput(t, createUserCmd2, true)

	// delete the user
	deleteUserCmd := exec.Command("dgraph", "acl", "del", "-a", dgraphEndpoint, "-u", userid,
		"-x", "password")
	checkOutput(t, deleteUserCmd, false)

	// now we should be able to create the user again
	createUserCmd3 := exec.Command("dgraph", "acl", "add", "-a", dgraphEndpoint, "-u", userid,
		"-p", userpassword, "-x", "password")
	checkOutput(t, createUserCmd3, false)
}

func resetUser(t *testing.T) {
	// delete and recreate the user to ensure a clean state
	deleteUserCmd := exec.Command("dgraph", "acl", "del", "-a", dgraphEndpoint,
		"-u", userid, "-x", "password")
	deleteUserCmd.Run()
	glog.Infof("deleted user")

	createUserCmd := exec.Command("dgraph", "acl", "add", "-a", dgraphEndpoint, "-u",
		userid, "-p", userpassword, "-x", "password")
	checkOutput(t, createUserCmd, false)
	glog.Infof("created user")
}

func TestReservedPredicates(t *testing.T) {
	// This test uses the groot account to ensure that reserved predicates
	// cannot be altered even if the permissions allow it.
	ctx := context.Background()

	dg1, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	alterReservedPredicates(t, dg1)

	dg2, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	if err := dg2.Login(ctx, x.GrootId, "password"); err != nil {
		t.Fatalf("unable to login using the groot account:%v", err)
	}
	alterReservedPredicates(t, dg2)
}

func TestAuthorization(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping because -short=true")
	}

	glog.Infof("testing with port 9180")
	dg1, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	testAuthorization(t, dg1)
	glog.Infof("done")

	glog.Infof("testing with port 9182")
	dg2, err := testutil.DgraphClientWithGroot(":9182")
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	testAuthorization(t, dg2)
	glog.Infof("done")
}

func testAuthorization(t *testing.T, dg *dgo.Dgraph) {
	createAccountAndData(t, dg)
	ctx := context.Background()
	if err := dg.Login(ctx, userid, userpassword); err != nil {
		t.Fatalf("unable to login using the account %v", userid)
	}

	// initially the query, mutate and alter operations should all fail
	// when there are no rules defined on the predicates
	queryPredicateWithUserAccount(t, dg, true)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)
	createGroupAndAcls(t, unusedGroup, false)
	// wait for 6 seconds to ensure the new acl have reached all acl caches
	glog.Infof("Sleeping for 6 seconds for acl caches to be refreshed")
	time.Sleep(6 * time.Second)

	// now all these operations should fail since there are rules defined on the unusedGroup
	queryPredicateWithUserAccount(t, dg, true)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)
	// create the dev group and add the user to it
	createGroupAndAcls(t, devGroup, true)

	// wait for 6 seconds to ensure the new acl have reached all acl caches
	glog.Infof("Sleeping for 6 seconds for acl caches to be refreshed")
	time.Sleep(6 * time.Second)

	// now the operations should succeed again through the devGroup
	queryPredicateWithUserAccount(t, dg, false)
	// sleep long enough (10s per the docker-compose.yml)
	// for the accessJwt to expire in order to test auto login through refresh jwt
	glog.Infof("Sleeping for 4 seconds for accessJwt to expire")
	time.Sleep(4 * time.Second)
	mutatePredicateWithUserAccount(t, dg, false)
	glog.Infof("Sleeping for 4 seconds for accessJwt to expire")
	time.Sleep(4 * time.Second)
	alterPredicateWithUserAccount(t, dg, false)
}

var predicateToRead = "predicate_to_read"
var queryAttr = "name"
var predicateToWrite = "predicate_to_write"
var predicateToAlter = "predicate_to_alter"
var devGroup = "dev"
var unusedGroup = "unusedGroup"
var query = fmt.Sprintf(`
	{
		q(func: eq(%s, "SF")) {
			%s
		}
	}`, predicateToRead, queryAttr)
var schemaQuery = "schema {}"

func alterReservedPredicates(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	// Test that alter requests are allowed if the new update is the same as
	// the initial update for a reserved predicate.
	err := dg.Alter(ctx, &api.Operation{
		Schema: "dgraph.xid: string @index(exact) @upsert .",
	})
	require.NoError(t, err)

	err = dg.Alter(ctx, &api.Operation{
		Schema: "dgraph.xid: int .",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"predicate dgraph.xid is reserved and is not allowed to be modified")

	err = dg.Alter(ctx, &api.Operation{
		DropAttr: "dgraph.xid",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"predicate dgraph.xid is reserved and is not allowed to be dropped")

	// Test that reserved predicates act as case-insensitive.
	err = dg.Alter(ctx, &api.Operation{
		Schema: "dgraph.XID: int .",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"predicate dgraph.XID is reserved and is not allowed to be modified")
}

func queryPredicateWithUserAccount(t *testing.T, dg *dgo.Dgraph, shouldFail bool) {
	ctx := context.Background()
	txn := dg.NewTxn()
	_, err := txn.Query(ctx, query)

	if shouldFail {
		require.Error(t, err, "the query should have failed")
	} else {
		require.NoError(t, err, "the query should have succeeded")
	}
}

func querySchemaWithUserAccount(t *testing.T, dg *dgo.Dgraph, shouldFail bool) {
	ctx := context.Background()
	txn := dg.NewTxn()
	_, err := txn.Query(ctx, schemaQuery)

	if shouldFail {
		require.Error(t, err, "the query should have failed")
	} else {
		require.NoError(t, err, "the query should have succeeded")
	}
}

func mutatePredicateWithUserAccount(t *testing.T, dg *dgo.Dgraph, shouldFail bool) {
	ctx := context.Background()
	txn := dg.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
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
	err := dg.Alter(ctx, &api.Operation{
		Schema: fmt.Sprintf(`%s: int .`, predicateToAlter),
	})
	if shouldFail {
		require.Error(t, err, "the alter should have failed")
	} else {
		require.NoError(t, err, "the alter should have succeeded")
	}
}

func createAccountAndData(t *testing.T, dg *dgo.Dgraph) {
	// use the groot account to clean the database
	ctx := context.Background()
	if err := dg.Login(ctx, x.GrootId, "password"); err != nil {
		t.Fatalf("unable to login using the groot account:%v", err)
	}
	op := api.Operation{
		DropAll: true,
	}
	if err := dg.Alter(ctx, &op); err != nil {
		t.Fatalf("Unable to cleanup db:%v", err)
	}
	require.NoError(t, dg.Alter(ctx, &api.Operation{
		Schema: fmt.Sprintf(`%s: string @index(exact) .`, predicateToRead),
	}))
	// wait for 6 seconds to ensure the new acl have reached all acl caches
	glog.Infof("Sleeping for 6 seconds for acl caches to be refreshed")
	time.Sleep(6 * time.Second)

	// create some data, e.g. user with name alice
	resetUser(t)

	txn := dg.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("_:a <%s> \"SF\" .", predicateToRead)),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))
}

func createGroupAndAcls(t *testing.T, group string, addUserToGroup bool) {
	// create a new group
	createGroupCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "add",
		"-a", dgraphEndpoint,
		"-g", group, "-x", "password")
	if errOutput, err := createGroupCmd.CombinedOutput(); err != nil {
		t.Fatalf("Unable to create group: %v", string(errOutput))
	}

	// add the user to the group
	if addUserToGroup {
		addUserToGroupCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
			"acl", "mod",
			"-a", dgraphEndpoint,
			"-u", userid, "--group_list", group, "-x", "password")
		if errOutput, err := addUserToGroupCmd.CombinedOutput(); err != nil {
			t.Fatalf("Unable to add user %s to group %s:%v", userid, group, string(errOutput))
		}
	}

	// add READ permission on the predicateToRead to the group
	addReadPermCmd1 := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "mod",
		"-a", dgraphEndpoint,
		"-g", group, "-p", predicateToRead, "-m", strconv.Itoa(int(Read.Code)), "-x",
		"password")
	if errOutput, err := addReadPermCmd1.CombinedOutput(); err != nil {
		t.Fatalf("Unable to add READ permission on %s to group %s: %v",
			predicateToRead, group, string(errOutput))
	}

	// also add read permission to the attribute queryAttr, which is used inside the query block
	addReadPermCmd2 := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "mod",
		"-a", dgraphEndpoint,
		"-g", group, "-p", queryAttr, "-m", strconv.Itoa(int(Read.Code)), "-x",
		"password")
	if errOutput, err := addReadPermCmd2.CombinedOutput(); err != nil {
		t.Fatalf("Unable to add READ permission on %s to group %s: %v", queryAttr, group,
			string(errOutput))
	}

	// add WRITE permission on the predicateToWrite
	addWritePermCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "mod",
		"-a", dgraphEndpoint,
		"-g", group, "-p", predicateToWrite, "-m", strconv.Itoa(int(Write.Code)), "-x",
		"password")
	if errOutput, err := addWritePermCmd.CombinedOutput(); err != nil {
		t.Fatalf("Unable to add permission on %s to group %s: %v", predicateToWrite, group,
			string(errOutput))
	}

	// add MODIFY permission on the predicateToAlter
	addModifyPermCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"acl", "mod",
		"-a", dgraphEndpoint,
		"-g", group, "-p", predicateToAlter, "-m", strconv.Itoa(int(Modify.Code)), "-x",
		"password")
	if errOutput, err := addModifyPermCmd.CombinedOutput(); err != nil {
		t.Fatalf("Unable to add permission on %s to group %s: %v", predicateToAlter, group,
			string(errOutput))
	}
}

func TestPredicatePermission(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping because -short=true")
	}

	glog.Infof("testing with port 9180")
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	createAccountAndData(t, dg)
	ctx := context.Background()
	err = dg.Login(ctx, userid, userpassword)
	require.NoError(t, err, "Logging in with the current password should have succeeded")

	// Schema query is allowed to all logged in users.
	querySchemaWithUserAccount(t, dg, false)

	// The operations should be blocked when no rule is defined.
	queryPredicateWithUserAccount(t, dg, true)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)
	createGroupAndAcls(t, unusedGroup, false)

	// Wait for 6 seconds to ensure the new acl have reached all acl caches.
	glog.Infof("Sleeping for 6 seconds for acl caches to be refreshed")
	time.Sleep(6 * time.Second)
	// The operations should all fail when there is a rule defined, but the current user
	// is not allowed.
	queryPredicateWithUserAccount(t, dg, true)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)
	// Schema queries should still succeed since they are not tied to specific predicates.
	querySchemaWithUserAccount(t, dg, false)
}

func TestAccessWithoutLoggingIn(t *testing.T) {
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	createAccountAndData(t, dg)
	dg, err = testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)

	// Without logging in, the anonymous user should be evaluated as if the user does not
	// belong to any group, and access should not be granted if there is no ACL rule defined
	// for a predicate.
	queryPredicateWithUserAccount(t, dg, true)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)

	// Schema queries should fail if the user has not logged in.
	querySchemaWithUserAccount(t, dg, true)
}

func TestUnauthorizedDeletion(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)
	unAuthPred := "unauthorizedPredicate"

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	op := api.Operation{
		DropAll: true,
	}
	require.NoError(t, dg.Alter(ctx, &op))

	op = api.Operation{
		Schema: fmt.Sprintf("%s: string @index(exact) .", unAuthPred),
	}
	require.NoError(t, dg.Alter(ctx, &op))

	resetUser(t)
	createDevGroup := exec.Command("dgraph", "acl", "add", "-a", dgraphEndpoint,
		"-g", devGroup, "-x", "password")
	require.NoError(t, createDevGroup.Run())

	addUserToDev := exec.Command("dgraph", "acl", "mod", "-a", dgraphEndpoint, "-u", userid,
		"-l", devGroup, "-x", "password")
	require.NoError(t, addUserToDev.Run())

	txn := dg.NewTxn()
	mutation := &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("_:a <%s> \"testdata\" .", unAuthPred)),
		CommitNow: true,
	}
	resp, err := txn.Mutate(ctx, mutation)
	require.NoError(t, err)

	nodeUID, ok := resp.Uids["a"]
	require.True(t, ok)

	setPermissionCmd := exec.Command("dgraph", "acl", "mod", "-a", dgraphEndpoint, "-g",
		devGroup, "-p", unAuthPred, "-m", "0", "-x", "password")
	require.NoError(t, setPermissionCmd.Run())

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(6 * time.Second)

	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	txn = userClient.NewTxn()
	mutString := fmt.Sprintf("<%s> <%s> * .", nodeUID, unAuthPred)
	mutation = &api.Mutation{
		DelNquads: []byte(mutString),
		CommitNow: true,
	}
	_, err = txn.Mutate(ctx, mutation)

	require.Error(t, err)
	require.Contains(t, err.Error(), "PermissionDenied")
}

func TestGuardianAccess(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)
	op := api.Operation{Schema: "unauthpred: string @index(exact) ."}
	require.NoError(t, dg.Alter(ctx, &op))

	err = addNewUserToGroup("guardian", "guardianpass", "guardians")
	require.NoError(t, err, "Error while adding user to guardians group")

	mutation := &api.Mutation{
		SetNquads: []byte("_:a <unauthpred> \"testdata\" ."),
		CommitNow: true,
	}
	resp, err := dg.NewTxn().Mutate(ctx, mutation)
	require.NoError(t, err)

	nodeUID, ok := resp.Uids["a"]
	require.True(t, ok)

	time.Sleep(6 * time.Second)
	gClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err, "Error while creating client")

	gClient.Login(ctx, "guardian", "guardianpass")

	mutString := fmt.Sprintf("<%s> <unauthpred> \"testdata\" .", nodeUID)
	mutation = &api.Mutation{SetNquads: []byte(mutString), CommitNow: true}
	_, err = gClient.NewTxn().Mutate(ctx, mutation)
	require.NoError(t, err, "Error while mutating unauthorized predicate")

	query := `
                 {
                     me(func: eq(unauthpred, "testdata")) {
                         uid
                     }
                 }`

	resp, err = gClient.NewTxn().Query(ctx, query)
	require.NoError(t, err, "Error while querying unauthorized predicate")
	require.Contains(t, string(resp.GetJson()), "uid")

	op = api.Operation{Schema: "unauthpred: int ."}
	require.NoError(t, gClient.Alter(ctx, &op), "Error while altering unauthorized predicate")

	err = removeUserFromGroups("guardian")
	require.NoError(t, err, "Error while removing guardian from guardians group")

	_, err = gClient.NewTxn().Query(ctx, query)
	require.Error(t, err, "Query succeeded. It should have failed.")
}

func addNewUserToGroup(userName, password, groupName string) error {
	createGuardian := exec.Command("dgraph", "acl", "add", "-a", dgraphEndpoint,
		"-u", userName, "-p", password, "-x", "password")
	if err := createGuardian.Run(); err != nil {
		return err
	}

	makeGuardian := exec.Command("dgraph", "acl", "mod", "-a", dgraphEndpoint, "-u", "guardian",
		"-l", x.GuardiansId, "-x", "password")
	if err := makeGuardian.Run(); err != nil {
		return err
	}

	return nil
}

func removeUserFromGroups(userName string) error {
	removeUser := exec.Command("dgraph", "acl", "mod", "-a", dgraphEndpoint, "-u", userName,
		"-l", "", "-x", "password")
	return removeUser.Run()
}

func TestUnauthorizedDeletion(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)
	unAuthPred := "unauthorizedPredicate"

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	op := api.Operation{
		DropAll: true,
	}
	require.NoError(t, dg.Alter(ctx, &op))

	op = api.Operation{
		Schema: fmt.Sprintf("%s: string @index(exact) .", unAuthPred),
	}
	require.NoError(t, dg.Alter(ctx, &op))

	resetUser(t)
	createDevGroup := exec.Command("dgraph", "acl", "add", "-a", dgraphEndpoint,
		"-g", devGroup, "-x", "password")
	require.NoError(t, createDevGroup.Run())

	addUserToDev := exec.Command("dgraph", "acl", "mod", "-a", dgraphEndpoint, "-u", userid,
		"-l", devGroup, "-x", "password")
	require.NoError(t, addUserToDev.Run())

	txn := dg.NewTxn()
	mutation := &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("_:a <%s> \"testdata\" .", unAuthPred)),
		CommitNow: true,
	}
	resp, err := txn.Mutate(ctx, mutation)
	require.NoError(t, err)

	nodeUID, ok := resp.Uids["a"]
	require.True(t, ok)

	setPermissionCmd := exec.Command("dgraph", "acl", "mod", "-a", dgraphEndpoint, "-g",
		devGroup, "-p", unAuthPred, "-m", "0", "-x", "password")
	require.NoError(t, setPermissionCmd.Run())

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(6 * time.Second)

	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	txn = userClient.NewTxn()
	mutString := fmt.Sprintf("<%s> <%s> * .", nodeUID, unAuthPred)
	mutation = &api.Mutation{
		DelNquads: []byte(mutString),
		CommitNow: true,
	}
	_, err = txn.Mutate(ctx, mutation)

	require.Error(t, err)
	require.Contains(t, err.Error(), "PermissionDenied")
}
