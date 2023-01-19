package auth

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
)

func (c *Column) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
		mutation addColumn($column: AddColumnInput!) {
		  addColumn(input: [$column]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"column": c},
	}
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func (l *Log) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
		mutation addLog($pwd: String!, $logs: String, $random: String) {
		  addLog(input: [{pwd: $pwd, logs: $logs, random: $random}]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"pwd": "password", "logs": l.Logs, "random": l.Random},
	}
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func (i *Issue) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
		mutation addIssue($issue: AddIssueInput!) {
		  addIssue(input: [$issue]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"issue": i},
	}
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func (m *Movie) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
		mutation addMovie($movie: AddMovieInput!) {
		  addMovie(input: [$movie]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"movie": m},
	}
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func (cl *ComplexLog) add(t *testing.T, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, "", role, metaInfo),
		Query: `
		mutation addComplexLog($complexlog: AddComplexLogInput!) {
		  addComplexLog(input: [$complexlog]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"complexlog": cl},
	}
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func (q *Question) add(t *testing.T, user string, ans bool) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWTForInterfaceAuth(t, user, "", ans, metaInfo),
		Query: `
		mutation addQuestion($text: String!,$id: ID!, $ans: Boolean, $pwd: String! ){
			addQuestion(input: [{text: $text, author: {id: $id}, answered: $ans, pwd: $pwd }]){
			  numUids
			}
		  }
		`,
		Variables: map[string]interface{}{"text": q.Text, "ans": q.Answered, "id": q.Author.Id, "pwd": "password"},
	}
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func (a *Answer) add(t *testing.T, user string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, "", metaInfo),
		Query: `
		mutation addAnswer($text: String!,$id: ID!, $pwd: String!){
			addAnswer(input: [{text: $text, pwd: $pwd, author: {id: $id}}]){
			  numUids
			}
		  }
		`,
		Variables: map[string]interface{}{"text": a.Text, "id": a.Author.Id, "pwd": "password"},
	}
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func (f *FbPost) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, user, role, metaInfo),
		Query: `
		mutation addFbPost($text: String!,$id1: ID!,$id2:ID!, $id3: ID!, $postCount: Int!, $pwd: String! ){
			addFbPost(input: [{text: $text, author: {id: $id1},sender: {id: $id2}, receiver: {id: $id3}, postCount: $postCount, pwd: $pwd }]){
			  numUids
			}
		  }
		`,
		Variables: map[string]interface{}{"text": f.Text, "id1": f.Author.Id, "id2": f.Sender.Id, "id3": f.Receiver.Id, "postCount": f.PostCount, "pwd": "password"},
	}
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func getComplexLog(t *testing.T, role string) ([]*ComplexLog, []string) {
	getParams := &common.GraphQLParams{
		Query: `
		query queryComplexLog {
		  queryComplexLog {
			id
			logs
			visible
		  }
		}
		`,
	}

	getParams.Headers = common.GetJWT(t, "", role, metaInfo)
	gqlResponse := getParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	var result struct {
		QueryComplexLog []*ComplexLog
	}
	var complexLogs []*ComplexLog
	err := json.Unmarshal(gqlResponse.Data, &result)
	require.NoError(t, err)

	var keys []string
	for _, i := range result.QueryComplexLog {
		keys = append(keys, i.Id)
		i.Id = ""
		complexLogs = append(complexLogs, i)
	}
	return complexLogs, keys
}

func TestAuth_DeleteOnInterfaceWithAuthRules(t *testing.T) {
	testCases := []TestCase{{
		name:   "Only 3 nodes satisfy auth rules with the given values and hence they should be deleted",
		user:   "user1@dgraph.io",
		role:   "ADMIN",
		ans:    true,
		result: `{"deletePost": {"numUids":3}}`,
	}, {
		name:   "Only 2 nodes satisfy auth rules with the given values and hence they should be deleted",
		user:   "user1@dgraph.io",
		role:   "USER",
		ans:    true,
		result: `{"deletePost": {"numUids":2}}`,
	}, {
		name:   "Only 1 node satisfies auth rules with the given value of user and hence it should be deleted",
		user:   "user2@dgraph.io",
		result: `{"deletePost": {"numUids":1}}`,
	}, {
		name:   "No node satisfies auth rules with the given value of user",
		user:   "user3@dgraph.io",
		result: `{"deletePost": {"numUids":0}}`,
	},
	}

	query := `
		mutation ($posts: [ID!]) {
			deletePost(filter: {id: $posts}) {
				numUids
			}
		}
	`

	for _, tcase := range testCases {
		// Fetch all the types implementing `Post` interface.
		allQuestions, allAnswers, allFbPosts, allPostsIds := getAllPosts(t, []string{"user1@dgraph.io", "user2@dgraph.io"}, []string{"ADMIN"}, []bool{true, false})
		require.True(t, len(allQuestions) == 3)
		require.True(t, len(allAnswers) == 2)
		require.True(t, len(allFbPosts) == 2)
		require.True(t, len(allPostsIds) == 7)

		deleteQuestions, deleteAnswers, deleteFbPosts, _ := getAllPosts(t, []string{tcase.user}, []string{tcase.role}, []bool{tcase.ans})

		params := &common.GraphQLParams{
			Headers:   common.GetJWTForInterfaceAuth(t, tcase.user, tcase.role, tcase.ans, metaInfo),
			Query:     query,
			Variables: map[string]interface{}{"posts": allPostsIds},
		}

		gqlResponse := params.ExecuteAsPost(t, common.GraphqlURL)
		common.RequireNoGQLErrors(t, gqlResponse)
		require.JSONEq(t, tcase.result, string(gqlResponse.Data))

		// Restore the deleted Questions, Answers and FbPosts for other test cases.
		for _, question := range deleteQuestions {
			question.add(t, tcase.user, tcase.ans)
		}
		for _, answer := range deleteAnswers {
			answer.add(t, tcase.user)
		}
		for _, fbpost := range deleteFbPosts {
			fbpost.add(t, tcase.user, tcase.role)
		}
	}
}

func TestAuth_DeleteTypeWithRBACFilteronInterface(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1@dgraph.io",
		role:   "ADMIN",
		result: `{"deleteFbPost": {"numUids":1}}`,
	}, {
		user:   "user1@dgraph.io",
		role:   "USER",
		result: `{"deleteFbPost": {"numUids":0}}`,
	}, {
		user:   "user2@dgraph.io",
		role:   "ROLE",
		result: `{"deleteFbPost": {"numUids":0}}`,
	}, {
		user:   "user2@dgraph.io",
		role:   "ADMIN",
		result: `{"deleteFbPost": {"numUids":1}}`,
	},
	}

	query := `
		mutation ($fbposts: [ID!]) {
			deleteFbPost(filter: {id: $fbposts}) {
				numUids
			}
		}
	`

	for _, tcase := range testCases {
		_, allFbPostsIds := getAllFbPosts(t, []string{"user1@dgraph.io", "user2@dgraph.io"}, []string{"ADMIN"})
		require.True(t, len(allFbPostsIds) == 2)
		deleteFbPosts, _ := getAllFbPosts(t, []string{tcase.user}, []string{tcase.role})

		params := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: map[string]interface{}{"questions": allFbPostsIds},
		}

		gqlResponse := params.ExecuteAsPost(t, common.GraphqlURL)
		common.RequireNoGQLErrors(t, gqlResponse)
		require.JSONEq(t, tcase.result, string(gqlResponse.Data))

		// Restore the deleted FbPosts for other test cases.
		for _, fbpost := range deleteFbPosts {
			fbpost.add(t, tcase.user, tcase.role)
		}
	}
}

func TestAuth_DeleteOnTypeWithGraphTraversalAuthRuleOnInterface(t *testing.T) {
	testCases := []TestCase{{
		name:   "One node is deleted as there is one node with the following `user` and `ans`.",
		user:   "user1@dgraph.io",
		ans:    true,
		result: `{"deleteQuestion": {"numUids": 1}}`,
	}, {
		name:   "One node is deleted as there is one node with the following `user` and `ans`.",
		user:   "user1@dgraph.io",
		ans:    false,
		result: `{"deleteQuestion": {"numUids": 1}}`,
	}, {
		name:   "One node is deleted as there is one node with the following `user` and `ans`.",
		user:   "user2@dgraph.io",
		ans:    true,
		result: `{"deleteQuestion": {"numUids": 1}}`,
	}, {
		name:   "No node is deleted as there is no node with the following `user` and `ans`.",
		user:   "user2@dgraph.io",
		ans:    false,
		result: `{"deleteQuestion": {"numUids": 0}}`,
	},
	}

	query := `
		mutation ($questions: [ID!]) {
			deleteQuestion(filter: {id: $questions}) {
				numUids
			}
		}
	`

	for _, tcase := range testCases {
		t.Run(tcase.user+strconv.FormatBool(tcase.ans), func(t *testing.T) {
			// Get all Question ids.
			_, allQuestionsIds := getAllQuestions(t, []string{"user1@dgraph.io", "user2@dgraph.io"}, []bool{true, false})
			require.True(t, len(allQuestionsIds) == 3)
			deleteQuestions, _ := getAllQuestions(t, []string{tcase.user}, []bool{tcase.ans})

			params := &common.GraphQLParams{
				Headers:   common.GetJWTForInterfaceAuth(t, tcase.user, "", tcase.ans, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"questions": allQuestionsIds},
			}

			gqlResponse := params.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, tcase.result, string(gqlResponse.Data))

			// Restore the deleted Questions for other test cases.
			for _, question := range deleteQuestions {
				question.add(t, tcase.user, tcase.ans)
			}
		})
	}
}

func TestDeleteRootFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"deleteColumn": {"numUids": 1}}`,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"deleteColumn": {"numUids": 3}}`,
	}, {
		user:   "user4",
		role:   "USER",
		result: `{"deleteColumn": {"numUids": 2}}`,
	}}

	query := `
		mutation ($cols: [ID!]) {
			deleteColumn(filter: {colID: $cols}) {
          		numUids
		    }
	    }
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			// Get all Column ids.
			_, allColumnIds := getAllColumns(t, []string{"user1", "user2", "user4"}, []string{"USER"})
			require.True(t, len(allColumnIds) == 3)

			// Columns that will be deleted.
			deleteColumns, _ := getAllColumns(t, []string{tcase.user}, []string{tcase.role})

			getUserParams := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"cols": allColumnIds},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, tcase.result, string(gqlResponse.Data))

			// Restore the deleted Columns.
			for _, column := range deleteColumns {
				column.add(t, tcase.user, tcase.role)
			}
		})
	}
}

