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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

var (
	userid       = "alice"
	userpassword = "simplepassword"
)

func makeRequestAndRefreshTokenIfNecessary(t *testing.T, token *testutil.HttpToken, params testutil.GraphQLParams) *testutil.GraphQLResponse {
	resp := testutil.MakeGQLRequestWithAccessJwt(t, &params, token.AccessJwt)
	if len(resp.Errors) == 0 || !strings.Contains(resp.Errors.Error(), "Token is expired") {
		return resp
	}
	var err error
	newtoken, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:   adminEndpoint,
		UserID:     token.UserId,
		Passwd:     token.Password,
		RefreshJwt: token.RefreshToken,
	})
	require.NoError(t, err)
	token.AccessJwt = newtoken.AccessJwt
	token.RefreshToken = newtoken.RefreshToken
	return testutil.MakeGQLRequestWithAccessJwt(t, &params, token.AccessJwt)
}
func createUser(t *testing.T, token *testutil.HttpToken, username, password string) *testutil.GraphQLResponse {
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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
	return resp
}

func getCurrentUser(t *testing.T, token *testutil.HttpToken) *testutil.GraphQLResponse {
	query := `
	query {
		getCurrentUser {
			name
		}
	}`

	resp := makeRequestAndRefreshTokenIfNecessary(t, token, testutil.GraphQLParams{Query: query})
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

func deleteUser(t *testing.T, token *testutil.HttpToken, username string, confirmDeletion bool) *testutil.GraphQLResponse {
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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)

	if confirmDeletion {
		resp.RequireNoGraphQLErrors(t)
		require.JSONEq(t, `{"deleteUser":{"msg":"Deleted","numUids":1}}`, string(resp.Data))
	}
	return resp
}

