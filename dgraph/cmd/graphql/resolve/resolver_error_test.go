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
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/test"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/gqlerror"
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
	resp     string
	assigned map[string]string
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
scalar DateTime

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
	author: Author!
}

type Query {
	getAuthor(id: ID!): Author
}

input AuthorRef {
    id: ID!
}

input PostInput {
	title: String!
	text: String
	author: AuthorRef!
}

input UpdatePostInput {
	id: ID!
	patch: PatchPost!
}

input PatchPost {
	title: String
	text: String
}

type AddPostPayload {
    post: Post
}

type UpdatePostPayload {
    post: Post
}

type Mutation {
	addPost(input: PostInput!): AddPostPayload
	updatePost(input: UpdatePostInput!): UpdatePostPayload
}
`

func (dg *dgraphClient) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	return []byte(dg.resp), nil
}

func (dg *dgraphClient) Mutate(ctx context.Context, val interface{}) (map[string]string, error) {
	return dg.assigned, nil
}

func (dg *dgraphClient) DeleteNodes(ctx context.Context, query, mutation string) error {
	// Not needed in testing responses
	return nil
}

func (dg *dgraphClient) AssertType(ctx context.Context, uid uint64, typ string) error {
	return nil
}

// Tests in resolver_test.yaml are about what gets into a completed result (addition
// of "null", errors and error propagation).  Exact JSON result (e.g. order) doesn't
// matter here - that makes for easier to format and read tests for these many cases.
//
// The []bytes built by Resolve() have some other properties, such as ordering of
// fields, which are tested by TestResponseOrder().
func TestResolver(t *testing.T) {
	b, err := ioutil.ReadFile("resolver_error_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []QueryCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchema(t, testGQLSchema)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			resp := resolve(gqlSchema, tcase.GQLQuery, tcase.Response)

			test.RequireJSONEq(t, tcase.Errors, resp.Errors)
			require.JSONEq(t, tcase.Expected, resp.Data.String(), tcase.Explanation)
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

	gqlSchema := test.LoadSchema(t, testGQLSchema)

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			resp := resolve(gqlSchema, test.GQLQuery, test.Response)

			require.Nil(t, resp.Errors)
			require.Equal(t, test.Expected, resp.Data.String())
		})
	}
}

// For add and update mutations, we don't need to re-test all the cases from the
// query tests.  So just test enough to demonstrate that we'll catch it if we were
// to delete the call to completeDgraphResult before adding to the response.
func TestAddMutationUsesErrorPropagation(t *testing.T) {
	mutation := `mutation {
		addPost(input: {title: "A Post", text: "Some text", author: {id: "0x1"}}) {
			post {
				title
				text
				author {
					name
					dob
				}
			}
		}
	}`

	tests := map[string]struct {
		explanation   string
		mutResponse   map[string]string
		queryResponse string
		expected      string
		errors        gqlerror.List
	}{
		"Add mutation adds missing nullable fields": {
			explanation: "Field 'dob' is nullable, so null should be inserted " +
				"if the mutation's query doesn't return a value.",
			mutResponse: map[string]string{"newnode": "0x1"},
			queryResponse: `{ "post" : [
				{ "title": "A Post",
				"text": "Some text",
				"author": { "name": "A.N. Author" } } ] }`,
			expected: `{ "addPost": { "post" :
				{ "title": "A Post",
				"text": "Some text",
				"author": { "name": "A.N. Author", "dob": null } } } }`,
		},
		"Add mutation triggers GraphQL error propagation": {
			explanation: "An Author's name is non-nullable, so if that's missing, " +
				"the author is squashed to null, but that's also non-nullable, so the " +
				"propagates to the query root.",
			mutResponse: map[string]string{"newnode": "0x1"},
			queryResponse: `{ "post" : [
				{ "title": "A Post",
				"text": "Some text",
				"author": { "dob": "2000-01-01" } } ] }`,
			expected: `{ "addPost": { "post" : null } }`,
			errors: gqlerror.List{&gqlerror.Error{
				Message: `Non-nullable field 'name' (type String!) ` +
					`was not present in result from Dgraph.  GraphQL error propagation triggered.`,
				Locations: []gqlerror.Location{{Column: 6, Line: 7}},
				Path:      []interface{}{"post", "author", "name"}}},
		},
	}

	gqlSchema := test.LoadSchema(t, testGQLSchema)

	for name, tcase := range tests {
		t.Run(name, func(t *testing.T) {
			resp := resolveWithClient(gqlSchema, mutation,
				&dgraphClient{resp: tcase.queryResponse, assigned: tcase.mutResponse})

			test.RequireJSONEq(t, tcase.errors, resp.Errors)
			require.JSONEq(t, tcase.expected, resp.Data.String(), tcase.explanation)
		})
	}
}

func TestUpdateMutationUsesErrorPropagation(t *testing.T) {
	mutation := `mutation {
		updatePost(input: { id: "0x1", patch: { text: "Some more text" } }) {
			post {
				title
				text
				author {
					name
					dob
				}
			}
		}
	}`

	tests := map[string]struct {
		explanation   string
		mutResponse   map[string]string
		queryResponse string
		expected      string
		errors        gqlerror.List
	}{
		"Update Mutation adds missing nullable fields": {
			explanation: "Field 'dob' is nullable, so null should be inserted " +
				"if the mutation's query doesn't return a value.",
			mutResponse: map[string]string{"newnode": "0x1"},
			queryResponse: `{ "post" : [
				{ "title": "A Post",
				"text": "Some text",
				"author": { "name": "A.N. Author" } } ] }`,
			expected: `{ "updatePost": { "post" :
				{ "title": "A Post",
				"text": "Some text",
				"author": { "name": "A.N. Author", "dob": null } } } }`,
		},
		"Update Mutation triggers GraphQL error propagation": {
			explanation: "An Author's name is non-nullable, so if that's missing, " +
				"the author is squashed to null, but that's also non-nullable, so the " +
				"propagates to the query root.",
			mutResponse: map[string]string{"newnode": "0x1"},
			queryResponse: `{ "post" : [ {
				"title": "A Post",
				"text": "Some text",
				"author": { "dob": "2000-01-01" } } ] }`,
			expected: `{ "updatePost": { "post" : null } }`,
			errors: gqlerror.List{&gqlerror.Error{
				Message: `Non-nullable field 'name' (type String!) ` +
					`was not present in result from Dgraph.  GraphQL error propagation triggered.`,
				Locations: []gqlerror.Location{{Column: 6, Line: 7}},
				Path:      []interface{}{"post", "author", "name"}}},
		},
	}

	gqlSchema := test.LoadSchema(t, testGQLSchema)

	for name, tcase := range tests {
		t.Run(name, func(t *testing.T) {
			resp := resolveWithClient(gqlSchema, mutation,
				&dgraphClient{resp: tcase.queryResponse, assigned: tcase.mutResponse})

			test.RequireJSONEq(t, tcase.errors, resp.Errors)
			require.JSONEq(t, tcase.expected, resp.Data.String(), tcase.explanation)
		})
	}
}

func resolve(gqlSchema schema.Schema, gqlQuery string, dgResponse string) *schema.Response {
	return resolveWithClient(gqlSchema, gqlQuery, &dgraphClient{resp: dgResponse})
}

func resolveWithClient(
	gqlSchema schema.Schema,
	gqlQuery string,
	client dgraph.Client) *schema.Response {
	resolver := New(
		gqlSchema,
		client,
		dgraph.NewQueryRewriter(),
		dgraph.NewMutationRewriter())
	resolver.GqlReq = &schema.Request{Query: gqlQuery}
	return resolver.Resolve(context.Background())
}
