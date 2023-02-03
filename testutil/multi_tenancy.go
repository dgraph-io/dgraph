/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/x"
)

type Rule struct {
	Predicate  string `json:"predicate"`
	Permission int32  `json:"permission"`
}

func MakeRequest(t *testing.T, token *HttpToken, params GraphQLParams) *GraphQLResponse {
	resp := MakeGQLRequestWithAccessJwt(t, &params, token.AccessJwt)
	if len(resp.Errors) == 0 || !strings.Contains(resp.Errors.Error(), "Token is expired") {
		return resp
	}
	var err error
	newtoken, err := HttpLogin(&LoginParams{
		Endpoint:   AdminUrl(),
		UserID:     token.UserId,
		Passwd:     token.Password,
		RefreshJwt: token.RefreshToken,
	})
	require.NoError(t, err)
	token.AccessJwt = newtoken.AccessJwt
	token.RefreshToken = newtoken.RefreshToken
	return MakeGQLRequestWithAccessJwt(t, &params, token.AccessJwt)
}

func Login(t *testing.T, loginParams *LoginParams) *HttpToken {
	if loginParams.Endpoint == "" {
		loginParams.Endpoint = AdminUrl()
	}
	var token *HttpToken
	err := x.RetryUntilSuccess(10, 100*time.Millisecond, func() error {
		var err error
		token, err = HttpLogin(loginParams)
		return err
	})
	require.NoError(t, err, "login failed")
	return token
}

func ResetPassword(t *testing.T, token *HttpToken, userID, newPass string, nsID uint64) (string, error) {
	resetpasswd := `mutation resetPassword($userID: String!, $newpass: String!, $namespaceId: Int!){
		resetPassword(input: {userId: $userID, password: $newpass, namespace: $namespaceId}) {
		  userId
		  message
		}
	  }`

	params := GraphQLParams{
		Query: resetpasswd,
		Variables: map[string]interface{}{
			"namespaceId": nsID,
			"userID":      userID,
			"newpass":     newPass,
		},
	}

	resp := MakeRequest(t, token, params)

	if len(resp.Errors) > 0 {
		return "", errors.Errorf(resp.Errors.Error())
	}

	var result struct {
		ResetPassword struct {
			UserId  string `json:"userId"`
			Message string `json:"message"`
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &result))
	require.Equal(t, userID, result.ResetPassword.UserId)
	require.Contains(t, result.ResetPassword.Message, "Reset password is successful")
	return result.ResetPassword.UserId, nil
}

func CreateNamespaceWithRetry(t *testing.T, token *HttpToken) (uint64, error) {
	createNs := `mutation {
					 addNamespace
					  {
					    namespaceId
					    message
					  }
					}`

	params := GraphQLParams{
		Query: createNs,
	}
	var resp *GraphQLResponse
	for {
		resp = MakeRequest(t, token, params)
		if len(resp.Errors) > 0 {
			// retry if necessary
			if strings.Contains(resp.Errors.Error(), "Predicate dgraph.xid is not indexed") ||
				strings.Contains(resp.Errors.Error(), "opIndexing is already running") {
				glog.Warningf("error while creating namespace %v", resp.Errors)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return 0, errors.Errorf(resp.Errors.Error())
		}
		break
	}

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

func DeleteNamespace(t *testing.T, token *HttpToken, nsID uint64) error {
	deleteReq := `mutation deleteNamespace($namespaceId: Int!) {
			deleteNamespace(input: {namespaceId: $namespaceId}){
    		namespaceId
    		message
  		}
	}`

	params := GraphQLParams{
		Query: deleteReq,
		Variables: map[string]interface{}{
			"namespaceId": nsID,
		},
	}
	resp := MakeRequest(t, token, params)
	if len(resp.Errors) > 0 {
		return errors.Errorf(resp.Errors.Error())
	}
	var result struct {
		DeleteNamespace struct {
			NamespaceId int    `json:"namespaceId"`
			Message     string `json:"message"`
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &result))
	require.Equal(t, int(nsID), result.DeleteNamespace.NamespaceId)
	require.Contains(t, result.DeleteNamespace.Message, "Deleted namespace successfully")
	return nil
}

func CreateUser(t *testing.T, token *HttpToken, username,
	password string) {
	addUser := `
	mutation addUser($name: String!, $pass: String!) {
		addUser(input: [{name: $name, password: $pass}]) {
			user {
				name
			}
		}
	}`

	params := GraphQLParams{
		Query: addUser,
		Variables: map[string]interface{}{
			"name": username,
			"pass": password,
		},
	}
	resp := MakeRequest(t, token, params)
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

func CreateGroup(t *testing.T, token *HttpToken, name string) {
	addGroup := `
	mutation addGroup($name: String!) {
		addGroup(input: [{name: $name}]) {
			group {
				name
			}
		}
	}`

	params := GraphQLParams{
		Query: addGroup,
		Variables: map[string]interface{}{
			"name": name,
		},
	}
	resp := MakeRequest(t, token, params)
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

func AddToGroup(t *testing.T, token *HttpToken, userName, group string) {
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

	params := GraphQLParams{
		Query: addUserToGroup,
		Variables: map[string]interface{}{
			"name":  userName,
			"group": group,
		},
	}
	resp := MakeRequest(t, token, params)
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

func AddRulesToGroup(t *testing.T, token *HttpToken, group string, rules []Rule, newGroup bool) {
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

	params := GraphQLParams{
		Query: addRuleToGroup,
		Variables: map[string]interface{}{
			"name":  group,
			"rules": rules,
		},
	}
	resp := MakeRequest(t, token, params)
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
	if newGroup {
		CompareJSON(t, expectedOutput, string(resp.Data))
	}
}

func DgClientWithLogin(t *testing.T, id, password string, ns uint64) *dgo.Dgraph {
	userClient, err := DgraphClient(SockAddr)
	require.NoError(t, err)

	require.NoError(t, x.RetryUntilSuccess(10, 100*time.Millisecond, func() error {
		return userClient.LoginIntoNamespace(context.Background(), id, password, ns)
	}))
	return userClient
}

func AddData(t *testing.T, dg *dgo.Dgraph) {
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

func QueryData(t *testing.T, dg *dgo.Dgraph, query string) []byte {
	resp, err := dg.NewReadOnlyTxn().Query(context.Background(), query)
	require.NoError(t, err)
	return resp.GetJson()
}
