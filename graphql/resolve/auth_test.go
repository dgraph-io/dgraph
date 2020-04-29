/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package resolve

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/dgraph-io/dgraph/testutil"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	_ "github.com/vektah/gqlparser/v2/validator/rules" // make gql validator init() all rules
	"gopkg.in/yaml.v2"
)

type AuthQueryRewritingCase struct {
	Name        string
	GQLQuery    string
	Variables   string
	DGMutations []*dgraphMutation
	DGQuery     string
	User        string
	Role        string

	// needed?
	Error           *x.GqlError
	ValidationError *x.GqlError
}

// Tests showing that the query rewriter produces the expected Dgraph queries
// when it also needs to write in auth.
func TestAuthQueryRewriting(t *testing.T) {
	b, err := ioutil.ReadFile("auth_query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []AuthQueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	testRewriter := NewQueryRewriter()

	sch, err := ioutil.ReadFile("../e2e/auth/schema.graphql")
	require.NoError(t, err, "Unable to read schema file")

	strSchema := string(sch)
	gqlSchema := test.LoadSchemaFromString(t, strSchema)
	metainfo := &authorization.AuthMeta{}
	metainfo.Parse(strSchema)
	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {

			op, err := gqlSchema.Operation(
				&schema.Request{
					Query: tcase.GQLQuery,
					// Variables: tcase.Variables,
				})
			require.NoError(t, err)
			gqlQuery := test.GetQuery(t, op)

			authVars := map[string]interface{}{
				"USER": "user1",
				"ROLE": tcase.Role,
			}

			ctx := testutil.AddClaimsToContext(context.Background(), t, authVars, metainfo)

			dgQuery, err := testRewriter.Rewrite(ctx, gqlQuery)
			require.Nil(t, err)
			require.Equal(t, tcase.DGQuery, dgraph.AsString(dgQuery))
		})
	}
}

// Tests that the queries that run after a mutation get auth correctly added in.
func TestAuthMutationQueryRewriting(t *testing.T) {
	tests := map[string]struct {
		gqlMut   string
		rewriter func() MutationRewriter
		assigned map[string]string
		result   map[string]interface{}
		dgQuery  string
	}{
		"Add Ticket": {
			gqlMut: `mutation {
				addTicket(input: [{title: "A ticket", onColumn: {colID: "0x1"}}]) {
				  ticket {
					id
					title
					onColumn {
						colID
						name
					}
				  }
				}
			  }`,
			rewriter: NewAddRewriter,
			assigned: map[string]string{"Ticket1": "0x4"},
			dgQuery: `query {
  ticket(func: uid(Ticket1)) @filter(uid(Ticket2)) {
    id : uid
    title : Ticket.title
    onColumn : Ticket.onColumn {
      colID : uid
      name : Column.name
    }
  }
  Ticket1 as var(func: uid(0x4))
  Ticket2 as var(func: uid(Ticket1)) @cascade {
    onColumn : Ticket.onColumn {
      inProject : Column.inProject {
        roles : Project.roles @filter(eq(Role.permission, "VIEW")) {
          assignedTo : Role.assignedTo @filter(eq(User.username, "user1"))
          dgraph.uid : uid
        }
        dgraph.uid : uid
      }
      dgraph.uid : uid
    }
    dgraph.uid : uid
  }
}`,
		},
		"Update Ticket": {
			gqlMut: `mutation {
				updateTicket(input: {filter: {id: ["0x4"]}, set: {title: "Updated title"} }) {
					ticket {
						id
						title
						onColumn {
							colID
							name
						}
					  }
				}
			  }`,
			rewriter: NewUpdateRewriter,
			result: map[string]interface{}{
				"updateTicket": []interface{}{map[string]interface{}{"uid": "0x4"}}},
			dgQuery: `query {
  ticket(func: uid(Ticket1)) @filter(uid(Ticket2)) {
    id : uid
    title : Ticket.title
    onColumn : Ticket.onColumn {
      colID : uid
      name : Column.name
    }
  }
  Ticket1 as var(func: uid(0x4))
  Ticket2 as var(func: uid(Ticket1)) @cascade {
    onColumn : Ticket.onColumn {
      inProject : Column.inProject {
        roles : Project.roles @filter(eq(Role.permission, "VIEW")) {
          assignedTo : Role.assignedTo @filter(eq(User.username, "user1"))
          dgraph.uid : uid
        }
        dgraph.uid : uid
      }
      dgraph.uid : uid
    }
    dgraph.uid : uid
  }
}`,
		},
	}

	sch, err := ioutil.ReadFile("../e2e/auth/schema.graphql")
	require.NoError(t, err, "Unable to read schema file")

	strSchema := string(sch)
	gqlSchema := test.LoadSchemaFromString(t, strSchema)
	metainfo := &authorization.AuthMeta{}
	metainfo.Parse(strSchema)

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// -- Arrange --
			rewriter := tt.rewriter()
			op, err := gqlSchema.Operation(&schema.Request{Query: tt.gqlMut})
			require.NoError(t, err)
			gqlMutation := test.GetMutation(t, op)
			authVars := map[string]interface{}{
				"USER": "user1",
			}
			ctx := testutil.AddClaimsToContext(context.Background(), t, authVars, metainfo)
			_, err = rewriter.Rewrite(ctx, gqlMutation)
			require.Nil(t, err)

			// -- Act --
			dgQuery, err := rewriter.FromMutationResult(
				ctx, gqlMutation, tt.assigned, tt.result)

			// -- Assert --
			require.Nil(t, err)
			require.Equal(t, tt.dgQuery, dgraph.AsString(dgQuery))
		})

	}
}

