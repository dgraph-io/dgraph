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

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/dgrijalva/jwt-go/v4"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
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
	Code             string    `json:"code,omitempty"`
	Hidden           bool      `json:"hidden,omitempty"`
	RegionsAvailable []*Region `json:"regionsAvailable,omitempty"`
}

type Issue struct {
	Id    string       `json:"id,omitempty"`
	Msg   string       `json:"msg,omitempty"`
	Owner *common.User `json:"owner,omitempty"`
}

type Author struct {
	Id    string      `json:"id,omitempty"`
	Name  string      `json:"name,omitempty"`
	Posts []*Question `json:"posts,omitempty"`
}

type Post struct {
	Id     string  `json:"id,omitempty"`
	Text   string  `json:"text,omitempty"`
	Author *Author `json:"author,omitempty"`
}

type Question struct {
	Id       string  `json:"id,omitempty"`
	Text     string  `json:"text,omitempty"`
	Answered bool    `json:"answered,omitempty"`
	Author   *Author `json:"author,omitempty"`
	Pwd      string  `json:"pwd,omitempty"`
}

type Answer struct {
	Id     string  `json:"id,omitempty"`
	Text   string  `json:"text,omitempty"`
	Author *Author `json:"author,omitempty"`
}

type FbPost struct {
	Id        string  `json:"id,omitempty"`
	Text      string  `json:"text,omitempty"`
	Author    *Author `json:"author,omitempty"`
	Sender    *Author `json:"sender,omitempty"`
	Receiver  *Author `json:"receiver,omitempty"`
	PostCount int     `json:"postCount,omitempty"`
	Pwd       string  `json:"pwd,omitempty"`
}

type Log struct {
	Id     string `json:"id,omitempty"`
	Logs   string `json:"logs,omitempty"`
	Random string `json:"random,omitempty"`
	Pwd    string `json:"pwd,omitempty"`
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

type Country struct {
	Id      string   `json:"id,omitempty"`
	Name    string   `json:"name,omitempty"`
	OwnedBy string   `json:"ownedBy,omitempty"`
	States  []*State `json:"states,omitempty"`
}

type State struct {
	Code    string   `json:"code,omitempty"`
	Name    string   `json:"name,omitempty"`
	OwnedBy string   `json:"ownedBy,omitempty"`
	Country *Country `json:"country,omitempty"`
}

type Project struct {
	ProjID  string    `json:"projID,omitempty"`
	Name    string    `json:"name,omitempty"`
	Roles   []*Role   `json:"roles,omitempty"`
	Columns []*Column `json:"columns,omitempty"`
	Pwd     string    `json:"pwd,omitempty"`
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
	user          string
	role          string
	ans           bool
	result        string
	name          string
	jwt           string
	filter        map[string]interface{}
	variables     map[string]interface{}
	query         string
	expectedError bool
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
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
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
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
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
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
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

	gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
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

	gqlResponse = getUserParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

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
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
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
	gqlResponse := mutation.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
	require.JSONEq(t, result, string(gqlResponse.Data))
}

func TestAddMutationWithXid(t *testing.T) {
	mutation := `
	mutation addTweets($tweet: AddTweetsInput!){
      addTweets(input: [$tweet]) {
        numUids
      }
    }
	`

	tweet := common.Tweets{
		Id:        "tweet1",
		Text:      "abc",
		Timestamp: "2020-10-10",
	}
	user := "foo"
	addTweetsParams := &common.GraphQLParams{
		Headers:   common.GetJWT(t, user, "", metaInfo),
		Query:     mutation,
		Variables: map[string]interface{}{"tweet": tweet},
	}

	// Add the tweet for the first time.
	gqlResponse := addTweetsParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	// Re-adding the tweet should fail.
	gqlResponse = addTweetsParams.ExecuteAsPost(t, common.GraphqlURL)
	require.Error(t, gqlResponse.Errors)
	require.Equal(t, len(gqlResponse.Errors), 1)
	require.Contains(t, gqlResponse.Errors[0].Error(),
		"GraphQL debug: id tweet1 already exists for field id inside type Tweets")

	// Clear the tweet.
	tweet.DeleteByID(t, user, metaInfo)
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
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			}
			gqlResponse := queryParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
		})
	}

	// Clean up
	for _, student := range students {
		student.deleteByEmail(t)
	}
}

