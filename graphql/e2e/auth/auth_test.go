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
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

const (
	graphqlURL = "http://localhost:8180/graphql"
)

type User struct {
	Username string
	Age      uint64
	IsPublic bool
	Disabled bool
}

type Region struct {
	Id    uint64
	Name  string
	Users []*User
}

type Movie struct {
	Id               uint64
	Content          string
	Disabled         bool
	RegionsAvailable []*Region
}

type Issue struct {
	Id    uint64
	Msg   string
	Owner *User
}

type Log struct {
	Id   uint64
	Logs string
}

type Role struct {
	Id         uint64
	Permission string
	AssignedTo []User
}

type Ticket struct {
	Id         uint64
	OnColumn   Column
	Title      string
	AssignedTo []User
}

type Column struct {
	InProject Project
	Name      string
	Tickets   []Ticket
}

type Project struct {
	ProjID  uint64
	Name    string
	Roles   []Role
	Columns []Column
}

type TestCase struct {
	user   string
	role   string
	result string
	name   string
}

func getJWT(t *testing.T, user, role string) http.Header {
	type MyCustomClaims struct {
		Foo map[string]interface{} `json:"https://dgraph.io/jwt/claims"`
		jwt.StandardClaims
	}

	// Create the Claims
	claims := MyCustomClaims{
		map[string]interface{}{},
		jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Minute * 1000).Unix(),
			Issuer:    "test",
		},
	}

	claims.Foo["USER"] = user
	claims.Foo["ROLE"] = role

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte("Secretkey"))
	require.NoError(t, err)

	h := make(http.Header)
	h.Add("X-Dgraph-AuthorizationToken", ss)

	return h
}

func TestOrRBACFilter(t *testing.T) {
	t.Skip()
	testCases := []TestCase{{
		user: "user1",
		role: "ADMIN",
		result: `{
                            "queryProject": [
                              {
                                "name": "Project1"
                              },
                              {
                                "name": "Project2"
                              }
                            ]
                        }`,
	}, {
		user: "user1",
		role: "USER",
		result: `{
                            "queryProject": [
                              {
                                "name": "Project1"
                              }
                            ]
                        }`,
	}, {
		user: "user4",
		role: "USER",
		result: `{
                            "queryProject": [
                              {
                                "name": "Project2"
                              }
                            ]
                        }`,
	}}

	query := `
            query {
                queryProject (order: {asc: name}) {
			name
		}
	    }
	`

	var result, data struct {
		QueryProject []*Project
	}

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.Nil(t, err)

			err = json.Unmarshal([]byte(tcase.result), &data)
			require.Nil(t, err)

			if diff := cmp.Diff(result, data); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRootGetFilter(t *testing.T) {
	tcases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"getColumn": {"name": "Column1"}}`,
		name:   "Column1",
	}, {
		user:   "user1",
		role:   "USER",
		result: `{"getColumn": null}`,
		name:   "Column2",
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"getColumn": {"name": "Column2"}}`,
		name:   "Column2",
	}}

	query := `
		query($name: String!) {
		    getColumn(name: $name) {
			name
		    }
		}
	`

	var result, data struct {
		GetColumn *Column
	}

	for _, tcase := range tcases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers:   getJWT(t, tcase.user, tcase.role),
				Query:     query,
				Variables: map[string]interface{}{"name": tcase.name},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.Nil(t, err)

			err = json.Unmarshal([]byte(tcase.result), &data)
			require.Nil(t, err)

			if diff := cmp.Diff(result, data); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRootFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"queryColumn": [{"name": "Column1"}]}`,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"queryColumn": [{"name": "Column1"}, {"name": "Column2"}, {"name": "Column3"}]}`,
	}, {
		user:   "user4",
		role:   "USER",
		result: `{"queryColumn": [{"name": "Column2"}, {"name": "Column3"}]}`,
	}}
	query := `
	query {
		queryColumn(order: {asc: name}) {
			name
		}
	}
	`

	var result, data struct {
		QueryColumn []*Column
	}

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.Nil(t, err)

			err = json.Unmarshal([]byte(tcase.result), &data)
			require.Nil(t, err)

			if diff := cmp.Diff(result, data); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRBACFilter(t *testing.T) {
	t.Skip()
	testCases := []TestCase{
		{role: "USER", result: `{"queryLog": []}`},
		{role: "ADMIN", result: `{
    "queryLog": [
      {
        "logs": "Log1"
      },
      {
        "logs": "Log2"
      }
    ]
  }`},
	}
	query := `
		query {
                    queryLog (order: {asc: logs}) {
		    	logs
		    }
		}
	`

	var result, data struct {
		QueryLog []*Log
	}

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.Nil(t, err)

			err = json.Unmarshal([]byte(tcase.result), &data)
			require.Nil(t, err)

			if diff := cmp.Diff(result, data); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAndRBACFilter(t *testing.T) {
	t.Skip()
	testCases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"queryIssue": []}`,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"queryIssue": []}`,
	}, {
		user:   "user2",
		role:   "ADMIN",
		result: `{"queryIssue": [{"msg": "Issue2"}]}`,
	}}
	query := `
		query {
                queryIssue (order: {asc: msg}) {
			msg
		}
		}
	`

	var result, data struct {
		QueryIssue []*Issue
	}

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.Nil(t, err)

			err = json.Unmarshal([]byte(tcase.result), &data)
			require.Nil(t, err)

			if diff := cmp.Diff(result, data); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}

}

func TestAndFilter(t *testing.T) {
	testCases := []TestCase{{
		user: "user1",
		role: "USER",
		result: `
{
   "queryMovie": [
      {
         "content": "Movie2",
         "regionsAvailable": [
            {
               "name": "Region1"
            }
         ]
      },
      {
         "content": "Movie3",
         "regionsAvailable": [
            {
               "name": "Region1"
            },
            {
               "name": "Region4"
            }
         ]
      }
   ]
}
		`,
	}, {
		user: "user2",
		role: "USER",
		result: `
{
   "queryMovie": [
      {
         "content": "Movie1",
         "regionsAvailable": [
            {
               "name": "Region2"
            },
            {
               "name": "Region3"
            }
         ]
      },
      {
         "content": "Movie2",
         "regionsAvailable": [
            {
               "name": "Region1"
            }
         ]
      },
      {
         "content": "Movie3",
         "regionsAvailable": [
            {
               "name": "Region1"
            },
            {
               "name": "Region4"
            }
         ]
      }
   ]
}
		`,
	}}

	query := `
		query {
                    queryMovie (order: {asc: content}) {
		           content
		           regionsAvailable (order: {asc: name}) {
		           	name
		           }
		    }
		}
	`

	var result, data struct {
		QueryMovie []*Movie
	}

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.Nil(t, err)

			err = json.Unmarshal([]byte(tcase.result), &data)
			require.Nil(t, err)

			if diff := cmp.Diff(result, data); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
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