// Tests showing that the query rewriter produces the expected Dgraph queries
// for delete when it also needs to write in auth - this doesn't extend to other nodes
// it only ever applies at the top level because delete only deletes the nodes
// referenced by the filter, not anything deeper.
func TestAuthDeleteRewriting(t *testing.T) {
	b, err := ioutil.ReadFile("auth_delete_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []AuthQueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	sch, err := ioutil.ReadFile("../e2e/auth/schema.graphql")
	require.NoError(t, err, "Unable to read schema file")

	strSchema := string(sch)
	gqlSchema := test.LoadSchemaFromString(t, strSchema)
	metainfo := &authorization.AuthMeta{}
	metainfo.Parse(strSchema)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			// -- Arrange --
			var vars map[string]interface{}
			if tcase.Variables != "" {
				err := json.Unmarshal([]byte(tcase.Variables), &vars)
				require.NoError(t, err)
			}

			op, err := gqlSchema.Operation(
				&schema.Request{
					Query:     tcase.GQLQuery,
					Variables: vars,
				})
			require.NoError(t, err)
			mut := test.GetMutation(t, op)
			rewriterToTest := NewDeleteRewriter()

			authVars := map[string]interface{}{
				"USER": "user1",
				"ROLE": tcase.Role,
			}

			ctx := testutil.AddClaimsToContext(context.Background(), t, authVars, metainfo)

			// -- Act --
			upsert, err := rewriterToTest.Rewrite(ctx, mut)
			q := upsert.Query
			muts := upsert.Mutations

			// -- Assert --
			if tcase.Error != nil || err != nil {
				require.Equal(t, tcase.Error.Error(), err.Error())
			} else {
				require.Equal(t, tcase.DGQuery, dgraph.AsString(q))
				require.Len(t, muts, len(tcase.DGMutations))
				for i, expected := range tcase.DGMutations {
					require.Equal(t, expected.Cond, muts[i].Cond)
					if len(muts[i].SetJson) > 0 || expected.SetJSON != "" {
						require.JSONEq(t, expected.SetJSON, string(muts[i].SetJson))
					}
					if len(muts[i].DeleteJson) > 0 || expected.DeleteJSON != "" {
						require.JSONEq(t, expected.DeleteJSON, string(muts[i].DeleteJson))
					}
				}
			}
		})
	}
}
