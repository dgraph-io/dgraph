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

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/test"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// Tests showing that the query pipeline produces the expected Dgraph queries

type QueryRewritingCase struct {
	Name     string
	GQLQuery string
	DGQuery  string
}

// A queryRecorder mocs the Dgraph interface under the resolver and just records
// the query it gets
type queryRecorder struct {
	query string
}

func (qr *queryRecorder) Query(ctx context.Context, qb *dgraph.QueryBuilder) ([]byte, error) {
	var err error
	qr.query, err = qb.AsQueryString()
	return []byte("{}"), err
}

func (qr *queryRecorder) Mutate(ctx context.Context, val interface{}) (map[string]string, error) {
	return map[string]string{"newnode": "0x4"}, nil
}

func (qr *queryRecorder) DeleteNode(ctx context.Context, uid uint64) error {
	return nil
}

func (qr *queryRecorder) AssertType(ctx context.Context, uid uint64, typ string) error {
	return nil
}

func TestQueryRewriting(t *testing.T) {
	b, err := ioutil.ReadFile("resolver_query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []QueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchema(t, testGQLSchema)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			client := &queryRecorder{}
			resp := resolveWithClient(gqlSchema, tcase.GQLQuery, client)

			require.Nil(t, resp.Errors)
			require.Equal(t, tcase.DGQuery, client.query)
		})
	}
}
