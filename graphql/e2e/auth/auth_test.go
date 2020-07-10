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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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

type User struct {
	Username string `json:"username,omitempty"`
	Age      uint64 `json:"age,omitempty"`
	IsPublic bool   `json:"isPublic,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type UserSecret struct {
	Id      string `json:"id,omitempty"`
	ASecret string `json:"aSecret,omitempty"`
	OwnedBy string `json:"ownedBy,omitempty"`
}

type Region struct {
	Id     string  `json:"id,omitempty"`
	Name   string  `json:"name,omitempty"`
	Users  []*User `json:"users,omitempty"`
	Global bool    `json:"global,omitempty"`
}

type Movie struct {
	Id               string    `json:"id,omitempty"`
	Content          string    `json:"content,omitempty"`
	Hidden           bool      `json:"hidden,omitempty"`
	RegionsAvailable []*Region `json:"regionsAvailable,omitempty"`
}

type Issue struct {
	Id    string `json:"id,omitempty"`
	Msg   string `json:"msg,omitempty"`
	Owner *User  `json:"owner,omitempty"`
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
	Id         string  `json:"id,omitempty"`
	Permission string  `json:"permission,omitempty"`
	AssignedTo []*User `json:"assignedTo,omitempty"`
}

type Ticket struct {
	Id         string  `json:"id,omitempty"`
	OnColumn   *Column `json:"onColumn,omitempty"`
	Title      string  `json:"title,omitempty"`
	AssignedTo []*User `json:"assignedTo,omitempty"`
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

type TestCase struct {
	user      string
	role      string
	result    string
	name      string
	filter    map[string]interface{}
	variables map[string]interface{}
	query     string
}

type uidResult struct {
	Query []struct {
		UID string
	}
}

func (r *Region) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(t, user, role),
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
		Headers: getJWT(t, user, role),
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

func getJWT(t *testing.T, user, role string) http.Header {
	metaInfo.AuthVars = map[string]interface{}{}
	if user != "" {
		metaInfo.AuthVars["USER"] = user
	}

	if role != "" {
		metaInfo.AuthVars["ROLE"] = role
	}

	jwtToken, err := metaInfo.GetSignedToken("./sample_private_key.pem")
	require.NoError(t, err)

	h := make(http.Header)
	h.Add(metaInfo.Header, jwtToken)
	return h
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
		Headers: getJWT(t, user, role),
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

func TestAuthWithDgraphDirective(t *testing.T) {
	students := []Student{
		{
			Email: "user1@gmail.com",
		},
		{
			Email: "user2@gmail.com",
		},
	}
	for _, student := range students {
		student.add(t)
	}

	testCases := []TestCase{{
		user:   students[0].Email,
		role:   "ADMIN",
		result: `{"queryStudent":[{"email":"` + students[0].Email + `"}]}`,
	}, {
		user:   students[0].Email,
		role:   "USER",
		result: `{"queryStudent" : []}`,
	}}

	queryStudent := `
	query {
		queryStudent {
			email
		}
	}`

	for _, tcase := range testCases {
		t.Run(tcase.role+"_"+tcase.user, func(t *testing.T) {
			queryParams := &common.GraphQLParams{
				Query:   queryStudent,
				Headers: getJWT(t, tcase.user, tcase.role),
			}
			gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
		})
	}

	// Clean up
	for _, student := range students {
		student.deleteByEmail(t)
	}
}

func TestAuthRulesWithMissingJWT(t *testing.T) {
	testCases := []TestCase{
		{name: "Query non auth field without JWT Token",
			query: `
			query {
			  queryRole(filter: {permission: { eq: EDIT }}) {
				permission
			  }
			}`,
			result: `{"queryRole":[{"permission":"EDIT"}]}`,
		},
		{name: "Query auth field without JWT Token",
			query: `
			query {
				queryMovie {
					content
				}
			}`,
			result: `{"queryMovie":[{"content":"Movie4"}]}`,
		},
		{name: "Query empty auth field without JWT Token",
			query: `
			query {
				queryReview {
					comment
				}
			}`,
			result: `{"queryReview":[{"comment":"Nice movie"}]}`,
		},
		{name: "Query auth field with partial JWT Token",
			query: `
			query {
				queryProject {
					name
				}
			}`,
			user:   "user1",
			result: `{"queryProject":[{"name":"Project1"}]}`,
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
	}

	for _, tcase := range testCases {
		queryParams := &common.GraphQLParams{
			Query: tcase.query,
		}

		testInvalidKey := strings.HasSuffix(tcase.name, "invalid JWT Token")
		if testInvalidKey {
			queryParams.Headers = getJWT(t, tcase.user, tcase.role)
			jwtVar := queryParams.Headers.Get(metaInfo.Header)

			// Create a invalid JWT signature.
			jwtVar = jwtVar + "A"
			queryParams.Headers.Set(metaInfo.Header, jwtVar)
		} else if tcase.user != "" || tcase.role != "" {
			queryParams.Headers = getJWT(t, tcase.user, tcase.role)
		}

		gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
		if testInvalidKey {
			require.Contains(t, gqlResponse.Errors[0].Error(),
				"couldn't rewrite query queryProject because unable to parse jwt token")
		} else {
			require.Nil(t, gqlResponse.Errors)
		}

		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("Test: %s result mismatch (-want +got):\n%s", tcase.name, diff)
		}
	}
}

func TestOrRBACFilter(t *testing.T) {
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

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func getColID(t *testing.T, tcase TestCase) string {
	query := `
		query($name: String!) {
		    queryColumn(filter: {name: {eq: $name}}) {
		        colID
		    	name
		    }
		}
	`

	var result struct {
		QueryColumn []*Column
	}

	getUserParams := &common.GraphQLParams{
		Headers:   getJWT(t, tcase.user, tcase.role),
		Query:     query,
		Variables: map[string]interface{}{"name": tcase.name},
	}

	gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	err := json.Unmarshal(gqlResponse.Data, &result)
	require.Nil(t, err)

	if len(result.QueryColumn) > 0 {
		return result.QueryColumn[0].ColID
	}

	return ""
}

func TestRootGetFilter(t *testing.T) {
	idCol1 := getColID(t, TestCase{user: "user1", role: "USER", name: "Column1"})
	idCol2 := getColID(t, TestCase{user: "user2", role: "USER", name: "Column2"})
	require.NotEqual(t, idCol1, "")
	require.NotEqual(t, idCol2, "")

	tcases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"getColumn": {"name": "Column1"}}`,
		name:   idCol1,
	}, {
		user:   "user1",
		role:   "USER",
		result: `{"getColumn": null}`,
		name:   idCol2,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"getColumn": {"name": "Column2"}}`,
		name:   idCol2,
	}}

	query := `
		query($id: ID!) {
		    getColumn(colID: $id) {
				name
		    }
		}
	`

	for _, tcase := range tcases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers:   getJWT(t, tcase.user, tcase.role),
				Query:     query,
				Variables: map[string]interface{}{"id": tcase.name},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func getProjectID(t *testing.T, tcase TestCase) string {
	query := `
		query($name: String!) {
		    queryProject(filter: {name: {eq: $name}}) {
		        projID
		    }
		}
	`

	var result struct {
		QueryProject []*Project
	}

	getUserParams := &common.GraphQLParams{
		Headers:   getJWT(t, tcase.user, tcase.role),
		Query:     query,
		Variables: map[string]interface{}{"name": tcase.name},
	}

	gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	err := json.Unmarshal(gqlResponse.Data, &result)
	require.Nil(t, err)

	if len(result.QueryProject) > 0 {
		return result.QueryProject[0].ProjID
	}

	return ""
}

func TestRootGetDeepFilter(t *testing.T) {
	idProject1 := getProjectID(t, TestCase{user: "user1", role: "USER", name: "Project1"})
	idProject2 := getProjectID(t, TestCase{user: "user2", role: "USER", name: "Project2"})
	require.NotEqual(t, idProject1, "")
	require.NotEqual(t, idProject2, "")

	tcases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"getProject":{"name":"Project1","columns":[{"name":"Column1"}]}}`,
		name:   idProject1,
	}, {
		user:   "user1",
		role:   "USER",
		result: `{"getProject": null}`,
		name:   idProject2,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"getProject":{"name":"Project2","columns":[{"name":"Column2"},{"name":"Column3"}]}}`,
		name:   idProject2,
	}}

	query := `
		query($id: ID!) {
		    getProject(projID: $id) {
				name
				columns(order: {asc: name}) {
          				name
				}
		    }
		}
	`

	for _, tcase := range tcases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers:   getJWT(t, tcase.user, tcase.role),
				Query:     query,
				Variables: map[string]interface{}{"id": tcase.name},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestDeepFilter(t *testing.T) {
	tcases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"queryProject":[{"name":"Project1","columns":[{"name":"Column1"}]}]}`,
		name:   "Column1",
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"queryProject":[{"name":"Project1","columns":[{"name":"Column1"}]}, {"name":"Project2","columns":[]}]}`,
		name:   "Column1",
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"queryProject":[{"name":"Project1","columns":[]}, {"name":"Project2","columns":[{"name":"Column3"}]}]}`,
		name:   "Column3",
	}}

	query := `
		query($name: String!) {
		    queryProject (order: {asc: name}) {
		       name
		       columns (filter: {name: {eq: $name}}, first: 1) {
		       	   name
                       }
                    }
                 }
	`

	for _, tcase := range tcases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers:   getJWT(t, tcase.user, tcase.role),
				Query:     query,
				Variables: map[string]interface{}{"name": tcase.name},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)
			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
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
	}`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestDeepRBACValue(t *testing.T) {
	testCases := []TestCase{
		{user: "user1", role: "USER", result: `{"queryUser": [{"username": "user1", "issues":[]}]}`},
		{user: "user1", role: "ADMIN", result: `{"queryUser":[{"username":"user1","issues":[{"msg":"Issue1"}]}]}`},
	}

	query := `
	query {
	  queryUser (filter:{username:{eq:"user1"}}) {
		username
		issues {
		  msg
		}
	  }
	}
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestRBACFilter(t *testing.T) {
	testCases := []TestCase{
		{role: "USER", result: `{"queryLog": []}`},
		{result: `{"queryLog": []}`},
		{role: "ADMIN", result: `{"queryLog": [{"logs": "Log1"},{"logs": "Log2"}]}`}}

	query := `
		query {
			queryLog (order: {asc: logs}) {
		    	logs
		    }
		}
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestAndRBACFilter(t *testing.T) {
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

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestNestedFilter(t *testing.T) {
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
      },
      {
         "content": "Movie4",
         "regionsAvailable": [
            {
               "name": "Region5"
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
      },
      {
         "content": "Movie4",
         "regionsAvailable": [
            {
               "name": "Region5"
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

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: getJWT(t, tcase.user, tcase.role),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestDeleteAuthRule(t *testing.T) {
	AddDeleteAuthTestData(t)
	testCases := []TestCase{
		{
			name: "user with secret info",
			user: "user1",
			filter: map[string]interface{}{
				"aSecret": map[string]interface{}{
					"anyofterms": "Secret data",
				},
			},
			result: `{"deleteUserSecret":{"msg":"Deleted","numUids":1}}`,
		},
		{
			name: "user without secret info",
			user: "user2",
			filter: map[string]interface{}{
				"aSecret": map[string]interface{}{
					"anyofterms": "Sensitive information",
				},
			},
			result: `{"deleteUserSecret":{"msg":"No nodes were deleted","numUids":0}}`,
		},
	}
	query := `
		 mutation deleteUserSecret($filter: UserSecretFilter!){
		  deleteUserSecret(filter: $filter) {
			msg
			numUids
		  }
		}
	`

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers: getJWT(t, tcase.user, tcase.role),
			Query:   query,
			Variables: map[string]interface{}{
				"filter": tcase.filter,
			},
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		require.Nilf(t, gqlResponse.Errors, "%+v", gqlResponse.Errors)

		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	}
}

func AddDeleteAuthTestData(t *testing.T) {
	client, err := testutil.DgraphClient(common.AlphagRPC)
	require.NoError(t, err)
	data := `[{
		"uid": "_:usersecret1",
		"dgraph.type": "UserSecret",
		"UserSecret.aSecret": "Secret data",
		"UserSecret.ownedBy": "user1"
		}]`

	err = common.PopulateGraphQLData(client, []byte(data))
	require.NoError(t, err)
}

func AddDeleteDeepAuthTestData(t *testing.T) {
	client, err := testutil.DgraphClient(common.AlphagRPC)
	require.NoError(t, err)

	userQuery := `{
	query(func: type(User)) @filter(eq(User.username, "user1") or eq(User.username, "user3") or
		eq(User.username, "user5") ) {
    	uid
  	} }`

	txn := client.NewTxn()
	resp, err := txn.Query(context.Background(), userQuery)
	require.NoError(t, err)

	var user uidResult
	err = json.Unmarshal(resp.Json, &user)
	require.NoError(t, err)
	require.True(t, len(user.Query) == 3)

	columnQuery := `{
  	query(func: type(Column)) @filter(eq(Column.name, "Column1")) {
		uid
		Column.name
  	} }`

	resp, err = txn.Query(context.Background(), columnQuery)
	require.NoError(t, err)

	var column uidResult
	err = json.Unmarshal(resp.Json, &column)
	require.NoError(t, err)
	require.True(t, len(column.Query) == 1)

	data := fmt.Sprintf(`[{
		"uid": "_:ticket1",
		"dgraph.type": "Ticket",
		"Ticket.onColumn": {"uid": "%s"},
		"Ticket.title": "Ticket1",
		"ticket.assignedTo": [{"uid": "%s"}, {"uid": "%s"}, {"uid": "%s"}]
	}]`, column.Query[0].UID, user.Query[0].UID, user.Query[1].UID, user.Query[2].UID)

	err = common.PopulateGraphQLData(client, []byte(data))
	require.NoError(t, err)
}

func TestDeleteDeepAuthRule(t *testing.T) {
	AddDeleteDeepAuthTestData(t)
	testCases := []TestCase{
		{
			name: "ticket without edit permission",
			user: "user3",
			filter: map[string]interface{}{
				"title": map[string]interface{}{
					"anyofterms": "Ticket2",
				},
			},
			result: `{"deleteTicket":{"msg":"No nodes were deleted","numUids":0}}`,
		},
		{
			name: "ticket with edit permission",
			user: "user5",
			filter: map[string]interface{}{
				"title": map[string]interface{}{
					"anyofterms": "Ticket1",
				},
			},
			result: `{"deleteTicket":{"msg":"Deleted","numUids":1}}`,
		},
	}
	query := `
		mutation deleteTicket($filter: TicketFilter!) {
		  deleteTicket(filter: $filter) {
			msg
			numUids
		  }
		}
	`

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers: getJWT(t, tcase.user, tcase.role),
			Query:   query,
			Variables: map[string]interface{}{
				"filter": tcase.filter,
			},
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		require.Nil(t, gqlResponse.Errors)

		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
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

	jwtAlgo := []string{authorization.HMAC256, authorization.RSA256}
	for _, algo := range jwtAlgo {
		authSchema, err := testutil.AppendAuthInfo(schema, algo, "./sample_public_key.pem")
		if err != nil {
			panic(err)
		}

		authMeta, err := authorization.Parse(string(authSchema))
		if err != nil {
			panic(err)
		}

		metaInfo = &testutil.AuthMeta{
			PublicKey: authMeta.VerificationKey,
			Namespace: authMeta.Namespace,
			Algo:      authMeta.Algo,
			Header:    authMeta.Header,
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
	}
	os.Exit(0)
}