func deleteGroup(t *testing.T, token *testutil.HttpToken, name string, confirmDeletion bool) *testutil.GraphQLResponse {
	delGroup := `
	mutation deleteGroup($name: String!) {
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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)

	if confirmDeletion {
		resp.RequireNoGraphQLErrors(t)
		require.JSONEq(t, `{"deleteGroup":{"msg":"Deleted","numUids":1}}`, string(resp.Data))
	}
	return resp
}

func deleteUsingNQuad(userClient *dgo.Dgraph, sub, pred, val string) (*api.Response, error) {
	ctx := context.Background()
	txn := userClient.NewTxn()
	mutString := fmt.Sprintf("%s %s %s .", sub, pred, val)
	mutation := &api.Mutation{
		DelNquads: []byte(mutString),
		CommitNow: true,
	}
	return txn.Mutate(ctx, mutation)
}

func TestInvalidGetUser(t *testing.T) {
	currentUser := getCurrentUser(t, &testutil.HttpToken{AccessJwt: "invalid Token"})
	require.Equal(t, `{"getCurrentUser":null}`, string(currentUser.Data))
	require.Equal(t, x.GqlErrorList{{
		Message: "couldn't rewrite query getCurrentUser because unable to parse jwt token: token" +
			" contains an invalid number of segments",
		Path: []interface{}{"getCurrentUser"},
	}}, currentUser.Errors)
}

func TestPasswordReturn(t *testing.T) {
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	query := `
	query {
		getCurrentUser {
			name
			password
		}
	}`

	resp := makeRequestAndRefreshTokenIfNecessary(t, token, testutil.GraphQLParams{Query: query})
	require.Equal(t, resp.Errors, x.GqlErrorList{{
		Message: `Cannot query field "password" on type "User".`,
		Locations: []x.Location{{
			Line:   5,
			Column: 4,
		}},
	}})
}

func TestGetCurrentUser(t *testing.T) {
	token := testutil.GrootHttpLogin(adminEndpoint)

	currentUser := getCurrentUser(t, token)
	currentUser.RequireNoGraphQLErrors(t)
	require.Equal(t, string(currentUser.Data), `{"getCurrentUser":{"name":"groot"}}`)

	// clean up the user to allow repeated running of this test
	userid := "hamilton"
	deleteUserResp := deleteUser(t, token, userid, false)
	deleteUserResp.RequireNoGraphQLErrors(t)
	glog.Infof("cleaned up db user state")

	resp := createUser(t, token, userid, userpassword)
	resp.RequireNoGraphQLErrors(t)
	checkUserCount(t, resp.Data, 1)

	newToken, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    userid,
		Passwd:    userpassword,
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	currentUser = getCurrentUser(t, newToken)
	currentUser.RequireNoGraphQLErrors(t)
	require.Equal(t, string(currentUser.Data), `{"getCurrentUser":{"name":"hamilton"}}`)
}

func TestCreateAndDeleteUsers(t *testing.T) {
	resetUser(t)

	// adding the user again should fail
	token := testutil.GrootHttpLogin(adminEndpoint)
	resp := createUser(t, token, userid, userpassword)
	require.Equal(t, 1, len(resp.Errors))
	require.Equal(t, "couldn't rewrite mutation addUser because failed to rewrite mutation payload because id"+
		" alice already exists for field name inside type User", resp.Errors[0].Message)
	checkUserCount(t, resp.Data, 0)

	// delete the user
	_ = deleteUser(t, token, userid, true)

	resp = createUser(t, token, userid, userpassword)
	resp.RequireNoGraphQLErrors(t)
	// now we should be able to create the user again
	checkUserCount(t, resp.Data, 1)
}

func resetUser(t *testing.T) {
	token := testutil.GrootHttpLogin(adminEndpoint)
	// clean up the user to allow repeated running of this test
	deleteUserResp := deleteUser(t, token, userid, false)
	deleteUserResp.RequireNoGraphQLErrors(t)
	glog.Infof("deleted user")

	resp := createUser(t, token, userid, userpassword)
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
}

func getGrootAndGuardiansUid(t *testing.T, dg *dgo.Dgraph) (string, string) {
	ctx := context.Background()
	txn := dg.NewTxn()
	grootUserQuery := `
	{
		grootUser(func:eq(dgraph.xid, "groot")){
			uid
		}
	}`

	// Structs to parse groot user query response
	type userNode struct {
		Uid string `json:"uid"`
	}

	type userQryResp struct {
		GrootUser []userNode `json:"grootUser"`
	}

	resp, err := txn.Query(ctx, grootUserQuery)
	require.NoError(t, err, "groot user query failed")

	var userResp userQryResp
	if err := json.Unmarshal(resp.GetJson(), &userResp); err != nil {
		t.Fatal("Couldn't unmarshal response from groot user query")
	}
	grootUserUid := userResp.GrootUser[0].Uid

	txn = dg.NewTxn()
	guardiansGroupQuery := `
	{
		guardiansGroup(func:eq(dgraph.xid, "guardians")){
			uid
		}
	}`

	// Structs to parse guardians group query response
	type groupNode struct {
		Uid string `json:"uid"`
	}

	type groupQryResp struct {
		GuardiansGroup []groupNode `json:"guardiansGroup"`
	}

	resp, err = txn.Query(ctx, guardiansGroupQuery)
	require.NoError(t, err, "guardians group query failed")

	var groupResp groupQryResp
	if err := json.Unmarshal(resp.GetJson(), &groupResp); err != nil {
		t.Fatal("Couldn't unmarshal response from guardians group query")
	}
	guardiansGroupUid := groupResp.GuardiansGroup[0].Uid

	return grootUserUid, guardiansGroupUid
}

const defaultTimeToSleep = 500 * time.Millisecond

const timeout = 5 * time.Second

const expireJwtSleep = 21 * time.Second

func testAuthorization(t *testing.T, dg *dgo.Dgraph) {
	createAccountAndData(t, dg)
	ctx := context.Background()
	if err := dg.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace); err != nil {
		t.Fatalf("unable to login using the account %v", userid)
	}

	// initially the query should return empty result, mutate and alter
	// operations should all fail when there are no rules defined on the predicates
	queryPredicateWithUserAccount(t, dg, false)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)
	createGroupAndAcls(t, unusedGroup, false)
	// wait for 5 seconds to ensure the new acl have reached all acl caches
	glog.Infof("Sleeping for acl caches to be refreshed")
	time.Sleep(defaultTimeToSleep)

	// now all these operations except query should fail since
	// there are rules defined on the unusedGroup
	queryPredicateWithUserAccount(t, dg, false)
	mutatePredicateWithUserAccount(t, dg, true)
	alterPredicateWithUserAccount(t, dg, true)
	// create the dev group and add the user to it
	createGroupAndAcls(t, devGroup, true)

	// wait for 5 seconds to ensure the new acl have reached all acl caches
	glog.Infof("Sleeping for acl caches to be refreshed")
	time.Sleep(defaultTimeToSleep)

	// now the operations should succeed again through the devGroup
	queryPredicateWithUserAccount(t, dg, false)
	// sleep long enough (10s per the docker-compose.yml)
	// for the accessJwt to expire in order to test auto login through refresh jwt
	glog.Infof("Sleeping for accessJwt to expire")
	time.Sleep(expireJwtSleep)
	mutatePredicateWithUserAccount(t, dg, false)
	glog.Infof("Sleeping for accessJwt to expire")
	time.Sleep(expireJwtSleep)
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
	if err := dg.LoginIntoNamespace(ctx, x.GrootId, "password", x.GalaxyNamespace); err != nil {
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
	t.Logf("Sleeping for acl caches to be refreshed\n")
	time.Sleep(defaultTimeToSleep)

	// create some data, e.g. user with name alice
	resetUser(t)

	txn := dg.NewTxn()
	_, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("_:a <%s> \"SF\" .", predicateToRead)),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))
}

func createGroup(t *testing.T, token *testutil.HttpToken, name string) []byte {
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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
	resp.RequireNoGraphQLErrors(t)
	return resp.Data
}

func createGroupWithRules(t *testing.T, token *testutil.HttpToken, name string, rules []rule) *group {
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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, queryParams)
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

func updateGroup(t *testing.T, token *testutil.HttpToken, name string, setRules []rule,
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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, queryParams)
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

func addToGroup(t *testing.T, token *testutil.HttpToken, userName, group string) {
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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
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

func addRulesToGroup(t *testing.T, token *testutil.HttpToken, group string, rules []rule) {
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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
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
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	// create a new group
	resp := createGroup(t, token, group)
	checkGroupCount(t, resp, 1)

	// add the user to the group
	if addUserToGroup {
		addToGroup(t, token, userid, group)
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
	addRulesToGroup(t, token, group, rules)
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
	err = dg.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
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
	t.Logf("Sleeping for acl caches to be refreshed")
	time.Sleep(defaultTimeToSleep)

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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

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

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	createGroup(t, token, devGroup)

	addToGroup(t, token, userid, devGroup)

	txn := dg.NewTxn()
	mutation := &api.Mutation{
		SetNquads: []byte(fmt.Sprintf("_:a <%s> \"testdata\" .", unAuthPred)),
		CommitNow: true,
	}
	resp, err := txn.Mutate(ctx, mutation)
	require.NoError(t, err)

	nodeUID, ok := resp.Uids["a"]
	require.True(t, ok)

	addRulesToGroup(t, token, devGroup, []rule{{unAuthPred, 0}})

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(defaultTimeToSleep)

	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
	require.NoError(t, err)

	_, err = deleteUsingNQuad(userClient, "<"+nodeUID+">", "<"+unAuthPred+">", "*")

	require.Error(t, err)
	require.Contains(t, err.Error(), "PermissionDenied")
}

func TestGuardianAccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

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

	time.Sleep(defaultTimeToSleep)
	gClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err, "Error while creating client")

	gClient.LoginIntoNamespace(ctx, "guardian", "guardianpass", x.GalaxyNamespace)

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
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	resp := createUser(t, token, userName, password)
	resp.RequireNoGraphQLErrors(t)
	checkUserCount(t, resp.Data, 1)

	addToGroup(t, token, userName, groupName)
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

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
	return resp
}

func TestQueryRemoveUnauthorizedPred(t *testing.T) {
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
			age: int
		}
	`}
	require.NoError(t, dg.Alter(ctx, &op))

	resetUser(t)
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	createGroup(t, token, devGroup)
	addToGroup(t, token, userid, devGroup)

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
	addRulesToGroup(t, token, devGroup, []rule{{"name", Read.Code}})

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(defaultTimeToSleep)

	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
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
		tc := tc // capture range variable
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()
			testutil.PollTillPassOrTimeout(t, userClient, tc.input, tc.output, timeout)
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

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	createGroup(t, token, devGroup)
	createGroup(t, token, sreGroup)

	addRulesToGroup(t, token, sreGroup, []rule{{"age", Read.Code}, {"name", Write.Code}})
	addToGroup(t, token, userid, devGroup)

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
	time.Sleep(defaultTimeToSleep)

	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
	require.NoError(t, err)

	// Query via user when user has no permissions
	testutil.PollTillPassOrTimeout(t, userClient, query, `{}`, timeout)

	// Login to groot to modify accesses (1)
	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	// Give read access of <name>, write access of <age> to dev
	addRulesToGroup(t, token, devGroup, []rule{{"age", Write.Code}, {"name", Read.Code}})

	testutil.PollTillPassOrTimeout(t, userClient, query,
		`{"me":[{"name":"RandomGuy"},{"name":"RandomGuy2"}]}`, timeout)

	// Login to groot to modify accesses (2)
	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	// Add alice to sre group which has read access to <age> and write access to <name>
	addToGroup(t, token, userid, sreGroup)

	testutil.PollTillPassOrTimeout(t, userClient, query,
		`{"me":[{"name":"RandomGuy","age":23},{"name":"RandomGuy2","age":25}]}`, timeout)

	// Login to groot to modify accesses (3)
	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	// Give read access of <name> and <nickname>, write access of <age> to dev
	addRulesToGroup(t, token, devGroup, []rule{{"age", Write.Code}, {"name", Read.Code}, {"nickname", Read.Code}})

	testutil.PollTillPassOrTimeout(t, userClient, query,
		`{"me":[{"name":"RandomGuy","age":23, "nickname":"RG"},{"name":"RandomGuy2","age":25, "nickname":"RG2"}]}`, timeout)

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

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	createGroup(t, token, devGroup)

	addToGroup(t, token, userid, devGroup)

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
	addRulesToGroup(t, token, devGroup, []rule{{"name", Write.Code}, {"age", Write.Code}})

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(defaultTimeToSleep)

	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
	require.NoError(t, err)

	// delete S * * (user now has permission to name and age)
	_, err = deleteUsingNQuad(userClient, "<"+nodeUID+">", "*", "*")
	require.NoError(t, err)

	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	resp, err = dg.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err, "Error while querying data")
	// Only name and age predicates got deleted via user - alice
	testutil.CompareJSON(t, `{"q1":[{"nickname": "RG"},{"name":"RandomGuy2","age":25,  "nickname": "RG2"}]}`,
		string(resp.GetJson()))

	// Give write access of <name> <dgraph.type> to dev
	addRulesToGroup(t, token, devGroup, []rule{{"name", Write.Code}, {"age", Write.Code}, {"dgraph.type", Write.Code}})
	time.Sleep(defaultTimeToSleep)

	// delete S * * (user now has permission to name, age and dgraph.type)
	_, err = deleteUsingNQuad(userClient, "<"+nodeUID+">", "*", "*")
	require.NoError(t, err)

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

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	createGroup(t, token, devGroup)
	// createGroup(t, accessJwt, sreGroup)

	// addRulesToGroup(t, accessJwt, sreGroup, []rule{{"age", Read.Code}, {"name", Write.Code}})
	addToGroup(t, token, userid, devGroup)

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
	time.Sleep(defaultTimeToSleep)

	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
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
	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	// Give read access of <name> to dev
	addRulesToGroup(t, token, devGroup, []rule{{"name", Read.Code}})
	time.Sleep(defaultTimeToSleep)

	for _, tc := range tests {
		desc := tc.descriptionNamePerm
		t.Run(desc, func(t *testing.T) {
			resp, err := userClient.NewTxn().Query(ctx, tc.input)
			require.NoError(t, err)
			testutil.CompareJSON(t, tc.outputNamePerm, string(resp.Json))
		})
	}

	// Login to groot to modify accesses (1)
	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	// Give read access of <name> and <age> to dev
	addRulesToGroup(t, token, devGroup, []rule{{"name", Read.Code}, {"age", Read.Code}})
	time.Sleep(defaultTimeToSleep)

	for _, tc := range tests {
		desc := tc.descriptionNameAgePerm
		t.Run(desc, func(t *testing.T) {
			resp, err := userClient.NewTxn().Query(ctx, tc.input)
			require.NoError(t, err)
			testutil.CompareJSON(t, tc.outputNameAgePerm, string(resp.Json))
		})
	}

}

