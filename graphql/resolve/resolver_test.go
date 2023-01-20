/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/dgraph-io/dgraph/x"
)

func TestErrorOnIncorrectValueType(t *testing.T) {
	tests := []QueryCase{
		{Name: "return error when object returned instead of scalar value",
			GQLQuery: `query { getAuthor(id: "0x1") { dob } }`,
			Response: `{ "getAuthor": { "dob": {"id": "0x1"} }}`,
			Expected: `{ "getAuthor": { "dob": null }}`,
			Errors: x.GqlErrorList{{
				Message:   schema.ErrExpectedScalar,
				Locations: []x.Location{{Line: 1, Column: 32}},
				Path:      []interface{}{"getAuthor", "dob"},
			}}},

		{Name: "return error when array is returned instead of scalar value",
			GQLQuery: `query { getAuthor(id: "0x1") { dob } }`,
			Response: `{ "getAuthor": { "dob": [{"id": "0x1"}] }}`,
			Expected: `{ "getAuthor": { "dob": null }}`,
			Errors: x.GqlErrorList{{
				Message:   schema.ErrExpectedScalar,
				Locations: []x.Location{{Line: 1, Column: 32}},
				Path:      []interface{}{"getAuthor", "dob"},
			}}},

		{Name: "return error when scalar is returned instead of object value",
			GQLQuery: `query { getAuthor(id: "0x1") { country { name } } }`,
			Response: `{ "getAuthor": { "country": "Rwanda" }}`,
			Expected: `{ "getAuthor": { "country": null }}`,
			Errors: x.GqlErrorList{{
				Message:   "Error coercing value 'Rwanda' for field 'country' to type Country.",
				Locations: []x.Location{{Line: 1, Column: 32}},
				Path:      []interface{}{"getAuthor", "country"},
			}}},
		{Name: "return error when array is returned instead of object value",
			GQLQuery: `query { getAuthor(id: "0x1") { country { name } } }`,
			Response: `{ "getAuthor": { "country": [{"name": "Rwanda"},{"name": "Rwanda"}] }}`,
			Expected: `{ "getAuthor": { "country": null }}`,
			Errors: x.GqlErrorList{{
				Message:   schema.ErrExpectedSingleItem,
				Locations: []x.Location{{Line: 1, Column: 32}},
				Path:      []interface{}{"getAuthor", "country"},
			}}},

		{Name: "return error when scalar is returned instead of array value",
			GQLQuery: `query { getAuthor(id: "0x1") { posts { text } } }`,
			Response: `{ "getAuthor": { "posts": "Rwanda" }}`,
			Expected: `{ "getAuthor": null}`,
			Errors: x.GqlErrorList{{
				Message:   schema.ErrExpectedList,
				Locations: []x.Location{{Line: 1, Column: 32}},
				Path:      []interface{}{"getAuthor"},
			}}},
		{Name: "return error when object is returned instead of array value",
			GQLQuery: `query { getAuthor(id: "0x1") { posts { text } } }`,
			Response: `{ "getAuthor": { "posts": {"text": "Random post"} }}`,
			Expected: `{ "getAuthor": null}`,
			Errors: x.GqlErrorList{{
				Message:   schema.ErrExpectedList,
				Locations: []x.Location{{Line: 1, Column: 32}},
				Path:      []interface{}{"getAuthor"},
			}}},
	}
	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			resp := complete(t, gqlSchema, tcase.GQLQuery, tcase.Response)
			if diff := cmp.Diff(tcase.Errors, resp.Errors); diff != "" {
				t.Errorf("errors mismatch (-want +got):\n%s", diff)
			}

			require.JSONEq(t, tcase.Expected, resp.Data.String())
		})
	}
}

