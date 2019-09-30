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

// Tests that mutate the GraphQL database should return the database state to what it
// was at the begining of the test.  The GraphQL query tests rely on a fixed input
// dataset and mutating and leaving unexpected data will result in flaky tests.

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
	var newCountry *country
	var newAuthor *author
	var newPost *post

	// Add single object :
	// Country is a single object not linked to anything else.
	// So only need to check that it gets added as expected.
	t.Run("add Country", func(t *testing.T) {
		newCountry = addCountry(t)

		t.Run("check Country", func(t *testing.T) {
			// addCountry() asserts that the mutation response was as expected.
			// Let's also check that what's in the DB is what we expect.
			requireCountry(t, newCountry.ID, newCountry)
		})
	})

	// Add object with reference to existing object :
	// An Author links to an existing country.  So need to check that the author
	// was added and that it has the link to the right Country.
	t.Run("add Author", func(t *testing.T) {
		newAuthor = addAuthor(t, newCountry.ID)

		t.Run("check Author", func(t *testing.T) {
			requireAuthor(t, newAuthor.ID, newAuthor)
		})
	})

	// Add with @hasInverse :
	// Posts link to an Author and the Author has a link back to all their Posts.
	// So need to check that the Post was added to the right Author
	// AND that the Author's posts now includes the new post.
	t.Run("add Post", func(t *testing.T) {
		newPost = addPost(t, newAuthor.ID, newCountry.ID)

		t.Run("check Post", func(t *testing.T) {
			requirePost(t, newPost.PostID, newPost)
		})
	})

	cleanUp(t, []*country{newCountry}, []*author{newAuthor}, []*post{newPost})
}