func TestAuthOnInterfaces(t *testing.T) {
	TestCases := []TestCase{
		{
			name: "Types inherit Interface's auth rules and its own rules",
			query: `
		query{
			queryQuestion{
				text
			}
		}
		`,
			user:   "user1@dgraph.io",
			ans:    true,
			result: `{"queryQuestion":[{"text": "A Question"}]}`,
		},
		{
			name: "Query Should return empty for non-existent user",
			query: `
		query{
			queryQuestion{
				text
			}
		}
		`,
			user:   "user3@dgraph.io",
			ans:    true,
			result: `{"queryQuestion":[]}`,
		},
		{
			name: "Types inherit Only Interface's auth rules if it doesn't have its own auth rules",
			query: `
			query{
				queryAnswer{
					text
				}
			}
			`,
			user:   "user1@dgraph.io",
			result: `{"queryAnswer": [{"text": "A Answer"}]}`,
		},
		{
			name: "Types inherit auth rules from all the different Interfaces",
			query: `
			query{
				queryFbPost{
					text
				}
			}
			`,
			user:   "user2@dgraph.io",
			role:   "ADMIN",
			result: `{"queryFbPost": [{"text": "B FbPost"}]}`,
		},
		{
			name: "Query Interface should inherit auth rules from all the interfaces",
			query: `
			query{
				queryPost(order: {asc: text}){
					text
				}
			}
			`,
			user:   "user1@dgraph.io",
			ans:    true,
			role:   "ADMIN",
			result: `{"queryPost":[{"text": "A Answer"},{"text": "A FbPost"},{"text": "A Question"}]}`,
		},
		{
			name: "Query Interface should return those implementing type whose auth rules are satisfied",
			query: `
			query{
				queryPost(order: {asc: text}){
					text
				}
			}
			`,
			user:   "user1@dgraph.io",
			ans:    true,
			result: `{"queryPost":[{"text": "A Answer"},{"text": "A Question"}]}`,
		},
		{
			name: "Query Interface should return empty if the Auth rules of interface are not satisfied",
			query: `
			query{
				queryPost(order: {asc: text}){
					text
				}
			}
			`,
			ans:    true,
			result: `{"queryPost":[]}`,
		},
	}

	for _, tcase := range TestCases {
		t.Run(tcase.name, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: common.GetJWTForInterfaceAuth(t, tcase.user, tcase.role, tcase.ans, metaInfo),
				Query:   tcase.query,
			}
			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
		})
	}
}

func TestNestedAndAuthRulesWithMissingJWT(t *testing.T) {
	addParams := &common.GraphQLParams{
		Query: `
		mutation($user1: String!, $user2: String!){
			addGroup(input: [{users: {username: $user1}, createdBy: {username: $user2}}, {users: {username: $user2}, createdBy: {username: $user1}}]){
			  numUids
			}
		  }
		`,
		Variables: map[string]interface{}{"user1": "user1", "user2": "user2"},
	}
	gqlResponse := addParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
	require.JSONEq(t, `{"addGroup": {"numUids": 2}}`, string(gqlResponse.Data))

	queryParams := &common.GraphQLParams{
		Query: `
		query{
			queryGroup{
			  users{
				username
			  }
			}
		  }
		`,
		Headers: common.GetJWT(t, "user1", nil, metaInfo),
	}

	expectedJSON := `{"queryGroup": [{"users": [{"username": "user1"}]}]}`

	gqlResponse = queryParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
	require.JSONEq(t, expectedJSON, string(gqlResponse.Data))

	deleteFilter := map[string]interface{}{"has": "users"}
	common.DeleteGqlType(t, "Group", deleteFilter, 2, nil)
}

