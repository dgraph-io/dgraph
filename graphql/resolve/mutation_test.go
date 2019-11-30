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
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// Tests showing that GraphQL mutations -> Dgraph mutations
// is working as expected.
//
// Note: this doesn't include GQL validation errors!  The rewriting code assumes
// it's rewriting a mutation that's valid (with valid variables) for the schema.
// So can't test GQL errors here - that's integration testing on the pipeline to
// ensure that those errors get caught before they reach rewriting.

type TestCase struct {
	Name         string
	GQLMutation  string
	GQLVariables string
	Explanation  string
	DgraphSet    string
	DgraphDelete string
	DgraphQuery  string
	Condition    string
	Error        *x.GqlError
}

func TestMutationRewriting(t *testing.T) {
	t.Run("Add Mutation Rewriting", func(t *testing.T) {
		mutationRewriting(t, "add_mutation_test.yaml", NewAddRewriter())
	})
	t.Run("Update Mutation Rewriting", func(t *testing.T) {
		mutationRewriting(t, "update_mutation_test.yaml", NewUpdateRewriter())
	})
	t.Run("Delete Mutation Rewriting", func(t *testing.T) {
		mutationRewriting(t, "delete_mutation_test.yaml", NewDeleteRewriter())
	})
}

func mutationRewriting(t *testing.T, file string, rewriterToTest MutationRewriter) {
	b, err := ioutil.ReadFile(file)
	require.NoError(t, err, "Unable to read test file")

	var tests []TestCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			var vars map[string]interface{}
			if tcase.GQLVariables != "" {
				err := json.Unmarshal([]byte(tcase.GQLVariables), &vars)
				require.NoError(t, err)
			}

			op, err := gqlSchema.Operation(
				&schema.Request{
					Query:     tcase.GQLMutation,
					Variables: vars,
				})
			require.NoError(t, err)

			mut := test.GetMutation(t, op)

			q, muts, err := rewriterToTest.Rewrite(mut)

			if tcase.Error != nil || err != nil {
				require.Equal(t, tcase.Error.Error(), err.Error())
			} else {
				require.Len(t, muts, 1)
				require.Equal(t, tcase.Condition, muts[0].Cond)
				require.Equal(t, tcase.DgraphQuery, dgraph.AsString(q))

				setMut := string(muts[0].SetJson)
				if setMut != "" || tcase.DgraphSet != "" {
					require.JSONEq(t, tcase.DgraphSet, setMut)
				}

				delMut := string(muts[0].DeleteJson)
				if delMut != "" || tcase.DgraphDelete != "" {
					require.JSONEq(t, tcase.DgraphDelete, delMut)
				}
			}
		})
	}
}

func TestMutationQueryRewriting(t *testing.T) {
	testTypes := map[string]struct {
		mut      string
		rewriter MutationRewriter
	}{
		"Add Post ": {
			`addPost(input: {title: "A Post", author: {id: "0x1"}})`,
			NewAddRewriter(),
		},
		"Update Post ": {
			`updatePost(input: { filter: { ids:  ["0x4"] }, set: { text: "Updated text" } }) `,
			NewUpdateRewriter(),
		},
	}

	allowedTestTypes := map[string][]string{
		"UPDATE_MUTATION":     []string{"Update Post "},
		"ADD_UPDATE_MUTATION": []string{"Add Post ", "Update Post "},
	}

	b, err := ioutil.ReadFile("mutation_query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests map[string][]QueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	for testType := range tests {
		for _, name := range allowedTestTypes[testType] {
			tt := testTypes[name]
			for _, tcase := range tests[testType] {
				t.Run(name+testType+tcase.Name, func(t *testing.T) {

					gqlMutationStr := strings.Replace(tcase.GQLQuery, testType, tt.mut, 1)
					op, err := gqlSchema.Operation(
						&schema.Request{
							Query:     gqlMutationStr,
							Variables: tcase.Variables,
						})
					require.NoError(t, err)
					gqlMutation := test.GetMutation(t, op)

					assigned := map[string]string{}
					mutated := map[string][]string{}
					if name == "Add Post " {
						assigned["newnode"] = "0x4"
					} else {
						mutated[mutationQueryVar] = []string{"0x4"}
					}
					dgQuery, err := tt.rewriter.FromMutationResult(
						gqlMutation, assigned, mutated)

					require.Nil(t, err)
					require.Equal(t, tcase.DGQuery, dgraph.AsString(dgQuery))
				})
			}
		}
	}
}
