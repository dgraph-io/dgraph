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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
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

func createUser(t *testing.T, accessToken, username, password string) *testutil.GraphQLResponse {
	addUser := `
	mutation addUser($name: String!, $pass: String!) {
		addUser(input: [{name: $name, password: $pass}]) {
			user {
				name
			}
		}
	}`

	params := testutil.GraphQLParams{
		Query: addUser,
		Variables: map[string]interface{}{
			"name": username,
			"pass": password,
		},
	}
	resp := makeRequest(t, accessToken, params)
	return resp
}

func getCurrentUser(t *testing.T, accessToken string) *testutil.GraphQLResponse {
	query := `
	query {
		getCurrentUser {
			name
		}
	}`

	resp := makeRequest(t, accessToken, testutil.GraphQLParams{Query: query})
	return resp
}

func checkUserCount(t *testing.T, resp []byte, expected int) {
	type Response struct {
		AddUser struct {
			User []struct {
				Name string
			}
		}
	}

	var r Response
	err := json.Unmarshal(resp, &r)
	require.NoError(t, err)
	require.Equal(t, expected, len(r.AddUser.User))
}

func deleteUser(t *testing.T, accessToken, username string, confirmDeletion bool) {
	delUser := `
	mutation deleteUser($name: String!) {
		deleteUser(filter: {name: {eq: $name}}) {
			msg
			numUids
		}
	}`

	params := testutil.GraphQLParams{
		Query: delUser,
		Variables: map[string]interface{}{
			"name": username,
		},
	}
	resp := makeRequest(t, accessToken, params)
	resp.RequireNoGraphQLErrors(t)

	if confirmDeletion {
		require.JSONEq(t, `{"deleteUser":{"msg":"Deleted","numUids":1}}`, string(resp.Data))
	}
}

func deleteGroup(t *testing.T, accessToken, name string) {
	delGroup := `
	mutation deleteUser($name: String!) {
		deleteGroup(filter: {name: {eq: $name}}) {
			msg
			numUids
		}
	}`

	params := testutil.GraphQLParams{
		Query: delGroup,
		Variables: map[string]interface{}{
			"name": name,
		},
	}
	resp := makeRequest(t, accessToken, params)
	resp.RequireNoGraphQLErrors(t)

	require.JSONEq(t, `{"deleteGroup":{"msg":"Deleted","numUids":1}}`, string(resp.Data))
}

func TestInvalidGetUser(t *testing.T) {
	currentUser := getCurrentUser(t, "invalid token")
	require.Equal(t, `{"getCurrentUser":null}`, string(currentUser.Data))
	require.Equal(t, x.GqlErrorList{{
		Message: "couldn't rewrite query getCurrentUser because unable to parse jwt token: token" +
			" contains an invalid number of segments",
	}}, currentUser.Errors)
}

func TestPasswordReturn(t *testing.T) {
	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	query := `
	query {
		getCurrentUser {
			name
			password
		}
	}`

	resp := makeRequest(t, accessJwt, testutil.GraphQLParams{Query: query})
	require.Equal(t, resp.Errors, x.GqlErrorList{{
		Message: `Cannot query field "password" on type "User".`,
		Locations: []x.Location{{
			Line:   5,
			Column: 4,
		}},
	}})
}

func TestGetCurrentUser(t *testing.T) {
	accessJwt, _ := testutil.GrootHttpLogin(adminEndpoint)
	currentUser := getCurrentUser(t, accessJwt)
	currentUser.RequireNoGraphQLErrors(t)
	require.Equal(t, string(currentUser.Data), `{"getCurrentUser":{"name":"groot"}}`)

	// clean up the user to allow repeated running of this test
	userid := "hamilton"
	deleteUser(t, accessJwt, userid, false)
	glog.Infof("cleaned up db user state")

	resp := createUser(t, accessJwt, userid, userpassword)
	resp.RequireNoGraphQLErrors(t)
	checkUserCount(t, resp.Data, 1)

	newJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   userid,
		Passwd:   userpassword,
	})
	require.NoError(t, err, "login failed")

	currentUser = getCurrentUser(t, newJwt)
	currentUser.RequireNoGraphQLErrors(t)
	require.Equal(t, string(currentUser.Data), `{"getCurrentUser":{"name":"hamilton"}}`)
}

func TestCreateAndDeleteUsers(t *testing.T) {
	resetUser(t)

	// adding the user again should fail
	accessJwt, _ := testutil.GrootHttpLogin(adminEndpoint)
	resp := createUser(t, accessJwt, userid, userpassword)
	require.Equal(t, x.GqlErrorList{{
		Message: "couldn't rewrite query for mutation addUser because id alice already exists" +
			" for type User",
	}}, resp.Errors)
	checkUserCount(t, resp.Data, 0)

	// delete the user
	deleteUser(t, accessJwt, userid, true)

	resp = createUser(t, accessJwt, userid, userpassword)
	resp.RequireNoGraphQLErrors(t)
	// now we should be able to create the user again
	checkUserCount(t, resp.Data, 1)
}

func resetUser(t *testing.T) {
	accessJwt, _ := testutil.GrootHttpLogin(adminEndpoint)

	// clean up the user to allow repeated running of this test
	deleteUser(t, accessJwt, userid, false)
	glog.Infof("deleted user")

	resp := createUser(t, accessJwt, userid, userpassword)
	resp.RequireNoGraphQLErrors(t)
	checkUserCount(t, resp.Data, 1)
	glog.Infof("created user")
}

func TestPreDefinedPredicates(t *testing.T) {
	// This test uses the groot account to ensure that pre-defined predicates
	// cannot be altered even if the permissions allow it.
	dg1, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err, "Error while getting a dgraph client")

	alterPreDefinedPredicates(t, dg1)
}

func TestPreDefinedTypes(t *testing.T) {
	// This test uses the groot account to ensure that pre-defined types
	// cannot be altered even if the permissions allow it.
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err, "Error while getting a dgraph client")

	alterPreDefinedTypes(t, dg)
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

	// initially the query should return empty result, mutate and alter
	// operations should all fail when there are no rules defined on the predicates
	queryPredicateWithUserAccount(t, dg, false)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)
	createGroupAndAcls(t, unusedGroup, false)
	// wait for 5 seconds to ensure the new acl have reached all acl caches
	glog.Infof("Sleeping for 5 seconds for acl caches to be refreshed")
	time.Sleep(5 * time.Second)

	// now all these operations except query should fail since
	// there are rules defined on the unusedGroup
	queryPredicateWithUserAccount(t, dg, false)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)
	// create the dev group and add the user to it
	createGroupAndAcls(t, devGroup, true)

	// wait for 5 seconds to ensure the new acl have reached all acl caches
	glog.Infof("Sleeping for 5 seconds for acl caches to be refreshed")
	time.Sleep(5 * time.Second)

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
var sreGroup = "sre"
var unusedGroup = "unusedGroup"
var query = fmt.Sprintf(`
	{
		q(func: eq(%s, "SF")) {
			%s
		}
	}`, predicateToRead, queryAttr)
var schemaQuery = "schema {}"