func TestAuthRulesWithNullValuesInJWT(t *testing.T) {
	testCases := []TestCase{
		{
			name: "Query with null value in jwt",
			query: `
			query {
				queryProject {
					name
				}
			}
			`,
			result: `{"queryProject":[]}`,
		},
		{
			name: "Query with null value in jwt: deep level",
			query: `
			query {
				queryUser(order: {desc: username}, first: 1) {
					username
					issues {
						msg
					}
				}
			}
			`,
			role:   "ADMIN",
			result: `{"queryUser":[{"username":"user8","issues":[]}]}`,
		},
	}

	for _, tcase := range testCases {
		queryParams := &common.GraphQLParams{
			Headers: common.GetJWTWithNullUser(t, tcase.role, metaInfo),
			Query:   tcase.query,
		}
		gqlResponse := queryParams.ExecuteAsPost(t, common.GraphqlURL)
		common.RequireNoGQLErrors(t, gqlResponse)

		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("Test: %s result mismatch (-want +got):\n%s", tcase.name, diff)
		}
	}
}

func TestAuthOnInterfaceWithRBACPositive(t *testing.T) {
	getVehicleParams := &common.GraphQLParams{
		Query: `
		query {
			queryVehicle{
				owner
			}
		}`,
		Headers: common.GetJWT(t, "Alice", "ADMIN", metaInfo),
	}
	gqlResponse := getVehicleParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	result := `
	{
		"queryVehicle": [
		  {
			"owner": "Bob"
		  }
		]
	  }`

	require.JSONEq(t, result, string(gqlResponse.Data))
}

