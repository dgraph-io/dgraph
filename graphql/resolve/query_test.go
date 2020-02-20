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
	"io/ioutil"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/stretchr/testify/require"
	_ "github.com/vektah/gqlparser/v2/validator/rules" // make gql validator init() all rules
	"gopkg.in/yaml.v2"
)

// Tests showing that the query rewriter produces the expected Dgraph queries

type QueryRewritingCase struct {
	Name      string
	GQLQuery  string
	Variables map[string]interface{}
	DGQuery   string
}

func TestQueryRewriting(t *testing.T) {
	b, err := ioutil.ReadFile("query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []QueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	testRewriter := NewQueryRewriter()

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {

			op, err := gqlSchema.Operation(
				&schema.Request{
					Query:     tcase.GQLQuery,
					Variables: tcase.Variables,
				})
			require.NoError(t, err)
			gqlQuery := test.GetQuery(t, op)

			dgQuery, err := testRewriter.Rewrite(context.Background(), gqlQuery)
			require.Nil(t, err)
			require.Equal(t, tcase.DGQuery, dgraph.AsString(dgQuery))
		})
	}
}
