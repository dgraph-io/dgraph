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
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

const (
	graphqlURL = "http://localhost:8180/graphql"
)

var (
	metaInfo *testutil.AuthMeta
)

type Region struct {
	Id     string         `json:"id,omitempty"`
	Name   string         `json:"name,omitempty"`
	Users  []*common.User `json:"users,omitempty"`
	Global bool           `json:"global,omitempty"`
}

type Movie struct {
	Id               string    `json:"id,omitempty"`
	Content          string    `json:"content,omitempty"`
	Hidden           bool      `json:"hidden,omitempty"`
	RegionsAvailable []*Region `json:"regionsAvailable,omitempty"`
}

type Issue struct {
	Id    string       `json:"id,omitempty"`
	Msg   string       `json:"msg,omitempty"`
	Owner *common.User `json:"owner,omitempty"`
}

type Log struct {
	Id     string `json:"id,omitempty"`
	Logs   string `json:"logs,omitempty"`
	Random string `json:"random,omitempty"`
}

type ComplexLog struct {
	Id      string `json:"id,omitempty"`
	Logs    string `json:"logs,omitempty"`
	Visible bool   `json:"visible,omitempty"`
}

type Role struct {
	Id         string         `json:"id,omitempty"`
	Permission string         `json:"permission,omitempty"`
	AssignedTo []*common.User `json:"assignedTo,omitempty"`
}

type Ticket struct {
	Id         string         `json:"id,omitempty"`
	OnColumn   *Column        `json:"onColumn,omitempty"`
	Title      string         `json:"title,omitempty"`
	AssignedTo []*common.User `json:"assignedTo,omitempty"`
}

type Column struct {
	ColID     string    `json:"colID,omitempty"`
	InProject *Project  `json:"inProject,omitempty"`
	Name      string    `json:"name,omitempty"`
	Tickets   []*Ticket `json:"tickets,omitempty"`
}

type Project struct {
	ProjID  string    `json:"projID,omitempty"`
	Name    string    `json:"name,omitempty"`
	Roles   []*Role   `json:"roles,omitempty"`
	Columns []*Column `json:"columns,omitempty"`
}

type Student struct {
	Id    string `json:"id,omitempty"`
	Email string `json:"email,omitempty"`
}

type Task struct {
	Id          string            `json:"id,omitempty"`
	Name        string            `json:"name,omitempty"`
	Occurrences []*TaskOccurrence `json:"occurrences,omitempty"`
}

type TaskOccurrence struct {
	Id   string `json:"id,omitempty"`
	Due  string `json:"due,omitempty"`
	Comp string `json:"comp,omitempty"`
}

type TestCase struct {
	user            string
	role            string
	result          string
	name            string
	filter          map[string]interface{}
	variables       map[string]interface{}
	query           string
	closedByDefault bool
}

type uidResult struct {
	Query []struct {
		UID string
	}
}

type Tasks []Task