func alterPreDefinedPredicates(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	// Test that alter requests are allowed if the new update is the same as
	// the initial update for a pre-defined predicate.
	err := dg.Alter(ctx, &api.Operation{
		Schema: "dgraph.xid: string @index(exact) @upsert .",
	})
	require.NoError(t, err)

	err = dg.Alter(ctx, &api.Operation{
		Schema: "dgraph.xid: int .",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"predicate dgraph.xid is pre-defined and is not allowed to be modified")

	err = dg.Alter(ctx, &api.Operation{
		DropAttr: "dgraph.xid",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"predicate dgraph.xid is pre-defined and is not allowed to be dropped")

	// Test that pre-defined predicates act as case-insensitive.
	err = dg.Alter(ctx, &api.Operation{
		Schema: "dgraph.XID: int .",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"predicate dgraph.XID is pre-defined and is not allowed to be modified")
}

func alterPreDefinedTypes(t *testing.T, dg *dgo.Dgraph) {
	ctx := context.Background()

	// Test that alter requests are allowed if the new update is the same as
	// the initial update for a pre-defined type.
	err := dg.Alter(ctx, &api.Operation{
		Schema: `
			type dgraph.type.Group {
				dgraph.xid
				dgraph.acl.rule
			}
		`,
	})
	require.NoError(t, err)

	err = dg.Alter(ctx, &api.Operation{
		Schema: `
			type dgraph.type.Group {
				dgraph.xid
			}
		`,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"type dgraph.type.Group is pre-defined and is not allowed to be modified")

	err = dg.Alter(ctx, &api.Operation{
		DropOp:    api.Operation_TYPE,
		DropValue: "dgraph.type.Group",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"type dgraph.type.Group is pre-defined and is not allowed to be dropped")
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
	// wait for 5 seconds to ensure the new acl have reached all acl caches
	glog.Infof("Sleeping for 5 seconds for acl caches to be refreshed")
	time.Sleep(5 * time.Second)

	// create some data, e.g. user with name alice
	resetUser(t)

	txn := dg.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("_:a <%s> \"SF\" .", predicateToRead)),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))
}

func createGroup(t *testing.T, accessToken, name string) []byte {
	addGroup := `
	mutation addGroup($name: String!) {
		addGroup(input: [{name: $name}]) {
			group {
				name
			}
		}
	}`

	params := testutil.GraphQLParams{
		Query: addGroup,
		Variables: map[string]interface{}{
			"name": name,
		},
	}
	resp := makeRequest(t, accessToken, params)
	resp.RequireNoGraphQLErrors(t)
	return resp.Data
}

func createGroupWithRules(t *testing.T, accessJwt, name string, rules []rule) *group {
	queryParams := testutil.GraphQLParams{
		Query: `
		mutation addGroup($name: String!, $rules: [RuleRef]){
			addGroup(input: [
				{
					name: $name
					rules: $rules
				}
			]) {
				group {
					name
					rules {
						predicate
						permission
					}
				}
			}
		}`,
		Variables: map[string]interface{}{
			"name":  name,
			"rules": rules,
		},
	}
	resp := makeRequest(t, accessJwt, queryParams)
	resp.RequireNoGraphQLErrors(t)

	var addGroupResp struct {
		AddGroup struct {
			Group []group
		}
	}
	err := json.Unmarshal(resp.Data, &addGroupResp)
	require.NoError(t, err)
	require.Len(t, addGroupResp.AddGroup.Group, 1)

	return &addGroupResp.AddGroup.Group[0]
}

func updateGroup(t *testing.T, accessJwt, name string, setRules []rule,
	removeRules []string) *group {
	queryParams := testutil.GraphQLParams{
		Query: `
		mutation updateGroup($name: String!, $set: SetGroupPatch, $remove: RemoveGroupPatch){
			updateGroup(input: {
				filter: {
					name: {
						eq: $name
					}
				}
				set: $set
				remove: $remove
			}) {
				group {
					name
					rules {
						predicate
						permission
					}
				}
			}
		}`,
		Variables: map[string]interface{}{
			"name":   name,
			"set":    nil,
			"remove": nil,
		},
	}
	if len(setRules) != 0 {
		queryParams.Variables["set"] = map[string]interface{}{
			"rules": setRules,
		}
	}
	if len(removeRules) != 0 {
		queryParams.Variables["remove"] = map[string]interface{}{
			"rules": removeRules,
		}
	}
	resp := makeRequest(t, accessJwt, queryParams)
	resp.RequireNoGraphQLErrors(t)

	var result struct {
		UpdateGroup struct {
			Group []group
		}
	}
	err := json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)
	require.Len(t, result.UpdateGroup.Group, 1)

	return &result.UpdateGroup.Group[0]
}

func checkGroupCount(t *testing.T, resp []byte, expected int) {
	type Response struct {
		AddGroup struct {
			Group []struct {
				Name string
			}
		}
	}

	var r Response
	err := json.Unmarshal(resp, &r)
	require.NoError(t, err)
	require.Equal(t, expected, len(r.AddGroup.Group))
}

func addToGroup(t *testing.T, accessToken, userName, group string) {
	addUserToGroup := `mutation updateUser($name: String!, $group: String!) {
		updateUser(input: {
			filter: {
				name: {
					eq: $name
				}
			},
			set: {
				groups: [
					{ name: $group }
				]
			}
		}) {
			user {
				name
				groups {
					name
				}
			}
		}
	}`

	params := testutil.GraphQLParams{
		Query: addUserToGroup,
		Variables: map[string]interface{}{
			"name":  userName,
			"group": group,
		},
	}
	resp := makeRequest(t, accessToken, params)
	resp.RequireNoGraphQLErrors(t)

	var result struct {
		UpdateUser struct {
			User []struct {
				Name   string
				Groups []struct {
					Name string
				}
			}
			Name string
		}
	}
	err := json.Unmarshal(resp.Data, &result)
	require.NoError(t, err)

	// There should be a user in response.
	require.Len(t, result.UpdateUser.User, 1)
	// User's name must be <userName>
	require.Equal(t, userName, result.UpdateUser.User[0].Name)

	var foundGroup bool
	for _, usr := range result.UpdateUser.User {
		for _, grp := range usr.Groups {
			if grp.Name == group {
				foundGroup = true
				break
			}
		}
	}
	require.True(t, foundGroup)
}

type rule struct {
	Predicate  string `json:"predicate"`
	Permission int32  `json:"permission"`
}

type group struct {
	Name  string `json:"name"`
	Rules []rule `json:"rules"`
}

func makeRequest(t *testing.T, accessToken string, params testutil.GraphQLParams) *testutil.
	GraphQLResponse {
	return testutil.MakeGQLRequestWithAccessJwt(t, &params, accessToken)
}

func addRulesToGroup(t *testing.T, accessToken, group string, rules []rule) {
	addRuleToGroup := `mutation updateGroup($name: String!, $rules: [RuleRef!]!) {
		updateGroup(input: {
			filter: {
				name: {
					eq: $name
				}
			},
			set: {
				rules: $rules
			}
		}) {
			group {
				name
				rules {
					predicate
					permission
				}
			}
		}
	}`

	params := testutil.GraphQLParams{
		Query: addRuleToGroup,
		Variables: map[string]interface{}{
			"name":  group,
			"rules": rules,
		},
	}
	resp := makeRequest(t, accessToken, params)
	resp.RequireNoGraphQLErrors(t)
	rulesb, err := json.Marshal(rules)
	require.NoError(t, err)
	expectedOutput := fmt.Sprintf(`{
		  "updateGroup": {
			"group": [
			  {
				"name": "%s",
				"rules": %s
			  }
			]
		  }
	  }`, group, rulesb)
	testutil.CompareJSON(t, expectedOutput, string(resp.Data))
}

func createGroupAndAcls(t *testing.T, group string, addUserToGroup bool) {
	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	// create a new group
	resp := createGroup(t, accessJwt, group)
	checkGroupCount(t, resp, 1)

	// add the user to the group
	if addUserToGroup {
		addToGroup(t, accessJwt, userid, group)
	}

	rules := []rule{
		{
			predicateToRead, Read.Code,
		},
		{
			queryAttr, Read.Code,
		},
		{
			predicateToWrite, Write.Code,
		},
		{
			predicateToAlter, Modify.Code,
		},
	}

	// add READ permission on the predicateToRead to the group
	// also add read permission to the attribute queryAttr, which is used inside the query block
	// add WRITE permission on the predicateToWrite
	// add MODIFY permission on the predicateToAlter
	addRulesToGroup(t, accessJwt, group, rules)
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

	// The query should return emptry response, alter and mutation
	// should be blocked when no rule is defined.
	queryPredicateWithUserAccount(t, dg, false)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)
	createGroupAndAcls(t, unusedGroup, false)

	// Wait for 5 seconds to ensure the new acl have reached all acl caches.
	glog.Infof("Sleeping for 5 seconds for acl caches to be refreshed")
	time.Sleep(5 * time.Second)
	// The operations except query should fail when there is a rule defined, but the
	// current user is not allowed.
	queryPredicateWithUserAccount(t, dg, false)
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

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")
	createGroup(t, accessJwt, devGroup)

	addToGroup(t, accessJwt, userid, devGroup)

	txn := dg.NewTxn()
	mutation := &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("_:a <%s> \"testdata\" .", unAuthPred)),
		CommitNow: true,
	}
	resp, err := txn.Mutate(ctx, mutation)
	require.NoError(t, err)

	nodeUID, ok := resp.Uids["a"]
	require.True(t, ok)

	addRulesToGroup(t, accessJwt, devGroup, []rule{{unAuthPred, 0}})

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)

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

	addNewUserToGroup(t, "guardian", "guardianpass", "guardians")

	mutation := &api.Mutation{
		SetNquads: []byte("_:a <unauthpred> \"testdata\" ."),
		CommitNow: true,
	}
	resp, err := dg.NewTxn().Mutate(ctx, mutation)
	require.NoError(t, err)

	nodeUID, ok := resp.Uids["a"]
	require.True(t, ok)

	time.Sleep(5 * time.Second)
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

	gqlResp := removeUserFromGroup(t, "guardian", "guardians")
	gqlResp.RequireNoGraphQLErrors(t)
	expectedOutput := `{"updateUser":{"user":[{"name":"guardian","groups":[]}]}}`
	require.JSONEq(t, expectedOutput, string(gqlResp.Data))

	_, err = gClient.NewTxn().Query(ctx, query)
	require.Error(t, err, "Query succeeded. It should have failed.")
}

