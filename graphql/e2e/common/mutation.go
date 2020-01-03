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

package common

// Tests that mutate the GraphQL database should return the database state to what it
// was at the begining of the test.  The GraphQL query tests rely on a fixed input
// dataset and mutating and leaving unexpected data will result in flaky tests.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// TestAddMutation tests that add mutations work as expected.  There's a few angles
// that need testing:
// - add single object,
// - add object with reference to existing object, and
// - add where @hasInverse edges need linking.
//
// These really need to run as one test because the created uid from the Country
// needs to flow to the author, etc.
func addMutation(t *testing.T) {
	add(t, postExecutor)
}

func add(t *testing.T, executeRequest requestExecutor) {
	var newCountry *country
	var newAuthor *author
	var newPost *post

	// Add single object :
	// Country is a single object not linked to anything else.
	// So only need to check that it gets added as expected.
	newCountry = addCountry(t, executeRequest)

	// addCountry() asserts that the mutation response was as expected.
	// Let's also check that what's in the DB is what we expect.
	requireCountry(t, newCountry.ID, newCountry, executeRequest)

	// Add object with reference to existing object :
	// An Author links to an existing country.  So need to check that the author
	// was added and that it has the link to the right Country.
	newAuthor = addAuthor(t, newCountry.ID, executeRequest)
	requireAuthor(t, newAuthor.ID, newAuthor, executeRequest)

	// Add with @hasInverse :
	// Posts link to an Author and the Author has a link back to all their Posts.
	// So need to check that the Post was added to the right Author
	// AND that the Author's posts now includes the new post.
	newPost = addPost(t, newAuthor.ID, newCountry.ID, executeRequest)
	requirePost(t, newPost.PostID, newPost, true, executeRequest)

	cleanUp(t, []*country{newCountry}, []*author{newAuthor}, []*post{newPost})
}

