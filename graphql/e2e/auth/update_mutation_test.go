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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
)

func getAllProjects(t *testing.T, users, roles []string) []string {
	var result struct {
		QueryProject []*Project
	}

	getParams := &common.GraphQLParams{
		Query: `
			query queryProject {
				queryProject {
					projID
				}
			}
		`,
	}

	ids := make(map[string]struct{})
	for _, user := range users {
		for _, role := range roles {
			getParams.Headers = common.GetJWT(t, user, role, metaInfo)
			gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.NoError(t, err)

			for _, i := range result.QueryProject {
				ids[i.ProjID] = struct{}{}
			}
		}
	}

	var keys []string
	for key := range ids {
		keys = append(keys, key)
	}

	return keys
}

func getAllColumns(t *testing.T, users, roles []string) ([]*Column, []string) {
	ids := make(map[string]struct{})
	getParams := &common.GraphQLParams{
		Query: `
			query queryColumn {
				queryColumn {
					colID
					name
					inProject {
						projID
					}
					tickets {
						id
					}
				}
			}
		`,
	}

	var result struct {
		QueryColumn []*Column
	}
	var columns []*Column
	for _, user := range users {
		for _, role := range roles {
			getParams.Headers = common.GetJWT(t, user, role, metaInfo)
			gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			err := json.Unmarshal(gqlResponse.Data, &result)
			require.NoError(t, err)

			for _, i := range result.QueryColumn {
				if _, ok := ids[i.ColID]; ok {
					continue
				}
				ids[i.ColID] = struct{}{}
				i.ColID = ""
				columns = append(columns, i)
			}
		}
	}

	var keys []string
	for key := range ids {
		keys = append(keys, key)
	}

	return columns, keys
}

func getAllQuestions(t *testing.T, users []string, answers []bool) ([]*Question, []string) {
	ids := make(map[string]struct{})
	getParams := &common.GraphQLParams{
		Query: `
			query queryQuestion {
				queryQuestion {
					id
					text
					author {
						id
						name
					}
					answered
				}
			}
		`,
	}

	var result struct {
		QueryQuestion []*Question
	}
	var questions []*Question
	for _, user := range users {
		for _, ans := range answers {
			getParams.Headers = common.GetJWTForInterfaceAuth(t, user, "", ans, metaInfo)
			gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			err := json.Unmarshal(gqlResponse.Data, &result)
			require.NoError(t, err)

			for _, i := range result.QueryQuestion {
				if _, ok := ids[i.Id]; ok {
					continue
				}
				ids[i.Id] = struct{}{}
				i.Id = ""
				questions = append(questions, i)
			}
		}
	}

	var keys []string
	for key := range ids {
		keys = append(keys, key)
	}

	return questions, keys
}

func getAllPosts(t *testing.T, users []string, roles []string, answers []bool) ([]*Question, []*Answer, []*FbPost, []string) {
	Questions, getAllQuestionIds := getAllQuestions(t, users, answers)
	Answers, getAllAnswerIds := getAllAnswers(t, users)
	FbPosts, getAllFbPostIds := getAllFbPosts(t, users, roles)
	var postIds []string
	postIds = append(postIds, getAllQuestionIds...)
	postIds = append(postIds, getAllAnswerIds...)
	postIds = append(postIds, getAllFbPostIds...)
	return Questions, Answers, FbPosts, postIds

}

func getAllFbPosts(t *testing.T, users []string, roles []string) ([]*FbPost, []string) {
	ids := make(map[string]struct{})
	getParams := &common.GraphQLParams{
		Query: `
			query queryFbPost {
				queryFbPost {
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
					postCount
				}
			}
		`,
	}

	var result struct {
		QueryFbPost []*FbPost
	}
	var fbposts []*FbPost
	for _, user := range users {
		for _, role := range roles {
			getParams.Headers = common.GetJWT(t, user, role, metaInfo)
			gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			err := json.Unmarshal(gqlResponse.Data, &result)
			require.NoError(t, err)

			for _, i := range result.QueryFbPost {
				if _, ok := ids[i.Id]; ok {
					continue
				}
				ids[i.Id] = struct{}{}
				i.Id = ""
				fbposts = append(fbposts, i)
			}
		}
	}

	var keys []string
	for key := range ids {
		keys = append(keys, key)
	}

	return fbposts, keys
}