func TestAllPredsPermission(t *testing.T) {
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

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	createGroup(t, token, devGroup)
	addToGroup(t, token, userid, devGroup)

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
	}

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(defaultTimeToSleep)

	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
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
	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	// Give read access of all predicates to dev
	addRulesToGroup(t, token, devGroup, []rule{{"dgraph.all", Read.Code}})
	time.Sleep(defaultTimeToSleep)

	for _, tc := range tests {
		desc := tc.descriptionNameAgePerm
		t.Run(desc, func(t *testing.T) {
			resp, err := userClient.NewTxn().Query(ctx, tc.input)
			require.NoError(t, err)
			testutil.CompareJSON(t, tc.outputNameAgePerm, string(resp.Json))
		})
	}

	// Mutation shall fail.
	mutation = &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "RandomGuy" .
			_:a <age> "23" .
			_:a <dgraph.type> "TypeName" .
		`),
		CommitNow: true,
	}
	txn = userClient.NewTxn()
	_, err = txn.Mutate(ctx, mutation)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unauthorized to mutate")

	// Give write access of all predicates to dev. Now mutation should succeed.
	addRulesToGroup(t, token, devGroup, []rule{{"dgraph.all", Write.Code | Read.Code}})
	time.Sleep(defaultTimeToSleep)
	txn = userClient.NewTxn()
	_, err = txn.Mutate(ctx, mutation)
	require.NoError(t, err)
}

func TestNewACLPredicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(defaultTimeToSleep)

	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
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
		tc := tc // capture range variable
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()
			ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
			defer cancel()

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
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
			defer cancel()

			_, err := userClient.NewTxn().Mutate(ctx, &api.Mutation{
				SetNquads: []byte(tc.input),
				CommitNow: true,
			})
			require.True(t, (err == nil) == (tc.err == nil))
		})
	}
}

func removeRuleFromGroup(t *testing.T, token *testutil.HttpToken, group string,
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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
	return resp
}

func TestDeleteRule(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	_ = addDataAndRules(ctx, t, dg)

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(defaultTimeToSleep)

	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
	require.NoError(t, err)

	queryName := "{me(func: has(name)) {name}}"
	resp, err := userClient.NewReadOnlyTxn().Query(ctx, queryName)
	require.NoError(t, err, "Error while querying data")

	testutil.CompareJSON(t, `{"me":[{"name":"RandomGuy"},{"name":"RandomGuy2"}]}`,
		string(resp.GetJson()))

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	removeRuleFromGroup(t, token, devGroup, "name")
	time.Sleep(defaultTimeToSleep)

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

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	addRulesToGroup(t, token, devGroup, []rule{{"name", Read.Code}})
}

func TestQueryUserInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    userid,
		Passwd:    userpassword,
		Namespace: x.GalaxyNamespace,
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
	gqlResp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
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

	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
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
	gqlResp = makeRequestAndRefreshTokenIfNecessary(t, token, params)
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
	gqlResp = makeRequestAndRefreshTokenIfNecessary(t, token, params)
	gqlResp.RequireNoGraphQLErrors(t)
	testutil.CompareJSON(t, `{"getGroup": null}`, string(gqlResp.Data))
}

func TestQueriesWithUserAndGroupOfSameName(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)
	// Creates a user -- alice
	resetUser(t)

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

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	createGroup(t, token, "alice")
	addToGroup(t, token, userid, "alice")

	// add rules to groups
	addRulesToGroup(t, token, "alice", []rule{{Predicate: "name", Permission: Read.Code}})

	query := `
	{
		q(func: has(name)) {
			name
			age
		}
	}
	`

	dc := testutil.DgClientWithLogin(t, userid, userpassword, x.GalaxyNamespace)
	testutil.PollTillPassOrTimeout(t, dc, query, `{"q":[{"name":"RandomGuy"},{"name":"RandomGuy2"}]}`, timeout)
}

func TestQueriesForNonGuardianUserWithoutGroup(t *testing.T) {
	// Create a new user without any groups, queryGroup should return an empty result.
	resetUser(t)

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    userid,
		Passwd:    userpassword,
		Namespace: x.GalaxyNamespace,
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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
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
	resp = makeRequestAndRefreshTokenIfNecessary(t, token, params)
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
		"predicate":"dgraph.drop.op",
		"type":"string"
	},
	{
		"predicate":"dgraph.graphql.p_query",
		"type":"string",
		"index":true,
		"tokenizer":["sha256"]
	},
    {
      "predicate": "dgraph.graphql.schema",
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
				"name": "dgraph.graphql.p_query"
			}
		],
		"name": "dgraph.graphql.persisted_query"
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
		"fields":[],
		"name":"dgraph.graphql.persisted_query"
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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	addDataAndRules(ctx, t, dg)
	time.Sleep(defaultTimeToSleep) // wait for ACL cache to refresh, otherwise it will be flaky test

	// the other user should be able to view only the part of schema for which it has read access
	dg, err = testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	require.NoError(t, dg.LoginIntoNamespace(context.Background(), userid, userpassword, x.GalaxyNamespace))
	resp, err = dg.NewReadOnlyTxn().Query(context.Background(), schemaQuery)
	require.NoError(t, err)
	require.JSONEq(t, aliceSchema, string(resp.GetJson()))
}

func TestDeleteUserShouldDeleteUserFromGroup(t *testing.T) {
	resetUser(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    x.GrootId,
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	_ = deleteUser(t, token, userid, true)

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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
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
	resp = makeRequestAndRefreshTokenIfNecessary(t, token, params)
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

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    x.GrootId,
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	_ = deleteGroup(t, token, "dev-a", true)

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
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
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
	resp = makeRequestAndRefreshTokenIfNecessary(t, token, params)
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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

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
	token := testutil.GrootHttpLogin(adminEndpoint)

	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
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
		t.Logf("Got health: %+v\n", v)
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

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    userid,
		Passwd:    userpassword,
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)

	require.Len(t, resp.Errors, 1)
	require.Contains(t, resp.Errors[0].Message, "rpc error: code = PermissionDenied")
	require.Contains(t, resp.Errors[0].Message, fmt.Sprintf(
		"Only guardians are allowed access. User '%s' is not a member of guardians group.",
		userid))
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
	guardianErr        string
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
			guardianErr:        "you must specify a 'destination' value",
			guardianData:       `{"backup": null}`,
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
			guardianErr:        `The uri path: "" doesn't exist`,
			guardianData:       `{"listBackups": []}`,
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
			guardianErr:        "cache_mb must be non-negative",
			guardianData:       `{"config": null}`,
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
			guardianErr:        "",
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
			guardianErr:        "",
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
			guardianErr:        "invalid export format: invalid",
			guardianData:       `{"export": null}`,
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
			guardianErr:        `The uri path: "" doesn't exist`,
			guardianData:       `{"restore": {"code": "Failure"}}`,
		},
		{
			name: "removeNode has guardian auth",
			query: `
					mutation {
					  removeNode(input: {nodeId: 1, groupId: 2147483640}) {
						response {
							code
						}
					  }
					}`,
			queryName:          "removeNode",
			testGuardianAccess: true,
			guardianErr:        "No group with groupId 2147483640 found",
			guardianData:       `{"removeNode": null}`,
		},
		{
			name: "moveTablet has guardian auth",
			query: `
					mutation {
					  moveTablet(input: {tablet: "non_existent_pred", groupId: 2147483640}) {
						response {
							code
							message
						}
					  }
					}`,
			queryName:          "moveTablet",
			testGuardianAccess: true,
			guardianErr:        "group: [2147483640] is not a known group",
			guardianData:       `{"moveTablet": null}`,
		},
		{
			name: "assign has guardian auth",
			query: `
					mutation {
					  assign(input: {what: UID, num: 0}) {
						response {
							startId
							endId
							readOnly
						}
					  }
					}`,
			queryName:          "assign",
			testGuardianAccess: true,
			guardianErr:        "Nothing to be leased",
			guardianData:       `{"assign": null}`,
		},
		{
			name: "enterpriseLicense has guardian auth",
			query: `
					mutation {
					  enterpriseLicense(input: {license: ""}) {
						response {
							code
						}
					  }
					}`,
			queryName:          "enterpriseLicense",
			testGuardianAccess: true,
			guardianErr: "while extracting enterprise details from the license: while decoding" +
				" license file: EOF",
			guardianData: `{"enterpriseLicense": null}`,
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
			guardianErr:        "",
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
			guardianErr:        "",
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
			guardianErr:        "",
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
				token := testutil.GrootHttpLogin(adminEndpoint)
				resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)

				if tcase.guardianErr == "" {
					resp.RequireNoGraphQLErrors(t)
				} else {
					require.Len(t, resp.Errors, 1)
					require.Contains(t, resp.Errors[0].Message, tcase.guardianErr)
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
	token := testutil.GrootHttpLogin(adminEndpoint)

	addedGroup := createGroupWithRules(t, token, groupName, addedRules)

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
	updatedGroup := updateGroup(t, token, groupName, updatedRules, nil)

	require.Equal(t, groupName, updatedGroup.Name)
	require.Len(t, updatedGroup.Rules, 3)
	require.ElementsMatch(t, []rule{updatedRules[0], addedRules[2], updatedRules[2]},
		updatedGroup.Rules)

	updatedGroup1 := updateGroup(t, token, groupName, nil,
		[]string{"test1", "test1", "test3"})

	require.Equal(t, groupName, updatedGroup1.Name)
	require.Len(t, updatedGroup1.Rules, 2)
	require.ElementsMatch(t, []rule{updatedRules[0], updatedRules[2]}, updatedGroup1.Rules)

	// cleanup
	_ = deleteGroup(t, token, groupName, true)
}

func TestAllowUIDAccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)
	op := api.Operation{Schema: `
		name	 : string @index(exact) .
	`}
	require.NoError(t, dg.Alter(ctx, &op))

	resetUser(t)
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	createGroup(t, token, devGroup)
	addToGroup(t, token, userid, devGroup)

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
	addRulesToGroup(t, token, devGroup, []rule{{"name", Read.Code}})

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(defaultTimeToSleep)

	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)
	resetUser(t)

	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
	require.NoError(t, err)

	// Alice doesn't have access to create new predicate.
	err = userClient.Alter(ctx, &api.Operation{
		Schema: `newpred: string .`,
	})
	require.Error(t, err, "User can't create new predicate. Alter should have returned error.")

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	addToGroup(t, token, userid, "guardians")
	time.Sleep(expireJwtSleep)

	// Alice is a guardian now, it can create new predicate.
	err = userClient.Alter(ctx, &api.Operation{
		Schema: `newpred: string .`,
	})
	require.NoError(t, err, "User is a guardian. Alter should have succeeded.")
}

func TestCrossGroupPermission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)

	err = dg.Alter(ctx, &api.Operation{
		Schema: `newpred: string .`,
	})
	require.NoError(t, err)

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err)

	// create groups
	createGroup(t, token, "reader")
	createGroup(t, token, "writer")
	createGroup(t, token, "alterer")
	// add rules to groups
	addRulesToGroup(t, token, "reader", []rule{{Predicate: "newpred", Permission: 4}})
	addRulesToGroup(t, token, "writer", []rule{{Predicate: "newpred", Permission: 2}})
	addRulesToGroup(t, token, "alterer", []rule{{Predicate: "newpred", Permission: 1}})
	// Wait for acl cache to be refreshed
	time.Sleep(defaultTimeToSleep)

	token, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err)

	// create 8 users.
	for i := 0; i < 8; i++ {
		userIdx := strconv.Itoa(i)
		createUser(t, token, "user"+userIdx, "password"+userIdx)
	}

	// add users to groups. we create all possible combination
	// of groups and assign a user for that combination.
	for i := 0; i < 8; i++ {
		userIdx := strconv.Itoa(i)
		if i&1 > 0 {
			addToGroup(t, token, "user"+userIdx, "alterer")
		}
		if i&2 > 0 {
			addToGroup(t, token, "user"+userIdx, "writer")
		}
		if i&4 > 0 {
			addToGroup(t, token, "user"+userIdx, "reader")
		}
	}
	time.Sleep(defaultTimeToSleep)

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

		err = userClient.LoginIntoNamespace(ctx, "user"+userIdx, "password"+userIdx, x.GalaxyNamespace)
		require.NoError(t, err, "Login error")

		dgQuery(userClient, false, "user"+userIdx) // Query won't fail, will return empty result instead.
		dgMutation(userClient, i&2 == 0, "user"+userIdx)
		dgAlter(userClient, i&1 == 0, "user"+userIdx)
	}
}

