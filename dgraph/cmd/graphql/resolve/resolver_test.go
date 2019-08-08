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
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
	"gopkg.in/yaml.v2"
)

// Tests that result completion and GraphQL error propagation are working properly.

// All the tests work on a mocked json response, rather than a running Dgraph.
// It's better to mock the Dgraph client interface in these tests and have cases
// where one can directly see the json response and how it gets modified, than
// to try and orchestrate conditions for all these complicated tests in a live
// Dgraph instance.  Done on a real Dgraph, you also can't see the responses
// to see what the test is actually doing.

type dgraphClient struct {
	resp string
}

type QueryCase struct {
	Name        string
	GQLQuery    string
	Explanation string
	Response    string // Dgraph json response
	Expected    string // Expected data from Resolve()
	Errors      gqlerror.List
}

var testGQLSchema = `
scalar ID
scalar String
scalar DateTime
scalar Int

type Author {
	id: ID!
	name: String!
	dob: DateTime
	postsRequired: [Post!]!	
	postsElmntRequired: [Post!]	
	postsNullable: [Post]
	postsNullableListRequired: [Post]!
}

type Post {
	id: ID!
	title: String!
	text: String
	numViews: Int!
	authorRequired: Author!
	authorNullable: Author
}

type Query {
	getAuthor(id: ID!): Author
	getAuthorRequired(id: ID!): Author!

	queryPosts: [Post]
	queryPostsRequired: [Post!]!
}
`

func (dg *dgraphClient) Query(ctx context.Context, query *dgraph.QueryBuilder) ([]byte, error) {
	return []byte(dg.resp), nil
}

func (dg *dgraphClient) Mutate(ctx context.Context, val interface{}) (map[string]string, error) {
	// To be filled in for mutate tests
	return nil, nil
}

func (dg *dgraphClient) DeleteNode(ctx context.Context, uid uint64) error {
	// Not needed in testing responses
	return nil
}

func (dg *dgraphClient) AssertType(ctx context.Context, uid uint64, typ string) error {
	// To be filled in for mutate tests
	return nil
}

// Tests in resolver_test.yaml are about what gets into a completed result (addition
// of "null", errors and error propagation).  Exact JSON result (e.g. order) doesn't
// matter here - that makes for easier to format and read tests for these many cases.
//
// The []bytes built by Resolve() have some other properties, such as ordering of
// fields, which are tested by TestResponseOrder().
func TestResolver(t *testing.T) {
	b, err := ioutil.ReadFile("resolver_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []QueryCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: testGQLSchema})
	require.Nil(t, gqlErr)
	// ^^ We can't use no error here because gqlErr is of type *gqlerror.Error,
	// so passing into something that just expects an error, will always be a
	// non-nil interface.

	gqlSchema, gqlErr := validator.ValidateSchemaDocument(doc)
	require.Nil(t, gqlErr)

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			resp := resolve(gqlSchema, test.GQLQuery, test.Response)

			require.Equal(t, test.Errors, resp.Errors)
			require.JSONEq(t, test.Expected, resp.Data.String(), test.Explanation)
		})
	}
}

// Ordering of results and inserted null values matters in GraphQL:
// https://graphql.github.io/graphql-spec/June2018/#sec-Serialized-Map-Ordering
func TestResponseOrder(t *testing.T) {
	query := `query {
		getAuthor(id: "0x1") {
			name
			dob
			postsNullable {
				title
				text
			}
		}
	}`

	tests := []QueryCase{
		{Name: "Response is in same order as GQL query",
			GQLQuery: query,
			Response: `{ "getAuthor": [ { "name": "A.N. Author", "dob": "2000-01-01", ` +
				`"postsNullable": [ ` +
				`{ "title": "A Title", "text": "Some Text" }, ` +
				`{ "title": "Another Title", "text": "More Text" } ] } ] }`,
			Expected: `{"getAuthor": {"name": "A.N. Author", "dob": "2000-01-01", ` +
				`"postsNullable": [` +
				`{"title": "A Title", "text": "Some Text"}, ` +
				`{"title": "Another Title", "text": "More Text"}]}}`},
		{Name: "Response is in same order as GQL query no matter Dgraph order",
			GQLQuery: query,
			Response: `{ "getAuthor": [ { "dob": "2000-01-01", "name": "A.N. Author", ` +
				`"postsNullable": [ ` +
				`{ "text": "Some Text", "title": "A Title" }, ` +
				`{ "title": "Another Title", "text": "More Text" } ] } ] }`,
			Expected: `{"getAuthor": {"name": "A.N. Author", "dob": "2000-01-01", ` +
				`"postsNullable": [` +
				`{"title": "A Title", "text": "Some Text"}, ` +
				`{"title": "Another Title", "text": "More Text"}]}}`},
		{Name: "Inserted null is in GQL query order",
			GQLQuery: query,
			Response: `{ "getAuthor": [ { "name": "A.N. Author", ` +
				`"postsNullable": [ ` +
				`{ "title": "A Title" }, ` +
				`{ "title": "Another Title", "text": "More Text" } ] } ] }`,
			Expected: `{"getAuthor": {"name": "A.N. Author", "dob": null, ` +
				`"postsNullable": [` +
				`{"title": "A Title", "text": null}, ` +
				`{"title": "Another Title", "text": "More Text"}]}}`},
	}

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: testGQLSchema})
	require.Nil(t, gqlErr)

	gqlSchema, gqlErr := validator.ValidateSchemaDocument(doc)
	require.Nil(t, gqlErr)

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			resp := resolve(gqlSchema, test.GQLQuery, test.Response)

			require.Nil(t, resp.Errors)
			require.Equal(t, test.Expected, resp.Data.String())
		})
	}
}

func resolve(gqlSchema *ast.Schema, gqlQuery string, dgResponse string) *schema.Response {
	client := &dgraphClient{resp: dgResponse}
	resolver := New(schema.AsSchema(gqlSchema), client)
	resolver.GqlReq = &schema.Request{Query: gqlQuery}
	return resolver.Resolve(context.Background())
}
