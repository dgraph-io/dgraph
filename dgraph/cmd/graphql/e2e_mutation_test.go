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

package graphql

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/test"
	"github.com/stretchr/testify/require"
)

// TestAddMutation tests that add mutations work as expected.  There's a few angles
// that need testing:
// - add single object,
// - add object with reference to existing object, and
// - add where @hasInverse edges need linking.
//
// These really need to run as one test because the created uid from the Country
// needs to flow to the author, etc.
func TestAddMutation(t *testing.T) {
	var countryUID, authorUID string

	// Add single object :
	// Country is a single object not linked to anything else.
	// So only need to check that it gets added as expected.
	t.Run("add Country", func(t *testing.T) {
		countryUID = addCountry(t)

		t.Run("get Country", func(t *testing.T) {
			// addCountry() asserts that the mutation response was as expected.
			// Let's also check that what's in the DB is what we expect.
			getCountry(t, countryUID)
		})
	})

	// Add object with reference to existing object :
	// An Author links to an existing country.  So need to check that the author
	// was added and that it has the link to the right Country.
	t.Run("add Author", func(t *testing.T) {
		authorUID = addAuthor(t, countryUID)

		t.Run("get Author", func(t *testing.T) {
			getAuthor(t, authorUID, countryUID)
		})
	})

	// Add with @hasInverse :
	// Posts link to an Author and the Author has a link back to all their Posts.
	// So need to check that the Post was added to the right Author
	// AND that the Author's posts now includes the new post.
	t.Run("add Post", func(t *testing.T) {
		postUID := addPost(t, authorUID)

		t.Run("get Post", func(t *testing.T) {
			getPost(t, postUID, authorUID, countryUID)
		})
	})
}