func getAllAnswers(t *testing.T, users []string) ([]*Answer, []string) {
	ids := make(map[string]struct{})
	getParams := &common.GraphQLParams{
		Query: `
			query Answer {
				queryAnswer {
					id
					text
					author {
						id
						name
					}
				}
			}
		`,
	}

	var result struct {
		QueryAnswer []*Answer
	}
	var answers []*Answer
	for _, user := range users {
		getParams.Headers = common.GetJWT(t, user, "", metaInfo)
		gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
		common.RequireNoGQLErrors(t, gqlResponse)

		err := json.Unmarshal(gqlResponse.Data, &result)
		require.NoError(t, err)

		for _, i := range result.QueryAnswer {
			if _, ok := ids[i.Id]; ok {
				continue
			}
			ids[i.Id] = struct{}{}
			i.Id = ""
			answers = append(answers, i)
		}
	}

	var keys []string
	for key := range ids {
		keys = append(keys, key)
	}

	return answers, keys
}

func getAllIssues(t *testing.T, users, roles []string) ([]*Issue, []string) {
	ids := make(map[string]struct{})
	getParams := &common.GraphQLParams{
		Query: `
			query queryIssue {
				queryIssue {
					id
					msg
					random
					owner {
						username
					}
				}
			}
		`,
	}

	var result struct {
		QueryIssue []*Issue
	}
	var issues []*Issue
	for _, user := range users {
		for _, role := range roles {
			getParams.Headers = common.GetJWT(t, user, role, metaInfo)
			gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			err := json.Unmarshal(gqlResponse.Data, &result)
			require.NoError(t, err)

			for _, i := range result.QueryIssue {
				if _, ok := ids[i.Id]; ok {
					continue
				}
				ids[i.Id] = struct{}{}
				i.Id = ""
				issues = append(issues, i)
			}
		}
	}

	var keys []string
	for key := range ids {
		keys = append(keys, key)
	}

	return issues, keys
}

func getAllMovies(t *testing.T, users, roles []string) ([]*Movie, []string) {
	ids := make(map[string]struct{})
	getParams := &common.GraphQLParams{
		Query: `
			query queryMovie {
				queryMovie {
					id
					content
					code
					hidden
					regionsAvailable {
						id
					}
				}
			}
		`,
	}

	var result struct {
		QueryMovie []*Movie
	}
	var movies []*Movie
	for _, user := range users {
		for _, role := range roles {
			getParams.Headers = common.GetJWT(t, user, role, metaInfo)
			gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			err := json.Unmarshal(gqlResponse.Data, &result)
			require.NoError(t, err)

			for _, i := range result.QueryMovie {
				if _, ok := ids[i.Id]; ok {
					continue
				}
				ids[i.Id] = struct{}{}
				i.Id = ""
				movies = append(movies, i)
			}
		}
	}

	var keys []string
	for key := range ids {
		keys = append(keys, key)
	}

	return movies, keys
}

func getAllLogs(t *testing.T, users, roles []string) ([]*Log, []string) {
	ids := make(map[string]struct{})
	getParams := &common.GraphQLParams{
		Query: `
			query queryLog {
				queryLog {
					id
					logs
					random
				}
			}
		`,
	}

	var result struct {
		QueryLog []*Log
	}
	var logs []*Log
	for _, user := range users {
		for _, role := range roles {
			getParams.Headers = common.GetJWT(t, user, role, metaInfo)
			gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			err := json.Unmarshal(gqlResponse.Data, &result)
			require.NoError(t, err)

			for _, i := range result.QueryLog {
				if _, ok := ids[i.Id]; ok {
					continue
				}
				ids[i.Id] = struct{}{}
				i.Id = ""
				logs = append(logs, i)
			}
		}
	}

	var keys []string
	for key := range ids {
		keys = append(keys, key)
	}

	return logs, keys
}