func addCountry(t *testing.T, executeRequest requestExecutor) *country {
	addCountryParams := &GraphQLParams{
		Query: `mutation addCountry($name: String!) {
			addCountry(input: [{ name: $name }]) {
				country {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"name": "Testland"},
	}
	addCountryExpected := `
		{ "addCountry": { "country": [{ "id": "_UID_", "name": "Testland" }] } }`

	gqlResponse := executeRequest(t, graphqlURL, addCountryParams)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		AddCountry struct {
			Country []*country
		}
	}
	err := json.Unmarshal([]byte(addCountryExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddCountry.Country[0].ID)

	// Always ignore the ID of the object that was just created.  That ID is
	// minted by Dgraph.
	opt := cmpopts.IgnoreFields(country{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddCountry.Country[0]
}

// requireCountry enforces that node with ID uid in the GraphQL store is of type
// Country and is value expectedCountry.
func requireCountry(t *testing.T, uid string, expectedCountry *country,
	executeRequest requestExecutor) {

	params := &GraphQLParams{
		Query: `query getCountry($id: ID!) {
			getCountry(id: $id) {
				id
				name
			}
		}`,
		Variables: map[string]interface{}{"id": uid},
	}
	gqlResponse := executeRequest(t, graphqlURL, params)
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

func addAuthor(t *testing.T, countryUID string,
	executeRequest requestExecutor) *author {

	addAuthorParams := &GraphQLParams{
		Query: `mutation addAuthor($author: AuthorInput!) {
			addAuthor(input: [$author]) {
			  	author {
					id
					name
					dob
					reputation
					country {
						id
				  		name
					}
					posts {
						title
						text
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
		"author": [{
			"id": "_UID_",
			"name": "Test Author",
			"dob": "2010-01-01T05:04:33Z",
			"reputation": 7.75,
			"country": {
				"id": "%s",
				"name": "Testland"
			},
			"posts": []
		}]
	} }`, countryUID)

	gqlResponse := executeRequest(t, graphqlURL, addAuthorParams)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		AddAuthor struct {
			Author []*author
		}
	}
	err := json.Unmarshal([]byte(addAuthorExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddAuthor.Author[0].ID)

	opt := cmpopts.IgnoreFields(author{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddAuthor.Author[0]
}

func requireAuthor(t *testing.T, authorID string, expectedAuthor *author,
	executeRequest requestExecutor) {

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
				posts {
					postID
					title
					text
				}
			}
		}`,
		Variables: map[string]interface{}{"id": authorID},
	}
	gqlResponse := executeRequest(t, graphqlURL, params)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		GetAuthor *author
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expectedAuthor, result.GetAuthor, ignoreOpts()...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func ignoreOpts() []cmp.Option {
	return []cmp.Option{
		cmpopts.IgnoreFields(author{}, "ID"),
		cmpopts.IgnoreFields(country{}, "ID"),
		cmpopts.IgnoreFields(post{}, "PostID"),
	}
}

func deepMutations(t *testing.T) {
	deepMutationsTest(t, postExecutor)
}

func deepMutationsTest(t *testing.T, executeRequest requestExecutor) {
	newCountry := addCountry(t, executeRequest)

	auth := &author{
		Name:    "New Author",
		Country: newCountry,
		Posts: []*post{
			{
				Title: "A New Post",
				Text:  "Text of new post",
			},
			{
				Title: "Another New Post",
				Text:  "Text of other new post",
			},
		},
	}

	newAuth := addAuthorFromRef(t, auth, executeRequest)
	requireAuthor(t, newAuth.ID, newAuth, executeRequest)

	anotherCountry := addCountry(t, executeRequest)

	patchSet := &author{
		Posts: []*post{
			{
				Title: "Creating in an update",
				Text:  "Text of new post",
			},
		},
		// Country: anotherCountry,
		// FIXME: Won't work till https://github.com/dgraph-io/dgraph/pull/4411 is merged
	}

	patchRemove := &author{
		Posts: []*post{newAuth.Posts[0]},
	}

	expectedAuthor := &author{
		Name: "New Author",
		// Country: anotherCountry,
		Country: newCountry,
		Posts:   []*post{newAuth.Posts[1], patchSet.Posts[0]},
	}

	updateAuthorParams := &GraphQLParams{
		Query: `mutation updateAuthor($id: ID!, $set: PatchAuthor!, $remove: PatchAuthor!) {
			updateAuthor(
				input: {
					filter: {ids: [$id]}, 
					set: $set,
					remove: $remove
				}
			) {
			  	author {
					id
					name
					country {
						id
				  		name
					}
					posts {
						title
						text
					}
			  	}
			}
		}`,
		Variables: map[string]interface{}{
			"id":     newAuth.ID,
			"set":    patchSet,
			"remove": patchRemove,
		},
	}

	gqlResponse := executeRequest(t, graphqlURL, updateAuthorParams)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		UpdateAuthor struct {
			Author []*author
		}
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)
	require.Len(t, result.UpdateAuthor.Author, 1)

	if diff :=
		cmp.Diff(expectedAuthor, result.UpdateAuthor.Author[0], ignoreOpts()...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	requireAuthor(t, newAuth.ID, expectedAuthor, executeRequest)
	p := &post{
		PostID: newAuth.Posts[0].PostID,
		Title:  newAuth.Posts[0].Title,
		Text:   newAuth.Posts[0].Text,
		Tags:   []string{},
		Author: nil,
	}
	requirePost(t, newAuth.Posts[0].PostID, p, false, executeRequest)

	cleanUp(t,
		[]*country{newCountry, anotherCountry},
		[]*author{newAuth},
		[]*post{newAuth.Posts[0], newAuth.Posts[0], patchSet.Posts[0]})
}

func addAuthorFromRef(t *testing.T, newAuthor *author, executeRequest requestExecutor) *author {

	addAuthorParams := &GraphQLParams{
		Query: `mutation addAuthor($author: AuthorInput!) {
			addAuthor(input: [$author]) {
			  	author {
					id
					name
					country {
						id
				  		name
					}
					posts(order: { asc: title }) {
						postID
						title
						text
					}
			  	}
			}
		}`,
		Variables: map[string]interface{}{"author": newAuthor},
	}

	gqlResponse := executeRequest(t, graphqlURL, addAuthorParams)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		AddAuthor struct {
			Author []*author
		}
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddAuthor.Author[0].ID)

	if diff := cmp.Diff(newAuthor, result.AddAuthor.Author[0], ignoreOpts()...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddAuthor.Author[0]
}

func addMultiplePosts(t *testing.T) {

}

func addPost(t *testing.T, authorID, countryID string,
	executeRequest requestExecutor) *post {

	addPostParams := &GraphQLParams{
		Query: `mutation addPost($post: PostInput!) {
			addPost(input: [$post]) {
			  post {
				postID
				title
				text
				isPublished
				tags
				numLikes
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
			"numLikes":    1000,
			"tags":        []string{"example", "test"},
			"author":      map[string]interface{}{"id": authorID},
		}},
	}

	addPostExpected := fmt.Sprintf(`{ "addPost": {
		"post": [{
			"postID": "_UID_",
			"title": "Test Post",
			"text": "This post is just a test.",
			"isPublished": true,
			"tags": ["example", "test"],
			"numLikes": 1000,
			"author": {
				"id": "%s",
				"name": "Test Author",
				"country": {
					"id": "%s",
					"name": "Testland"
				}
			}
		}]
	} }`, authorID, countryID)

	gqlResponse := executeRequest(t, graphqlURL, addPostParams)
	requireNoGQLErrors(t, gqlResponse)

	var expected, result struct {
		AddPost struct {
			Post []*post
		}
	}
	err := json.Unmarshal([]byte(addPostExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddPost.Post[0].PostID)

	opt := cmpopts.IgnoreFields(post{}, "PostID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddPost.Post[0]
}

func requirePost(
	t *testing.T,
	postID string,
	expectedPost *post,
	getAuthor bool,
	executeRequest requestExecutor) {

	params := &GraphQLParams{
		Query: `query getPost($id: ID!, $getAuthor: Boolean!)  {
			getPost(id: $id) {
				postID
				title
				text
				isPublished
				tags
				numLikes
				author @include(if: $getAuthor) {
					id
					name
					country {
						id
						name
					}
				}
			}
		}`,
		Variables: map[string]interface{}{
			"id":        postID,
			"getAuthor": getAuthor,
		},
	}

	gqlResponse := executeRequest(t, graphqlURL, params)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		GetPost *post
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expectedPost, result.GetPost); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func updateMutationByIds(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	anotherCountry := addCountry(t, postExecutor)

	t.Run("update Country", func(t *testing.T) {
		filter := map[string]interface{}{
			"ids": []string{newCountry.ID, anotherCountry.ID},
		}
		newName := "updated name"
		updateCountry(t, filter, newName, true)
		newCountry.Name = newName
		anotherCountry.Name = newName

		requireCountry(t, newCountry.ID, newCountry, postExecutor)
		requireCountry(t, anotherCountry.ID, anotherCountry, postExecutor)
	})

	cleanUp(t, []*country{newCountry, anotherCountry}, []*author{}, []*post{})
}

func nameRegexFilter(name string) map[string]interface{} {
	return map[string]interface{}{
		"name": map[string]interface{}{
			"regexp": "/" + name + "/",
		},
	}
}

func updateMutationByName(t *testing.T) {
	// Create two countries, update name of the first. Then do a conditional mutation which
	// should only update the name of the second country.
	newCountry := addCountry(t, postExecutor)
	t.Run("update Country", func(t *testing.T) {
		filter := nameRegexFilter(newCountry.Name)
		newName := "updated name"
		updateCountry(t, filter, newName, true)
		newCountry.Name = newName
		requireCountry(t, newCountry.ID, newCountry, postExecutor)
	})

	anotherCountry := addCountry(t, postExecutor)
	// Update name for country where name is anotherCountry.Name
	t.Run("update country by name", func(t *testing.T) {
		filter := nameRegexFilter(anotherCountry.Name)
		anotherCountry.Name = "updated another country name"
		updateCountry(t, filter, anotherCountry.Name, true)
	})

	t.Run("check updated Country", func(t *testing.T) {
		// newCountry should not have been updated.
		requireCountry(t, newCountry.ID, newCountry, postExecutor)
		requireCountry(t, anotherCountry.ID, anotherCountry, postExecutor)
	})

	cleanUp(t, []*country{newCountry, anotherCountry}, []*author{}, []*post{})
}

func updateMutationByNameNoMatch(t *testing.T) {
	// The countries shouldn't get updated as the query shouldn't match any nodes.
	newCountry := addCountry(t, postExecutor)
	anotherCountry := addCountry(t, postExecutor)
	t.Run("update Country", func(t *testing.T) {
		filter := nameRegexFilter("no match")
		updateCountry(t, filter, "new name", false)
		requireCountry(t, newCountry.ID, newCountry, postExecutor)
		requireCountry(t, anotherCountry.ID, anotherCountry, postExecutor)
	})

	cleanUp(t, []*country{newCountry, anotherCountry}, []*author{}, []*post{})
}

func updateDelete(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	newAuthor := addAuthor(t, newCountry.ID, postExecutor)
	newPost := addPost(t, newAuthor.ID, newCountry.ID, postExecutor)

	filter := map[string]interface{}{
		"ids": []string{newPost.PostID},
	}
	delPatch := map[string]interface{}{
		"text":        "This post is just a test.",
		"isPublished": nil,
		"tags":        []string{"test", "notatag"},
		"numLikes":    999,
	}

	updateParams := &GraphQLParams{
		Query: `mutation updPost($filter: PostFilter!, $del: PatchPost!) {
			updatePost(input: { filter: $filter, remove: $del }) {
				post {
					text
					isPublished
					tags
					numLikes
				}
			}
		}`,
		Variables: map[string]interface{}{"filter": filter, "del": delPatch},
	}

	gqlResponse := updateParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	require.JSONEq(t, `{
			"updatePost": {
				"post": [
					{
						"text": null,
						"isPublished": null,
						"tags": ["example"],
						"numLikes": 1000
					}
				]
			}
		}`,
		string([]byte(gqlResponse.Data)))

	newPost.Text = ""                  // was deleted because the given val was correct
	newPost.Tags = []string{"example"} // the intersection of the tags was deleted
	newPost.IsPublished = false        // must have been deleted because was set to nil in the patch
	// newPost.NumLikes stays the same because the value in the patch was wrong
	requirePost(t, newPost.PostID, newPost, true, postExecutor)

	cleanUp(t, []*country{newCountry}, []*author{newAuthor}, []*post{newPost})
}

func updateCountry(t *testing.T, filter map[string]interface{}, newName string, shouldUpdate bool) {
	updateParams := &GraphQLParams{
		Query: `mutation newName($filter: CountryFilter!, $newName: String!) {
			updateCountry(input: { filter: $filter, set: { name: $newName } }) {
				country {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"filter": filter, "newName": newName},
	}

	gqlResponse := updateParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		UpdateCountry struct {
			Country []*country
		}
	}

	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)
	if shouldUpdate {
		require.NotEqual(t, 0, len(result.UpdateCountry.Country))
	}
	for _, c := range result.UpdateCountry.Country {
		require.NotNil(t, c.ID)
		require.Equal(t, newName, c.Name)
	}
}

func filterInUpdate(t *testing.T) {
	countries := make([]country, 0, 4)
	for i := 0; i < 4; i++ {
		country := addCountry(t, postExecutor)
		country.Name = "updatedValue"
		countries = append(countries, *country)
	}
	countries[3].Name = "Testland"

	cases := map[string]struct {
		Filter          map[string]interface{}
		FilterCountries map[string]interface{}
		Expected        int
		Countries       []*country
	}{
		"Eq filter": {
			Filter: map[string]interface{}{
				"name": map[string]interface{}{
					"eq": "Testland",
				},
				"and": map[string]interface{}{
					"ids": []string{countries[0].ID, countries[1].ID},
				},
			},
			FilterCountries: map[string]interface{}{
				"ids": []string{countries[1].ID},
			},
			Expected:  1,
			Countries: []*country{&countries[0], &countries[1]},
		},

		"ID Filter": {
			Filter: map[string]interface{}{
				"ids": []string{countries[2].ID},
			},
			FilterCountries: map[string]interface{}{
				"ids": []string{countries[2].ID, countries[3].ID},
			},
			Expected:  1,
			Countries: []*country{&countries[2], &countries[3]},
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			updateParams := &GraphQLParams{
				Query: `mutation newName($filter: CountryFilter!, $newName: String!,
					 $filterCountries: CountryFilter!) {
			updateCountry(input: { filter: $filter, set: { name: $newName } }) {
				country(filter: $filterCountries) {
					id
					name
				}
			}
		}`,
				Variables: map[string]interface{}{
					"filter":          test.Filter,
					"newName":         "updatedValue",
					"filterCountries": test.FilterCountries,
				},
			}

			gqlResponse := updateParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			var result struct {
				UpdateCountry struct {
					Country []*country
				}
			}

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.NoError(t, err)

			require.Equal(t, len(result.UpdateCountry.Country), test.Expected)
			for i := 0; i < test.Expected; i++ {
				require.Equal(t, result.UpdateCountry.Country[i].Name, "updatedValue")
			}

			for _, country := range test.Countries {
				requireCountry(t, country.ID, country, postExecutor)
			}
			cleanUp(t, test.Countries, nil, nil)
		})
	}
}

func deleteMutationWithMultipleIds(t *testing.T) {
	country := addCountry(t, postExecutor)
	anotherCountry := addCountry(t, postExecutor)
	t.Run("delete Country", func(t *testing.T) {
		deleteCountryExpected := `{"deleteCountry" : { "msg": "Deleted" } }`
		filter := map[string]interface{}{"ids": []string{country.ID, anotherCountry.ID}}
		deleteCountry(t, filter, deleteCountryExpected, nil)
	})

	t.Run("check Country is deleted", func(t *testing.T) {
		requireCountry(t, country.ID, nil, postExecutor)
		requireCountry(t, anotherCountry.ID, nil, postExecutor)
	})
}

func deleteMutationWithSingleID(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	anotherCountry := addCountry(t, postExecutor)
	t.Run("delete Country", func(t *testing.T) {
		deleteCountryExpected := `{"deleteCountry" : { "msg": "Deleted" } }`
		filter := map[string]interface{}{"ids": []string{newCountry.ID}}
		deleteCountry(t, filter, deleteCountryExpected, nil)
	})

	// In this case anotherCountry shouldn't be deleted.
	t.Run("check Country is deleted", func(t *testing.T) {
		requireCountry(t, newCountry.ID, nil, postExecutor)
		requireCountry(t, anotherCountry.ID, anotherCountry, postExecutor)
	})
	cleanUp(t, []*country{anotherCountry}, nil, nil)
}

func deleteMutationByName(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	anotherCountry := addCountry(t, postExecutor)
	anotherCountry.Name = "New country"
	filter := map[string]interface{}{
		"ids": []string{anotherCountry.ID},
	}
	updateCountry(t, filter, anotherCountry.Name, true)

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
		requireCountry(t, newCountry.ID, nil, postExecutor)
		requireCountry(t, anotherCountry.ID, anotherCountry, postExecutor)
	})
	cleanUp(t, []*country{anotherCountry}, nil, nil)
}