func TestMutationWithValueVar(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

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
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   "groot",
		Passwd:   "password",
	})
	require.NoError(t, err)
	createUser(t, token, userid, userpassword)
	createGroup(t, token, devGroup)
	addToGroup(t, token, userid, devGroup)
	addRulesToGroup(t, token, devGroup, []rule{
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
	time.Sleep(defaultTimeToSleep)

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
	err = userClient.LoginIntoNamespace(ctx, userid, userpassword, x.GalaxyNamespace)
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

func TestFailedLogin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	grootClient, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	op := api.Operation{DropAll: true}
	if err := grootClient.Alter(ctx, &op); err != nil {
		t.Fatalf("Unable to cleanup db:%v", err)
	}
	require.NoError(t, err)

	client, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)

	// User is not present
	err = client.LoginIntoNamespace(ctx, userid, "simplepassword", x.GalaxyNamespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), x.ErrorInvalidLogin.Error())

	resetUser(t)
	// User is present
	err = client.LoginIntoNamespace(ctx, userid, "randomstring", x.GalaxyNamespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), x.ErrorInvalidLogin.Error())
}

func TestDeleteGuardiansGroupShouldFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    x.GrootId,
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	resp := deleteGroup(t, token, "guardians", false)
	require.Contains(t, resp.Errors.Error(),
		"guardians group and groot user cannot be deleted.")
}

func TestDeleteGrootUserShouldFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    x.GrootId,
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	resp := deleteUser(t, token, "groot", false)
	require.Contains(t, resp.Errors.Error(),
		"guardians group and groot user cannot be deleted.")
}

func TestDeleteGrootUserFromGuardiansGroupShouldFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	require.NoError(t, err, "login failed")

	gqlresp := removeUserFromGroup(t, "groot", "guardians")

	require.Contains(t, gqlresp.Errors.Error(),
		"guardians group and groot user cannot be deleted.")
}

func TestDeleteGrootAndGuardiansUsingDelNQuadShouldFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	require.NoError(t, err, "login failed")

	grootUid, guardiansUid := getGrootAndGuardiansUid(t, dg)

	// Try deleting groot user
	_, err = deleteUsingNQuad(dg, "<"+grootUid+">", "*", "*")
	require.Error(t, err, "Deleting groot user should have returned an error")
	require.Contains(t, err.Error(), "Properties of guardians group and groot user cannot be deleted")

	// Try deleting guardians group
	_, err = deleteUsingNQuad(dg, "<"+guardiansUid+">", "*", "*")
	require.Error(t, err, "Deleting guardians group should have returned an error")
	require.Contains(t, err.Error(), "Properties of guardians group and groot user cannot be deleted")
}

