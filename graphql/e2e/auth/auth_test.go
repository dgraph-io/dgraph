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

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/testutil"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

const (
	graphqlURL = "http://localhost:8180/graphql"
)

var (
	metainfo = &authorization.AuthMeta{}
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

type Permission int

const (
	VIEW Permission = iota
	EDIT
	ADMIN
)

type Role struct {
	Id          uint64
	Permissions []Permission
	AssignedTo  []User
}

type Ticket struct {
	Id         uint64
	OnColumn   Column
	Title      string
	AssignedTo []User
}

type Column struct {
	ColID     uint64
	InProject Project
	Name      string
	Tickets   []Ticket
}

type Project struct {
	ProjID  uint64
	Name    string
	Roles   []Role
	columns []Column
}

type TestCase struct {
	name   string
	user   string
	role   string
	result string
	filter map[string]interface{}
}

func TestOrRBACFilter(t *testing.T) {
	t.Skip()

	testCases := []TestCase{}
	query := `
                 queryProject (order: {asc: name}) {
			name
		}
	`

	var result, data struct {
		QueryProject []*Project
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			// Authorization: getJWT(t, tcase.user, tcase.role), // FIXME:
			Query: query,
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
	}
}

func rootGetFilter(t *testing.T, id uint64, user string) {
	testCases := []TestCase{}
	query := `
		getColumn(colID: {asc: name}) {
			name
		}
	`

	var result, data struct {
		QueryColumn []*Column
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			// Authorization: getJWT(t, tcase.user, tcase.role), // FIXME:
			Query: query,
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
	}

}

func TestRootFilter(t *testing.T) {
	t.Skip()

	testCases := []TestCase{}
	query := `
		queryColumn(order: {asc: name}) {
			colID
			name
		}
	`

	var result, data struct {
		QueryColumn []*Column
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			// Authorization: getJWT(t, tcase.user, tcase.role),  // FIXME:
			Query: query,
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

		if len(result.QueryColumn) > 0 {
			rootGetFilter(t, result.QueryColumn[0].ColID, tcase.user)
		}
	}
}

func TestRBACFilter(t *testing.T) {
	t.Skip()

	testCases := []TestCase{}
	query := `
                 queryLog (order: {asc: logs}) {
			logs
		}
	`

	var result, data struct {
		QueryLog []*Log
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			// Authorization: getJWT(t, tcase.user, tcase.role), // FIXME:
			Query: query,
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
	}
}

func TestDeepFilter(t *testing.T) {
	t.Skip()

	testCases := []TestCase{}
	query := `
                 queryProject (order: {asc: name}) {
			name
			roles {
				permissions
				assignedTo {
					username
				}
			}
			columns {
				name 
				tickets {
					title
					assignedTo  {
						username
					}
				}
			}
		}
	`

	var result, data struct {
		QueryProject []*Project
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			// Authorization: getJWT(t, tcase.user, tcase.role), // FIXME:
			Query: query,
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
	}
}

func TestAndRBACFilter(t *testing.T) {
	t.Skip()

	testCases := []TestCase{}
	query := `
                 queryIssue (order: {asc: msg}) {
			msg
			user {
				username
			}
		}
	`

	var result, data struct {
		QueryIssue []*Issue
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			// Authorization: getJWT(t, tcase.user, tcase.role), // FIXME:
			Query: query,
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
	}

}

func TestAndFilter(t *testing.T) {
	t.Skip()

	testCases := []TestCase{}
	query := `
                 queryMovie (order: {asc: content}) {
			content
			regionsAvailable (order: {asc: name}) {
				name
				users (order: {asc: username}) {
					username
				}
			}
		}
	`

	var result, data struct {
		QueryMovie []*Movie
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			// Authorization: getJWT(t, tcase.user, tcase.role), // FIXME:
			Query: query,
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
	}
}

func TestDeepFieldFilters(t *testing.T) {
	t.Skip()

	testCases := []TestCase{}
	query := `
                 queryProject (order: {asc: name}) {
			name
			roles {
				permissions
				assignedTo {
					username
					age
				}
			}
		}
	`

	var result, data struct {
		QueryProject []*Project
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			// Authorization: getJWT(t, tcase.user, tcase.role), // FIXME:
			Query: query,
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
	}
}

func TestFieldFilters(t *testing.T) {
	t.Skip()

	testCases := []TestCase{}
	query := `
                queryUser (order: {asc: username}) {
			username
			age
			isPublic
		}
	`

	var result, data struct {
		QueryUser []*User
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			// Authorization: getJWT(t, tcase.user, tcase.role), // FIXME:
			Query: query,
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
	}
}

func TestDeleteAuthRule(t *testing.T) {
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
			result: `{"deleteUserSecret":{"msg":"Deleted","numUids":0}}`,
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
			Headers: map[string][]string{},
			Query:   query,
			Variables: map[string]interface{}{
				"filter": tcase.filter,
			},
		}

		authMeta := testutil.AuthMeta{
			PublicKey: metainfo.PublicKey,
			Namespace: metainfo.Namespace,
			AuthVars: map[string]interface{}{
				"USER": tcase.user,
			},
		}
		jwtToken, err := authMeta.GetSignedToken()
		require.NoError(t, err)
		getUserParams.Headers.Add(metainfo.Header, jwtToken)

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		require.Nil(t, gqlResponse.Errors)

		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestDeleteDeepAuthRule(t *testing.T) {
	testCases := []TestCase{
		{
			name: "ticket with only view permission",
			user: "user3",
			filter: map[string]interface{}{
				"title": map[string]interface{}{
					"anyofterms": "Ticket2",
				},
			},
			result: `{"deleteTicket":{"msg":"Deleted","numUids":0}}`,
		},
		{
			name: "ticket with edit permission but not belonging to user",
			user: "user3",
			filter: map[string]interface{}{
				"title": map[string]interface{}{
					"anyofterms": "Ticket1",
				},
			},
			result: `{"deleteTicket":{"msg":"Deleted","numUids":0}}`,
		},
		{
			name: "ticket with edit permission",
			user: "user1",
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
			Headers: map[string][]string{},
			Query:   query,
			Variables: map[string]interface{}{
				"filter": tcase.filter,
			},
		}

		authMeta := testutil.AuthMeta{
			PublicKey: metainfo.PublicKey,
			Namespace: metainfo.Namespace,
			AuthVars: map[string]interface{}{
				"USER": tcase.user,
			},
		}
		jwtToken, err := authMeta.GetSignedToken()
		require.NoError(t, err)
		getUserParams.Headers.Add(metainfo.Header, jwtToken)

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		require.Nil(t, gqlResponse.Errors)

		if diff := cmp.Diff(tcase.result, string(gqlResponse.Data)); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	}
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
	metainfo.Parse(string(schema))

	jsonFile := "test_data.json"
	data, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		panic(errors.Wrapf(err, "Unable to read file %s.", jsonFile))
	}

	common.BootstrapServer(schema, data)

	os.Exit(m.Run())
}
