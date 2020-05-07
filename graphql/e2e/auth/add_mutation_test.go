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
	"testing"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
)

func TestAddDeepFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user6",
		role:   "ADMIN",
		result: ``,
		variables: map[string]interface{}{"column": &Column{
			Name: "column_add_1",
			InProject: &Project{
				Name: "project_add_1",
			},
		}},
	}, {
		user:   "user6",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"column": &Column{
			Name: "column_add_2",
			InProject: &Project{
				Name: "project_add_2",
				Roles: []*Role{{
					Permission: "ADMIN",
					AssignedTo: []*User{{
						Username: "user2",
					}},
				}},
			},
		}},
	}, {
		user:   "user6",
		role:   "USER",
		result: `{"addColumn":{"column":[{"name":"column_add_3","inProject":{"name":"project_add_4"}}]}}`,
		variables: map[string]interface{}{"column": &Column{
			Name: "column_add_3",
			InProject: &Project{
				Name: "project_add_4",
				Roles: []*Role{{
					Permission: "ADMIN",
					AssignedTo: []*User{{
						Username: "user6",
					}},
				}, {
					Permission: "VIEW",
					AssignedTo: []*User{{
						Username: "user6",
					}},
				}},
			},
		}},
	}}

	query := `
		mutation addColumn($column: AddColumnInput!) {
			addColumn(input: [$column]) {
				column {
			             name
				     inProject {
				           projID
				           name
				     }
				}
			}
		}
	`

	var expected, result struct {
		AddColumn struct {
			Column []*Column
		}
	}

	var colids []string
	var projids []string
	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   getJWT(t, tcase.user, tcase.role),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Equal(t, gqlResponse.Errors[0].Error(),
				"mutation failed because authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal([]byte(gqlResponse.Data), &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Column{}, "ColID")
		opt1 := cmpopts.IgnoreFields(Project{}, "ProjID")
		if diff := cmp.Diff(expected, result, opt, opt1); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddColumn.Column {
			colids = append(colids, i.ColID)
			projids = append(projids, i.InProject.ProjID)
		}
	}

	getParams := &common.GraphQLParams{
		Headers: getJWT(t, "user1", "admin"),
		Query: `
			mutation deleteColumn($colids: [ID!]) {
				deleteColumn(filter:{colID:$colids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"colids": colids},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	getParams = &common.GraphQLParams{
		Headers: getJWT(t, "user1", "admin"),
		Query: `
			mutation deleteProject($ids: [ID!]) {
				deleteProject(filter:{projID:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": projids},
	}
	gqlResponse = getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func TestAddOrRBACFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		role:   "ADMIN",
		result: `{"addProject": {"project":[{"name":"project_add_1"}]}}`,
		variables: map[string]interface{}{"project": &Project{
			Name: "project_add_1",
		}},
	}, {
		user:   "user1",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"project": &Project{
			Name: "project_add_2",
			Roles: []*Role{{
				Permission: "ADMIN",
				AssignedTo: []*User{{
					Username: "user2",
				}},
			}},
		}},
	}, {
		user:   "user1",
		role:   "USER",
		result: `{"addProject": {"project":[{"name":"project_add_3"}]}}`,
		variables: map[string]interface{}{"project": &Project{
			Name: "project_add_3",
			Roles: []*Role{{
				Permission: "ADMIN",
				AssignedTo: []*User{{
					Username: "user1",
				}},
			}, {
				Permission: "VIEW",
				AssignedTo: []*User{{
					Username: "user1",
				}},
			}},
		}},
	}}

	query := `
		mutation addProject($project: AddProjectInput!) {
			addProject(input: [$project]) {
				project {
				      projID
				      name
				}
			}
		}
	`

	var expected, result struct {
		AddProject struct {
			Project []*Project
		}
	}

	var ids []string
	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   getJWT(t, tcase.user, tcase.role),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Equal(t, gqlResponse.Errors[0].Error(),
				"mutation failed because authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal([]byte(gqlResponse.Data), &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Project{}, "ProjID")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddProject.Project {
			ids = append(ids, i.ProjID)
		}
	}

	getParams := &common.GraphQLParams{
		Headers: getJWT(t, "user1", "admin"),
		Query: `
			mutation deleteProject($ids: [ID!]) {
				deleteProject(filter:{projID:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": ids},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func TestAddAndRBACFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		role:   "ADMIN",
		result: `{"addIssue": {"issue":[{"msg":"issue_add_1"}]}}`,
		variables: map[string]interface{}{"issue": &Issue{
			Msg:   "issue_add_1",
			Owner: &User{Username: "user1"},
		}},
	}, {
		user:   "user1",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"issue": &Issue{
			Msg:   "issue_add_2",
			Owner: &User{Username: "user1"},
		}},
	}}

	query := `
		mutation addIssue($issue: AddIssueInput!) {
			addIssue(input: [$issue]) {
				issue {
				      id
				      msg
				}
			}
		}
	`
	var expected, result struct {
		AddIssue struct {
			Issue []*Issue
		}
	}

	var ids []string
	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   getJWT(t, tcase.user, tcase.role),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Equal(t, gqlResponse.Errors[0].Error(),
				"mutation failed because authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal([]byte(gqlResponse.Data), &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Issue{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddIssue.Issue {
			ids = append(ids, i.Id)
		}
	}

	getParams := &common.GraphQLParams{
		Headers: getJWT(t, "user1", "admin"),
		Query: `
			mutation deleteIssue($ids: [ID!]) {
				deleteIssue(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": ids},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func TestAddComplexFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"movie": &Movie{
			Content: "add_movie_1",
			Hidden:  true,
		}},
	}, {
		user:   "user1",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"movie": &Movie{
			Content: "add_movie_2",
			Hidden:  false,
			RegionsAvailable: []*Region{{
				Name:   "add_region_1",
				Global: false,
			}},
		}},
	}, {
		user:   "user1",
		role:   "USER",
		result: `{"addMovie": {"movie": [{"content": "add_movie_3"}]}}`,
		variables: map[string]interface{}{"movie": &Movie{
			Content: "add_movie_3",
			Hidden:  false,
			RegionsAvailable: []*Region{{
				Name:   "add_region_1",
				Global: true,
			}},
		}},
	}, {
		user:   "user1",
		role:   "USER",
		result: `{"addMovie": {"movie": [{"content": "add_movie_4"}]}}`,
		variables: map[string]interface{}{"movie": &Movie{
			Content: "add_movie_4",
			Hidden:  false,
			RegionsAvailable: []*Region{{
				Name:   "add_region_2",
				Global: false,
				Users: []*User{{
					Username: "user1",
				}},
			}},
		}},
	}}

	query := `
		mutation addMovie($movie: AddMovieInput!) {
			addMovie(input: [$movie]) {
				movie {
					id
					content
				}
			}
		}
	`

	var expected, result struct {
		AddMovie struct {
			Movie []*Movie
		}
	}

	var ids []string
	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   getJWT(t, tcase.user, tcase.role),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Equal(t, gqlResponse.Errors[0].Error(),
				"mutation failed because authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal([]byte(gqlResponse.Data), &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Movie{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddMovie.Movie {
			ids = append(ids, i.Id)
		}
	}

	getParams := &common.GraphQLParams{
		Headers: getJWT(t, "user1", "admin"),
		Query: `
			mutation deleteMovie($ids: [ID!]) {
				deleteMovie(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": ids},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func TestAddRBACFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		role:   "ADMIN",
		result: `{"addLog": {"log":[{"logs":"log_add_1"}]}}`,
		variables: map[string]interface{}{"issue": &Log{
			Logs: "log_add_1",
		}},
	}, {
		user:   "user1",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"issue": &Log{
			Logs: "log_add_2",
		}},
	}}

	query := `
		mutation addLog($issue: AddLogInput!) {
			addLog(input: [$issue]) {
				log {
				      id
				      logs
				}
			}
		}
	`

	var expected, result struct {
		AddLog struct {
			Log []*Log
		}
	}

	var ids []string
	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   getJWT(t, tcase.user, tcase.role),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Equal(t, gqlResponse.Errors[0].Error(),
				"mutation failed because authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal([]byte(gqlResponse.Data), &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Log{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddLog.Log {
			ids = append(ids, i.Id)
		}
	}

	getParams := &common.GraphQLParams{
		Headers: getJWT(t, "user1", "admin"),
		Query: `
			mutation deleteLog($ids: [ID!]) {
				deleteLog(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": ids},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func TestAddUserSecret(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		role:   "ADMIN",
		result: `{"addUserSecret":{"usersecret":[{"aSecret":"secret1"}]}}`,
		variables: map[string]interface{}{"user": &UserSecret{
			ASecret: "secret1",
			OwnedBy: "user1",
		}},
	},
		{
			user:   "user2",
			role:   "ADMIN",
			result: ``,
			variables: map[string]interface{}{"user": &UserSecret{
				ASecret: "secret2",
				OwnedBy: "user1",
			}},
		}}

	query := `
		mutation addUser($user: AddUserSecretInput!) {
			addUserSecret(input: [$user]) {
				usersecret {
					aSecret
				}
			}
		}
	`
	var expected, result struct {
		AddUserSecret struct {
			UserSecret []*UserSecret
		}
	}

	var ids []string
	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   getJWT(t, tcase.user, tcase.role),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Equal(t, gqlResponse.Errors[0].Error(),
				"mutation failed because authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal([]byte(gqlResponse.Data), &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(UserSecret{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddUserSecret.UserSecret {
			ids = append(ids, i.Id)
		}
	}

	getParams := &common.GraphQLParams{
		Headers: getJWT(t, "user1", "admin"),
		Query: `
			mutation deleteUserSecret($ids: [ID!]) {
				deleteUserSecret(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": ids},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}
