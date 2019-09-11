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

package dgraph

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/test"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/gqlerror"
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
	Name          string
	GQLMutation   string
	GQLVariables  string
	Explanation   string
	DgraphMuation string
	Error         *gqlerror.Error
}

func TestMutationRewriting(t *testing.T) {
	b, err := ioutil.ReadFile("mutation_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []TestCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	rewritererToTest := NewMutationRewriter()

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

			jsonMut, err := rewritererToTest.Rewrite(mut)

			test.RequireJSONEq(t, tcase.Error, err)
			if tcase.Error == nil {
				test.RequireJSONEqStr(t, tcase.DgraphMuation, jsonMut)
			}
		})
	}
}

func TestMutationQueryRewriting(t *testing.T) {
	testTypes := map[string]string{
		"Add Post ":    `addPost(input: {title: "A Post", author: {id: "0x1"}})`,
		"Update Post ": `updatePost(input: { postID: "0x4", patch: { text: "Updated text" } }) `,
	}

	b, err := ioutil.ReadFile("mutation_query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []QueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	testRewriter := NewQueryRewriter()

	for name, mut := range testTypes {
		for _, tcase := range tests {
			t.Run(name+tcase.Name, func(t *testing.T) {

				gqlMutationStr := strings.Replace(tcase.GQLQuery, "ADD_UPDATE_MUTATION", mut, 1)
				op, err := gqlSchema.Operation(
					&schema.Request{
						Query: gqlMutationStr,
					})
				require.NoError(t, err)
				gqlMutation := test.GetMutation(t, op)

				dgQuery, err := testRewriter.FromMutationResult(
					gqlMutation, map[string]string{"newnode": "0x4"})

				require.Nil(t, err)
				require.Equal(t, tcase.DGQuery, asString(dgQuery))
			})
		}
	}
}