func addNewUserToGroup(t *testing.T, userName, password, groupName string) {
	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	resp := createUser(t, accessJwt, userName, password)
	resp.RequireNoGraphQLErrors(t)
	checkUserCount(t, resp.Data, 1)

	addToGroup(t, accessJwt, userName, groupName)
}

func removeUserFromGroup(t *testing.T, userName, groupName string) *testutil.GraphQLResponse {
	removeUserGroups := `mutation updateUser($name: String!, $groupName: String!) {
			updateUser(input: {
				filter: {
					name: {
						eq: $name
					}
				},
				remove: {
					groups: [{ name: $groupName }]
				}
			}) {
				user {
					name
					groups {
						name
					}
				}
			}
		}`

	params := testutil.GraphQLParams{
		Query: removeUserGroups,
		Variables: map[string]interface{}{
			"name":      userName,
			"groupName": groupName,
		},
	}

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")
	resp := makeRequest(t, accessJwt, params)
	return resp
}

func TestQueryRemoveUnauthorizedPred(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)
	op := api.Operation{Schema: `
		name	 : string @index(exact) .
		nickname : string @index(exact) .
		age 	 : int .
		type TypeName {
			name: string
			age: int
		}
	`}
	require.NoError(t, dg.Alter(ctx, &op))

	resetUser(t)
	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")
	createGroup(t, accessJwt, devGroup)
	addToGroup(t, accessJwt, userid, devGroup)

	txn := dg.NewTxn()
	mutation := &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "RandomGuy" .
			_:a <age> "23" .
			_:a <nickname> "RG" .
			_:a <dgraph.type> "TypeName" .
			_:b <name> "RandomGuy2" .
			_:b <age> "25" .
			_:b <nickname> "RG2" .
			_:b <dgraph.type> "TypeName" .
		`),
		CommitNow: true,
	}
	_, err = txn.Mutate(ctx, mutation)
	require.NoError(t, err)

	// give read access of <name> to alice
	addRulesToGroup(t, accessJwt, devGroup, []rule{{"name", Read.Code}})

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)

	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	tests := []struct {
		input       string
		output      string
		description string
	}{
		{
			`
			{
				me(func: has(name), orderasc: name) {
					name
					age
				}
			}
			`,
			`{"me":[{"name":"RandomGuy"},{"name":"RandomGuy2"}]}`,
			"alice doesn't have access to <age>",
		},
		{
			`
			{
				me(func: has(age), orderasc: name) {
					name
					age
				}
			}
			`,
			`{}`,
			`alice doesn't have access to <age> so "has(age)" is unauthorized`,
		},
		{
			`
			{
				me1(func: has(name), orderdesc: age) {
					age
				}
				me2(func: has(name), orderasc: age) {
					age
				}
			}
			`,
			`{"me1":[],"me2":[]}`,
			`me1, me2 will have same results, can't order by <age> since it is unauthorized`,
		},
		{
			`
			{
				me(func: has(name), orderasc: name) @groupby(age) {
					count(name)
				}
			}
			`,
			`{}`,
			`can't groupby <age> since <age> is unauthorized`,
		},
		{
			`
			{
				me(func: has(name), orderasc: name) @filter(eq(nickname, "RG")) {
					name
					age
				}
			}
			`,
			`{"me":[{"name":"RandomGuy"},{"name":"RandomGuy2"}]}`,
			`filter won't work because <nickname> is unauthorized`,
		},
		{
			`
			{
				me(func: has(name)) {
					expand(_all_)
				}
			}
			`,
			`{"me":[{"name":"RandomGuy"},{"name":"RandomGuy2"}]}`,
			`expand(_all_) expands to only <name> because other predicates are unauthorized`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()
			resp, err := userClient.NewTxn().Query(ctx, tc.input)
			require.Nil(t, err)
			testutil.CompareJSON(t, tc.output, string(resp.Json))
		})
	}
}

func TestExpandQueryWithACLPermissions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)

	op := api.Operation{Schema: `
		name	 : string @index(exact) .
		nickname : string @index(exact) .
		age 	 : int .
		type TypeName {
			name: string
			nickname: string
			age: int
		}
	`}
	require.NoError(t, dg.Alter(ctx, &op))

	resetUser(t)

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	createGroup(t, accessJwt, devGroup)
	createGroup(t, accessJwt, sreGroup)

	addRulesToGroup(t, accessJwt, sreGroup, []rule{{"age", Read.Code}, {"name", Write.Code}})
	addToGroup(t, accessJwt, userid, devGroup)

	txn := dg.NewTxn()
	mutation := &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "RandomGuy" .
			_:a <age> "23" .
			_:a <nickname> "RG" .
			_:a <dgraph.type> "TypeName" .
			_:b <name> "RandomGuy2" .
			_:b <age> "25" .
			_:b <nickname> "RG2" .
			_:b <dgraph.type> "TypeName" .
		`),
		CommitNow: true,
	}
	_, err = txn.Mutate(ctx, mutation)
	require.NoError(t, err)

	query := "{me(func: has(name)){expand(_all_)}}"

	// Test that groot has access to all the predicates
	resp, err := dg.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err, "Error while querying data")
	testutil.CompareJSON(t, `{"me":[{"name":"RandomGuy","age":23, "nickname":"RG"},{"name":"RandomGuy2","age":25, "nickname":"RG2"}]}`,
		string(resp.GetJson()))

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)

	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	// Query via user when user has no permissions
	resp, err = userClient.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err, "Error while querying data")
	testutil.CompareJSON(t, `{}`, string(resp.GetJson()))

	// Login to groot to modify accesses (1)
	accessJwt, _, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	// Give read access of <name>, write access of <age> to dev
	addRulesToGroup(t, accessJwt, devGroup, []rule{{"age", Write.Code}, {"name", Read.Code}})
	time.Sleep(5 * time.Second)
	resp, err = userClient.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err, "Error while querying data")
	testutil.CompareJSON(t, `{"me":[{"name":"RandomGuy"},{"name":"RandomGuy2"}]}`,
		string(resp.GetJson()))

	// Login to groot to modify accesses (2)
	accessJwt, _, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")
	// Add alice to sre group which has read access to <age> and write access to <name>
	addToGroup(t, accessJwt, userid, sreGroup)
	time.Sleep(5 * time.Second)

	resp, err = userClient.NewReadOnlyTxn().Query(ctx, query)
	require.Nil(t, err)

	testutil.CompareJSON(t, `{"me":[{"name":"RandomGuy","age":23},{"name":"RandomGuy2","age":25}]}`,
		string(resp.GetJson()))

	// Login to groot to modify accesses (3)
	accessJwt, _, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	// Give read access of <name> and <nickname>, write access of <age> to dev
	addRulesToGroup(t, accessJwt, devGroup, []rule{{"age", Write.Code}, {"name", Read.Code}, {"nickname", Read.Code}})
	time.Sleep(5 * time.Second)

	resp, err = userClient.NewReadOnlyTxn().Query(ctx, query)
	require.Nil(t, err)

	testutil.CompareJSON(t, `{"me":[{"name":"RandomGuy","age":23, "nickname":"RG"},{"name":"RandomGuy2","age":25, "nickname":"RG2"}]}`,
		string(resp.GetJson()))

}
func TestDeleteQueryWithACLPermissions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)

	op := api.Operation{Schema: `
		name	 : string @index(exact) .
		nickname : string @index(exact) .
		age 	 : int .
		type Person {
			name: string
			nickname: string
			age: int
		}
	`}
	require.NoError(t, dg.Alter(ctx, &op))

	resetUser(t)

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	createGroup(t, accessJwt, devGroup)

	addToGroup(t, accessJwt, userid, devGroup)

	txn := dg.NewTxn()
	mutation := &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "RandomGuy" .
			_:a <age> "23" .
			_:a <nickname> "RG" .
			_:a <dgraph.type> "Person" .
			_:b <name> "RandomGuy2" .
			_:b <age> "25" .
			_:b <nickname> "RG2" .
			_:b <dgraph.type> "Person" .
		`),
		CommitNow: true,
	}
	resp, err := txn.Mutate(ctx, mutation)
	require.NoError(t, err)

	nodeUID := resp.Uids["a"]
	query := `{q1(func: type(Person)){
		expand(_all_)
    }}`

	// Test that groot has access to all the predicates
	resp, err = dg.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err, "Error while querying data")
	testutil.CompareJSON(t, `{"q1":[{"name":"RandomGuy","age":23, "nickname": "RG"},{"name":"RandomGuy2","age":25,  "nickname": "RG2"}]}`,
		string(resp.GetJson()))

	// Give Write Access to alice for name and age predicate
	addRulesToGroup(t, accessJwt, devGroup, []rule{{"name", Write.Code}, {"age", Write.Code}})

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)

	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	// delete S * * (user now has permission to name and age)
	txn = userClient.NewTxn()
	mutString := fmt.Sprintf("<%s> * * .", nodeUID)
	mutation = &api.Mutation{
		DelNquads: []byte(mutString),
		CommitNow: true,
	}
	_, err = txn.Mutate(ctx, mutation)
	require.NoError(t, err)

	accessJwt, _, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	resp, err = dg.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err, "Error while querying data")
	// Only name and age predicates got deleted via user - alice
	testutil.CompareJSON(t, `{"q1":[{"nickname": "RG"},{"name":"RandomGuy2","age":25,  "nickname": "RG2"}]}`,
		string(resp.GetJson()))

	// Give write access of <name> <dgraph.type> to dev
	addRulesToGroup(t, accessJwt, devGroup, []rule{{"name", Write.Code}, {"age", Write.Code}, {"dgraph.type", Write.Code}})
	time.Sleep(5 * time.Second)

	// delete S * * (user now has permission to name, age and dgraph.type)
	txn = userClient.NewTxn()
	mutString = fmt.Sprintf("<%s> * * .", nodeUID)
	mutation = &api.Mutation{
		DelNquads: []byte(mutString),
		CommitNow: true,
	}
	_, err = txn.Mutate(ctx, mutation)
	require.NoError(t, err)

	accessJwt, _, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	resp, err = dg.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err, "Error while querying data")
	// Because alise had permission to dgraph.type the node reference has been deleted
	testutil.CompareJSON(t, `{"q1":[{"name":"RandomGuy2","age":25,  "nickname": "RG2"}]}`,
		string(resp.GetJson()))

}

