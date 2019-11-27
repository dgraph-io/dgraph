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

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/test"
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
	Name           string
	GQLMutation    string
	GQLVariables   string
	Explanation    string
	DgraphMutation string
	DgraphQuery    string
	Condition      string
	Error          *x.GqlError
}

func TestMutationRewriting(t *testing.T) {
	t.Run("Add Mutation Rewriting", func(t *testing.T) {
		mutationRewriting(t, "add_mutation_test.yaml", NewAddRewriter())
	})
	t.Run("Update Mutation Rewriting", func(t *testing.T) {
		mutationRewriting(t, "update_mutation_test.yaml", NewUpdateRewriter())
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
				jsonMut := string(muts[0].SetJson)
				require.JSONEq(t, tcase.DgraphMutation, jsonMut)
				require.Equal(t, tcase.DgraphQuery, dgraph.AsString(q))
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
			`updatePost(input: { filter: { ids:  ["0x4"] }, patch: { text: "Updated text" } }) `,
			NewUpdateRewriter(),
		},
	}

	b, err := ioutil.ReadFile("mutation_query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []QueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	for name, tt := range testTypes {
		for _, tcase := range tests {
			t.Run(name+tcase.Name, func(t *testing.T) {

				gqlMutationStr := strings.Replace(tcase.GQLQuery, "ADD_UPDATE_MUTATION", tt.mut, 1)
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

func TestDeleteMutationRewriting(t *testing.T) {
	b, err := ioutil.ReadFile("delete_mutation_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []TestCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")
	rewriterToTest := NewDeleteRewriter()

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
			require.Len(t, muts, 1)
			m := string(muts[0].DelNquads)

			test.RequireJSONEq(t, tcase.Error, err)
			if tcase.Error == nil {
				require.Equal(t, tcase.DgraphMutation, m)
				require.Equal(t, tcase.DgraphQuery, dgraph.AsString(q))
			}
		})
	}
}
