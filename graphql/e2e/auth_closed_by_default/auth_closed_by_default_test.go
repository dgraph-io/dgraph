/*
 *    Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package auth_closed_by_default

import (
	"os"
	"testing"

	"github.com/dgrijalva/jwt-go/v4"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
)

type TestCase struct {
	name      string
	query     string
	variables map[string]interface{}
	result    string
}

func TestAuthRulesMutationWithClosedByDefaultFlag(t *testing.T) {
	testCases := []TestCase{{
		name: "Missing JWT from Mutation - type with auth directive",
		query: `
	          	mutation addUser($user: AddUserSecretInput!) {
		        	addUserSecret(input: [$user]) {
				      userSecret {
					    aSecret
				      }
			       }
	        	}`,
		variables: map[string]interface{}{"user": &common.UserSecret{
			ASecret: "secret1",
			OwnedBy: "user1",
		}},
		result: `{"addUserSecret":null}`,
	},
		{
			name: "Missing JWT from Mutation - type without auth directive",
			query: `
		       mutation addTodo($Todo: AddTodoInput!) {
			      addTodo(input: [$Todo]) {
				     todo {
					   text
                       owner
				     }
			      }
		       } `,
			variables: map[string]interface{}{"Todo": &common.Todo{
				Text:  "Hi Dgrap team!!",
				Owner: "Alice",
			}},
			result: `{"addTodo":null}`,
		},
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Query:     tcase.query,
			Variables: tcase.variables,
		}
		gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
		require.Equal(t, len(gqlResponse.Errors), 1)
		require.Contains(t, gqlResponse.Errors[0].Error(),
			"a valid JWT is required but was not provided")
		require.Equal(t, tcase.result, string(gqlResponse.Data))
	}
}

func TestAuthRulesQueryWithClosedByDefaultFlag(t *testing.T) {
	testCases := []TestCase{
		{name: "Missing JWT from Query - type with auth field",
			query: `
			query {
				queryProject {
					name
				}
			}`,
			result: `{"queryProject":[]}`,
		},
		{name: "Missing JWT from Query - type without auth field",
			query: `
			query {
				queryTodo {
					owner
				}
			}`,
			result: `{"queryTodo":[]}`,
		},
	}

	for _, tcase := range testCases {
		queryParams := &common.GraphQLParams{
			Query: tcase.query,
		}
		gqlResponse := queryParams.ExecuteAsPost(t, common.GraphqlURL)
		require.Equal(t, len(gqlResponse.Errors), 1)
		require.Contains(t, gqlResponse.Errors[0].Error(),
			"a valid JWT is required but was not provided")
		require.Equal(t, tcase.result, string(gqlResponse.Data))
	}
}

func TestAuthRulesUpdateWithClosedByDefaultFlag(t *testing.T) {
	testCases := []TestCase{{
		name: "Missing JWT from Update Mutation - type with auth field",
		query: `
	    mutation ($ids: [ID!]) {
		    updateIssue(input: {filter: {id: $ids}, set: {random: "test"}}) {
			issue (order: {asc: msg}) {
				msg
			}
		    }
	    }
	`,
		result: `{"updateIssue":null}`,
	},
		{
			name: "Missing JWT from Update Mutation - type without auth field",
			query: `
	    mutation ($ids: [ID!]) {
		    updateTodo(input: {filter: {id: $ids}, set: {text: "test"}}) {
			todo {
				text
			}
		    }
	    }
	`,
			result: `{"updateTodo":null}`,
		}}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Query:     tcase.query,
			Variables: map[string]interface{}{"ids": []string{"0x1"}},
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
		require.Equal(t, len(gqlResponse.Errors), 1)
		require.Contains(t, gqlResponse.Errors[0].Error(),
			"a valid JWT is required but was not provided")
		require.Equal(t, tcase.result, string(gqlResponse.Data))
	}
}

func TestDeleteOrRBACFilter(t *testing.T) {
	testCases := []TestCase{{
		name: "Missing JWT from delete Mutation- type with auth field",
		query: `
		mutation($ids: [ID!]) {
			deleteComplexLog (filter: { id: $ids}) {
         		numUids
		    }
		}
	`,
		result: `{"deleteComplexLog":null}`,
	}, {
		name: "Missing JWT from delete Mutation - type without auth field",
		query: `
		mutation($ids: [ID!]) {
			deleteTodo (filter: { id: $ids}) {
         		numUids
		    }
		}
	`,
		result: `{"deleteTodo":null}`,
	}}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Query:     tcase.query,
			Variables: map[string]interface{}{"ids": []string{"0x1"}},
		}
		gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
		require.Equal(t, len(gqlResponse.Errors), 1)
		require.Contains(t, gqlResponse.Errors[0].Error(),
			"a valid JWT is required but was not provided")
		require.Equal(t, tcase.result, string(gqlResponse.Data))
	}
}

func TestMain(m *testing.M) {
	algo := jwt.SigningMethodHS256.Name
	schema, data := common.BootstrapAuthData()
	authSchema, err := testutil.AppendAuthInfo(schema, algo, "../auth/sample_public_key.pem", true)
	if err != nil {
		panic(err)
	}
	common.BootstrapServer(authSchema, data)
	// Data is added only in the first iteration, but the schema is added every iteration.
	if data != nil {
		data = nil
	}
	exitCode := m.Run()
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	os.Exit(0)
}
