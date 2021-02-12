/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

var adminEndpoint string

type rule struct {
	Predicate  string `json:"predicate"`
	Permission int32  `json:"permission"`
}

func makeRequest(t *testing.T, token *testutil.HttpToken,
	params testutil.GraphQLParams) *testutil.GraphQLResponse {
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

func login(t *testing.T, userId, password string, namespace uint64) *testutil.HttpToken {
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    userId,
		Passwd:    password,
		Namespace: namespace,
	})
	require.NoError(t, err, "login failed")
	return token
}

func createNamespace(t *testing.T, token *testutil.HttpToken) (uint64, error) {
	createNs := `mutation {
					 addNamespace
					  {
					    namespaceId
					    message
					  }
					}`

	params := testutil.GraphQLParams{
		Query: createNs,
	}
	resp := makeRequest(t, token, params)
	var result struct {
		AddNamespace struct {
			NamespaceId int    `json:"namespaceId"`
			Message     string `json:"message"`
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &result))
	if strings.Contains(result.AddNamespace.Message, "Created namespace successfully") {
		return uint64(result.AddNamespace.NamespaceId), nil
	}
	return 0, errors.New(result.AddNamespace.Message)
}

func deleteNamespace(t *testing.T, token *testutil.HttpToken, nsID uint64) {
	deleteReq := `mutation deleteNamespace($namespaceId: Int!) {
			deleteNamespace(input: {namespaceId: $namespaceId}){
    		namespaceId
    		message
  		}
	}`

	params := testutil.GraphQLParams{
		Query: deleteReq,
		Variables: map[string]interface{}{
			"namespaceId": nsID,
		},
	}
	resp := makeRequest(t, token, params)
	var result struct {
		DeleteNamespace struct {
			NamespaceId int    `json:"namespaceId"`
			Message     string `json:"message"`
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &result))
	require.Equal(t, int(nsID), result.DeleteNamespace.NamespaceId)
	require.Contains(t, result.DeleteNamespace.Message, "Deleted namespace successfully")
}

func createUser(t *testing.T, token *testutil.HttpToken, username,
	password string) {
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
	resp := makeRequest(t, token, params)
	type Response struct {
		AddUser struct {
			User []struct {
				Name string
			}
		}
	}
	var r Response
	err := json.Unmarshal(resp.Data, &r)
	require.NoError(t, err)
}

func createGroup(t *testing.T, token *testutil.HttpToken, name string) {
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
	resp := makeRequest(t, token, params)
	resp.RequireNoGraphQLErrors(t)
	type Response struct {
		AddGroup struct {
			Group []struct {
				Name string
			}
		}
	}
	var r Response
	err := json.Unmarshal(resp.Data, &r)
	require.NoError(t, err)
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
	resp := makeRequest(t, token, params)
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
	resp := makeRequest(t, token, params)
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

func dgClientWithLogin(t *testing.T, id, password string, ns uint64) *dgo.Dgraph {
	userClient, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	err = userClient.LoginIntoNamespace(context.Background(), id, password, ns)
	require.NoError(t, err)
	return userClient
}

func addData(t *testing.T, dg *dgo.Dgraph) {
	mutation := &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "guy1" .
			_:a <nickname> "RG" .
			_:b <name> "guy2" .
			_:b <nickname> "RG2" .
		`),
		CommitNow: true,
	}
	_, err := dg.NewTxn().Mutate(context.Background(), mutation)
	require.NoError(t, err)
}

func queryData(t *testing.T, dg *dgo.Dgraph, query string) []byte {
	resp, err := dg.NewReadOnlyTxn().Query(context.Background(), query)
	require.NoError(t, err)
	return resp.GetJson()
}

// TODO(Ahsan): This is just a basic test, for the purpose of development. The functions used in
// this file can me made common to the other acl tests as well. Needs some refactoring as well.
func TestAclBasic(t *testing.T) {
	galaxyToken := login(t, "groot", "password", 0)
	time.Sleep(1 * time.Second)

	// Create a new namespace
	ns, err := createNamespace(t, galaxyToken)
	require.NoError(t, err)
	require.Equal(t, 1, int(ns))

	// Add some data to namespace 1
	dc := dgClientWithLogin(t, "groot", "password", 1)
	addData(t, dc)

	query := `
		{
			me(func: has(name)) {
				nickname
				name
			}
		}
	`
	resp := queryData(t, dc, query)
	testutil.CompareJSON(t,
		`{"me": [{"name":"guy1","nickname":"RG"},
		{"name": "guy2", "nickname":"RG2"}]}`,
		string(resp))

	// groot of namespace 0 should not see the data of namespace-1
	dc = dgClientWithLogin(t, "groot", "password", 0)
	resp = queryData(t, dc, query)
	testutil.CompareJSON(t, `{"me": []}`, string(resp))

	// Login to namespace 1 via groot and create new user alice.
	token := login(t, "groot", "password", 1)
	createUser(t, token, "alice", "newpassword")

	// Alice should not be able to see data added by groot in namespace 1
	dc = dgClientWithLogin(t, "alice", "newpassword", 1)
	resp = queryData(t, dc, query)
	testutil.CompareJSON(t, `{}`, string(resp))

	// Create a new group, add alice to that group and give read access of <name> to dev group.
	createGroup(t, token, "dev")
	addToGroup(t, token, "alice", "dev")
	addRulesToGroup(t, token, "dev", []rule{{"name", acl.Read.Code}})

	// Wait for acl cache to get updated
	time.Sleep(5 * time.Second)

	// Now alice should see the name predicate but not nickname.
	dc = dgClientWithLogin(t, "alice", "newpassword", 1)
	resp = queryData(t, dc, query)
	testutil.CompareJSON(t, `{"me": [{"name":"guy1"},{"name": "guy2"}]}`, string(resp))

}

func TestMain(m *testing.M) {
	adminEndpoint = "http://" + testutil.SockAddrHttp + "/admin"
	fmt.Printf("Using adminEndpoint : %s for multy-tenancy test.\n", adminEndpoint)
	os.Exit(m.Run())
}
