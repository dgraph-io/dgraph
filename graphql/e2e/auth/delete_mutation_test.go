package auth

import (
	"encoding/json"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/stretchr/testify/require"
)

func (c *Column) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(t, user, role),
		Query: `
		mutation addColumn($column: AddColumnInput!) {
		  addColumn(input: [$column]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"column": c},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (l *Log) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(t, user, role),
		Query: `
		mutation addLog($log: AddLogInput!) {
		  addLog(input: [$log]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"log": l},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (i *Issue) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(t, user, role),
		Query: `
		mutation addIssue($issue: AddIssueInput!) {
		  addIssue(input: [$issue]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"issue": i},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (m *Movie) add(t *testing.T, user, role string) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(t, user, role),
		Query: `
		mutation addMovie($movie: AddMovieInput!) {
		  addMovie(input: [$movie]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"movie": m},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func (cl *ComplexLog) add(t *testing.T, role string) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(t, "", role),
		Query: `
		mutation addComplexLog($complexlog: AddComplexLogInput!) {
		  addComplexLog(input: [$complexlog]) {
			numUids
		  }
		}
		`,
		Variables: map[string]interface{}{"complexlog": cl},
	}
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
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

	getParams.Headers = getJWT(t, "", role)
	gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

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
				Headers:   getJWT(t, tcase.user, tcase.role),
				Query:     query,
				Variables: map[string]interface{}{"cols": allColumnIds},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)
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
		{role: "USER", result: `{"deleteLog":{"numUids":0, "msg":"No nodes were deleted", "log":[]}}`},
		{role: "ADMIN", result: `{"deleteLog":{"numUids":2, "msg":"Deleted", "log":[{"logs":"Log1","random":"test"}, {"logs":"Log2","random":"test"}]}}`}}

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
				Headers:   getJWT(t, tcase.user, tcase.role),
				Query:     query,
				Variables: map[string]interface{}{"logs": allLogIds},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)
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
				Headers:   getJWT(t, tcase.user, tcase.role),
				Query:     query,
				Variables: map[string]interface{}{"ids": allComplexLogIds},
			}
			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)
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
				Headers:   getJWT(t, tcase.user, tcase.role),
				Query:     query,
				Variables: map[string]interface{}{"ids": ids},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)
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
		result: `{"deleteMovie":{"numUids":3,"movie":[{"content":"Movie2","regionsAvailable":[{"name":"Region1","global":null}]},{"content":"Movie3","regionsAvailable":[{"name":"Region1","global":null},{"name":"Region4","global":null}]},{"content":"Movie4","regionsAvailable":[{"name":"Region5","global":true}]}]}}`,
	}, {
		user:   "user2",
		role:   "USER",
		result: `{"deleteMovie":{"numUids":4,"movie":[{"content":"Movie1","regionsAvailable":[{"name":"Region2","global":null},{"name":"Region3","global":null}]},{"content":"Movie2","regionsAvailable":[{"name":"Region1","global":null}]},{"content":"Movie3","regionsAvailable":[{"name":"Region1","global":null},{"name":"Region4","global":null}]},{"content":"Movie4","regionsAvailable":[{"name":"Region5","global":true}]}]}}`,
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
				Headers:   getJWT(t, tcase.user, tcase.role),
				Query:     query,
				Variables: map[string]interface{}{"ids": ids},
			}

			gqlResponse := getUserParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)
			require.JSONEq(t, string(gqlResponse.Data), tcase.result)

			// Restore the deleted Movies.
			for _, movie := range deleteMovies {
				movie.add(t, tcase.user, tcase.role)
			}
		})
	}
}