func TestAuth_UpdateOnInterfaceWithAuthRules(t *testing.T) {
	_, _, _, ids := getAllPosts(t, []string{"user1@dgraph.io", "user2@dgraph.io"}, []string{"ADMIN"}, []bool{true, false})
	testCases := []TestCase{{
		name:   "Only 2 nodes satisfy auth rules with the given values and hence should be updated",
		user:   "user1@dgraph.io",
		ans:    true,
		result: `{"updatePost":{"numUids":2}}`,
	}, {
		name:   "Only 3 nodes satisfy auth rules with the given values and hence should be updated",
		user:   "user1@dgraph.io",
		role:   "ADMIN",
		ans:    true,
		result: `{"updatePost":{"numUids":3}}`,
	}, {
		name:   "Only 3 nodes satisfy auth rules with the given values and hence should be updated",
		user:   "user1@dgraph.io",
		role:   "ADMIN",
		ans:    false,
		result: `{"updatePost":{"numUids":3}}`,
	}, {
		name:   "No node satisfy auth rules with the given value of `user`",
		user:   "user3@dgraph.io",
		result: `{"updatePost":{"numUids":0}}`,
	},
	}

	query := `
		mutation($ids: [ID!]){
			updatePost(input: {filter: {id: $ids}, set: {topic: "A Topic"}}){
				numUids
			}
		}
	`

	for _, tcase := range testCases {
		t.Run(tcase.name, func(t *testing.T) {
			params := &common.GraphQLParams{
				Headers:   common.GetJWTForInterfaceAuth(t, tcase.user, tcase.role, tcase.ans, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"ids": ids},
			}

			gqlResponse := params.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, tcase.result, string(gqlResponse.Data))
		})
	}
}