func TestValueCoercion(t *testing.T) {
	tests := []QueryCase{
		// test int/float/bool can be coerced to String
		{Name: "int value should be coerced to string",
			GQLQuery: `query { getAuthor(id: "0x1") { name } }`,
			Response: `{ "getAuthor": { "name": 2 }}`,
			Expected: `{ "getAuthor": { "name": "2"}}`},
		{Name: "float value should be coerced to string",
			GQLQuery: `query { getAuthor(id: "0x1") { name } }`,
			Response: `{ "getAuthor": { "name": 2.134 }}`,
			Expected: `{ "getAuthor": { "name": "2.134"}}`},
		{Name: "bool value should be coerced to string",
			GQLQuery: `query {  getAuthor(id: "0x1") { name} }`,
			Response: `{ "getAuthor": { "name": false } }`,
			Expected: `{ "getAuthor": { "name": "false"}}`},

		// test int/float/bool can be coerced to Enum
		{Name: "int value should raise an error when coerced to postType",
			GQLQuery: `query { getPost(postID: "0x1") { postType } }`,
			Response: `{ "getPost": { "postType": [2] }}`,
			Errors: x.GqlErrorList{{
				Message:   "Error coercing value '2' for field 'postType' to type PostType.",
				Locations: []x.Location{{Line: 1, Column: 34}},
				Path:      []interface{}{"getPost", "postType", 0},
			}},
			Expected: `{ "getPost": { "postType": [null] }}`},
		{Name: "float value should raise error when coerced to postType",
			GQLQuery: `query { getPost(postID: "0x1") { postType } }`,
			Response: `{ "getPost": { "postType": [2.134] }}`,
			Errors: x.GqlErrorList{{
				Message:   "Error coercing value '2.134' for field 'postType' to type PostType.",
				Locations: []x.Location{{Line: 1, Column: 34}},
				Path:      []interface{}{"getPost", "postType", 0},
			}},
			Expected: `{ "getPost": { "postType": [null] }}`},
		{Name: "bool value should raise error when coerced to postType",
			GQLQuery: `query { getPost(postID: "0x1") { postType } }`,
			Response: `{ "getPost": { "postType": [false] }}`,
			Errors: x.GqlErrorList{{
				Message:   "Error coercing value 'false' for field 'postType' to type PostType.",
				Locations: []x.Location{{Line: 1, Column: 34}},
				Path:      []interface{}{"getPost", "postType", 0},
			}},
			Expected: `{ "getPost": { "postType": [null] }}`},
		{Name: "string value should raise error it has invalid enum value",
			GQLQuery: `query { getPost(postID: "0x1") { postType } }`,
			Response: `{ "getPost": { "postType": ["Random"] }}`,
			Errors: x.GqlErrorList{{
				Message:   "Error coercing value 'Random' for field 'postType' to type PostType.",
				Locations: []x.Location{{Line: 1, Column: 34}},
				Path:      []interface{}{"getPost", "postType", 0},
			}},
			Expected: `{ "getPost": { "postType": [null] }}`},
		{Name: "string value should be coerced to valid enum value",
			GQLQuery: `query { getPost(postID: "0x1") { postType } }`,
			Response: `{ "getPost": { "postType": ["Question"] }}`,
			Expected: `{ "getPost": { "postType": ["Question"] }}`},

		// test int/float/string can be coerced to Boolean
		{Name: "int value should be coerced to bool",
			GQLQuery: `query { getPost(postID: "0x1") { isPublished } }`,
			Response: `{ "getPost": { "isPublished": 2 }}`,
			Expected: `{ "getPost": { "isPublished": true}}`},
		{Name: "int value should be coerced to bool with false value for 0",
			GQLQuery: `query { getPost(postID: "0x1") { isPublished } }`,
			Response: `{ "getPost": { "isPublished": 0 }}`,
			Expected: `{ "getPost": { "isPublished": false}}`},
		{Name: "float value should be coerced to bool",
			GQLQuery: `query { getPost(postID: "0x1") { isPublished } }`,
			Response: `{ "getPost": { "isPublished": 2.134 }}`,
			Expected: `{ "getPost": { "isPublished": true}}`},
		{Name: "float value should be coerced to bool with false value for 0",
			GQLQuery: `query { getPost(postID: "0x1") { isPublished } }`,
			Response: `{ "getPost": { "isPublished": 0.000 }}`,
			Expected: `{ "getPost": { "isPublished": false}}`},
		{Name: "string value should be coerced to bool",
			GQLQuery: `query { getPost(postID: "0x1") { isPublished } }`,
			Response: `{ "getPost": { "isPublished": "name" }}`,
			Expected: `{ "getPost": { "isPublished": true }}`},
		{Name: "string value should be coerced to bool false value when empty",
			GQLQuery: `query { getPost(postID: "0x1") { isPublished } }`,
			Response: `{ "getPost": { "isPublished": "" }}`,
			Expected: `{ "getPost": { "isPublished": false }}`},

		// test bool/float/string can be coerced to Int
		{Name: "float value should be coerced to int",
			GQLQuery: `query { getPost(postID: "0x1") { numLikes } }`,
			Response: `{ "getPost": { "numLikes": 2.000 }}`,
			Expected: `{ "getPost": { "numLikes": 2}}`},
		{Name: "string value should be coerced to int",
			GQLQuery: `query { getPost(postID: "0x1") { numLikes } }`,
			Response: `{ "getPost": { "numLikes": "23" }}`,
			Expected: `{ "getPost": { "numLikes": 23}}`},
		{Name: "string float value should be coerced to int",
			GQLQuery: `query { getPost(postID: "0x1") { numLikes } }`,
			Response: `{ "getPost": { "numLikes": "23.00" }}`,
			Expected: `{ "getPost": { "numLikes": 23}}`},
		{Name: "bool true value should be coerced to int value 1",
			GQLQuery: `query { getPost(postID: "0x1") { numLikes } }`,
			Response: `{ "getPost": { "numLikes": true }}`,
			Expected: `{ "getPost": { "numLikes": 1}}`},
		{Name: "bool false value should be coerced to int",
			GQLQuery: `query { getPost(postID: "0x1") { numLikes } }`,
			Response: `{ "getPost": { "numLikes": false }}`,
			Expected: `{ "getPost": { "numLikes": 0}}`},
		{Name: "field should return an error when it is greater than int32" +
			" without losing data",
			GQLQuery: `query { getPost(postID: "0x1") { numLikes } }`,
			Response: `{ "getPost": { "numLikes": 2147483648 }}`,
			Errors: x.GqlErrorList{{
				Message: "Error coercing value '2147483648' for field 'numLikes' to type" +
					" Int.",
				Locations: []x.Location{{Line: 1, Column: 34}},
				Path:      []interface{}{"getPost", "numLikes"},
			}},
			Expected: `{"getPost": {"numLikes": null}}`,
		},
		{Name: "field should return an error when float can't be coerced to int" +
			" without losing data",
			GQLQuery: `query { getPost(postID: "0x1") { numLikes } }`,
			Response: `{ "getPost": { "numLikes": 123.23 }}`,
			Errors: x.GqlErrorList{{
				Message:   "Error coercing value '123.23' for field 'numLikes' to type Int.",
				Locations: []x.Location{{Line: 1, Column: 34}},
				Path:      []interface{}{"getPost", "numLikes"},
			}},
			Expected: `{"getPost": {"numLikes": null}}`,
		},
		{Name: "field should return an error when when it can't be coerced as int32" +
			" from a string",
			GQLQuery: `query { getPost(postID: "0x1") { numLikes } }`,
			Response: `{ "getPost": { "numLikes": "123.23" }}`,
			Errors: x.GqlErrorList{{
				Message:   "Error coercing value '123.23' for field 'numLikes' to type Int.",
				Locations: []x.Location{{Line: 1, Column: 34}},
				Path:      []interface{}{"getPost", "numLikes"},
			}},
			Expected: `{"getPost": {"numLikes": null}}`,
		},

		// test bool/int/string can be coerced to Float
		{Name: "int value should be coerced to float",
			GQLQuery: `query { getAuthor(id: "0x1") { reputation } }`,
			Response: `{ "getAuthor": { "reputation": 2 }}`,
			Expected: `{ "getAuthor": { "reputation": 2.0 }}`},
		{Name: "string value should be coerced to float",
			GQLQuery: `query { getAuthor(id: "0x1") { reputation } }`,
			Response: `{ "getAuthor": { "reputation": "23.123" }}`,
			Expected: `{ "getAuthor": { "reputation": 23.123 }}`},
		{Name: "bool true value should be coerced to float value 1.0",
			GQLQuery: `query { getAuthor(id: "0x1") { reputation } }`,
			Response: `{ "getAuthor": { "reputation": true }}`,
			Expected: `{ "getAuthor": { "reputation": 1.0 }}`},
		{Name: "bool false value should be coerced to float value 0.0",
			GQLQuery: `query { getAuthor(id: "0x1") { reputation } }`,
			Response: `{ "getAuthor": { "reputation": false }}`,
			Expected: `{ "getAuthor": { "reputation": 0.0}}`},

		// test bool/int/string/datetime can be coerced to Datetime
		{Name: "float value should raise an error when tried to be coerced to datetime",
			GQLQuery: `query { getAuthor(id: "0x1") { dob } }`,
			Response: `{ "getAuthor": { "dob": "23.123" }}`,
			Errors: x.GqlErrorList{{
				Message:   "Error coercing value '23.123' for field 'dob' to type DateTime.",
				Locations: []x.Location{{Line: 1, Column: 32}},
				Path:      []interface{}{"getAuthor", "dob"},
			}},
			Expected: `{ "getAuthor": { "dob": null }}`},
		{Name: "bool value should raise an error when coerced as datetime",
			GQLQuery: `query { getAuthor(id: "0x1") { dob } }`,
			Response: `{ "getAuthor": { "dob": true }}`,
			Errors: x.GqlErrorList{{
				Message:   "Error coercing value 'true' for field 'dob' to type DateTime.",
				Locations: []x.Location{{Line: 1, Column: 32}},
				Path:      []interface{}{"getAuthor", "dob"},
			}},
			Expected: `{ "getAuthor": { "dob": null }}`},
		{Name: "invalid string value should raise an error when tried to be coerced to datetime",
			GQLQuery: `query { getAuthor(id: "0x1") { dob } }`,
			Response: `{ "getAuthor": { "dob": "123" }}`,
			Errors: x.GqlErrorList{{
				Message:   "Error coercing value '123' for field 'dob' to type DateTime.",
				Locations: []x.Location{{Line: 1, Column: 32}},
				Path:      []interface{}{"getAuthor", "dob"},
			}},
			Expected: `{ "getAuthor": { "dob": null}}`},
		{Name: "int value should be coerced to datetime",
			GQLQuery: `query { getAuthor(id: "0x1") { dob } }`,
			Response: `{ "getAuthor": { "dob": 2 }}`,
			Expected: `{ "getAuthor": { "dob": "1970-01-01T00:00:02Z"}}`},
		{Name: "float value that can be truncated safely should be coerced to datetime",
			GQLQuery: `query { getAuthor(id: "0x1") { dob } }`,
			Response: `{ "getAuthor": { "dob": 2.0 }}`,
			Expected: `{ "getAuthor": { "dob": "1970-01-01T00:00:02Z"}}`},
		{Name: "val string value should be coerced to datetime",
			GQLQuery: `query { getAuthor(id: "0x1") { dob } }`,
			Response: `{ "getAuthor": { "dob": "2012-11-01T22:08:41+05:30" }}`,
			Expected: `{ "getAuthor": { "dob": "2012-11-01T22:08:41+05:30" }}`},
	}

	gqlSchema := test.LoadSchemaFromFile(t, "schema.graphql")

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			resp := complete(t, gqlSchema, tcase.GQLQuery, tcase.Response)
			if diff := cmp.Diff(tcase.Errors, resp.Errors); diff != "" {
				t.Errorf("errors mismatch (-want +got):\n%s", diff)
			}

			require.JSONEq(t, tcase.Expected, resp.Data.String())
		})
	}
}

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

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			resp := complete(t, gqlSchema, tcase.GQLQuery, tcase.Response)

			require.Nil(t, resp.Errors)
			require.JSONEq(t, tcase.Expected, resp.Data.String())
		})
	}
}

