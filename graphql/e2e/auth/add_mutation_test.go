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

func (p *Project) delete(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
			mutation deleteProject($ids: [ID!]) {
				deleteProject(filter:{projID:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": []string{p.ProjID}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (c *Column) delete(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
			mutation deleteColumn($colids: [ID!]) {
				deleteColumn(filter:{colID:$colids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"colids": []string{c.ColID}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (i *Issue) delete(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
			mutation deleteIssue($ids: [ID!]) {
				deleteIssue(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": []string{i.Id}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (l *Log) delete(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
			mutation deleteLog($ids: [ID!]) {
				deleteLog(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": []string{l.Id}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (m *Movie) delete(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
			mutation deleteMovie($ids: [ID!]) {
				deleteMovie(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": []string{m.Id}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (a *Author) delete(t *testing.T) {
	getParams := &common.GraphQLParams{
		Query: `
			mutation deleteAuthor($ids: [ID!]) {
				deleteAuthor(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": []string{a.Id}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (q *Question) delete(t *testing.T, user string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWTForInterfaceAuth(t, user, "", q.Answered, metaInfo),
		Query: `
			mutation deleteQuestion($ids: [ID!]) {
				deleteQuestion(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": []string{q.Id}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (f *FbPost) delete(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
			mutation deleteFbPost($ids: [ID!]) {
				deleteFbPost(filter:{id:$ids}) {
					msg
				}
			}
		`,
		Variables: map[string]interface{}{"ids": []string{f.Id}},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func TestAuth_AddOnTypeWithRBACRuleOnInterface(t *testing.T) {
	testCases := []TestCase{{
		user: "user1@dgraph.io",
		role: "ADMIN",
		variables: map[string]interface{}{"fbpost": &FbPost{
			Text: "New FbPost",
			Pwd:  "password",
			Author: &Author{
				Name: "user1@dgraph.io",
			},
			Sender: &Author{
				Name: "user1@dgraph.io",
			},
			Receiver: &Author{
				Name: "user2@dgraph.io",
			},
			PostCount: 5,
		}},
		expectedError: false,
		result:        `{"addFbPost":{"fbPost":[{"id":"0x15f","text":"New FbPost","author":{"id":"0x15e","name":"user1@dgraph.io"},"sender":{"id":"0x15d","name":"user1@dgraph.io"},"receiver":{"id":"0x160","name":"user2@dgraph.io"}}]}}`,
	}, {
		user: "user1@dgraph.io",
		role: "USER",
		variables: map[string]interface{}{"fbpost": &FbPost{
			Text: "New FbPost",
			Pwd:  "password",
			Author: &Author{
				Name: "user1@dgraph.io",
			},
			Sender: &Author{
				Name: "user1@dgraph.io",
			},
			Receiver: &Author{
				Name: "user2@dgraph.io",
			},
			PostCount: 5,
		}},
		expectedError: true,
	},
	}

	query := `
		mutation addFbPost($fbpost: AddFbPostInput!) {
			addFbPost(input: [$fbpost]) {
				fbPost {
					id
					text
					author {
						id
						name
					}
					sender {
						id
						name
					}
					receiver {
						id
						name
					}
				}
			}
		}
	`

	var expected, result struct {
		AddFbPost struct {
			FbPost []*FbPost
		}
	}

	for _, tcase := range testCases {
		params := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := params.ExecuteAsPost(t, graphqlURL)
		if tcase.expectedError {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)

		err = json.Unmarshal(gqlResponse.Data, &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(FbPost{}, "Id")
		opt1 := cmpopts.IgnoreFields(Author{}, "Id")
		if diff := cmp.Diff(expected, result, opt, opt1); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddFbPost.FbPost {
			i.Author.delete(t)
			i.Sender.delete(t)
			i.Receiver.delete(t)
			i.delete(t, tcase.user, tcase.role)
		}
	}
}

func TestAuth_AddOnTypeWithGraphTraversalRuleOnInterface(t *testing.T) {
	testCases := []TestCase{{
		user: "user1@dgraph.io",
		ans:  true,
		variables: map[string]interface{}{"question": &Question{
			Text: "A Question",
			Pwd:  "password",
			Author: &Author{
				Name: "user1@dgraph.io",
			},
			Answered: true,
		}},
		result: `{"addQuestion": {"question": [{"id": "0x123", "text": "A Question", "author": {"id": "0x124", "name": "user1@dgraph.io"}}]}}`,
	}, {
		user: "user1",
		ans:  false,
		variables: map[string]interface{}{"question": &Question{
			Text: "A Question",
			Pwd:  "password",
			Author: &Author{
				Name: "user1",
			},
			Answered: true,
		}},
		expectedError: true,
	},
		{
			user: "user2",
			ans:  true,
			variables: map[string]interface{}{"question": &Question{
				Text: "A Question",
				Pwd:  "password",
				Author: &Author{
					Name: "user1",
				},
				Answered: true,
			}},
			expectedError: true,
		},
	}

	query := `
		mutation addQuestion($question: AddQuestionInput!) {
			addQuestion(input: [$question]) {
				question {
					id
					text
					author {
						id
						name
					}
				}
			}
		}
	`
	var expected, result struct {
		AddQuestion struct {
			Question []*Question
		}
	}

	for _, tcase := range testCases {
		params := &common.GraphQLParams{
			Headers:   common.GetJWTForInterfaceAuth(t, tcase.user, tcase.role, tcase.ans, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := params.ExecuteAsPost(t, graphqlURL)
		if tcase.expectedError {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)

		err = json.Unmarshal(gqlResponse.Data, &result)
		require.NoError(t, err)
		opt := cmpopts.IgnoreFields(Question{}, "Id")
		opt1 := cmpopts.IgnoreFields(Author{}, "Id")
		if diff := cmp.Diff(expected, result, opt, opt1); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddQuestion.Question {
			i.Author.delete(t)
			i.delete(t, tcase.user)
		}
	}
}

func TestAddDeepFilter(t *testing.T) {
	// Column can only be added if the user has ADMIN role attached to the corresponding project.
	testCases := []TestCase{{
		// Test case fails as there are no roles.
		user:   "user6",
		role:   "ADMIN",
		result: ``,
		variables: map[string]interface{}{"column": &Column{
			Name: "column_add_1",
			InProject: &Project{
				Name: "project_add_1",
				Pwd:  "password1",
			},
		}},
	}, {
		// Test case fails as the role isn't assigned to the correct user.
		user:   "user6",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"column": &Column{
			Name: "column_add_2",
			InProject: &Project{
				Name: "project_add_2",
				Pwd:  "password2",
				Roles: []*Role{{
					Permission: "ADMIN",
					AssignedTo: []*common.User{{
						Username: "user2",
						Password: "password",
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
				Pwd:  "password4",
				Roles: []*Role{{
					Permission: "ADMIN",
					AssignedTo: []*common.User{{
						Username: "user6",
						Password: "password",
					}},
				}, {
					Permission: "VIEW",
					AssignedTo: []*common.User{{
						Username: "user6",
						Password: "password",
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

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal(gqlResponse.Data, &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Column{}, "ColID")
		opt1 := cmpopts.IgnoreFields(Project{}, "ProjID")
		if diff := cmp.Diff(expected, result, opt, opt1); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddColumn.Column {
			i.InProject.delete(t, tcase.user, tcase.role)
			i.delete(t, tcase.user, tcase.role)
		}
	}
}

func TestAddOrRBACFilter(t *testing.T) {
	// Column can only be added if the user has ADMIN role attached to the
	// corresponding project or if the user is ADMIN.

	testCases := []TestCase{{
		// Test case passses as user is ADMIN.
		user:   "user7",
		role:   "ADMIN",
		result: `{"addProject": {"project":[{"name":"project_add_1"}]}}`,
		variables: map[string]interface{}{"project": &Project{
			Name: "project_add_1",
			Pwd:  "password1",
		}},
	}, {
		// Test case fails as the role isn't assigned to the correct user
		user:   "user7",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"project": &Project{
			Name: "project_add_2",
			Pwd:  "password2",
			Roles: []*Role{{
				Permission: "ADMIN",
				AssignedTo: []*common.User{{
					Username: "user2",
					Password: "password",
				}},
			}},
		}},
	}, {
		user:   "user7",
		role:   "USER",
		result: `{"addProject": {"project":[{"name":"project_add_3"}]}}`,
		variables: map[string]interface{}{"project": &Project{
			Name: "project_add_3",
			Pwd:  "password3",
			Roles: []*Role{{
				Permission: "ADMIN",
				AssignedTo: []*common.User{{
					Username: "user7",
					Password: "password",
				}},
			}, {
				Permission: "VIEW",
				AssignedTo: []*common.User{{
					Username: "user7",
					Password: "password",
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

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal(gqlResponse.Data, &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Project{}, "ProjID")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddProject.Project {
			i.delete(t, tcase.user, tcase.role)
		}
	}
}

func TestAddAndRBACFilterMultiple(t *testing.T) {
	testCases := []TestCase{{
		user:   "user8",
		role:   "ADMIN",
		result: `{"addIssue": {"issue":[{"msg":"issue_add_5"}, {"msg":"issue_add_6"}, {"msg":"issue_add_7"}]}}`,
		variables: map[string]interface{}{"issues": []*Issue{{
			Msg:   "issue_add_5",
			Owner: &common.User{Username: "user8"},
		}, {
			Msg:   "issue_add_6",
			Owner: &common.User{Username: "user8"},
		}, {
			Msg:   "issue_add_7",
			Owner: &common.User{Username: "user8"},
		}}},
	}, {
		user:   "user8",
		role:   "ADMIN",
		result: ``,
		variables: map[string]interface{}{"issues": []*Issue{{
			Msg:   "issue_add_8",
			Owner: &common.User{Username: "user8"},
		}, {
			Msg:   "issue_add_9",
			Owner: &common.User{Username: "user8"},
		}, {
			Msg:   "issue_add_10",
			Owner: &common.User{Username: "user9"},
		}}},
	}}

	query := `
		mutation addIssue($issues: [AddIssueInput!]!) {
			addIssue(input: $issues) {
				issue (order: {asc: msg}) {
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

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal(gqlResponse.Data, &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Issue{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddIssue.Issue {
			i.delete(t, tcase.user, tcase.role)
		}
	}
}

func TestAddAndRBACFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user7",
		role:   "ADMIN",
		result: `{"addIssue": {"issue":[{"msg":"issue_add_1"}]}}`,
		variables: map[string]interface{}{"issue": &Issue{
			Msg:   "issue_add_1",
			Owner: &common.User{Username: "user7"},
		}},
	}, {
		user:   "user7",
		role:   "ADMIN",
		result: ``,
		variables: map[string]interface{}{"issue": &Issue{
			Msg:   "issue_add_2",
			Owner: &common.User{Username: "user8"},
		}},
	}, {
		user:   "user7",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"issue": &Issue{
			Msg:   "issue_add_3",
			Owner: &common.User{Username: "user7"},
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

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal(gqlResponse.Data, &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Issue{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddIssue.Issue {
			i.delete(t, tcase.user, tcase.role)
		}
	}
}

func TestAddComplexFilter(t *testing.T) {
	// To add a movie, it should be not hidden and either global or the user should be in the region
	testCases := []TestCase{{
		// Test case fails as the movie is hidden
		user:   "user8",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"movie": &Movie{
			Content: "add_movie_1",
			Hidden:  true,
		}},
	}, {
		// Test case fails as the movie is not global and the user isn't in the region
		user:   "user8",
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
		// Test case passes as the movie is global
		user:   "user8",
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
		// Test case passes as the user is in the region
		user:   "user8",
		role:   "USER",
		result: `{"addMovie": {"movie": [{"content": "add_movie_4"}]}}`,
		variables: map[string]interface{}{"movie": &Movie{
			Content: "add_movie_4",
			Hidden:  false,
			RegionsAvailable: []*Region{{
				Name:   "add_region_2",
				Global: false,
				Users: []*common.User{{
					Username: "user8",
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

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal(gqlResponse.Data, &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Movie{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddMovie.Movie {
			i.delete(t, tcase.user, tcase.role)
		}
	}
}

func TestAddRBACFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		role:   "ADMIN",
		result: `{"addLog": {"log":[{"logs":"log_add_1"}]}}`,
		variables: map[string]interface{}{"issue": &Log{
			Logs: "log_add_1",
			Pwd:  "password1",
		}},
	}, {
		user:   "user1",
		role:   "USER",
		result: ``,
		variables: map[string]interface{}{"issue": &Log{
			Logs: "log_add_2",
			Pwd:  "password2",
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

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal(gqlResponse.Data, &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(Log{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddLog.Log {
			i.delete(t, tcase.user, tcase.role)
		}
	}
}

func TestAddGQLOnly(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		result: `{"addUserSecret":{"usersecret":[{"aSecret":"secret1"}]}}`,
		variables: map[string]interface{}{"user": &common.UserSecret{
			ASecret: "secret1",
			OwnedBy: "user1",
		}},
	}, {
		user:   "user2",
		result: ``,
		variables: map[string]interface{}{"user": &common.UserSecret{
			ASecret: "secret2",
			OwnedBy: "user1",
		}},
	}}

	query := `
		mutation addUser($user: AddUserSecretInput!) {
			addUserSecret(input: [$user]) {
				userSecret {
					aSecret
				}
			}
		}
	`
	var expected, result struct {
		AddUserSecret struct {
			UserSecret []*common.UserSecret
		}
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}

		gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 1)
			require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")
			continue
		}

		require.Nil(t, gqlResponse.Errors)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal(gqlResponse.Data, &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(common.UserSecret{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddUserSecret.UserSecret {
			i.Delete(t, tcase.user, tcase.role, metaInfo)
		}
	}
}