func deleteGuardiansGroupAndGrootUserShouldFail(t *testing.T) {
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   x.GrootId,
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	// Try deleting guardians group should fail
	resp := deleteGroup(t, token, "guardians", false)
	require.Contains(t, resp.Errors.Error(),
		"guardians group and groot user cannot be deleted.")
	// Try deleting groot user should fail
	resp = deleteUser(t, token, "groot", false)
	require.Contains(t, resp.Errors.Error(),
		"guardians group and groot user cannot be deleted.")
}

func TestDropAllShouldResetGuardiansAndGroot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	addDataAndRules(ctx, t, dg)

	require.NoError(t, err, "login failed")

	// Try Drop All
	op := api.Operation{
		DropAll: true,
		DropOp:  api.Operation_ALL,
	}
	if err := dg.Alter(ctx, &op); err != nil {
		t.Fatalf("Unable to drop all. Error:%v", err)
	}

	time.Sleep(defaultTimeToSleep)
	deleteGuardiansGroupAndGrootUserShouldFail(t)

	// Try Drop Data
	op = api.Operation{
		DropOp: api.Operation_DATA,
	}
	if err := dg.Alter(ctx, &op); err != nil {
		t.Fatalf("Unable to drop data. Error:%v", err)
	}

	time.Sleep(defaultTimeToSleep)
	deleteGuardiansGroupAndGrootUserShouldFail(t)
}

func TestMain(m *testing.M) {
	adminEndpoint = "http://" + testutil.SockAddrHttp + "/admin"
	fmt.Printf("Using adminEndpoint for acl package: %s\n", adminEndpoint)
	os.Exit(m.Run())
}
