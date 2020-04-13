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

package auth

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

type user struct {
	Username string
	Todos    []*todo
}

type todo struct {
	ID               string
	Title            string
	IsPublic         bool
	SomethingPrivate string
	Owner            *user
	SharedWith       []*user
}

const (
	graphqlURL = "http://localhost:8180/graphql"
)

func getJWT(t *testing.T, user, role string) string {
	type MyCustomClaims struct {
		Foo map[string]interface{} `json:"https://dgraph.io/jwt/claims"`
		jwt.StandardClaims
	}

	// Create the Claims
	claims := MyCustomClaims{
		map[string]interface{}{},
		jwt.StandardClaims{
			ExpiresAt: 15000,
			Issuer:    "test",
		},
	}

	claims.Foo["X-MyApp-User"] = user
	claims.Foo["X-MyApp-Role"] = role

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte("Secret"))
	require.NoError(t, err)

	return ss
}

func TestQueryAllTodos(t *testing.T) {
	getUserParams := &common.GraphQLParams{
		Authorization: getJWT(t, "user1", "user"),
		Query: `
		query {
                  queryTodo(order:{
                    asc:title
                  }) {
                    title
                    isPublic
                    somethingPrivate
                    owner{
                      username
                    }
                    sharedWith{
                      username
                    }
                  }
                }
		`,
	}

	expected := `
{
    "queryTodo": [
      {
        "title": "Todo 1",
        "isPublic": true,
        "somethingPrivate": "privateInfo",
        "owner": {
          "username": "user1"
        },
        "sharedWith": [
          {
            "username": "user2"
          }
        ]
      },
      {
        "title": "Todo 2",
        "isPublic": false,
        "somethingPrivate": "privateInfo",
        "owner": {
          "username": "user1"
        },
        "sharedWith": [
          {
            "username": "user2"
          }
        ]
      },
      {
        "title": "Todo 3",
        "isPublic": true,
        "somethingPrivate": "privateInfo",
        "owner": {
          "username": "user1"
        },
        "sharedWith": []
      },
      {
        "title": "Todo 4",
        "isPublic": false,
        "somethingPrivate": "privateInfo",
        "owner": {
          "username": "user1"
        },
        "sharedWith": []
      },
      {
        "title": "Todo 5",
        "isPublic": true,
        "somethingPrivate": null,
        "owner": {
          "username": "user2"
        },
        "sharedWith": [
          {
            "username": "user1"
          }
        ]
      },
      {
        "title": "Todo 6",
        "isPublic": false,
        "somethingPrivate": null,
        "owner": {
          "username": "user2"
        },
        "sharedWith": [
          {
            "username": "user1"
          }
        ]
      },
      {
        "title": "Todo 7",
        "isPublic": true,
        "somethingPrivate": null,
        "owner": {
          "username": "user2"
        },
        "sharedWith": []
      }
    ]
  }
	`

	var result, data struct {
		QueryTodo []*todo
	}

	gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.Nil(t, err)

	err = json.Unmarshal([]byte(expected), &data)
	require.Nil(t, err)

	if diff := cmp.Diff(result, data); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestQueryAllUsers(t *testing.T) {
	getUserParams := &common.GraphQLParams{
		Authorization: getJWT(t, "user1", "user"),
		Query: `
		query {
                queryUser(order:{
                  asc: username
                }){
                  username
                  todos (order:{
                    asc:title
                  }) {
                    title
                    isPublic
                    somethingPrivate
                    sharedWith{
                      username
                    }
                  }
                }
              }`,
	}

	expected := `
{
"queryUser": [
      {
        "username": "user1",
        "todos": [
          {
            "title": "Todo 1",
            "isPublic": true,
            "somethingPrivate": "privateInfo",
            "sharedWith": [
              {
                "username": "user2"
              }
            ]
          },
          {
            "title": "Todo 2",
            "isPublic": false,
            "somethingPrivate": "privateInfo",
            "sharedWith": [
              {
                "username": "user2"
              }
            ]
          },
          {
            "title": "Todo 3",
            "isPublic": true,
            "somethingPrivate": "privateInfo",
            "sharedWith": []
          },
          {
            "title": "Todo 4",
            "isPublic": false,
            "somethingPrivate": "privateInfo",
            "sharedWith": []
          }
        ]
      },
      {
        "username": "user2",
        "todos": [
          {
            "title": "Todo 5",
            "isPublic": true,
            "somethingPrivate": null,
            "sharedWith": [
              {
                "username": "user1"
              }
            ]
          },
          {
            "title": "Todo 6",
            "isPublic": false,
            "somethingPrivate": null,
            "sharedWith": [
              {
                "username": "user1"
              }
            ]
          },
          {
            "title": "Todo 7",
            "isPublic": true,
            "somethingPrivate": null,
            "sharedWith": []
          }
        ]
      }
    ]
  }	
	`

	var result, data struct {
		QueryUser []*user
	}

	gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.Nil(t, err)

	err = json.Unmarshal([]byte(expected), &data)
	require.Nil(t, err)

	if diff := cmp.Diff(result, data); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestRunAll_Auth(t *testing.T) {
	//common.RunAll(t)
}

func TestSchema_Auth(t *testing.T) {
	// TODO write this test

	//t.Run("graphql schema", func(t *testing.T) {
	//	common.SchemaTest(t, expectedDgraphSchema)
	//})
}

func TestMain(m *testing.M) {
	schemaFile := "schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		panic(err)
	}

	jsonFile := "test_data.json"
	data, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		panic(errors.Wrapf(err, "Unable to read file %s.", jsonFile))
	}

	common.BootstrapServer(schema, data)

	os.Exit(m.Run())
}