func TestDeleteRBACFilter(t *testing.T) {
	testCases := []TestCase{
		{
			role: "USER",
			result: `
						{
							"deleteLog":
								{
									"numUids":0,
									"msg":"No nodes were deleted",
									"log":[]
								}
						}
					`,
		},
		{
			role: "ADMIN",
			result: `
						{
							"deleteLog":
								{
									"numUids":2,
									"msg":"Deleted",
									"log":
										[
											{
												"logs":"Log1",
												"random":"test"
											},
											{
												"logs":"Log2",
												"random":"test"
											}
										]
								}
						}
					`,
		},
	}

	query := `
		mutation ($logs: [ID!]) {
			deleteLog(filter: {id: $logs}) {
          		numUids
				msg
				log (order: { asc: logs }) {
					logs
					random
				}
		    }
	    }
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			// Get all Log ids.
			_, allLogIds := getAllLogs(t, []string{"user1"}, []string{"ADMIN"})
			require.True(t, len(allLogIds) == 2)

			// Logs that will be deleted.
			deletedLogs, _ := getAllLogs(t, []string{tcase.user}, []string{tcase.role})

			getUserParams := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"logs": allLogIds},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, tcase.result, string(gqlResponse.Data))

			// Restore the deleted logs.
			for _, log := range deletedLogs {
				log.add(t, tcase.user, tcase.role)
			}
		})
	}
}

func TestDeleteOrRBACFilter(t *testing.T) {
	testCases := []TestCase{{
		role:   "USER",
		result: `{"deleteComplexLog": {"numUids": 1}}`,
	}, {
		role:   "ADMIN",
		result: `{"deleteComplexLog": {"numUids": 2}}`,
	}}

	query := `
		mutation($ids: [ID!]) {
			deleteComplexLog (filter: { id: $ids}) {
         		numUids
		    }
		}
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			// Get all ComplexLog.
			allComplexLogs, allComplexLogIds := getComplexLog(t, "ADMIN")
			require.True(t, len(allComplexLogIds) == 2)

			getUserParams := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"ids": allComplexLogIds},
			}
			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, tcase.result, string(gqlResponse.Data))

			// Restore the deleted ComplexLog.
			for _, complexLog := range allComplexLogs {
				if tcase.role != "ADMIN" && complexLog.Visible == false {
					continue
				}
				complexLog.add(t, "ADMIN")
			}
		})
	}
}

func TestDeleteAndRBACFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"deleteIssue": {"numUids": 0}}`,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"deleteIssue": {"numUids": 0}}`,
	}, {
		user:   "user2",
		role:   "ADMIN",
		result: `{"deleteIssue": {"numUids": 1}}`,
	}}

	query := `
	    mutation ($ids: [ID!]) {
		    deleteIssue(filter: {id: $ids}) {
				numUids
		    }
	    }
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			// Get all Issue ids.
			_, ids := getAllIssues(t, []string{"user1", "user2"}, []string{"ADMIN"})
			require.True(t, len(ids) == 2)

			// Issues that will be deleted.
			deletedIssues, _ := getAllIssues(t, []string{tcase.user}, []string{tcase.role})

			getUserParams := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"ids": ids},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, string(gqlResponse.Data), tcase.result)

			// Restore the deleted Issues.
			for _, issue := range deletedIssues {
				issue.add(t, tcase.user, tcase.role)
			}
		})
	}
}

func TestDeleteNestedFilter(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		role:   "USER",
		result: `{"deleteMovie":{"numUids":3,"movie":[{"content":"Movie2","regionsAvailable":[{"name":"Region1","global":null}]},{"content":"Movie3","regionsAvailable":[{"name":"Region1","global":null},{"name":"Region4","global":null},{"name":"Region6","global":true}]},{"content":"Movie4","regionsAvailable":[{"name":"Region5","global":true}]}]}}`,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"deleteMovie":{"numUids":4,"movie":[{"content":"Movie1","regionsAvailable":[{"name":"Region2","global":null},{"name":"Region3","global":null}]},{"content":"Movie2","regionsAvailable":[{"name":"Region1","global":null}]},{"content":"Movie3","regionsAvailable":[{"name":"Region1","global":null},{"name":"Region4","global":null},{"name":"Region6","global":true}]},{"content":"Movie4","regionsAvailable":[{"name":"Region5","global":true}]}]}}`,
	}}

	query := `
	    mutation ($ids: [ID!]) {
		    deleteMovie(filter: {id: $ids}) {
				numUids
				movie (order: {asc: content}) {
					content
					regionsAvailable (order: {asc: name}) {
						name
						global
					}
				}
		    }
	    }
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			// Get all Movie ids.
			_, ids := getAllMovies(t, []string{"user1", "user2", "user3"}, []string{"ADMIN"})
			require.True(t, len(ids) == 4)

			// Movies that will be deleted.
			deleteMovies, _ := getAllMovies(t, []string{tcase.user}, []string{tcase.role})

			getUserParams := &common.GraphQLParams{
				Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:     query,
				Variables: map[string]interface{}{"ids": ids},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, string(gqlResponse.Data), tcase.result)

			// Restore the deleted Movies.
			for _, movie := range deleteMovies {
				movie.add(t, tcase.user, tcase.role)
			}
		})
	}
}

func TestDeleteRBACRuleInverseField(t *testing.T) {
	mutation := `
	mutation addTweets($tweet: AddTweetsInput!){
      addTweets(input: [$tweet]) {
        numUids
      }
    }
	`

	addTweetsParams := &common.GraphQLParams{
		Headers: common.GetJWT(t, "foo", "", metaInfo),
		Query:   mutation,
		Variables: map[string]interface{}{"tweet": common.Tweets{
			Id:        "tweet1",
			Text:      "abc",
			Timestamp: "2020-10-10",
			User: &common.User{
				Username: "foo",
			},
		}},
	}

	gqlResponse := addTweetsParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	testCases := []TestCase{
		{
			user:   "foobar",
			role:   "admin",
			result: `{"deleteTweets":{"numUids":0,"tweets":[]}}`,
		},
		{
			user:   "foo",
			role:   "admin",
			result: `{"deleteTweets":{"numUids":1,"tweets":[ {"text": "abc"}]}}`,
		},
	}

	mutation = `
	mutation {
      deleteTweets(
        filter: {
          text: {anyoftext: "abc"}
        }) {
		numUids
        tweets {
          text
        }
      }
    }
	`

	for _, tcase := range testCases {
		t.Run(tcase.role+tcase.user, func(t *testing.T) {
			deleteTweetsParams := &common.GraphQLParams{
				Headers: common.GetJWT(t, tcase.user, tcase.role, metaInfo),
				Query:   mutation,
			}

			gqlResponse := deleteTweetsParams.ExecuteAsPost(t, common.GraphqlURL)
			common.RequireNoGQLErrors(t, gqlResponse)
			require.JSONEq(t, string(gqlResponse.Data), tcase.result)
		})
	}
}