func TestQueryWithStandardClaims(t *testing.T) {
	if metaInfo.Algo == "RS256" {
		t.Skip()
	}
	testCases := []TestCase{
		{
			query: `
            query {
                queryProject (order: {asc: name}) {
					name
				}
			}`,
			jwt:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiZXhwIjozNTE2MjM5MDIyLCJlbWFpbCI6InRlc3RAZGdyYXBoLmlvIiwiVVNFUiI6InVzZXIxIiwiUk9MRSI6IkFETUlOIn0.cH_EcC8Sd0pawJs96XPhpRsYVXuTybT1oUkluBDS8B4",
			result: `{"queryProject":[{"name":"Project1"},{"name":"Project2"}]}`,
		},
		{
			query: `
			query {
				queryProject {
					name
				}
			}`,
			jwt:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiZXhwIjozNTE2MjM5MDIyLCJlbWFpbCI6InRlc3RAZGdyYXBoLmlvIiwiVVNFUiI6InVzZXIxIn0.wabcAkINZ6ycbEuziTQTSpv8T875Ky7JQu68ynoyDQE",
			result: `{"queryProject":[{"name":"Project1"}]}`,
		},
	}

	for _, tcase := range testCases {
		queryParams := &common.GraphQLParams{
			Headers: make(http.Header),
			Query:   tcase.query,
		}
		queryParams.Headers.Set(metaInfo.Header, tcase.jwt)

		gqlResponse := queryParams.ExecuteAsPost(t, common.GraphqlURL)
		common.RequireNoGQLErrors(t, gqlResponse)

		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("Test: %s result mismatch (-want +got):\n%s", tcase.name, diff)
		}
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
				queryMovie(order: {asc: content}) {
					content
				}
			}`,
			result: `{"queryMovie":[{"content":"Movie3"},{"content":"Movie4"}]}`,
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
			queryParams.Headers = common.GetJWT(t, tcase.user, tcase.role, metaInfo)
			jwtVar := queryParams.Headers.Get(metaInfo.Header)

			// Create a invalid JWT signature.
			jwtVar = jwtVar + "A"
			queryParams.Headers.Set(metaInfo.Header, jwtVar)
		} else if tcase.user != "" || tcase.role != "" {
			queryParams.Headers = common.GetJWT(t, tcase.user, tcase.role, metaInfo)
		}

		gqlResponse := queryParams.ExecuteAsPost(t, common.GraphqlURL)
		if testInvalidKey {
			require.Contains(t, gqlResponse.Errors[0].Error(),
				"couldn't rewrite query queryProject because unable to parse jwt token")
		} else {
			common.RequireNoGQLErrors(t, gqlResponse)
		}

		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("Test: %s result mismatch (-want +got):\n%s", tcase.name, diff)
		}
	}
}

func TestBearerToken(t *testing.T) {
	queryProjectParams := &common.GraphQLParams{
		Query: `
		query {
			queryProject {
				name
			}
		}`,
		Headers: http.Header{},
	}

	// querying with a bad bearer token should give back an error
	queryProjectParams.Headers.Set(metaInfo.Header, "Bearer bad token")
	resp := queryProjectParams.ExecuteAsPost(t, common.GraphqlURL)
	require.Contains(t, resp.Errors.Error(), "invalid Bearer-formatted header value for JWT (Bearer bad token)")
	require.Nil(t, resp.Data)

	// querying with a correct bearer token should give back expected results
	queryProjectParams.Headers.Set(metaInfo.Header, "Bearer "+common.GetJWT(t, "user1", "",
		metaInfo).Get(metaInfo.Header))
	resp = queryProjectParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, resp)
	testutil.CompareJSON(t, `{"queryProject":[{"name":"Project1"}]}`, string(resp.Data))
}

func TestOrderAndOffset(t *testing.T) {
	tasks := Tasks{
		Task{
			Name: "First Task four occurrence",
			Occurrences: []*TaskOccurrence{
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
			},
		},
		Task{
			Name: "Second Task single occurrence",
			Occurrences: []*TaskOccurrence{
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
			},
		},
		Task{
			Name:        "Third Task no occurrence",
			Occurrences: []*TaskOccurrence{},
		},
		Task{
			Name: "Fourth Task two occurrences",
			Occurrences: []*TaskOccurrence{
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
			},
		},
		Task{
			Name: "Fifth one, two occurrences",
			Occurrences: []*TaskOccurrence{
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
			},
		},
		Task{
			Name: "Sixth Task four occurrences",
			Occurrences: []*TaskOccurrence{
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
				{Due: "2020-07-19T08:00:00", Comp: "2020-07-19T08:00:00"},
			},
		},
	}
	tasks.add(t)

	query := `
	query {
	  queryTask(filter: {name: {anyofterms: "Task"}}, first: 4, offset: 1, order: {asc : name}) {
		name
		occurrences(first: 2) {
		  due
		  comp
		}
	  }
	}
	`
	testCases := []TestCase{{
		user: "user1",
		role: "ADMIN",
		result: `
		{
		"queryTask": [
		  {
			"name": "Fourth Task two occurrences",
			"occurrences": [
			  {
				"due": "2020-07-19T08:00:00Z",
				"comp": "2020-07-19T08:00:00Z"
			  },
			  {
				"due": "2020-07-19T08:00:00Z",
				"comp": "2020-07-19T08:00:00Z"
			  }
			]
		  },
		  {
			"name": "Second Task single occurrence",
			"occurrences": [
			  {
				"due": "2020-07-19T08:00:00Z",
				"comp": "2020-07-19T08:00:00Z"
			  }
			]
		  },
		  {
			"name": "Sixth Task four occurrences",
			"occurrences": [
			  {
				"due": "2020-07-19T08:00:00Z",
				"comp": "2020-07-19T08:00:00Z"
			  },
			  {
				"due": "2020-07-19T08:00:00Z",
				"comp": "2020-07-19T08:00:00Z"
			  }
			]
		  },
		  {
			"name": "Third Task no occurrence",
			"occurrences": []
		  }
		]
	  }
		`,
	}}

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
		})
	}

	// Clean up `Task`
	getParams := &common.GraphQLParams{
		Query: `
		mutation DelTask {
		  deleteTask(filter: {}) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"tasks": tasks},
	}
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	// Clean up `TaskOccurrence`
	getParams = &common.GraphQLParams{
		Query: `
		mutation DelTaskOccuerence {
		  deleteTaskOccurrence(filter: {}) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"tasks": tasks},
	}
	gqlResponse = getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func TestQueryAuthWithFilterOnIDType(t *testing.T) {
	testCases := []struct {
		user   []string
		result string
	}{{
		user: []string{"0xffe", "0xfff"},
		result: `{
			  "queryPerson": [
				{
				  "name": "Person1"
				},
				{
				  "name": "Person2"
				}
			  ]
		}`,
	}, {
		user: []string{"0xffd", "0xffe"},
		result: `{
			  "queryPerson": [
				{
				  "name": "Person1"
				}
			  ]
		}`,
	}, {
		user: []string{"0xaaa", "0xbbb"},
		result: `{
			  "queryPerson": []
		}`,
	}}

	query := `
		query {
			queryPerson(order: {asc: name}){
				name
			}
		}
	`
	for _, tcase := range testCases {
		t.Run(tcase.user[0]+tcase.user[1], func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: common.GetJWT(t, tcase.user, nil, metaInfo),
				Query:   query,
			}
			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
		})
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
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

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
		Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
		Query:     query,
		Variables: map[string]interface{}{"name": tcase.name},
	}

	gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

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
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"id": tcase.name},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

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
		Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
		Query:     query,
		Variables: map[string]interface{}{"name": tcase.name},
	}

	gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

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
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"id": tcase.name},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

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
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"name": tcase.name},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
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
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestRootAggregateQuery(t *testing.T) {
	testCases := []TestCase{
		{
			user: "user1",
			role: "USER",
			result: `
						{
							"aggregateColumn":
								{
									"count": 1,
									"nameMin": "Column1",
									"nameMax": "Column1"
								}
						}`,
		},
		{
			user: "user2",
			role: "USER",
			result: `
						{
							"aggregateColumn":
								{
									"count": 3,
									"nameMin": "Column1",
									"nameMax": "Column3"
								}
						}`,
		},
		{
			user: "user4",
			role: "USER",
			result: `
						{
							"aggregateColumn":
								{
									"count": 2,
									"nameMin": "Column2",
									"nameMax": "Column3"
								}
						}`,
		},
	}
	query := `
	query {
		aggregateColumn {
			count
			nameMin
			nameMax
		}
	}`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			params := &common.GraphQLParams{
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := params.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
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
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

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
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestRBACFilterWithAggregateQuery(t *testing.T) {
	testCases := []TestCase{
		{
			role: "USER",
			result: `
						{
							"aggregateLog": null
						}`,
		},
		{
			result: `
						{
							"aggregateLog": null
						}`,
		},
		{
			role: "ADMIN",
			result: `
						{
							"aggregateLog":
								{
									"count": 2,
									"randomMin": "test",
									"randomMax": "test"
								}
						}`,
		},
	}

	query := `
		query {
			aggregateLog {
		    	count
				randomMin
				randomMax
		    }
		}
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			params := &common.GraphQLParams{
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := params.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
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
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

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
            },
			{
				"name": "Region6"
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
            },
			{
				"name": "Region6"
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
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestAuthPaginationWithCascade(t *testing.T) {
	testCases := []TestCase{{
		name: "Auth query with @cascade and pagination at top level",
		user: "user1",
		role: "ADMIN",
		query: `
		query {	
			queryMovie (order: {asc: content}, first: 2, offset: 0) @cascade{
				content
				code
				regionsAvailable (order: {asc: name}){
					name
				}
			}
		}
`,
		result: `
		{
			"queryMovie": [
			  {
				"content": "Movie3",
				"code": "m3",
				"regionsAvailable": [
				  {
					"name": "Region1"
				  },
				  {
					"name": "Region4"
				  },
				  {
					"name": "Region6"
				  }
				]
			  },
			  {
				"content": "Movie4",
				"code": "m4",
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
		name: "Auth query with @cascade and pagination at deep level",
		user: "user1",
		role: "ADMIN",
		query: `
query {	
	queryMovie (order: {asc: content}, first: 2, offset: 1) {
		content
		regionsAvailable (order: {asc: name}, first: 1) @cascade{
			name
			global
		}
	}
}
`,
		result: `
		{
			"queryMovie": [
			  {
				"content": "Movie3",
				"regionsAvailable": [
				  {
					"name": "Region6",
					"global": true
				  }
				]
			  },
			  {
				"content": "Movie4",
				"regionsAvailable": [
				  {
					"name": "Region5",
					"global": true
				  }
				]
			  }
			]
		  }
		`,
	}}

	for _, tcase := range testCases {
		t.Run(tcase.name, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   tcase.query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
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
			Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:   query,
			Variables: map[string]interface{}{
				"filter": tcase.filter,
			},
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
		common.RequireNoGQLErrors(t, gqlResponse)

		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	}
}

func AddDeleteAuthTestData(t *testing.T) {
	client, err := testutil.DgraphClient(common.Alpha1gRPC)
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
	client, err := testutil.DgraphClient(common.Alpha1gRPC)
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
			Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:   query,
			Variables: map[string]interface{}{
				"filter": tcase.filter,
			},
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
		common.RequireNoGQLErrors(t, gqlResponse)

		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestDeepRBACValueCascade(t *testing.T) {
	testCases := []TestCase{
		{
			user: "user1",
			role: "USER",
			query: `
			query {
			  queryUser (filter:{username:{eq:"user1"}}) @cascade {
				username
				issues {
				  msg
				}
			  }
			}`,
			result: `{"queryUser": []}`,
		},
		{
			user: "user1",
			role: "USER",
			query: `
			query {
			  queryUser (filter:{username:{eq:"user1"}}) {
				username
				issues @cascade {
				  msg
				}
			  }
			}`,
			result: `{"queryUser": [{"username": "user1", "issues":[]}]}`,
		},
		{
			user: "user1",
			role: "ADMIN",
			query: `
			query {
			  queryUser (filter:{username:{eq:"user1"}}) @cascade {
				username
				issues {
				  msg
				}
			  }
			}`,
			result: `{"queryUser":[{"username":"user1","issues":[{"msg":"Issue1"}]}]}`,
		},
	}

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   tcase.query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestMain(m *testing.M) {
	schema, data := common.BootstrapAuthData()
	jwtAlgo := []string{jwt.SigningMethodHS256.Name, jwt.SigningMethodRS256.Name}
	for _, algo := range jwtAlgo {
		authSchema, err := testutil.AppendAuthInfo(schema, algo, "./sample_public_key.pem", false)
		if err != nil {
			panic(err)
		}

		authMeta, err := authorization.Parse(string(authSchema))
		if err != nil {
			panic(err)
		}

		metaInfo = &testutil.AuthMeta{
			PublicKey:      authMeta.VerificationKey,
			Namespace:      authMeta.Namespace,
			Algo:           authMeta.Algo,
			Header:         authMeta.Header,
			PrivateKeyPath: "./sample_private_key.pem",
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

func TestChildAggregateQueryWithDeepRBAC(t *testing.T) {
	testCases := []TestCase{
		{
			user: "user1",
			role: "USER",
			result: `{
						"queryUser":
							[
								{
									"username": "user1",
									"issuesAggregate": {
										"count": null,
										"msgMax": null,
										"msgMin": null
									}
								}
							]
					}`},
		{
			user: "user1",
			role: "ADMIN",
			result: `{
						"queryUser":
							[
								{
									"username":"user1",
									"issuesAggregate":
										{
											"count":1,
											"msgMax": "Issue1",
											"msgMin": "Issue1"
										}
								}
							]
					}`},
	}

	query := `
	query {
	  queryUser (filter:{username:{eq:"user1"}}) {
		username
		issuesAggregate {
		  count
		  msgMax
		  msgMin
		}
	  }
	}
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
		})
	}
}

func TestChildAggregateQueryWithOtherFields(t *testing.T) {
	testCases := []TestCase{
		{
			user: "user1",
			role: "USER",
			result: `{
						"queryUser":
							[
								{
									"username": "user1",
									"issues":[],
									"issuesAggregate": {
										"count": null,
										"msgMin": null,
										"msgMax": null
									}
								}
							]
					}`},
		{
			user: "user1",
			role: "ADMIN",
			result: `{
						"queryUser":
							[
								{
									"username":"user1",
									"issues":
										[
											{
												"msg":"Issue1"
											}
										],
									"issuesAggregate":
										{
											"count": 1,
											"msgMin": "Issue1",
											"msgMax": "Issue1"
										}
								}
							]
					}`},
	}

	query := `
	query {
	  queryUser (filter:{username:{eq:"user1"}}) {
		username
		issuesAggregate {
		  count
		  msgMin
		  msgMax
		}
		issues {
		  msg
		}
	  }
	}
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   query,
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
		})
	}
}

func checkLogPassword(t *testing.T, logID, pwd, role string) *common.GraphQLResponse {
	// Check Log Password for given logID, pwd, role
	checkLogParamsFalse := &common.GraphQLParams{
		Headers: common.GetJWT(t, "SomeUser", role, metaInfo),
		Query: `query checkLogPassword($name: ID!, $pwd: String!) {
			checkLogPassword(id: $name, pwd: $pwd) { id }
		}`,
		Variables: map[string]interface{}{
			"name": logID,
			"pwd":  pwd,
		},
	}

	gqlResponse := checkLogParamsFalse.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
	return gqlResponse
}

func deleteLog(t *testing.T, logID string) {
	deleteLogParams := &common.GraphQLParams{
		Query: `
		mutation DelLog($logID: ID!) {
		  deleteLog(filter:{id:[$logID]}) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"logID": logID},
		Headers:   common.GetJWT(t, "SomeUser", "ADMIN", metaInfo),
	}
	gqlResponse := deleteLogParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func deleteUser(t *testing.T, username string) {
	deleteUserParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, username, "ADMIN", metaInfo),
		Query: `
		mutation DelUser($username: String!) {
		  deleteUser(filter:{username: {eq: $username } } ) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"username": username},
	}
	gqlResponse := deleteUserParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func TestAuthWithSecretDirective(t *testing.T) {

	// Check that no auth rule is applied to checkUserPassword query.
	newUser := &common.User{
		Username: "Test User",
		Password: "password",
		IsPublic: true,
	}

	addUserParams := &common.GraphQLParams{
		Query: `mutation addUser($user: [AddUserInput!]!) {
			addUser(input: $user) {
				user {
					username
				}
			}
		}`,
		Variables: map[string]interface{}{"user": []*common.User{newUser}},
	}

	gqlResponse := addUserParams.ExecuteAsPost(t, common.GraphqlURL)
	require.Equal(t, `{"addUser":{"user":[{"username":"Test User"}]}}`,
		string(gqlResponse.Data))

	checkUserParams := &common.GraphQLParams{
		Query: `query checkUserPassword($name: String!, $pwd: String!) {
					checkUserPassword(username: $name, password: $pwd) { 
						username
						isPublic
					}
				}`,
		Variables: map[string]interface{}{
			"name": newUser.Username,
			"pwd":  newUser.Password,
		},
	}

	gqlResponse = checkUserParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	var result struct {
		CheckUserPassword *common.User `json:"checkUserPassword,omitempty"`
	}

	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.Nil(t, err)

	opt := cmpopts.IgnoreFields(common.User{}, "Password")
	if diff := cmp.Diff(newUser, result.CheckUserPassword, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
	deleteUser(t, newUser.Username)

	// Check that checkLogPassword works with RBAC rule
	newLog := &Log{
		Pwd: "password",
	}

	addLogParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, "Random", "ADMIN", metaInfo),
		Query: `mutation addLog($log: [AddLogInput!]!) {
			addLog(input: $log) {
				log {
					id
				}
			}
		}`,
		Variables: map[string]interface{}{"log": []*Log{newLog}},
	}

	gqlResponse = addLogParams.ExecuteAsPost(t, common.GraphqlURL)
	var addLogResult struct {
		AddLog struct {
			Log []*Log
		}
	}

	err = json.Unmarshal([]byte(gqlResponse.Data), &addLogResult)
	require.Nil(t, err)
	// Id of the created log
	logID := addLogResult.AddLog.Log[0].Id

	// checkLogPassword with RBAC rule true should work
	gqlResponse = checkLogPassword(t, logID, newLog.Pwd, "Admin")
	var resultLog struct {
		CheckLogPassword *Log `json:"checkLogPassword,omitempty"`
	}

	err = json.Unmarshal([]byte(gqlResponse.Data), &resultLog)
	require.Nil(t, err)

	require.Equal(t, resultLog.CheckLogPassword.Id, logID)

	// checkLogPassword with RBAC rule false should not work
	gqlResponse = checkLogPassword(t, logID, newLog.Pwd, "USER")
	require.JSONEq(t, `{"checkLogPassword": null}`, string(gqlResponse.Data))
	deleteLog(t, logID)
}

func TestAuthRBACEvaluation(t *testing.T) {
	query := `query {
			  queryBook{
				bookId
				name
				desc
			  }
			}`
	tcs := []struct {
		name   string
		header http.Header
	}{
		{
			name:   "Test Auth Eq Filter With Object As Token Val",
			header: common.GetJWT(t, map[string]interface{}{"a": "b"}, nil, metaInfo),
		},
		{
			name:   "Test Auth Eq Filter With Float Token Val",
			header: common.GetJWT(t, 123.12, nil, metaInfo),
		},
		{
			name:   "Test Auth Eq Filter With Int64 Token Val",
			header: common.GetJWT(t, 1237890123456, nil, metaInfo),
		},
		{
			name:   "Test Auth Eq Filter With Int Token Val",
			header: common.GetJWT(t, 1234, nil, metaInfo),
		},
		{
			name:   "Test Auth Eq Filter With Bool Token Val",
			header: common.GetJWT(t, true, nil, metaInfo),
		},
		{
			name:   "Test Auth In Filter With Object As Token Val",
			header: common.GetJWT(t, map[string]interface{}{"e": "f"}, nil, metaInfo),
		},
		{
			name:   "Test Auth In Filter With Float Token Val",
			header: common.GetJWT(t, 312.124, nil, metaInfo),
		},
		{
			name:   "Test Auth In Filter With Int64 Token Val",
			header: common.GetJWT(t, 1246879976444232435, nil, metaInfo),
		},
		{
			name:   "Test Auth In Filter With Int Token Val",
			header: common.GetJWT(t, 6872, nil, metaInfo),
		},
		{
			name:   "Test Auth Eq Filter From Token With Array Val",
			header: common.GetJWT(t, []int{456, 1234}, nil, metaInfo),
		},
		{
			name:   "Test Auth In Filter From Token With Array Val",
			header: common.GetJWT(t, []int{124324, 6872}, nil, metaInfo),
		},
		{
			name:   "Test Auth Regex Filter",
			header: common.GetJWT(t, "xyz@dgraph.io", nil, metaInfo),
		},
		{
			name:   "Test Auth Regex Filter From Token With Array Val",
			header: common.GetJWT(t, []string{"abc@def.com", "xyz@dgraph.io"}, nil, metaInfo),
		},
	}
	bookResponse := `{"queryBook":[{"bookId":"book1","name":"Introduction","desc":"Intro book"}]}`
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			queryParams := &common.GraphQLParams{
				Headers: tc.header,
				Query:   query,
			}

			gqlResponse := queryParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, string(gqlResponse.Data), bookResponse)
		})

	}
}

func TestFragmentInAuthRulesWithUserDefinedCascade(t *testing.T) {
	addHomeParams := &common.GraphQLParams{
		Query: `mutation {
			addHome(input: [
				{address: "Home1", members: [{dogRef: {breed: "German Shepherd", eats: [{plantRef: {breed: "Crop"}}]}}]},
				{address: "Home2", members: [{parrotRef: {repeatsWords: ["Hi", "Morning!"]}}]},
				{address: "Home3", members: [{plantRef: {breed: "Flower"}}]},
				{address: "Home4", members: [{dogRef: {breed: "Bulldog"}}]}
			]) {
				numUids
			}
		}`,
	}
	gqlResponse := addHomeParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	queryHomeParams := &common.GraphQLParams{
		Query: `query {
			queryHome {
				address
			}
		}`,
		Headers: common.GetJWT(t, "", "", metaInfo),
	}
	gqlResponse = queryHomeParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	// we should get back only Home1 and Home3
	testutil.CompareJSON(t, `{"queryHome": [
		{"address": "Home1"},
		{"address": "Home3"}
	]}`, string(gqlResponse.Data))

	// cleanup
	common.DeleteGqlType(t, "Home", map[string]interface{}{}, 4, nil)
	common.DeleteGqlType(t, "Dog", map[string]interface{}{}, 2, nil)
	common.DeleteGqlType(t, "Parrot", map[string]interface{}{}, 1, nil)
	common.DeleteGqlType(t, "Plant", map[string]interface{}{}, 2, nil)
}