func addCountry(t *testing.T) string {
	addCountryParams := &GraphQLParams{
		Query: `mutation addCountry($name: String!) {
			addCountry(input: { name: $name }) {
				country {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"name": "Testland"},
	}
	addCountryExpected := `
		{ "data": { "addCountry": { "country": { "id": "_UID_", "name": "Testland" } } } }`

	resp, err := addCountryParams.ExecuteAsPost(graphqlURL)
	require.NoError(t, err)

	replacedJSON, err := test.ReplaceJSON(resp,
		map[string]interface{}{"id": "_UID_", "requestID": "requestID"})
	require.JSONEq(t, addCountryExpected, string(replacedJSON))

	// Because the JSONEq ^^ passed we know the structure of resp an can just grab
	// out the created ID.
	var respJSON map[string]interface{}
	err = json.Unmarshal(resp, &respJSON)
	require.NoError(t, err)

	country :=
		respJSON["data"].(map[string]interface{})["addCountry"].(map[string]interface{})["country"]
	return country.(map[string]interface{})["id"].(string)
}

func getCountry(t *testing.T, uid string) {
	getCountryParams := &GraphQLParams{
		Query: `query getCountry($createdID: ID!) {
			getCountry(id: $createdID) {
				id
				name
			}
		}`,
		Variables: map[string]interface{}{"createdID": uid},
	}
	getCountryExpected := `{ "data": { "getCountry": { "id": "_UID_", "name": "Testland" } } }`

	resp, err := getCountryParams.ExecuteAsPost(graphqlURL)
	require.NoError(t, err)

	require.JSONEq(t,
		strings.ReplaceAll(getCountryExpected, "_UID_", uid),
		string(resp))
}

func addAuthor(t *testing.T, countryUID string) string {
	addAuthorParams := &GraphQLParams{
		Query: `mutation addAuthor($author: AuthorInput!) {
			addAuthor(input: $author) {
			  	author {
					id
					name
					dob
					reputation
					country {
						countryID : id
				  		name
					}
			  	}
			}
		}`,
		Variables: map[string]interface{}{"author": map[string]interface{}{
			"name":       "Test Author",
			"dob":        "2010-01-01T05:04:33Z",
			"reputation": 7.75,
			"country":    map[string]interface{}{"id": countryUID},
		}},
	}

	addAuthorExpected := fmt.Sprintf(`{ "data": { "addAuthor": { 
		"author": { 
			"id": "_UID_", 
			"name": "Test Author", 
			"dob": "2010-01-01T05:04:33Z",
			"reputation": 7.75,
			"country": {
				"countryID": "%s",
				"name": "Testland"
			}
		} 
	} } }`, countryUID)

	resp, err := addAuthorParams.ExecuteAsPost(graphqlURL)
	require.NoError(t, err)

	replacedJSON, err := test.ReplaceJSON(resp,
		map[string]interface{}{"id": "_UID_", "requestID": "requestID"})
	require.JSONEq(t, addAuthorExpected, string(replacedJSON))

	// Because the JSONEq ^^ passed we know the structure of resp an can just grab
	// out the created ID.
	var respJSON map[string]interface{}
	err = json.Unmarshal(resp, &respJSON)
	require.NoError(t, err)

	auth :=
		respJSON["data"].(map[string]interface{})["addAuthor"].(map[string]interface{})["author"]
	return auth.(map[string]interface{})["id"].(string)
}

func getAuthor(t *testing.T, authorUID, countryUID string) {
	getAuthorParams := &GraphQLParams{
		Query: `query getAuthor($createdID: ID!) {
			getAuthor(id: $createdID) {
				id
				name
				dob
				reputation
				country {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"createdID": authorUID},
	}
	getAuthorExpected := fmt.Sprintf(`{ "data": {
		"getAuthor": { 
			"id": "%s", 
			"name": "Test Author", 
			"dob": "2010-01-01T05:04:33Z",
			"reputation": 7.75,
			"country": {
				"id": "%s",
				"name": "Testland"
			}
		} 
	} }`, authorUID, countryUID)

	resp, err := getAuthorParams.ExecuteAsPost(graphqlURL)
	require.NoError(t, err)

	require.JSONEq(t, getAuthorExpected, string(resp))
}

func addPost(t *testing.T, authorUID string) string {
	addPostParams := &GraphQLParams{
		Query: `mutation addPost($post: PostInput!) {
			addPost(input: $post) {
			  post {
				postID
				title
				text
				isPublished
				tags
				author {
					id
					name
				}
			  }
			}
		}`,
		Variables: map[string]interface{}{"post": map[string]interface{}{
			"title":       "Test Post",
			"text":        "This post is just a test.",
			"isPublished": true,
			"tags":        []string{"example", "test"},
			"author":      map[string]interface{}{"id": authorUID},
		}},
	}

	addPostExpected := fmt.Sprintf(`{ "data": { "addPost": { 
		"post": { 
			"postID": "_UID_", 
			"title": "Test Post", 
			"text": "This post is just a test.",
			"isPublished": true,
			"tags": ["example", "test"],
			"author": {
				"id": "%s",
				"name": "Test Author"
			}
		} 
	} } }`, authorUID)

	resp, err := addPostParams.ExecuteAsPost(graphqlURL)
	require.NoError(t, err)

	replacedJSON, err := test.ReplaceJSON(resp,
		map[string]interface{}{"postID": "_UID_", "requestID": "requestID"})
	require.JSONEq(t, addPostExpected, string(replacedJSON))

	// Because the JSONEq ^^ passed we know the structure of resp an can just grab
	// out the created ID.
	var respJSON map[string]interface{}
	err = json.Unmarshal(resp, &respJSON)
	require.NoError(t, err)

	post := respJSON["data"].(map[string]interface{})["addPost"].(map[string]interface{})["post"]
	return post.(map[string]interface{})["postID"].(string)
}

func getPost(t *testing.T, postUID, authorUID, countryUID string) {
	getPostParams := &GraphQLParams{
		Query: `query getPost($createdID: ID!)  {
			getPost(id: $createdID) {
				postID
				title
				text
				isPublished
				tags
				author {
					id
					name
					country {
						id
						name
					}
				}
			}
		}`,
		Variables: map[string]interface{}{"createdID": postUID},
	}
	getPostExpected := fmt.Sprintf(`{ "data": { 
		"getPost": { 
			"postID": "%s", 
			"title": "Test Post", 
			"text": "This post is just a test.",
			"isPublished": true,
			"tags": ["example", "test"],
			"author": {
				"id": "%s",
				"name": "Test Author",
				"country": {
					"id": "%s",
					"name": "Testland"
				}
			}
		} 
	} }`, postUID, authorUID, countryUID)

	resp, err := getPostParams.ExecuteAsPost(graphqlURL)
	require.NoError(t, err)

	require.JSONEq(t, getPostExpected, string(resp))
}