func TestValQueryWithACLPermissions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)

	op := api.Operation{Schema: `
		name	 : string @index(exact) .
		nickname : string @index(exact) .
		age 	 : int .
		type TypeName {
			name: string
			nickname: string
			age: int
		}
	`}
	require.NoError(t, dg.Alter(ctx, &op))

	resetUser(t)

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	createGroup(t, accessJwt, devGroup)
	// createGroup(t, accessJwt, sreGroup)

	// addRulesToGroup(t, accessJwt, sreGroup, []rule{{"age", Read.Code}, {"name", Write.Code}})
	addToGroup(t, accessJwt, userid, devGroup)

	txn := dg.NewTxn()
	mutation := &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "RandomGuy" .
			_:a <age> "23" .
			_:a <nickname> "RG" .
			_:a <dgraph.type> "TypeName" .
			_:b <name> "RandomGuy2" .
			_:b <age> "25" .
			_:b <nickname> "RG2" .
			_:b <dgraph.type> "TypeName" .
		`),
		CommitNow: true,
	}
	_, err = txn.Mutate(ctx, mutation)
	require.NoError(t, err)

	query := `{q1(func: has(name)){
		v as name
		a as age
    }
    q2(func: eq(val(v), "RandomGuy")) {
		val(v)
		val(a)
	}}`

	// Test that groot has access to all the predicates
	resp, err := dg.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err, "Error while querying data")
	testutil.CompareJSON(t, `{"q1":[{"name":"RandomGuy","age":23},{"name":"RandomGuy2","age":25}],"q2":[{"val(v)":"RandomGuy","val(a)":23}]}`,
		string(resp.GetJson()))

	// All test cases
	tests := []struct {
		input                  string
		descriptionNoPerm      string
		outputNoPerm           string
		descriptionNamePerm    string
		outputNamePerm         string
		descriptionNameAgePerm string
		outputNameAgePerm      string
	}{
		{
			`
			{
				q1(func: has(name), orderasc: name) {
					n as name
					a as age
				}
				q2(func: eq(val(n), "RandomGuy")) {
					val(n)
					val(a)
				}
			}
			`,
			"alice doesn't have access to name or age",
			`{}`,

			`alice has access to name`,
			`{"q1":[{"name":"RandomGuy"},{"name":"RandomGuy2"}],"q2":[{"val(n)":"RandomGuy"}]}`,

			"alice has access to name and age",
			`{"q1":[{"name":"RandomGuy","age":23},{"name":"RandomGuy2","age":25}],"q2":[{"val(n)":"RandomGuy","val(a)":23}]}`,
		},
		{
			`{
				q1(func: has(name), orderasc: age) {
					a as age
				}
				q2(func: has(name)) {
					val(a)
				}
			}`,
			"alice doesn't have access to name or age",
			`{}`,

			`alice has access to name`,
			`{"q1":[],"q2":[]}`,

			"alice has access to name and age",
			`{"q1":[{"age":23},{"age":25}],"q2":[{"val(a)":23},{"val(a)":25}]}`,
		},
		{
			`{
				f as q1(func: has(name), orderasc: name) {
					n as name
					a as age
				}
				q2(func: uid(f), orderdesc: val(a), orderasc: name) {
					name
					val(n)
					val(a)
				}
			}`,
			"alice doesn't have access to name or age",
			`{"q2":[]}`,

			`alice has access to name`,
			`{"q1":[{"name":"RandomGuy"},{"name":"RandomGuy2"}],
			"q2":[{"name":"RandomGuy","val(n)":"RandomGuy"},{"name":"RandomGuy2","val(n)":"RandomGuy2"}]}`,

			"alice has access to name and age",
			`{"q1":[{"name":"RandomGuy","age":23},{"name":"RandomGuy2","age":25}],
			"q2":[{"name":"RandomGuy2","val(n)":"RandomGuy2","val(a)":25},{"name":"RandomGuy","val(n)":"RandomGuy","val(a)":23}]}`,
		},
		{
			`{
				f as q1(func: has(name), orderasc: name) {
					name
					age
				}
				q2(func: uid(f), orderasc: name) {
					name
					age
				}
			}`,
			"alice doesn't have access to name or age",
			`{"q2":[]}`,

			`alice has access to name`,
			`{"q1":[{"name":"RandomGuy"},{"name":"RandomGuy2"}],
			"q2":[{"name":"RandomGuy"},{"name":"RandomGuy2"}]}`,

			"alice has access to name and age",
			`{"q1":[{"name":"RandomGuy","age":23},{"name":"RandomGuy2","age":25}],
			"q2":[{"name":"RandomGuy2","age":25},{"name":"RandomGuy","age":23}]}`,
		},
	}

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)

	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	// Query via user when user has no permissions
	for _, tc := range tests {
		desc := tc.descriptionNoPerm
		t.Run(desc, func(t *testing.T) {
			resp, err := userClient.NewTxn().Query(ctx, tc.input)
			require.NoError(t, err)
			testutil.CompareJSON(t, tc.outputNoPerm, string(resp.Json))
		})
	}

	// Login to groot to modify accesses (1)
	accessJwt, _, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	// Give read access of <name> to dev
	addRulesToGroup(t, accessJwt, devGroup, []rule{{"name", Read.Code}})
	time.Sleep(5 * time.Second)

	for _, tc := range tests {
		desc := tc.descriptionNamePerm
		t.Run(desc, func(t *testing.T) {
			resp, err := userClient.NewTxn().Query(ctx, tc.input)
			require.NoError(t, err)
			testutil.CompareJSON(t, tc.outputNamePerm, string(resp.Json))
		})
	}

	// Login to groot to modify accesses (1)
	accessJwt, _, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	// Give read access of <name> and <age> to dev
	addRulesToGroup(t, accessJwt, devGroup, []rule{{"name", Read.Code}, {"age", Read.Code}})
	time.Sleep(5 * time.Second)

	for _, tc := range tests {
		desc := tc.descriptionNameAgePerm
		t.Run(desc, func(t *testing.T) {
			resp, err := userClient.NewTxn().Query(ctx, tc.input)
			require.NoError(t, err)
			testutil.CompareJSON(t, tc.outputNameAgePerm, string(resp.Json))
		})
	}

}
func TestNewACLPredicates(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)

	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	queryTests := []struct {
		input       string
		output      string
		description string
	}{
		{
			`
			{
				me(func: has(name)) {
					name
					nickname
				}
			}
			`,
			`{"me":[{"name":"RandomGuy"},{"name":"RandomGuy2"}]}`,
			"alice doesn't have read access to <nickname>",
		},
		{
			`
			{
				me(func: has(nickname)) {
					name
					nickname
				}
			}
			`,
			`{}`,
			`alice doesn't have access to <nickname> so "has(nickname)" is unauthorized`,
		},
	}

	for _, tc := range queryTests {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()
			resp, err := userClient.NewTxn().Query(ctx, tc.input)
			require.Nil(t, err)
			testutil.CompareJSON(t, tc.output, string(resp.Json))
		})
	}

	mutationTests := []struct {
		input       string
		output      string
		err         error
		description string
	}{
		{
			"_:a <name> \"Animesh\" .",
			"",
			errors.New(""),
			"alice doesn't have write access on <name>.",
		},
		{
			"_:a <nickname> \"Pathak\" .",
			"",
			nil,
			"alice can mutate <nickname> predicate.",
		},
	}
	for _, tc := range mutationTests {
		t.Run(tc.description, func(t *testing.T) {
			_, err := userClient.NewTxn().Mutate(ctx, &api.Mutation{
				SetNquads: []byte(tc.input),
				CommitNow: true,
			})
			require.True(t, (err == nil) == (tc.err == nil))
		})
	}
}