func addCountry(t *testing.T) *country {
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
		{ "addCountry": { "country": { "id": "_UID_", "name": "Testland" } } }`

	gqlResponse := addCountryParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		AddCountry struct {
			Country *country
		}
	}
	err := json.Unmarshal([]byte(addCountryExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddCountry.Country.ID)

	// Always ignore the ID of the object that was just created.  That ID is
	// minted by Dgraph.
	opt := cmpopts.IgnoreFields(country{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddCountry.Country
}

// requireCountry enforces that node with ID uid in the GraphQL store is of type
// Country and is value expectedCountry.
func requireCountry(t *testing.T, uid string, expectedCountry *country) {
	params := &GraphQLParams{
		Query: `query getCountry($id: ID!) {
			getCountry(id: $id) {
				id
				name
			}
		}`,
		Variables: map[string]interface{}{"id": uid},
	}
	gqlResponse := params.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		GetCountry *country
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expectedCountry, result.GetCountry); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func addAuthor(t *testing.T, countryUID string) *author {
	addAuthorParams := &GraphQLParams{
		Query: `mutation addAuthor($author: AuthorInput!) {
			addAuthor(input: $author) {
			  	author {
					id
					name
					dob
					reputation
					country {
						id
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

	addAuthorExpected := fmt.Sprintf(`{ "addAuthor": {
		"author": {
			"id": "_UID_",
			"name": "Test Author",
			"dob": "2010-01-01T05:04:33Z",
			"reputation": 7.75,
			"country": {
				"id": "%s",
				"name": "Testland"
			}
		}
	} }`, countryUID)

	gqlResponse := addAuthorParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		AddAuthor struct {
			Author *author
		}
	}
	err := json.Unmarshal([]byte(addAuthorExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddAuthor.Author.ID)

	opt := cmpopts.IgnoreFields(author{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddAuthor.Author
}

func requireAuthor(t *testing.T, authorID string, expectedAuthor *author) {
	params := &GraphQLParams{
		Query: `query getAuthor($id: ID!) {
			getAuthor(id: $id) {
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
		Variables: map[string]interface{}{"id": authorID},
	}
	gqlResponse := params.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		GetAuthor *author
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expectedAuthor, result.GetAuthor); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func addPost(t *testing.T, authorID, countryID string) *post {
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
					country {
						id
						name
					}
				}
			  }
			}
		}`,
		Variables: map[string]interface{}{"post": map[string]interface{}{
			"title":       "Test Post",
			"text":        "This post is just a test.",
			"isPublished": true,
			"tags":        []string{"example", "test"},
			"author":      map[string]interface{}{"id": authorID},
		}},
	}

	addPostExpected := fmt.Sprintf(`{ "addPost": {
		"post": {
			"postID": "_UID_",
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
	} }`, authorID, countryID)

	gqlResponse := addPostParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		AddPost struct {
			Post *post
		}
	}
	err := json.Unmarshal([]byte(addPostExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddPost.Post.PostID)

	opt := cmpopts.IgnoreFields(post{}, "PostID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddPost.Post
}

func requirePost(t *testing.T, postID string, expectedPost *post) {
	params := &GraphQLParams{
		Query: `query getPost($id: ID!)  {
			getPost(id: $id) {
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
		Variables: map[string]interface{}{"id": postID},
	}

	gqlResponse := params.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		GetPost *post
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expectedPost, result.GetPost); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateMutation(t *testing.T) {
	newCountry := addCountry(t)
	newCountry.Name = "updated name"

	t.Run("update Country", func(t *testing.T) {
		updateCountry(t, newCountry)
	})

	t.Run("check updated Country", func(t *testing.T) {
		requireCountry(t, newCountry.ID, newCountry)
	})

	cleanUp(t, []*country{newCountry}, []*author{}, []*post{})
}

func updateCountry(t *testing.T, updCountry *country) {
	updateParams := &GraphQLParams{
		Query: `mutation newName($id: ID!, $newName: String!) {
			updateCountry(input: { id: $id, patch: { name: $newName } }) {
				country {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"id": updCountry.ID, "newName": updCountry.Name},
	}

	gqlResponse := updateParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		UpdateCountry struct {
			Country *country
		}
	}
	expected.UpdateCountry.Country = updCountry
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestDeleteMutationWithMultipleIds(t *testing.T) {
	country := addCountry(t)
	anotherCountry := addCountry(t)
	t.Run("delete Country", func(t *testing.T) {
		deleteCountryExpected := `{"deleteCountry" : { "msg": "Deleted" } }`
		filter := map[string]interface{}{"ids": []string{country.ID, anotherCountry.ID}}
		deleteCountry(t, filter, deleteCountryExpected, nil)
	})

	t.Run("check Country is deleted", func(t *testing.T) {
		requireCountry(t, country.ID, nil)
		requireCountry(t, anotherCountry.ID, nil)
	})
}

func TestDeleteMutationWithSingleId(t *testing.T) {
	newCountry := addCountry(t)
	anotherCountry := addCountry(t)
	t.Run("delete Country", func(t *testing.T) {
		deleteCountryExpected := `{"deleteCountry" : { "msg": "Deleted" } }`
		filter := map[string]interface{}{"ids": []string{newCountry.ID}}
		deleteCountry(t, filter, deleteCountryExpected, nil)
	})

	// In this case anotherCountry shouldn't be deleted.
	t.Run("check Country is deleted", func(t *testing.T) {
		requireCountry(t, newCountry.ID, nil)
		requireCountry(t, anotherCountry.ID, anotherCountry)
	})
	cleanUp(t, []*country{anotherCountry}, nil, nil)
}

func TestDeleteMutationByName(t *testing.T) {
	newCountry := addCountry(t)
	anotherCountry := addCountry(t)
	anotherCountry.Name = "New country"
	updateCountry(t, anotherCountry)

	deleteCountryExpected := `{"deleteCountry" : { "msg": "Deleted" } }`
	t.Run("delete Country", func(t *testing.T) {
		filter := map[string]interface{}{
			"name": map[string]interface{}{
				"regexp": "/" + newCountry.Name + "/",
			},
		}
		deleteCountry(t, filter, deleteCountryExpected, nil)
	})

	// In this case anotherCountry shouldn't be deleted.
	t.Run("check Country is deleted", func(t *testing.T) {
		requireCountry(t, newCountry.ID, nil)
		requireCountry(t, anotherCountry.ID, anotherCountry)
	})
	cleanUp(t, []*country{anotherCountry}, nil, nil)
}

func deleteCountry(
	t *testing.T,
	filter map[string]interface{},
	deleteCountryExpected string,
	expectedErrors []*x.GqlError) {

	deleteCountryParams := &GraphQLParams{
		Query: `mutation deleteCountry($filter: CountryFilter!) {
			deleteCountry(filter: $filter) { msg }
		}`,
		Variables: map[string]interface{}{"filter": filter},
	}

	gqlResponse := deleteCountryParams.ExecuteAsPost(t, graphqlURL)
	require.JSONEq(t, deleteCountryExpected, string(gqlResponse.Data))

	if diff := cmp.Diff(expectedErrors, gqlResponse.Errors); diff != "" {
		t.Errorf("errors mismatch (-want +got):\n%s", diff)
	}
}

func deleteAuthor(
	t *testing.T,
	authorID string,
	deleteAuthorExpected string,
	expectedErrors []*x.GqlError) {

	deleteAuthorParams := &GraphQLParams{
		Query: `mutation deleteAuthor($filter: AuthorFilter!) {
			deleteAuthor(filter: $filter) { msg }
		}`,
		Variables: map[string]interface{}{
			"filter": map[string]interface{}{
				"ids": []string{authorID},
			},
		},
	}

	gqlResponse := deleteAuthorParams.ExecuteAsPost(t, graphqlURL)

	require.JSONEq(t, deleteAuthorExpected, string(gqlResponse.Data))

	if diff := cmp.Diff(expectedErrors, gqlResponse.Errors); diff != "" {
		t.Errorf("errors mismatch (-want +got):\n%s", diff)
	}
}

func deletePost(
	t *testing.T,
	postID string,
	deletePostExpected string,
	expectedErrors []*x.GqlError) {

	deletePostParams := &GraphQLParams{
		Query: `mutation deletePost($filter: PostFilter!) {
			deletePost(filter: $filter) { msg }
		}`,
		Variables: map[string]interface{}{"filter": map[string]interface{}{
			"ids": []string{postID},
		}},
	}

	gqlResponse := deletePostParams.ExecuteAsPost(t, graphqlURL)

	require.JSONEq(t, deletePostExpected, string(gqlResponse.Data))

	if diff := cmp.Diff(expectedErrors, gqlResponse.Errors); diff != "" {
		t.Errorf("errors mismatch (-want +got):\n%s", diff)
	}
}

func TestDeleteWrongID(t *testing.T) {
	t.Skip()
	// Skipping the test for now because wrong type of node while deleting is not an error.
	// After Dgraph returns the number of nodes modified from upsert, modify this test to check
	// count of nodes modified is 0.
	newCountry := addCountry(t)
	newAuthor := addAuthor(t, newCountry.ID)

	expectedData := `{ "deleteCountry": null }`
	expectedErrors := []*x.GqlError{
		&x.GqlError{Message: `input: couldn't complete deleteCountry because ` +
			fmt.Sprintf(`input: Node with id %s is not of type Country`, newAuthor.ID)}}

	filter := map[string]interface{}{"ids": []string{newAuthor.ID}}
	deleteCountry(t, filter, expectedData, expectedErrors)

	cleanUp(t, []*country{newCountry}, []*author{newAuthor}, []*post{})
}

func TestManyMutations(t *testing.T) {
	newCountry := addCountry(t)
	multiMutationParams := &GraphQLParams{
		Query: `mutation addCountries($name1: String!, $filter: CountryFilter!, $name2: String!) {
			add1: addCountry(input: { name: $name1 }) {
				country {
					id
					name
				}
			}

			deleteCountry(filter: $filter) { msg }

			add2: addCountry(input: { name: $name2 }) {
				country {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{
			"name1": "Testland1", "filter": map[string]interface{}{
				"ids": []string{newCountry.ID}}, "name2": "Testland2"},
	}
	multiMutationExpected := `{
		"add1": { "country": { "id": "_UID_", "name": "Testland1" } },
		"deleteCountry" : { "msg": "Deleted" },
		"add2": { "country": { "id": "_UID_", "name": "Testland2" } }
	}`

	gqlResponse := multiMutationParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		Add1 struct {
			Country *country
		}
		DeleteCountry struct {
			Msg string
		}
		Add2 struct {
			Country *country
		}
	}
	err := json.Unmarshal([]byte(multiMutationExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	opt := cmpopts.IgnoreFields(country{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	t.Run("country deleted", func(t *testing.T) {
		requireCountry(t, newCountry.ID, nil)
	})

	cleanUp(t, []*country{result.Add1.Country, result.Add2.Country}, []*author{}, []*post{})
}

// TestManyMutationsWithError : Multiple mutations run serially (queries would
// run in parallel) and if an error is encountered, the mutations following the
// error are not run.  The mutations that have succeeded are permanent -
// i.e. not rolled back.
//
// Note that there's 3 mutations, but one of those `add2` never gets executed,
// so there should be no field for it in the result - that's different to a field
// that starts execution, like `deleteCountry`, but fails.
func TestManyMutationsWithError(t *testing.T) {
	// Skipping the test for now because wrong type of node while deleting is not an error.
	// Modify the test to have some other mutation that fails.
	t.Skip()
	newCountry := addCountry(t)
	newAuthor := addAuthor(t, newCountry.ID)

	// add1 - should succeed
	// deleteCountry - should fail (given uid is not a Country)
	// add2 - is never executed
	multiMutationParams := &GraphQLParams{
		Query: `mutation addCountries($del: ID!, filter: PostFilter!) {
			add1: addCountry(input: { name: "Testland" }) {
				country {
					id
					name
				}
			}

			deleteCountry(filter: $filter) { msg }

			add2: addCountry(input: { name: "abc" }) {
				country {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"del": newAuthor.ID},
	}
	expectedData := `{
		"add1": { "country": { "id": "_UID_", "name": "Testland" } },
		"deleteCountry" : null
	}`

	expectedErrors := []*x.GqlError{
		&x.GqlError{Message: `input: couldn't complete deleteCountry because ` +
			fmt.Sprintf(`input: Node with id %s is not of type Country`, newAuthor.ID)},
		&x.GqlError{Message: `mutation add2 not executed because of previous error`}}

	gqlResponse := multiMutationParams.ExecuteAsPost(t, graphqlURL)

	var expected, result struct {
		Add1 struct {
			Country *country
		}
		DeleteCountry struct {
			Msg string
		}
	}
	err := json.Unmarshal([]byte(expectedData), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	opt := cmpopts.IgnoreFields(country{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(expectedErrors, gqlResponse.Errors); diff != "" {
		t.Errorf("errors mismatch (-want +got):\n%s", diff)
	}

	// Make sure that third mutation didn't run
	t.Run("Country wasn't added", func(t *testing.T) {
		queryCountryByRegExp(t, "/abc/", []*country{})
	})

	cleanUp(t, []*country{newCountry, result.Add1.Country}, []*author{newAuthor}, []*post{})
}

// After a successful mutation, the following query is executed.  That query can
// contain any depth or filtering that makes sense for the schema.
//
// I this case, we set up an author with existing posts, then add another post.
// The filter is down inside post->author->posts and finds just one of the
// author's posts.
func TestMutationWithDeepFilter(t *testing.T) {

	newCountry := addCountry(t)
	newAuthor := addAuthor(t, newCountry.ID)

	// Make sure they have a post not found by the filter
	newPost := addPost(t, newAuthor.ID, newCountry.ID)

	addPostParams := &GraphQLParams{
		Query: `mutation addPost($post: PostInput!) {
			addPost(input: $post) {
			  post {
				postID
				author {
					posts(filter: { title: { allofterms: "find me" }}) {
						title
					}
				}
			  }
			}
		}`,
		Variables: map[string]interface{}{"post": map[string]interface{}{
			"title":  "find me : a test of deep search after mutation",
			"author": map[string]interface{}{"id": newAuthor.ID},
		}},
	}

	// Expect the filter to find just the new post, not any of the author's existing posts.
	addPostExpected := `{ "addPost": {
		"post": {
			"postID": "_UID_",
			"author": {
				"posts": [ { "title": "find me : a test of deep search after mutation" } ]
			}
		}
	} }`

	gqlResponse := addPostParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		AddPost struct {
			Post *post
		}
	}
	err := json.Unmarshal([]byte(addPostExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddPost.Post.PostID)

	opt := cmpopts.IgnoreFields(post{}, "PostID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	cleanUp(t, []*country{newCountry}, []*author{newAuthor},
		[]*post{newPost, result.AddPost.Post})
}

func cleanUp(t *testing.T, countries []*country, authors []*author, posts []*post) {
	t.Run("cleaning up", func(t *testing.T) {
		for _, post := range posts {
			deletePost(t, post.PostID, `{"deletePost" : { "msg": "Deleted" } }`, nil)
		}

		for _, author := range authors {
			deleteAuthor(t, author.ID, `{"deleteAuthor" : { "msg": "Deleted" } }`, nil)
		}

		for _, country := range countries {
			filter := map[string]interface{}{"ids": []string{country.ID}}
			deleteCountry(t, filter, `{"deleteCountry" : { "msg": "Deleted" } }`, nil)
		}
	})
}

type starship struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Length float64 `json:"length"`
}

func addStarship(t *testing.T) *starship {
	addStarshipParams := &GraphQLParams{
		Query: `mutation addStarship($starship: StarshipInput!) {
			addStarship(input: $starship) {
				starship {
					id
					name
					length
			  	}
			}
		}`,
		Variables: map[string]interface{}{"starship": map[string]interface{}{
			"name":   "Millennium Falcon",
			"length": 2,
		}},
	}

	gqlResponse := addStarshipParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	addStarshipExpected := fmt.Sprintf(`{"addStarship":{
		"starship":{
			"name":"Millennium Falcon",
			"length":2
		}
	}}`)

	var expected, result struct {
		AddStarship struct {
			Starship *starship
		}
	}
	err := json.Unmarshal([]byte(addStarshipExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddStarship.Starship.ID)

	opt := cmpopts.IgnoreFields(starship{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddStarship.Starship
}

func addHuman(t *testing.T, starshipID string) string {
	addHumanParams := &GraphQLParams{
		Query: `mutation addHuman($human: HumanInput!) {
			addHuman(input: $human) {
				human {
					id
			  	}
			}
		}`,
		Variables: map[string]interface{}{"human": map[string]interface{}{
			"name":         "Han",
			"totalCredits": 10,
			"appearsIn":    []string{"EMPIRE"},
			"starships": []map[string]interface{}{{
				"id": starshipID,
			}},
		}},
	}

	gqlResponse := addHumanParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		AddHuman struct {
			Human struct {
				ID string
			}
		}
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddHuman.Human.ID)
	return result.AddHuman.Human.ID
}

func addDroid(t *testing.T) string {
	addDroidParams := &GraphQLParams{
		Query: `mutation addDroid($droid: DroidInput!) {
			addDroid(input: $droid) {
				droid {
					id
				}
			}
		}`,
		Variables: map[string]interface{}{"droid": map[string]interface{}{
			"name":            "R2-D2",
			"primaryFunction": "Robot",
			"appearsIn":       []string{"EMPIRE"},
		}},
	}

	gqlResponse := addDroidParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		AddDroid struct {
			Droid struct {
				ID string
			}
		}
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddDroid.Droid.ID)
	return result.AddDroid.Droid.ID
}

func updateCharacter(t *testing.T, id string) {
	updateCharacterParams := &GraphQLParams{
		Query: `mutation updateCharacter($character: UpdateCharacterInput!) {
			updateCharacter(input: $character) {
				character {
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"character": map[string]interface{}{
			"id": id,
			"patch": map[string]interface{}{
				"name": "Han Solo",
			},
		}},
	}

	gqlResponse := updateCharacterParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func TestQueryInterfaceAfterAddMutation(t *testing.T) {
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)
	droidID := addDroid(t)
	updateCharacter(t, humanID)

	queryCharacterParams := &GraphQLParams{
		Query: `query {
		queryCharacter {
		  name
		  appearsIn
		  ... on Human {
			starships {
				name
				length
			}
			totalCredits
		  }
		  ... on Droid {
			primaryFunction
		  }
		}
	  }`,
	}

	gqlResponse := queryCharacterParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	expected := `{
		"queryCharacter": [
		  {
			"name": "Han Solo",
			"appearsIn": "EMPIRE",
			"starships": [
			  {
				"name": "Millennium Falcon",
				"length": 2
			  }
			],
			"primaryFunction": null,
			"totalCredits": 10
		  },
		  {
			"name": "R2-D2",
			"appearsIn": "EMPIRE",
			"primaryFunction": "Robot",
			"starships": [],
			"totalCredits": null
		  }
		]
	  }`

	testutil.CompareJSON(t, expected, string(gqlResponse.Data))

	cleanupStarwars(t, newStarship.ID, humanID, droidID)
}

func cleanupStarwars(t *testing.T, starshipID, humanID, droidID string) {
	// Delete everything
	multiMutationParams := &GraphQLParams{
		Query: `mutation cleanup($starshipID: ID!, $humanID: ID!, $droidID: ID!) {
		deleteStarship(id: $starshipID) { msg }

		deleteHuman(id: $humanID) { msg }

		deleteDroid(id: $droidID) { msg }
	}`,
		Variables: map[string]interface{}{
			"starshipID": starshipID, "humanID": humanID, "droidID": droidID},
	}
	multiMutationExpected := `{
	"deleteStarship": { "msg": "Deleted" },
	"deleteHuman" : { "msg": "Deleted" },
	"deleteDroid": { "msg": "Deleted" }
}`

	gqlResponse := multiMutationParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		DeleteStarhip struct {
			Msg string
		}
		DeleteHuman struct {
			Msg string
		}
		DeleteDroid struct {
			Msg string
		}
	}

	err := json.Unmarshal([]byte(multiMutationExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}