func (tasks Tasks) add(t *testing.T) {
	getParams := &common.GraphQLParams{
		Query: `
		mutation AddTask($tasks : [AddTaskInput!]!) {
		  addTask(input: $tasks) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"tasks": tasks},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (r *Region) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
		mutation addRegion($region: AddRegionInput!) {
		  addRegion(input: [$region]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"region": r},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (r *Region) delete(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
		mutation deleteRegion($name: String) {
		  deleteRegion(filter:{name: { eq: $name}}) {
			msg
		  }
		}
		`,
		Variables: map[string]interface{}{"name": r.Name},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func TestOptimizedNestedAuthQuery(t *testing.T) {
	query := `
	query {
	  queryMovie {
		content
		 regionsAvailable {
		  name
		  global
		}
	  }
	}
	`
	user := "user1"
	role := "ADMIN"

	getUserParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query:   query,
	}

	gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
	beforeTouchUids := gqlResponse.Extensions["touched_uids"]
	beforeResult := gqlResponse.Data

	// Previously, Auth queries would have touched all the new `Regions`. But after the optimization
	// we should only touch necessary `Regions` which are assigned to some `Movie`. Hence, adding
	// these extra `Regions` would not increase the `touched_uids`.
	var regions []Region
	for i := 0; i < 100; i++ {
		r := Region{
			Name:   fmt.Sprintf("Test_Region_%d", i),
			Global: true,
		}
		r.add(t, user, role)
		regions = append(regions, r)
	}

	gqlResponse = getUserParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	afterTouchUids := gqlResponse.Extensions["touched_uids"]
	require.Equal(t, beforeTouchUids, afterTouchUids)
	require.Equal(t, beforeResult, gqlResponse.Data)

	// Clean up
	for _, region := range regions {
		region.delete(t, user, role)
	}
}

func (s Student) deleteByEmail(t *testing.T) {
	getParams := &common.GraphQLParams{
		Query: `
			mutation delStudent ($filter : StudentFilter!){
			  	deleteStudent (filter: $filter) {
					numUids
			  	}
			}
		`,
		Variables: map[string]interface{}{"filter": map[string]interface{}{
			"email": map[string]interface{}{"eq": s.Email},
		}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (s Student) add(t *testing.T) {
	mutation := &common.GraphQLParams{
		Query: `
		mutation addStudent($student : AddStudentInput!) {
			addStudent(input: [$student]) {
				numUids
			}
		}`,
		Variables: map[string]interface{}{"student": s},
	}
	result := `{"addStudent":{"numUids": 1}}`
	gqlResponse := mutation.ExecuteAsPost(t, graphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
	require.JSONEq(t, result, string(gqlResponse.Data))
}

func TestAuthRulesMutationWithClosed(t *testing.T) {
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
		user:   "user1",
		result: `{"addUserSecret":{"usersecret":[{"aSecret":"secret1"}]}}`,
		variables: map[string]interface{}{"user": &common.UserSecret{
			ASecret: "secret1",
			OwnedBy: "user1",
		}},
	},
		{
			name: "Invalid JWT - type with auth directive",
			query: `
	          	mutation addUser($user: AddUserSecretInput!) {
		        	addUserSecret(input: [$user]) {
				      userSecret {
					    aSecret
				      }
			       }
	        	}`,
			user:   "user1",
			result: `{"addUserSecret":{"usersecret":[{"aSecret":"secret1"}]}}`,
			variables: map[string]interface{}{"user": &common.UserSecret{
				ASecret: "secret1",
				OwnedBy: "user1",
			}},
		},

		{
			result: `{"addTodo":{"Todo":[]}}`,
			name:   "Missing JWT - type without auth directive",
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
		},
		{
			result: `{"addTodo":{"Todo":[]}}`,
			name:   "Invalid JWT - type without auth directive",
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
		}}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Query:     tcase.query,
			Variables: tcase.variables,
		}
		testMissingJWT := strings.HasPrefix(tcase.name, "Missing JWT")
		testInvalidKey := strings.HasPrefix(tcase.name, "Invalid JWT")
		if testInvalidKey {
			getUserParams.Headers = common.GetJWT(t, tcase.user, tcase.role, metaInfo)
			jwtVar := getUserParams.Headers.Get(metaInfo.Header)

			// Create a invalid JWT signature.
			jwtVar = jwtVar + "A"
			getUserParams.Headers.Set(metaInfo.Header, jwtVar)
		} else if !testMissingJWT {
			getUserParams.Headers = common.GetJWT(t, tcase.user, tcase.role, metaInfo)
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		require.Equal(t, len(gqlResponse.Errors), 1)
		if testMissingJWT {
			require.Contains(t, gqlResponse.Errors[0].Error(),
				"Jwt is required when ClosedByDefault flag is true")
		} else if testInvalidKey {
			require.Contains(t, gqlResponse.Errors[0].Error(),
				"unable to parse jwt token:token signature is invalid")

		}
	}
}

func TestAuthRulesQueryWithClosed(t *testing.T) {
	testCases := []TestCase{
		{name: "Missing JWT - Query auth field without JWT Token",
			query: `
			query {
				queryMovie {
					content
				}
			}`,
			result: `{"queryMovie":[]}`,
		},
		{name: "Missing JWT - Query empty auth field without JWT Token",
			query: `
			query {
				queryReview {
					comment
				}
			}`,
			result: `{"queryReview":[]}`,
		},
		{name: "Query auth field with invalid JWT Token",
			query: `
			query {
				queryProject {
					name
				}
			}`,
			user:   "user1",
			role:   "ADMIN",
			result: `{"queryProject":[]}`,
		},
		{name: "Missing JWT - type with auth field",
			query: `
			query {
				queryProject {
					name
				}
			}`,
			user:   "user1",
			role:   "ADMIN",
			result: `{"queryProject":[]}`,
		},
		{name: "Missing JWT - type without auth field",
			query: `
			query {
				queryTodo {
					owner
				}
			}`,
			user:   "user1",
			role:   "ADMIN",
			result: `{"queryTodo":[]}`,
		},
		{name: "non auth type with invalid JWT Token",
			query: `
			query {
				queryTodo {
					owner
				}
			}`,
			user:   "user1",
			role:   "ADMIN",
			result: `{"queryTodo":[]}`,
		},
	}

	for _, tcase := range testCases {
		queryParams := &common.GraphQLParams{
			Query: tcase.query,
		}
		testMissingJWT := strings.HasPrefix(tcase.name, "Missing JWT")
		testInvalidKey := strings.HasSuffix(tcase.name, "invalid JWT Token")
		if testInvalidKey {
			queryParams.Headers = common.GetJWT(t, tcase.user, tcase.role, metaInfo)
			jwtVar := queryParams.Headers.Get(metaInfo.Header)

			// Create a invalid JWT signature.
			jwtVar = jwtVar + "A"
			queryParams.Headers.Set(metaInfo.Header, jwtVar)
		} else if (tcase.user != "" || tcase.role != "") && (!testMissingJWT) {
			queryParams.Headers = common.GetJWT(t, tcase.user, tcase.role, metaInfo)
		}
		gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
		if testMissingJWT {
			require.Contains(t, gqlResponse.Errors[0].Error(),
				"Jwt is required when ClosedByDefault flag is true")
		} else if testInvalidKey {
			require.Contains(t, gqlResponse.Errors[0].Error(),
				"unable to parse jwt token:token signature is invalid")

		}
		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("Test: %s result mismatch (-want +got):\n%s", tcase.name, diff)
		}
	}
}

func TestMain(m *testing.M) {
	schemaFile := "../auth/schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		panic(err)
	}

	jsonFile := "test_data.json"
	data, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		panic(errors.Wrapf(err, "Unable to read file %s.", jsonFile))
	}

	algo := authorization.HMAC256
	authSchema, err := testutil.AppendAuthInfo(schema, algo, "../auth/sample_public_key.pem", true)
	if err != nil {
		panic(err)
	}

	authMeta := testutil.SetAuthMeta(string(authSchema))
	metaInfo = &testutil.AuthMeta{
		PublicKey:      authMeta.VerificationKey,
		Namespace:      authMeta.Namespace,
		Algo:           authMeta.Algo,
		Header:         authMeta.Header,
		PrivateKeyPath: "../auth/sample_private_key.pem",
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