func TestAuth_UpdateOnTypeWithGraphFilterOnInterface(t *testing.T) {
	_, ids := getAllQuestions(t, []string{"user1@dgraph.io", "user2@dgraph.io"}, []bool{true, false})

	testCases := []TestCase{{
		name:   "Only 1 Question Node, whose text is `A Question` satisfies the below `user` and `ans`",
		user:   "user1@dgraph.io",
		ans:    true,
		result: `{"updateQuestion": {"question":[{"text": "A Question", "topic": "A Topic"}]}}`,
	}, {
		name:   "Only 1 Question Node, whose text is `B Question` satisfies the below `user` and `ans`",
		user:   "user2@dgraph.io",
		ans:    true,
		result: `{"updateQuestion": {"question":[{"text": "B Question", "topic": "A Topic"}]}}`,
	}, {
		name:   "Only 1 Question Node, whose text is `C Question` satisfies the below `user` and `ans`",
		user:   "user1@dgraph.io",
		ans:    false,
		result: `{"updateQuestion": {"question":[{"text": "C Question", "topic": "A Topic"}]}}`,
	},
	}

	query := `
		mutation($ids: [ID!]){
			updateQuestion(input: {filter: {id: $ids}, set: {topic: "A Topic"}}){
				question{
					text
					topic
				}
			}
		}
	`

	for _, tcase := range testCases {
		t.Run(tcase.name, func(t *testing.T) {
			params := &common.GraphQLParams{
				Headers:   common.GetJWTForInterfaceAuth(t, tcase.user, "", tcase.ans, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"ids": ids},
			}

			gqlResponse := params.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestAuth_UpdateOnTypeWithRBACAuthRuleOnInterface(t *testing.T) {
	_, ids := getAllFbPosts(t, []string{"user1@dgraph.io", "user2@dgraph.io"}, []string{"ADMIN"})

	testCases := []TestCase{{
		name:   "Update node with given `user` as RBAC rule for FbPost is satisfied",
		user:   "user1@dgraph.io",
		role:   "ADMIN",
		result: `{"updateFbPost": {"fbPost":[{"text": "A FbPost", "topic": "Topic of FbPost"}]}}`,
	}, {
		name:   "Update node with given `user`  as RBAC rule for FbPost is satisfied",
		user:   "user2@dgraph.io",
		role:   "ADMIN",
		result: `{"updateFbPost": {"fbPost":[{"text": "B FbPost", "topic": "Topic of FbPost"}]}}`,
	}, {
		name:   "Authorization will fail for any role other than `ADMIN`",
		user:   "user1@dgraph.io",
		role:   "USER",
		result: `{"updateFbPost": {"fbPost":[]}}`,
	},
	}

	query := `
		mutation($ids: [ID!]){
			updateFbPost(input: {filter: {id: $ids}, set: {topic: "Topic of FbPost"}}){
				fbPost{
					text
					topic
				}
			}
		}
	`
	for _, tcase := range testCases {
		t.Run(tcase.user+tcase.role, func(t *testing.T) {
			params := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"ids": ids},
			}

			gqlResponse := params.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}

}

func TestUpdateOrRBACFilter(t *testing.T) {
	ids := getAllProjects(t, []string{"user1"}, []string{"ADMIN"})

	testCases := []TestCase{{
		user:   "user1",
		role:   "ADMIN",
		result: `{"updateProject": {"project": [{"name": "Project1"},{"name": "Project2"}]}}`,
	}, {
		user:   "user1",
		role:   "USER",
		result: `{"updateProject": {"project": [{"name": "Project1"}]}}`,
	}, {
		user:   "user4",
		role:   "USER",
		result: `{"updateProject": {"project": [{"name": "Project2"}]}}`,
	}}

	query := `
	    mutation ($projs: [ID!]) {
		    updateProject(input: {filter: {projID: $projs}, set: {random: "test"}}) {
			project (order: {asc: name}) {
				name
			}
		    }
	    }
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"projs": ids},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestUpdateRootFilter(t *testing.T) {
	_, ids := getAllColumns(t, []string{"user1", "user2", "user4"}, []string{"USER"})

	testCases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"updateColumn": {"column": [{"name": "Column1"}]}}`,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"updateColumn": {"column": [{"name": "Column1"}, {"name": "Column2"}, {"name": "Column3"}]}}`,
	}, {
		user:   "user4",
		role:   "USER",
		result: `{"updateColumn": {"column": [{"name": "Column2"}, {"name": "Column3"}]}}`,
	}}

	query := `
	    mutation ($cols: [ID!]) {
		    updateColumn(input: {filter: {colID: $cols}, set: {random: "test"}}) {
			column (order: {asc: name}) {
				name
			}
		    }
	    }
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"cols": ids},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestUpdateRBACFilter(t *testing.T) {
	_, ids := getAllLogs(t, []string{"user1"}, []string{"ADMIN"})

	testCases := []TestCase{
		{role: "USER", result: `{"updateLog": {"log": []}}`},
		{role: "ADMIN", result: `{"updateLog": {"log": [{"logs": "Log1"},{"logs": "Log2"}]}}`}}

	query := `
	    mutation ($ids: [ID!]) {
		    updateLog(input: {filter: {id: $ids}, set: {random: "test"}}) {
			log (order: {asc: logs}) {
				logs
			}
		    }
	    }
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"ids": ids},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestUpdateAndRBACFilter(t *testing.T) {
	_, ids := getAllIssues(t, []string{"user1", "user2"}, []string{"ADMIN"})

	testCases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"updateIssue": {"issue": []}}`,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"updateIssue": {"issue": []}}`,
	}, {
		user:   "user2",
		role:   "ADMIN",
		result: `{"updateIssue": {"issue": [{"msg": "Issue2"}]}}`,
	}}

	query := `
	    mutation ($ids: [ID!]) {
		    updateIssue(input: {filter: {id: $ids}, set: {random: "test"}}) {
			issue (order: {asc: msg}) {
				msg
			}
		    }
	    }
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"ids": ids},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}

func TestUpdateNestedFilter(t *testing.T) {
	_, ids := getAllMovies(t, []string{"user1", "user2", "user3"}, []string{"ADMIN"})

	testCases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"updateMovie": {"movie": [{"content": "Movie2"}, {"content": "Movie3"}, { "content": "Movie4" }]}}`,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"updateMovie": {"movie": [{ "content": "Movie1" }, { "content": "Movie2" }, { "content": "Movie3" }, { "content": "Movie4" }]}}`,
	}}

	query := `
	    mutation ($ids: [ID!]) {
		    updateMovie(input: {filter: {id: $ids}, set: {random: "test"}}) {
			movie (order: {asc: content}) {
				content
			}
		    }
	    }
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			getUserParams := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"ids": ids},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)

			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}
