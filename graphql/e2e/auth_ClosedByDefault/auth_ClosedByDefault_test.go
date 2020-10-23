/*
 *    Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package auth_ClosedByDefault

import (
	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"testing"
)

const (
	graphqlURL = "http://localhost:8180/graphql"
)

type TestCase struct {
	name      string
	query     string
	variables map[string]interface{}
	result    string
}

func TestAuthRulesMutationWithClosedByDefaultFlag(t *testing.T) {
	testCases := []TestCase{{
		name: "Missing JWT - type with auth directive",
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
			name: "Missing JWT - type without auth directive",
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
		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		require.Equal(t, len(gqlResponse.Errors), 1)
		require.Contains(t, gqlResponse.Errors[0].Error(),
			"Jwt is required when ClosedByDefault flag is true")
		require.Equal(t, tcase.result, string(gqlResponse.Data))
	}
}

func TestAuthRulesQueryWithClosedByDefaultAFlag(t *testing.T) {
	testCases := []TestCase{
		{name: "Missing JWT - type with auth field",
			query: `
			query {
				queryProject {
					name
				}
			}`,
			result: `{"queryProject":[]}`,
		},
		{name: "Missing JWT - type without auth field",
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
		gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
		require.Equal(t, len(gqlResponse.Errors), 1)
		require.Contains(t, gqlResponse.Errors[0].Error(),
			"Jwt is required when ClosedByDefault flag is true")
		require.Equal(t, tcase.result, string(gqlResponse.Data))
	}
}

func TestMain(m *testing.M) {
	schemaFile := "../auth/schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		panic(err)
	}

	jsonFile := "../auth/test_data.json"
	data, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		panic(errors.Wrapf(err, "Unable to read file %s.", jsonFile))
	}

	algo := authorization.HMAC256
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
