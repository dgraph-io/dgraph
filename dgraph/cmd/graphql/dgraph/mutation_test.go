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

var testGQLSchema = `
scalar ID
scalar String
scalar DateTime

directive @hasInverse(field: String!) on FIELD_DEFINITION

type Country {
	id: ID!
	name: String!	
}

type Author {
	id: ID!
	name: String!
	dob: DateTime
	country: Country
	posts: [Post!] @hasInverse(field: "author")
}

type Post {
	postID: ID!
	title: String!
	text: String
	author: Author! @hasInverse(field: "posts")
}

type Mutation {
	addAuthor(input: AuthorInput!): AddAuthorPayload

	addPost(input: PostInput!): AddPostPayload
	updatePost(input: UpdatePostInput!): UpdatePostPayload
}

input AuthorInput {
	name: String!
	dob: DateTime
	country: CountryRef
	posts: [PostRef!]
}

type AddAuthorPayload {
    author: Author
}

input AuthorRef {
    id: ID!
}

input CountryRef {
    id: ID!
}

input PostInput {
	title: String!
	text: String
	author: AuthorRef!
}

input PostRef {
    postID: ID!
}

type AddPostPayload {
    post: Post
}

input PatchPost {
	title: String
	text: String
}

input UpdatePostInput {
	postID: ID!
	patch: PatchPost!
}

type UpdatePostPayload {
    post: Post
}
`

func TestMutationRewriting(t *testing.T) {
	b, err := ioutil.ReadFile("mutation_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []TestCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := schema.AsSchema(test.LoadSchema(t, testGQLSchema))

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