func removeRuleFromGroup(t *testing.T, accessToken, group string,
	rulePredicate string) *testutil.GraphQLResponse {
	removeRuleFromGroup := `mutation updateGroup($name: String!, $rules: [String!]!) {
		updateGroup(input: {
			filter: {
				name: {
					eq: $name
				}
			},
			remove: {
				rules: $rules
			}
		}) {
			group {
				name
				rules {
					predicate
					permission
				}
			}
		}
	}`

	params := testutil.GraphQLParams{
		Query: removeRuleFromGroup,
		Variables: map[string]interface{}{
			"name":  group,
			"rules": []string{rulePredicate},
		},
	}
	resp := makeRequest(t, accessToken, params)
	return resp
}

func TestDeleteRule(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	_ = addDataAndRules(ctx, t, dg)

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)

	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	queryName := "{me(func: has(name)) {name}}"
	resp, err := userClient.NewReadOnlyTxn().Query(ctx, queryName)
	require.NoError(t, err, "Error while querying data")

	testutil.CompareJSON(t, `{"me":[{"name":"RandomGuy"},{"name":"RandomGuy2"}]}`,
		string(resp.GetJson()))

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")
	removeRuleFromGroup(t, accessJwt, devGroup, "name")
	time.Sleep(5 * time.Second)

	resp, err = userClient.NewReadOnlyTxn().Query(ctx, queryName)
	require.NoError(t, err, "Error while querying data")
	testutil.CompareJSON(t, string(resp.GetJson()), `{}`)
}

