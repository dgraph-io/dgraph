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

type testCase struct {
	Name            string
	GQLMutation     string
	GQLVariables    string
	Explanation     string
	DGMutations     []*dgraphMutation
	DGQuery         string
	Error           *x.GqlError
	ValidationError *x.GqlError
}

type dgraphMutation struct {
	SetJSON    string
	DeleteJSON string
	Cond       string
}

func TestMutationRewriting(t *testing.T) {
	t.Run("Validate Mutations", func(t *testing.T) {
		mutationValidation(t, "validate_mutation_test.yaml", NewAddRewriter)
	})
	t.Run("Add Mutation Rewriting", func(t *testing.T) {
		mutationRewriting(t, "add_mutation_test.yaml", NewAddRewriter)
	})
	t.Run("Update Mutation Rewriting", func(t *testing.T) {
		mutationRewriting(t, "update_mutation_test.yaml", NewUpdateRewriter)
	})
	t.Run("Delete Mutation Rewriting", func(t *testing.T) {
		mutationRewriting(t, "delete_mutation_test.yaml", NewDeleteRewriter)
	})
}

func mutationValidation(t *testing.T, file string, rewriterFactory func() MutationRewriter) {
	b, err := ioutil.ReadFile(file)
	require.NoError(t, err, "Unable to read test file")

	var tests []testCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			// -- Arrange --
			var vars map[string]interface{}
			if tcase.GQLVariables != "" {
				err := json.Unmarshal([]byte(tcase.GQLVariables), &vars)
				require.NoError(t, err)
			}

			_, err := gqlSchema.Operation(
				&schema.Request{
					Query:     tcase.GQLMutation,
					Variables: vars,
				})
			require.NotNil(t, err)
			require.Equal(t, err.Error(), tcase.ValidationError.Error())
		})
	}
}

func mutationRewriting(t *testing.T, file string, rewriterFactory func() MutationRewriter) {
	b, err := ioutil.ReadFile(file)
	require.NoError(t, err, "Unable to read test file")

	var tests []testCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			// -- Arrange --
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
			rewriterToTest := rewriterFactory()

			// -- Act --
			q, muts, err := rewriterToTest.Rewrite(mut)

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

func TestMutationQueryRewriting(t *testing.T) {
	testTypes := map[string]struct {
		mut      string
		rewriter func() MutationRewriter
		assigned map[string]string
		result   map[string]interface{}
	}{
		"Add Post ": {
			mut:      `addPost(input: [{title: "A Post", author: {id: "0x1"}}])`,
			rewriter: NewAddRewriter,
			assigned: map[string]string{"Post1": "0x4"},
		},
		"Update Post ": {
			mut: `updatePost(input: {filter: {postID
				:  ["0x4"]}, set: {text: "Updated text"} }) `,
			rewriter: NewUpdateRewriter,
			result: map[string]interface{}{
				"updatePost": []interface{}{map[string]interface{}{"uid": "0x4"}}},
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
					rewriter := tt.rewriter()
					// -- Arrange --
					gqlMutationStr := strings.Replace(tcase.GQLQuery, testType, tt.mut, 1)
					op, err := gqlSchema.Operation(
						&schema.Request{
							Query:     gqlMutationStr,
							Variables: tcase.Variables,
						})
					require.NoError(t, err)
					gqlMutation := test.GetMutation(t, op)
					_, _, err = rewriter.Rewrite(gqlMutation)
					require.Nil(t, err)

					// -- Act --
					dgQuery, err := rewriter.FromMutationResult(
						gqlMutation, tt.assigned, tt.result)

					// -- Assert --
					require.Nil(t, err)
					require.Equal(t, tcase.DGQuery, dgraph.AsString(dgQuery))
				})
			}
		}
	}
}
