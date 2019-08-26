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
	"io/ioutil"
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/test"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// Tests showing that the mutation pipeline produces the expected Dgraph mutations,
// queries and failure cases.

func TestMutationQueryRewriting(t *testing.T) {
	testTypes := map[string]string{
		"Add Post ":    `addPost(input: {title: "A Post", author: {id: "0x1"}})`,
		"Update Post ": `updatePost(id: "0x4", input: {text: "Updated text"}) `,
	}

	b, err := ioutil.ReadFile("resolver_mutation_query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []QueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchema(t, testGQLSchema)

	for name, mut := range testTypes {
		for _, tcase := range tests {
			t.Run(name+tcase.Name, func(t *testing.T) {
				client := &queryRecorder{}
				qry := strings.Replace(tcase.GQLQuery, "ADD_UPDATE_MUTATION", mut, 1)
				resp := resolveWithClient(gqlSchema, qry, client)

				require.Nil(t, resp.Errors)
				require.Equal(t, tcase.DGQuery, client.query)
			})
		}
	}
}