func addDataAndRules(ctx context.Context, t *testing.T, dg *dgo.Dgraph) map[string]string {
	testutil.DropAll(t, dg)
	op := api.Operation{Schema: `
		name	 : string @index(exact) .
		nickname : string @index(exact) .
	`}
	require.NoError(t, dg.Alter(ctx, &op))

	resetUser(t)

	// TODO - We should be adding this data using the GraphQL API.
	// We create three groups here, dev, dev-a and dev-b and add alice to two of them.
	devGroupMut := `
		_:g  <dgraph.xid>        "dev" .
		_:g  <dgraph.type>       "dgraph.type.Group" .
		_:g1  <dgraph.xid>       "dev-a" .
		_:g1  <dgraph.type>      "dgraph.type.Group" .
		_:g2  <dgraph.xid>       "dev-b" .
		_:g2  <dgraph.type>      "dgraph.type.Group" .
		_:g  <dgraph.acl.rule>   _:r1 .
		_:r1 <dgraph.type> "dgraph.type.Rule" .
		_:r1 <dgraph.rule.predicate>  "name" .
		_:r1 <dgraph.rule.permission> "4" .
		_:g  <dgraph.acl.rule>   _:r2 .
		_:r2 <dgraph.type> "dgraph.type.Rule" .
		_:r2 <dgraph.rule.predicate>  "nickname" .
		_:r2 <dgraph.rule.permission> "2" .
	`
	resp, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(devGroupMut),
		CommitNow: true,
	})
	require.NoError(t, err, "Error adding group and permissions")

	idQuery := fmt.Sprintf(`
	{
		userid as var(func: eq(dgraph.xid, "%s"))
		gid as var(func: eq(dgraph.type, "dgraph.type.Group")) @filter(eq(dgraph.xid, "dev") OR
			eq(dgraph.xid, "dev-a"))
	}`, userid)
	addAliceToGroups := &api.NQuad{
		Subject:   "uid(userid)",
		Predicate: "dgraph.user.group",
		ObjectId:  "uid(gid)",
	}
	_, err = dg.NewTxn().Do(ctx, &api.Request{
		CommitNow: true,
		Query:     idQuery,
		Mutations: []*api.Mutation{
			{
				Set: []*api.NQuad{addAliceToGroups},
			},
		},
	})
	require.NoError(t, err, "Error adding user to dev group")

	mutation := &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "RandomGuy" .
			_:a <nickname> "RG" .
			_:b <name> "RandomGuy2" .
			_:b <age> "25" .
			_:b <nickname> "RG2" .
		`),
		CommitNow: true,
	}
	_, err = dg.NewTxn().Mutate(ctx, mutation)
	require.NoError(t, err)
	return resp.GetUids()
}

func TestNonExistentGroup(t *testing.T) {
	t.Skip()
	// This test won't return an error anymore as if an update in a GraphQL mutation doesn't find
	// anything to update then it just returns an empty result.
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")
	addRulesToGroup(t, accessJwt, devGroup, []rule{{"name", Read.Code}})
}

func TestQueryUserInfo(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   userid,
		Passwd:   userpassword,
	})
	require.NoError(t, err, "login failed")

	gqlQuery := `
	query {
		queryUser {
			name
			groups {
				name
				rules {
					predicate
					permission
				}
				users {
					name
				}
			}
		}
	}
	`

	params := testutil.GraphQLParams{
		Query: gqlQuery,
	}
	gqlResp := makeRequest(t, accessJwt, params)
	gqlResp.RequireNoGraphQLErrors(t)

	testutil.CompareJSON(t, `
	{
	  "queryUser": [
		{
		  "name": "alice",
		  "groups": [
			{
			  "name": "dev",
			  "rules": [
				{
				  "predicate": "name",
				  "permission": 4
				},
				{
				  "predicate": "nickname",
				  "permission": 2
				}
			  ],
			  "users": [
				  {
					  "name": "alice"
				  }
			  ]
			},
			{
				"name": "dev-a",
				"rules": [],
				"users": [
					{
						"name": "alice"
					}
				]
			  }
		  ]
		}
	  ]
	}`, string(gqlResp.Data))

	query := `
	{
		me(func: type(dgraph.type.User)) {
			dgraph.xid
			dgraph.user.group {
				dgraph.xid
				dgraph.acl.rule {
					dgraph.rule.predicate
					dgraph.rule.permission
				}
			}
		}
	}
	`

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)

	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	resp, err := userClient.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err, "Error while querying ACL")

	testutil.CompareJSON(t, `{"me":[]}`, string(resp.GetJson()))

	gqlQuery = `
	query {
		queryGroup {
			name
			users {
				name
			}
			rules {
				predicate
				permission
			}
		}
	}
	`

	params = testutil.GraphQLParams{
		Query: gqlQuery,
	}
	gqlResp = makeRequest(t, accessJwt, params)
	gqlResp.RequireNoGraphQLErrors(t)
	// The user should only be able to see their group dev and themselves as the user.
	testutil.CompareJSON(t, `{
		  "queryGroup": [
			{
			  "name": "dev",
			  "users": [
				{
				  "name": "alice"
				}
			  ],
			  "rules": [
				{
				  "predicate": "name",
				  "permission": 4
				},
				{
				  "predicate": "nickname",
				  "permission": 2
				}
			  ]
			},
			{
				"name": "dev-a",
				"users": [
				  {
					"name": "alice"
				  }
				],
				"rules": []
			  }

		  ]
	  }`, string(gqlResp.Data))

	gqlQuery = `
	query {
		getGroup(name: "guardians") {
			name
			rules {
				predicate
				permission
			}
			users {
				name
			}
		}
	}
	`

	params = testutil.GraphQLParams{
		Query: gqlQuery,
	}
	gqlResp = makeRequest(t, accessJwt, params)
	gqlResp.RequireNoGraphQLErrors(t)
	testutil.CompareJSON(t, `{"getGroup": null}`, string(gqlResp.Data))
}

func TestQueriesForNonGuardianUserWithoutGroup(t *testing.T) {
	// Create a new user without any groups, queryGroup should return an empty result.
	resetUser(t)

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   userid,
		Passwd:   userpassword,
	})
	require.NoError(t, err, "login failed")

	gqlQuery := `
	query {
		queryGroup {
			name
			users {
				name
			}
		}
	}
	`

	params := testutil.GraphQLParams{
		Query: gqlQuery,
	}
	resp := makeRequest(t, accessJwt, params)
	resp.RequireNoGraphQLErrors(t)
	testutil.CompareJSON(t, `{"queryGroup": []}`, string(resp.Data))

	gqlQuery = `
	query {
		queryUser {
			name
			groups {
				name
			}
		}
	}
	`

	params = testutil.GraphQLParams{
		Query: gqlQuery,
	}
	resp = makeRequest(t, accessJwt, params)
	resp.RequireNoGraphQLErrors(t)
	testutil.CompareJSON(t, `{"queryUser": [{ "groups": [], "name": "alice"}]}`, string(resp.Data))
}

func TestSchemaQueryWithACL(t *testing.T) {
	schemaQuery := "schema{}"
	grootSchema := `{
  "schema": [
    {
      "predicate": "dgraph.acl.rule",
      "type": "uid",
      "list": true
	},
	{
		"predicate": "dgraph.cors",
		"type": "string",
		"list": true,
		"index": true,
      	"tokenizer": [
          "exact"
      	],
      	"upsert": true
	},
    {
      "predicate": "dgraph.graphql.schema",
      "type": "string"
	},
	{
		"predicate": "dgraph.graphql.schema_created_at",
		"type": "datetime"
	},
	{
		"predicate": "dgraph.graphql.schema_history",
		"type": "string"
	},
    {
      "predicate": "dgraph.graphql.xid",
      "type": "string",
      "index": true,
      "tokenizer": [
        "exact"
      ],
      "upsert": true
    },
    {
      "predicate": "dgraph.password",
      "type": "password"
    },
    {
      "predicate": "dgraph.rule.permission",
      "type": "int"
    },
    {
      "predicate": "dgraph.rule.predicate",
      "type": "string",
      "index": true,
      "tokenizer": [
        "exact"
      ],
      "upsert": true
    },
    {
      "predicate": "dgraph.type",
      "type": "string",
      "index": true,
      "tokenizer": [
        "exact"
      ],
      "list": true
    },
    {
      "predicate": "dgraph.user.group",
      "type": "uid",
      "reverse": true,
      "list": true
    },
    {
      "predicate": "dgraph.xid",
      "type": "string",
      "index": true,
      "tokenizer": [
        "exact"
      ],
      "upsert": true
    }
  ],
  "types": [
    {
      "fields": [
        {
          "name": "dgraph.graphql.schema"
        },
        {
          "name": "dgraph.graphql.xid"
        }
      ],
      "name": "dgraph.graphql"
	},
	{
		"fields": [
			{
				"name": "dgraph.graphql.schema_history"
			},{
				"name": "dgraph.graphql.schema_created_at"
			}
		],
		"name": "dgraph.graphql.history"
	},
    {
      "fields": [
        {
          "name": "dgraph.xid"
        },
        {
          "name": "dgraph.acl.rule"
        }
      ],
      "name": "dgraph.type.Group"
    },
    {
      "fields": [
        {
          "name": "dgraph.rule.predicate"
        },
        {
          "name": "dgraph.rule.permission"
        }
      ],
      "name": "dgraph.type.Rule"
    },
    {
      "fields": [
        {
          "name": "dgraph.xid"
        },
        {
          "name": "dgraph.password"
        },
        {
          "name": "dgraph.user.group"
        }
      ],
      "name": "dgraph.type.User"
    }
  ]
}`
	aliceSchema := `{
  "schema": [
    {
      "predicate": "name",
      "type": "string",
      "index": true,
      "tokenizer": [
        "exact"
      ]
    }
  ],
  "types": [
    {
      "fields": [],
      "name": "dgraph.graphql"
	},
	{
		"fields": [],
		"name": "dgraph.graphql.history"
	},
    {
      "fields": [],
      "name": "dgraph.type.Group"
    },
    {
      "fields": [],
      "name": "dgraph.type.Rule"
    },
    {
      "fields": [],
      "name": "dgraph.type.User"
    }
  ]
}`

	// guardian user should be able to view full schema
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	testutil.DropAll(t, dg)
	resp, err := dg.NewReadOnlyTxn().Query(context.Background(), schemaQuery)
	require.NoError(t, err)
	require.JSONEq(t, grootSchema, string(resp.GetJson()))

	// add another user and some data for that user with permissions on predicates
	resetUser(t)
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)
	addDataAndRules(ctx, t, dg)
	time.Sleep(5 * time.Second) // wait for ACL cache to refresh, otherwise it will be flaky test

	// the other user should be able to view only the part of schema for which it has read access
	dg, err = testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	require.NoError(t, dg.Login(context.Background(), userid, userpassword))
	resp, err = dg.NewReadOnlyTxn().Query(context.Background(), schemaQuery)
	require.NoError(t, err)
	require.JSONEq(t, aliceSchema, string(resp.GetJson()))
}

func TestDeleteUserShouldDeleteUserFromGroup(t *testing.T) {
	resetUser(t)

	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   x.GrootId,
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	deleteUser(t, accessJwt, userid, true)

	gqlQuery := `
	query {
		queryUser {
			name
		}
	}
	`

	params := testutil.GraphQLParams{
		Query: gqlQuery,
	}
	resp := makeRequest(t, accessJwt, params)
	resp.RequireNoGraphQLErrors(t)
	require.JSONEq(t, `{"queryUser":[{"name":"groot"}]}`, string(resp.Data))

	// The user should also be deleted from the dev group.
	gqlQuery = `
	query {
		queryGroup {
			name
			users {
				name
			}
		}
	}
	`

	params = testutil.GraphQLParams{
		Query: gqlQuery,
	}
	resp = makeRequest(t, accessJwt, params)
	resp.RequireNoGraphQLErrors(t)
	testutil.CompareJSON(t, `{
		  "queryGroup": [
			{
			  "name": "guardians",
			  "users": [
				{
				  "name": "groot"
				}
			  ]
			},
			{
			  "name": "dev",
			  "users": []
			},
			{
				"name": "dev-a",
				"users": []
			},
			{
				"name": "dev-b",
				"users": []
			}
		  ]
	  }`, string(resp.Data))
}

func TestGroupDeleteShouldDeleteGroupFromUser(t *testing.T) {
	resetUser(t)

	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   x.GrootId,
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	deleteGroup(t, accessJwt, "dev-a")

	gqlQuery := `
	query {
		queryGroup {
			name
		}
	}
	`

	params := testutil.GraphQLParams{
		Query: gqlQuery,
	}
	resp := makeRequest(t, accessJwt, params)
	resp.RequireNoGraphQLErrors(t)
	testutil.CompareJSON(t, `{
		  "queryGroup": [
			{
			  "name": "guardians"
			},
			{
			  "name": "dev"
			},
			{
			  "name": "dev-b"
			}
		  ]
	  }`, string(resp.Data))

	gqlQuery = `
	query {
		getUser(name: "alice") {
			name
			groups {
				name
			}
		}
	}
	`

	params = testutil.GraphQLParams{
		Query: gqlQuery,
	}
	resp = makeRequest(t, accessJwt, params)
	resp.RequireNoGraphQLErrors(t)
	testutil.CompareJSON(t, `{
		"getUser": {
			"name": "alice",
			"groups": [
				{
					"name": "dev"
				}
			]
		}
	}`, string(resp.Data))
}

func TestWrongPermission(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	ruleMutation := `
		_:dev <dgraph.type> "dgraph.type.Group" .
		_:dev <dgraph.xid> "dev" .
		_:dev <dgraph.acl.rule> _:rule1 .
		_:rule1 <dgraph.rule.predicate> "name" .
		_:rule1 <dgraph.rule.permission> "9" .
	`

	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(ruleMutation),
		CommitNow: true,
	})

	require.Error(t, err, "Setting permission to 9 should have returned error")
	require.Contains(t, err.Error(), "Value for this predicate should be between 0 and 7")

	ruleMutation = `
		_:dev <dgraph.type> "dgraph.type.Group" .
		_:dev <dgraph.xid> "dev" .
		_:dev <dgraph.acl.rule> _:rule1 .
		_:rule1 <dgraph.rule.predicate> "name" .
		_:rule1 <dgraph.rule.permission> "-1" .
	`

	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(ruleMutation),
		CommitNow: true,
	})

	require.Error(t, err, "Setting permission to -1 should have returned error")
	require.Contains(t, err.Error(), "Value for this predicate should be between 0 and 7")
}

func TestHealthForAcl(t *testing.T) {
	params := testutil.GraphQLParams{
		Query: `
		query {
			health {
				instance
				address
				lastEcho
				status
				version
				uptime
				group
			}
		}`,
	}

	// assert errors for non-guardians
	assertNonGuardianFailure(t, "health", false, params)

	// assert data for guardians
	accessJwt, _ := testutil.GrootHttpLogin(adminEndpoint)

	resp := makeRequest(t, accessJwt, params)
	resp.RequireNoGraphQLErrors(t)
	var guardianResp struct {
		Health []struct {
			Instance string
			Address  string
			LastEcho int64
			Status   string
			Version  string
			UpTime   int64
			Group    string
		}
	}
	err := json.Unmarshal(resp.Data, &guardianResp)

	require.NoError(t, err, "health request failed")
	// we have 9 instances of alphas/zeros in teamcity environment
	require.Len(t, guardianResp.Health, 9)
	for _, v := range guardianResp.Health {
		require.Contains(t, []string{"alpha", "zero"}, v.Instance)
		require.NotEmpty(t, v.Address)
		require.NotEmpty(t, v.LastEcho)
		require.Equal(t, "healthy", v.Status)
		require.NotEmpty(t, v.Version)
		require.NotEmpty(t, v.UpTime)
		require.NotEmpty(t, v.Group)
	}
}

func assertNonGuardianFailure(t *testing.T, queryName string, respIsNull bool,
	params testutil.GraphQLParams) {
	resetUser(t)

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   userid,
		Passwd:   userpassword,
	})
	require.NoError(t, err, "login failed")
	resp := makeRequest(t, accessJwt, params)

	require.Len(t, resp.Errors, 1)
	require.Contains(t, resp.Errors[0].Message,
		fmt.Sprintf("rpc error: code = PermissionDenied desc = Only guardians are allowed access."+
			" User '%s' is not a member of guardians group.", userid))
	if len(resp.Data) != 0 {
		queryVal := "null"
		if !respIsNull {
			queryVal = "[]"
		}
		require.JSONEq(t, fmt.Sprintf(`{"%s": %s}`, queryName, queryVal), string(resp.Data))
	}
}

type graphQLAdminEndpointTestCase struct {
	name               string
	query              string
	queryName          string
	respIsArray        bool
	testGuardianAccess bool
	guardianErrs       x.GqlErrorList
	// specifying this as empty string means it won't be compared with response data
	guardianData string
}

func TestGuardianOnlyAccessForAdminEndpoints(t *testing.T) {
	tcases := []graphQLAdminEndpointTestCase{
		{
			name: "backup has guardian auth",
			query: `
					mutation {
					  backup(input: {destination: ""}) {
						response {
						  code
						  message
						}
					  }
					}`,
			queryName:          "backup",
			testGuardianAccess: true,
			guardianErrs: x.GqlErrorList{{
				Message:   "resolving backup failed because you must specify a 'destination' value",
				Locations: []x.Location{{Line: 3, Column: 8}},
			}},
			guardianData: `{"backup": null}`,
		},
		{
			name: "listBackups has guardian auth",
			query: `
					query {
					  listBackups(input: {location: ""}) {
					  	backupId
					  }
					}`,
			queryName:          "listBackups",
			respIsArray:        true,
			testGuardianAccess: true,
			guardianErrs: x.GqlErrorList{{
				Message: "resolving listBackups failed because Error: cannot read manfiests at " +
					"location : The path \"\" does not exist or it is inaccessible.",
				Locations: []x.Location{{Line: 3, Column: 8}},
			}},
			guardianData: `{"listBackups": []}`,
		},
		{
			name: "config update has guardian auth",
			query: `
					mutation {
					  config(input: {cacheMb: -1}) {
						response {
						  code
						  message
						}
					  }
					}`,
			queryName:          "config",
			testGuardianAccess: true,
			guardianErrs: x.GqlErrorList{{
				Message:   "resolving config failed because cache_mb must be non-negative",
				Locations: []x.Location{{Line: 3, Column: 8}},
			}},
			guardianData: `{"config": null}`,
		},
		{
			name: "config get has guardian auth",
			query: `
					query {
					  config {
						cacheMb
					  }
					}`,
			queryName:          "config",
			testGuardianAccess: true,
			guardianErrs:       nil,
			guardianData:       "",
		},
		{
			name: "draining has guardian auth",
			query: `
					mutation {
					  draining(enable: false) {
						response {
						  code
						  message
						}
					  }
					}`,
			queryName:          "draining",
			testGuardianAccess: true,
			guardianErrs:       nil,
			guardianData: `{
								"draining": {
									"response": {
										"code": "Success",
										"message": "draining mode has been set to false"
									}
								}
							}`,
		},
		{
			name: "export has guardian auth",
			query: `
					mutation {
					  export(input: {format: "invalid"}) {
						response {
						  code
						  message
						}
					  }
					}`,
			queryName:          "export",
			testGuardianAccess: true,
			guardianErrs: x.GqlErrorList{{
				Message:   "resolving export failed because invalid export format: invalid",
				Locations: []x.Location{{Line: 3, Column: 8}},
			}},
			guardianData: `{"export": null}`,
		},
		{
			name: "restore has guardian auth",
			query: `
					mutation {
					  restore(input: {location: "", backupId: "", encryptionKeyFile: ""}) {
						code
					  }
					}`,
			queryName:          "restore",
			testGuardianAccess: true,
			guardianErrs: x.GqlErrorList{{
				Message: "resolving restore failed because failed to verify backup: while retrieving" +
					" manifests: The path \"\" does not exist or it is inaccessible.",
				Locations: []x.Location{{Line: 3, Column: 8}},
			}},
			guardianData: `{"restore": {"code": "Failure"}}`,
		},
		{
			name: "getGQLSchema has guardian auth",
			query: `
					query {
					  getGQLSchema {
						id
					  }
					}`,
			queryName:          "getGQLSchema",
			testGuardianAccess: true,
			guardianErrs:       nil,
			guardianData:       "",
		},
		{
			name: "updateGQLSchema has guardian auth",
			query: `
					mutation {
					  updateGQLSchema(input: {set: {schema: ""}}) {
						gqlSchema {
						  id
						}
					  }
					}`,
			queryName:          "updateGQLSchema",
			testGuardianAccess: false,
			guardianErrs:       nil,
			guardianData:       "",
		},
		{
			name: "shutdown has guardian auth",
			query: `
					mutation {
					  shutdown {
						response {
						  code
						  message
						}
					  }
					}`,
			queryName:          "shutdown",
			testGuardianAccess: false,
			guardianErrs:       nil,
			guardianData:       "",
		},
	}

	for _, tcase := range tcases {
		t.Run(tcase.name, func(t *testing.T) {
			params := testutil.GraphQLParams{Query: tcase.query}

			// assert ACL error for non-guardians
			assertNonGuardianFailure(t, tcase.queryName, !tcase.respIsArray, params)

			// for guardians, assert non-ACL error or success
			if tcase.testGuardianAccess {
				accessJwt, _ := testutil.GrootHttpLogin(adminEndpoint)
				resp := makeRequest(t, accessJwt, params)

				if tcase.guardianErrs == nil {
					resp.RequireNoGraphQLErrors(t)
				} else {
					require.Equal(t, tcase.guardianErrs, resp.Errors)
				}

				if tcase.guardianData != "" {
					require.JSONEq(t, tcase.guardianData, string(resp.Data))
				}
			}
		})
	}
}

func TestAddUpdateGroupWithDuplicateRules(t *testing.T) {
	groupName := "testGroup"
	addedRules := []rule{
		{
			Predicate:  "test",
			Permission: 1,
		},
		{
			Predicate:  "test",
			Permission: 2,
		},
		{
			Predicate:  "test1",
			Permission: 3,
		},
	}
	grootJwt, _ := testutil.GrootHttpLogin(adminEndpoint)

	addedGroup := createGroupWithRules(t, grootJwt, groupName, addedRules)

	require.Equal(t, groupName, addedGroup.Name)
	require.Len(t, addedGroup.Rules, 2)
	require.ElementsMatch(t, addedRules[1:], addedGroup.Rules)

	updatedRules := []rule{
		{
			Predicate:  "test",
			Permission: 3,
		},
		{
			Predicate:  "test2",
			Permission: 1,
		},
		{
			Predicate:  "test2",
			Permission: 2,
		},
	}
	updatedGroup := updateGroup(t, grootJwt, groupName, updatedRules, nil)

	require.Equal(t, groupName, updatedGroup.Name)
	require.Len(t, updatedGroup.Rules, 3)
	require.ElementsMatch(t, []rule{updatedRules[0], addedRules[2], updatedRules[2]},
		updatedGroup.Rules)

	updatedGroup1 := updateGroup(t, grootJwt, groupName, nil,
		[]string{"test1", "test1", "test3"})

	require.Equal(t, groupName, updatedGroup1.Name)
	require.Len(t, updatedGroup1.Rules, 2)
	require.ElementsMatch(t, []rule{updatedRules[0], updatedRules[2]}, updatedGroup1.Rules)

	// cleanup
	deleteGroup(t, grootJwt, groupName)
}

func TestAllowUIDAccess(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)
	op := api.Operation{Schema: `
		name	 : string @index(exact) .
	`}
	require.NoError(t, dg.Alter(ctx, &op))

	resetUser(t)
	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")
	createGroup(t, accessJwt, devGroup)
	addToGroup(t, accessJwt, userid, devGroup)

	require.NoError(t, testutil.AssignUids(101))
	mutation := &api.Mutation{
		SetNquads: []byte(`
			<100> <name> "100th User" .
		`),
		CommitNow: true,
	}
	_, err = dg.NewTxn().Mutate(ctx, mutation)
	require.NoError(t, err)

	// give read access of <name> to alice
	addRulesToGroup(t, accessJwt, devGroup, []rule{{"name", Read.Code}})

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(5 * time.Second)

	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	uidQuery := `
	{
		me(func: uid(100)) {
			uid
			name
		}
	}
	`

	resp, err := userClient.NewReadOnlyTxn().Query(ctx, uidQuery)
	require.Nil(t, err)
	testutil.CompareJSON(t, `{"me":[{"name":"100th User", "uid": "0x64"}]}`, string(resp.GetJson()))
}

func TestAddNewPredicate(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)
	resetUser(t)

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	// Alice doesn't have access to create new predicate.
	err = userClient.Alter(ctx, &api.Operation{
		Schema: `newpred: string .`,
	})
	require.Error(t, err, "User can't create new predicate. Alter should have returned error.")

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")
	addToGroup(t, accessJwt, userid, "guardians")
	time.Sleep(5 * time.Second)

	// Alice is a guardian now, it can create new predicate.
	err = userClient.Alter(ctx, &api.Operation{
		Schema: `newpred: string .`,
	})
	require.NoError(t, err, "User is a guardian. Alter should have succeeded.")
}

func TestCrossGroupPermission(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)

	err = dg.Alter(ctx, &api.Operation{
		Schema: `newpred: string .`,
	})
	require.NoError(t, err)

	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err)

	// TODO(@Animesh): I am Running all the operations with the same accessJwt
	// but access token will expire after 3s. Find an idomatic way to run these.

	// create groups
	createGroup(t, accessJwt, "reader")
	createGroup(t, accessJwt, "writer")
	createGroup(t, accessJwt, "alterer")

	// add rules to groups
	addRulesToGroup(t, accessJwt, "reader", []rule{{Predicate: "newpred", Permission: 4}})
	addRulesToGroup(t, accessJwt, "writer", []rule{{Predicate: "newpred", Permission: 2}})
	addRulesToGroup(t, accessJwt, "alterer", []rule{{Predicate: "newpred", Permission: 1}})
	// Wait for acl cache to be refreshed
	time.Sleep(5 * time.Second)

	accessJwt, _, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err)

	// create 8 users.
	for i := 0; i < 8; i++ {
		userIdx := strconv.Itoa(i)
		createUser(t, accessJwt, "user"+userIdx, "password"+userIdx)
	}

	// add users to groups. we create all possible combination
	// of groups and assign a user for that combination.
	for i := 0; i < 8; i++ {
		userIdx := strconv.Itoa(i)
		if i&1 > 0 {
			addToGroup(t, accessJwt, "user"+userIdx, "alterer")
		}
		if i&2 > 0 {
			addToGroup(t, accessJwt, "user"+userIdx, "writer")
		}
		if i&4 > 0 {
			addToGroup(t, accessJwt, "user"+userIdx, "reader")
		}
	}
	time.Sleep(5 * time.Second)

	// operations
	dgQuery := func(client *dgo.Dgraph, shouldFail bool, user string) {
		_, err := client.NewTxn().Query(ctx, `
		{
			me(func: has(newpred)) {
				newpred
			}
		}
		`)
		require.True(t, (err != nil) == shouldFail,
			"Query test Failed for: "+user+", shouldFail: "+strconv.FormatBool(shouldFail))
	}
	dgMutation := func(client *dgo.Dgraph, shouldFail bool, user string) {
		_, err := client.NewTxn().Mutate(ctx, &api.Mutation{
			Set: []*api.NQuad{
				{
					Subject:     "_:a",
					Predicate:   "newpred",
					ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "testval"}},
				},
			},
			CommitNow: true,
		})
		require.True(t, (err != nil) == shouldFail,
			"Mutation test failed for: "+user+", shouldFail: "+strconv.FormatBool(shouldFail))
	}
	dgAlter := func(client *dgo.Dgraph, shouldFail bool, user string) {
		err := client.Alter(ctx, &api.Operation{Schema: `newpred: string @index(exact) .`})
		require.True(t, (err != nil) == shouldFail,
			"Alter test failed for: "+user+", shouldFail: "+strconv.FormatBool(shouldFail))

		// set back the schema to initial value
		err = client.Alter(ctx, &api.Operation{Schema: `newpred: string .`})
		require.True(t, (err != nil) == shouldFail,
			"Alter test failed for: "+user+", shouldFail: "+strconv.FormatBool(shouldFail))
	}

	// test user access.
	for i := 0; i < 8; i++ {
		userIdx := strconv.Itoa(i)
		userClient, err := testutil.DgraphClient(testutil.SockAddr)
		require.NoError(t, err, "Client creation error")

		err = userClient.Login(ctx, "user"+userIdx, "password"+userIdx)
		require.NoError(t, err, "Login error")

		dgQuery(userClient, false, "user"+userIdx) // Query won't fail, will return empty result instead.
		dgMutation(userClient, i&2 == 0, "user"+userIdx)
		dgAlter(userClient, i&1 == 0, "user"+userIdx)
	}
}

func TestMutationWithValueVar(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)

	err = dg.Alter(ctx, &api.Operation{
		Schema: `
			name	: string @index(exact) .
			nickname: string .
			age     : int .
		`,
	})
	require.NoError(t, err)

	data := &api.Mutation{
		SetNquads: []byte(`
			_:u1 <name> "RandomGuy" .
			_:u1 <nickname> "r1" .
		`),
		CommitNow: true,
	}
	_, err = dg.NewTxn().Mutate(ctx, data)
	require.NoError(t, err)

	resetUser(t)
	accessJwt, _, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err)
	createUser(t, accessJwt, userid, userpassword)
	createGroup(t, accessJwt, devGroup)
	addToGroup(t, accessJwt, userid, devGroup)
	addRulesToGroup(t, accessJwt, devGroup, []rule{
		{
			Predicate:  "name",
			Permission: Read.Code | Write.Code,
		},
		{
			Predicate:  "nickname",
			Permission: Read.Code,
		},
		{
			Predicate:  "age",
			Permission: Write.Code,
		},
	})
	time.Sleep(5 * time.Second)

	query := `
		{
			u1 as var(func: has(name)) {
				nick1 as nickname
				age1 as age
			}
		}
	`

	mutation1 := &api.Mutation{
		SetNquads: []byte(`
			uid(u1) <name> val(nick1) .
			uid(u1) <name> val(age1) .
		`),
		CommitNow: true,
	}

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	err = userClient.Login(ctx, userid, userpassword)
	require.NoError(t, err)

	_, err = userClient.NewTxn().Do(ctx, &api.Request{
		Query:     query,
		Mutations: []*api.Mutation{mutation1},
		CommitNow: true,
	})
	require.NoError(t, err)

	query = `
		{
			me(func: has(name)) {
				nickname
				name
				age
			}
		}
	`

	resp, err := userClient.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err)

	testutil.CompareJSON(t, `{"me": [{"name":"r1","nickname":"r1"}]}`, string(resp.GetJson()))
}