func TestMutationAlias(t *testing.T) {
	t.Skipf("TODO(abhimanyu): port it to e2e")
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
					existenceQueriesResp: `{ "Author_1": [{"uid":"0x1"}]}`,
					resp:                 tcase.queryResponse,
					assigned:             tcase.mutResponse,
					result:               tcase.mutQryResp,
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
			Expected: `{"getAuthor":{"name":"A.N. Author","dob":"2000-01-01T00:00:00Z",` +
				`"postsNullable":[` +
				`{"title":"A Title","text":"Some Text"},` +
				`{"title":"Another Title","text":"More Text"}]}}`},
		{Name: "Response is in same order as GQL query no matter Dgraph order",
			GQLQuery: query,
			Response: `{ "getAuthor": [ { "dob": "2000-01-01", "name": "A.N. Author", ` +
				`"postsNullable": [ ` +
				`{ "text": "Some Text", "title": "A Title" }, ` +
				`{ "title": "Another Title", "text": "More Text" } ] } ] }`,
			Expected: `{"getAuthor":{"name":"A.N. Author","dob":"2000-01-01T00:00:00Z",` +
				`"postsNullable":[` +
				`{"title":"A Title","text":"Some Text"},` +
				`{"title":"Another Title","text":"More Text"}]}}`},
		{Name: "Inserted null is in GQL query order",
			GQLQuery: query,
			Response: `{ "getAuthor": [ { "name": "A.N. Author", ` +
				`"postsNullable": [ ` +
				`{ "title": "A Title" }, ` +
				`{ "title": "Another Title", "text": "More Text" } ] } ] }`,
			Expected: `{"getAuthor":{"name":"A.N. Author","dob":null,` +
				`"postsNullable":[` +
				`{"title":"A Title","text":null},` +
				`{"title":"Another Title","text":"More Text"}]}}`},
		// TODO(abhimanyu): add e2e for following test
		{Name: "Whole operation GQL query order",
			GQLQuery: `query { ` +
				`getAuthor(id: "0x1") { name }` +
				`getPost(id: "0x2") { title } }`,
			Response: `{ "getAuthor": [ { "name": "A.N. Author" } ],` +
				`"getPost": [ { "title": "A Post" } ] }`,
			Expected: `{"getAuthor":{"name":"A.N. Author"},` +
				`"getPost":{"title":"A Post"}}`},
	}

	gqlSchema := test.LoadSchemaFromString(t, testGQLSchema)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			resp := complete(t, gqlSchema, tcase.GQLQuery, tcase.Response)

			require.Nil(t, resp.Errors)
			require.Equal(t, tcase.Expected, resp.Data.String())
		})
	}
}