func deleteCountry(
	t *testing.T,
	filter map[string]interface{},
	deleteCountryExpected string,
	expectedErrors x.GqlErrorList) {

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
	expectedErrors x.GqlErrorList) {

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
	expectedErrors x.GqlErrorList) {

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

func deleteWrongID(t *testing.T) {
	t.Skip()
	// Skipping the test for now because wrong type of node while deleting is not an error.
	// After Dgraph returns the number of nodes modified from upsert, modify this test to check
	// count of nodes modified is 0.
	//
	// FIXME: Test cases : with a wrongID, a malformed ID "blah", and maybe a filter that
	// doesn't match anything.
	newCountry := addCountry(t, postExecutor)
	newAuthor := addAuthor(t, newCountry.ID, postExecutor)

	expectedData := `{ "deleteCountry": null }`
	expectedErrors := x.GqlErrorList{
		&x.GqlError{Message: `input: couldn't complete deleteCountry because ` +
			fmt.Sprintf(`input: Node with id %s is not of type Country`, newAuthor.ID)}}

	filter := map[string]interface{}{"ids": []string{newAuthor.ID}}
	deleteCountry(t, filter, expectedData, expectedErrors)

	cleanUp(t, []*country{newCountry}, []*author{newAuthor}, []*post{})
}

func manyMutations(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	multiMutationParams := &GraphQLParams{
		Query: `mutation addCountries($name1: String!, $filter: CountryFilter!, $name2: String!) {
			add1: addCountry(input: [{ name: $name1 }]) {
				country {
					id
					name
				}
			}

			deleteCountry(filter: $filter) { msg }

			add2: addCountry(input: [{ name: $name2 }]) {
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
		"add1": { "country": [{ "id": "_UID_", "name": "Testland1" }] },
		"deleteCountry" : { "msg": "Deleted" },
		"add2": { "country": [{ "id": "_UID_", "name": "Testland2" }] }
	}`

	gqlResponse := multiMutationParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		Add1 struct {
			Country []*country
		}
		DeleteCountry struct {
			Msg string
		}
		Add2 struct {
			Country []*country
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
		requireCountry(t, newCountry.ID, nil, postExecutor)
	})

	cleanUp(t, append(result.Add1.Country, result.Add2.Country...), []*author{}, []*post{})
}

// After a successful mutation, the following query is executed.  That query can
// contain any depth or filtering that makes sense for the schema.
//
// I this case, we set up an author with existing posts, then add another post.
// The filter is down inside post->author->posts and finds just one of the
// author's posts.
func mutationWithDeepFilter(t *testing.T) {

	newCountry := addCountry(t, postExecutor)
	newAuthor := addAuthor(t, newCountry.ID, postExecutor)

	// Make sure they have a post not found by the filter
	newPost := addPost(t, newAuthor.ID, newCountry.ID, postExecutor)

	addPostParams := &GraphQLParams{
		Query: `mutation addPost($post: PostInput!) {
			addPost(input: [$post]) {
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
		"post": [{
			"postID": "_UID_",
			"author": {
				"posts": [ { "title": "find me : a test of deep search after mutation" } ]
			}
		}]
	} }`

	gqlResponse := addPostParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		AddPost struct {
			Post []*post
		}
	}
	err := json.Unmarshal([]byte(addPostExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddPost.Post[0].PostID)

	opt := cmpopts.IgnoreFields(post{}, "PostID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	cleanUp(t, []*country{newCountry}, []*author{newAuthor},
		[]*post{newPost, result.AddPost.Post[0]})
}

// TestManyMutationsWithQueryError : If there are multiple mutations and an error
// occurs in the mutation, then then following mutations aren't executed.  That's
// tested by TestManyMutationsWithError in the resolver tests.
//
// However, there can also be an error in the query following a mutation, but
// that shouldn't stop the following mutations because the actual mutation
// went through without error.
func manyMutationsWithQueryError(t *testing.T) {
	newCountry := addCountry(t, postExecutor)

	// delete the country's name.
	// The schema states type Country `{ ... name: String! ... }`
	// so a query error will be raised if we ask for the country's name in a
	// query.  Don't think a GraphQL update can do this ATM, so do through Dgraph.
	d, err := grpc.Dial(alphagRPC, grpc.WithInsecure())
	require.NoError(t, err)
	client := dgo.NewDgraphClient(api.NewDgraphClient(d))
	mu := &api.Mutation{
		CommitNow: true,
		DelNquads: []byte(fmt.Sprintf("<%s> <Country.name> * .", newCountry.ID)),
	}
	_, err = client.NewTxn().Mutate(context.Background(), mu)
	require.NoError(t, err)

	// add1 - should succeed
	// add2 - should succeed and also return an error (country doesn't have a name)
	// add3 - should succeed
	multiMutationParams := &GraphQLParams{
		Query: `mutation addCountries($countryID: ID!) {
			add1: addAuthor(input: [{ name: "A. N. Author", country: { id: $countryID }}]) {
				author {
					id
					name
					country {
						id
					}
				}
			}

			add2: addAuthor(input: [{ name: "Ann Other Author", country: { id: $countryID }}]) {
				author {
					id
					name
					country {
						id
						name
					}
				}
			}

			add3: addCountry(input: [{ name: "abc" }]) {
				country {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"countryID": newCountry.ID},
	}
	expectedData := fmt.Sprintf(`{
		"add1": { "author": [{ "id": "_UID_", "name": "A. N. Author", "country": { "id": "%s" } }] },
		"add2": { "author": [{ "id": "_UID_", "name": "Ann Other Author", "country": null }] },
		"add3": { "country": [{ "id": "_UID_", "name": "abc" }] }
	}`, newCountry.ID)

	expectedErrors := x.GqlErrorList{
		&x.GqlError{Message: `Non-nullable field 'name' (type String!) was not present ` +
			`in result from Dgraph.  GraphQL error propagation triggered.`,
			Locations: []x.Location{{Line: 18, Column: 7}},
			Path:      []interface{}{"add2", "author", float64(0), "country", "name"}}}

	gqlResponse := multiMutationParams.ExecuteAsPost(t, graphqlURL)

	if diff := cmp.Diff(expectedErrors, gqlResponse.Errors); diff != "" {
		t.Errorf("errors mismatch (-want +got):\n%s", diff)
	}

	var expected, result struct {
		Add1 struct {
			Author []*author
		}
		Add2 struct {
			Author []*author
		}
		Add3 struct {
			Country []*country
		}
	}
	err = json.Unmarshal([]byte(expectedData), &expected)
	require.NoError(t, err)

	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	opt1 := cmpopts.IgnoreFields(author{}, "ID")
	opt2 := cmpopts.IgnoreFields(country{}, "ID")
	if diff := cmp.Diff(expected, result, opt1, opt2); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	cleanUp(t,
		[]*country{newCountry, result.Add3.Country[0]},
		[]*author{result.Add1.Author[0], result.Add2.Author[0]},
		[]*post{})
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
			addStarship(input: [$starship]) {
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
		"starship":[{
			"name":"Millennium Falcon",
			"length":2
		}]
	}}`)

	var expected, result struct {
		AddStarship struct {
			Starship []*starship
		}
	}
	err := json.Unmarshal([]byte(addStarshipExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddStarship.Starship[0].ID)

	opt := cmpopts.IgnoreFields(starship{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddStarship.Starship[0]
}

func addHuman(t *testing.T, starshipID string) string {
	addHumanParams := &GraphQLParams{
		Query: `mutation addHuman($human: HumanInput!) {
			addHuman(input: [$human]) {
				human {
					id
			  	}
			}
		}`,
		Variables: map[string]interface{}{"human": map[string]interface{}{
			"name":         "Han",
			"ename":        "Han_employee",
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
			Human []struct {
				ID string
			}
		}
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddHuman.Human[0].ID)
	return result.AddHuman.Human[0].ID
}

func addDroid(t *testing.T) string {
	addDroidParams := &GraphQLParams{
		Query: `mutation addDroid($droid: DroidInput!) {
			addDroid(input: [$droid]) {
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
			Droid []struct {
				ID string
			}
		}
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddDroid.Droid[0].ID)
	return result.AddDroid.Droid[0].ID
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
			"filter": map[string]interface{}{
				"ids": []string{id},
			},
			"set": map[string]interface{}{
				"name": "Han Solo",
			},
		}},
	}

	gqlResponse := updateCharacterParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
}

func queryInterfaceAfterAddMutation(t *testing.T) {
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)
	droidID := addDroid(t)
	updateCharacter(t, humanID)

	t.Run("test query all characters", func(t *testing.T) {
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
				"appearsIn": ["EMPIRE"],
				"starships": [
				  {
					"name": "Millennium Falcon",
					"length": 2
				  }
				],
				"totalCredits": 10
			  },
			  {
				"name": "R2-D2",
				"appearsIn": ["EMPIRE"],
				"primaryFunction": "Robot"
			  }
			]
		  }`

		testutil.CompareJSON(t, expected, string(gqlResponse.Data))
	})

	t.Run("test query characters by name", func(t *testing.T) {
		queryCharacterByNameParams := &GraphQLParams{
			Query: `query {
		queryCharacter(filter: { name: { eq: "Han Solo" } }) {
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

		gqlResponse := queryCharacterByNameParams.ExecuteAsPost(t, graphqlURL)
		requireNoGQLErrors(t, gqlResponse)

		expected := `{
		"queryCharacter": [
		  {
			"name": "Han Solo",
			"appearsIn": ["EMPIRE"],
			"starships": [
			  {
				"name": "Millennium Falcon",
				"length": 2
			  }
			],
			"totalCredits": 10
		  }
		]
	  }`
		testutil.CompareJSON(t, expected, string(gqlResponse.Data))
	})

	t.Run("test query all humans", func(t *testing.T) {
		queryHumanParams := &GraphQLParams{
			Query: `query {
		queryHuman {
		  name
		  appearsIn
		  starships {
			name
			length
		  }
		  totalCredits
		}
	  }`,
		}

		gqlResponse := queryHumanParams.ExecuteAsPost(t, graphqlURL)
		requireNoGQLErrors(t, gqlResponse)

		expected := `{
		"queryHuman": [
		  {
			"name": "Han Solo",
			"appearsIn": ["EMPIRE"],
			"starships": [
			  {
				"name": "Millennium Falcon",
				"length": 2
			  }
			],
			"totalCredits": 10
		  }
		]
	  }`
		testutil.CompareJSON(t, expected, string(gqlResponse.Data))
	})

	t.Run("test query humans by name", func(t *testing.T) {
		queryHumanParamsByName := &GraphQLParams{
			Query: `query {
		queryHuman(filter: { name: { eq: "Han Solo" } }) {
		  name
		  appearsIn
		  starships {
			name
			length
		  }
		  totalCredits
		}
	  }`,
		}

		gqlResponse := queryHumanParamsByName.ExecuteAsPost(t, graphqlURL)
		requireNoGQLErrors(t, gqlResponse)

		expected := `{
		"queryHuman": [
		  {
			"name": "Han Solo",
			"appearsIn": ["EMPIRE"],
			"starships": [
			  {
				"name": "Millennium Falcon",
				"length": 2
			  }
			],
			"totalCredits": 10
		  }
		]
	  }`

		testutil.CompareJSON(t, expected, string(gqlResponse.Data))
	})

	cleanupStarwars(t, newStarship.ID, humanID, droidID)
}

func cleanupStarwars(t *testing.T, starshipID, humanID, droidID string) {
	// Delete everything
	multiMutationParams := &GraphQLParams{
		Query: `mutation cleanup($starshipFilter: StarshipFilter!, $humanFilter: HumanFilter!,
			$droidFilter: DroidFilter!) {
		deleteStarship(filter: $starshipFilter) { msg }

		deleteHuman(filter: $humanFilter) { msg }

		deleteDroid(filter: $droidFilter) { msg }
	}`,
		Variables: map[string]interface{}{
			"starshipFilter": map[string]interface{}{
				"ids": []string{starshipID},
			},
			"humanFilter": map[string]interface{}{
				"ids": []string{humanID},
			},
			"droidFilter": map[string]interface{}{
				"ids": []string{droidID},
			},
		},
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

func requireState(t *testing.T, uid string, expectedState *state,
	executeRequest requestExecutor) {

	params := &GraphQLParams{
		Query: `query getState($id: ID!) {
			getState(id: $id) {
				id
				xcode
				name
			}
		}`,
		Variables: map[string]interface{}{"id": uid},
	}
	gqlResponse := executeRequest(t, graphqlURL, params)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		GetState *state
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expectedState, result.GetState); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func addState(t *testing.T, name string, executeRequest requestExecutor) *state {
	addStateParams := &GraphQLParams{
		Query: `mutation addState($xcode: String!, $name: String) {
			addState(input: [{ xcode: $xcode, name: $name }]) {
				state {
					id
					xcode
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"name": name, "xcode": "cal"},
	}
	addStateExpected := `
		{ "addState": { "state": [{ "id": "_UID_", "name": "` + name + `", "xcode": "cal" } ]} }`

	gqlResponse := executeRequest(t, graphqlURL, addStateParams)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		AddState struct {
			State []*state
		}
	}
	err := json.Unmarshal([]byte(addStateExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	requireUID(t, result.AddState.State[0].ID)

	// Always ignore the ID of the object that was just created.  That ID is
	// minted by Dgraph.
	opt := cmpopts.IgnoreFields(state{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddState.State[0]
}

func deleteState(
	t *testing.T,
	filter map[string]interface{},
	deleteStateExpected string,
	expectedErrors x.GqlErrorList) {

	deleteStateParams := &GraphQLParams{
		Query: `mutation deleteState($filter: StateFilter!) {
			deleteState(filter: $filter) { msg }
		}`,
		Variables: map[string]interface{}{"filter": filter},
	}

	gqlResponse := deleteStateParams.ExecuteAsPost(t, graphqlURL)
	require.JSONEq(t, deleteStateExpected, string(gqlResponse.Data))

	if diff := cmp.Diff(expectedErrors, gqlResponse.Errors); diff != "" {
		t.Errorf("errors mismatch (-want +got):\n%s", diff)
	}
}

func addMutationWithXid(t *testing.T, executeRequest requestExecutor) {
	newState := addState(t, "California", executeRequest)
	requireState(t, newState.ID, newState, executeRequest)

	// Try add again, it should fail this time.
	name := "Calgary"
	addStateParams := &GraphQLParams{
		Query: `mutation addState($xcode: String!, $name: String) {
			addState(input: [{ xcode: $xcode, name: $name }]) {
				state {
					id
					xcode
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"name": name, "xcode": "cal"},
	}

	gqlResponse := executeRequest(t, graphqlURL, addStateParams)
	require.NotNil(t, gqlResponse.Errors)
	require.Contains(t, gqlResponse.Errors[0].Error(),
		"because id cal already exists for type State")

	deleteStateExpected := `{"deleteState" : { "msg": "Deleted" } }`
	filter := map[string]interface{}{"xcode": map[string]interface{}{"eq": "cal"}}
	deleteState(t, filter, deleteStateExpected, nil)
}

func addMutationWithXID(t *testing.T) {
	addMutationWithXid(t, postExecutor)
}
