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
	"testing"

	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/stretchr/testify/require"
)

func TestQueryAlias(t *testing.T) {
	tests := []QueryCase{
		{Name: "top level alias",
			GQLQuery: `query { auth : getAuthor(id: "0x1") { name } }`,
			Response: `{ "getAuthor": [ { "name": "A.N. Author" } ] }`,
			Expected: `{"auth": {"name": "A.N. Author"}}`},
		{Name: "field level alias",
			GQLQuery: `query { getAuthor(id: "0x1") { authName: name } }`,
			Response: `{ "getAuthor": [ { "name": "A.N. Author" } ] }`,
			Expected: `{"getAuthor": {"authName": "A.N. Author"}}`},
		{Name: "deep alias",
			GQLQuery: `query { getAuthor(id: "0x1") { name posts { theTitle : title } } }`,
			Response: `{"getAuthor": [{"name": "A.N. Author", "posts": [{"title": "A Post"}]}]}`,
			Expected: `{"getAuthor": {"name": "A.N. Author", "posts": [{"theTitle": "A Post"}]}}`},
		{Name: "many aliases",
			GQLQuery: `query { 
				auth : getAuthor(id: "0x1") { name myPosts : posts { theTitle : title } }
				post : getPost(postID: "0x2") { postTitle: title } }`,
			Response: `{ 
				"getAuthor": [{"name": "A.N. Author", "posts": [{"title": "A Post"}]}],
				"getPost": [ { "title": "A Post" } ] }`,
			Expected: `{"auth": {"name": "A.N. Author", "myPosts": [{"theTitle": "A Post"}]},` +
				`"post": {"postTitle": "A Post"}}`},
	}

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			resp := resolve(gqlSchema, test.GQLQuery, test.Response)

			require.Nil(t, resp.Errors)
			require.JSONEq(t, test.Expected, resp.Data.String())
		})
	}
}

func TestMutationAlias(t *testing.T) {

	tests := map[string]struct {
		gqlQuery      string
		mutResponse   map[string]string
		mutQryResp    map[string]interface{}
		queryResponse string
		expected      string
	}{
		"mutation top level alias ": {
			gqlQuery: `mutation {
				add: addPost(input: [{title: "A Post", text: "Some text", author: {id: "0x1"}}]) {
					post { title }
				}
			}`,
			mutResponse: map[string]string{"Post1": "0x2"},
			mutQryResp: map[string]interface{}{
				"Author2": []interface{}{map[string]string{"uid": "0x1"}}},
			queryResponse: `{ "post" : [ { "title": "A Post" } ] }`,
			expected:      `{ "add": { "post" : [{ "title": "A Post"}] } }`,
		},
		"mutation deep alias ": {
			gqlQuery: `mutation {
				addPost(input: [{title: "A Post", text: "Some text", author: {id: "0x1"}}]) {
					thePosts : post { postTitle : title }
				}
			}`,
			mutResponse: map[string]string{"Post1": "0x2"},
			mutQryResp: map[string]interface{}{
				"Author2": []interface{}{map[string]string{"uid": "0x1"}}},
			queryResponse: `{ "post" : [ { "title": "A Post" } ] }`,
			expected:      `{ "addPost": { "thePosts" : [{ "postTitle": "A Post"}] } }`,
		},
		"mutation many aliases ": {
			gqlQuery: `mutation {
				add1: addPost(input: [{title: "A Post", text: "Some text", author: {id: "0x1"}}]) {
					thePosts : post { postTitle : title }
				}
				add2: addPost(input: [{title: "A Post", text: "Some text", author: {id: "0x1"}}]) {
					otherPosts : post { t : title }
				}
			}`,
			mutResponse: map[string]string{"Post1": "0x2"},
			mutQryResp: map[string]interface{}{
				"Author2": []interface{}{map[string]string{"uid": "0x1"}}},
			queryResponse: `{ "post" : [ { "title": "A Post" } ] }`,
			expected: `{ 
				"add1": { "thePosts" : [{ "postTitle": "A Post"}] },
				"add2": { "otherPosts" : [{ "t": "A Post"}] } }`,
		},
	}

	gqlSchema := test.LoadSchemaFromString(t, testGQLSchema)

	for name, tcase := range tests {
		t.Run(name, func(t *testing.T) {
			resp := resolveWithClient(gqlSchema, tcase.gqlQuery, nil,
				&executor{
					resp:     tcase.queryResponse,
					assigned: tcase.mutResponse,
					result:   tcase.mutQryResp,
				})

			require.Nil(t, resp.Errors)
			require.JSONEq(t, tcase.expected, resp.Data.String())
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
		{Name: "Whole operation GQL query order",
			GQLQuery: `query { ` +
				`getAuthor(id: "0x1") { name }` +
				`getPost(id: "0x2") { title } }`,
			Response: `{ "getAuthor": [ { "name": "A.N. Author" } ],` +
				`"getPost": [ { "title": "A Post" } ] }`,
			Expected: `{"getAuthor": {"name": "A.N. Author"},` +
				`"getPost": {"title": "A Post"}}`},
	}

	gqlSchema := test.LoadSchemaFromString(t, testGQLSchema)

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			resp := resolve(gqlSchema, test.GQLQuery, test.Response)

			require.Nil(t, resp.Errors)
			require.Equal(t, test.Expected, resp.Data.String())
		})
	}
}
