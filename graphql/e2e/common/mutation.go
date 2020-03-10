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
	"sort"
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
	requireCountry(t, newCountry.ID, newCountry, false, executeRequest)

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
	requireNoGQLErrors(t, gqlResponse)

	var expected, result struct {
		AddCountry struct {
			Country []*country
		}
	}
	err := json.Unmarshal([]byte(addCountryExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	require.Equal(t, len(result.AddCountry.Country), 1)
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
func requireCountry(t *testing.T, uid string, expectedCountry *country, includeStates bool,
	executeRequest requestExecutor) {

	params := &GraphQLParams{
		Query: `query getCountry($id: ID!, $includeStates: Boolean!) {
			getCountry(id: $id) {
				id
				name
				states(order: { asc: xcode }) @include(if: $includeStates) {
					id
					xcode
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"id": uid, "includeStates": includeStates},
	}
	gqlResponse := executeRequest(t, graphqlURL, params)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		GetCountry *country
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expectedCountry, result.GetCountry, ignoreOpts()...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func addAuthor(t *testing.T, countryUID string,
	executeRequest requestExecutor) *author {

	addAuthorParams := &GraphQLParams{
		Query: `mutation addAuthor($author: AddAuthorInput!) {
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
	requireNoGQLErrors(t, gqlResponse)

	var expected, result struct {
		AddAuthor struct {
			Author []*author
		}
	}
	err := json.Unmarshal([]byte(addAuthorExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	require.Equal(t, len(result.AddAuthor.Author), 1)
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
				posts(order: { asc: title }) {
					postID
					title
					text
					tags
					category {
						id
						name
					}
				}
			}
		}`,
		Variables: map[string]interface{}{"id": authorID},
	}
	gqlResponse := executeRequest(t, graphqlURL, params)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		GetAuthor *author
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expectedAuthor, result.GetAuthor, ignoreOpts()...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func addCategory(t *testing.T, executeRequest requestExecutor) *category {
	addCategoryParams := &GraphQLParams{
		Query: `mutation addCategory($name: String!) {
			addCategory(input: [{ name: $name }]) {
				category {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"name": "A Category"},
	}
	addCategoryExpected := `
		{ "addCategory": { "category": [{ "id": "_UID_", "name": "A Category" }] } }`

	gqlResponse := executeRequest(t, graphqlURL, addCategoryParams)
	requireNoGQLErrors(t, gqlResponse)

	var expected, result struct {
		AddCategory struct {
			Category []*category
		}
	}
	err := json.Unmarshal([]byte(addCategoryExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result, ignoreOpts()...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddCategory.Category[0]
}

func ignoreOpts() []cmp.Option {
	return []cmp.Option{
		cmpopts.IgnoreFields(author{}, "ID"),
		cmpopts.IgnoreFields(country{}, "ID"),
		cmpopts.IgnoreFields(post{}, "PostID"),
		cmpopts.IgnoreFields(state{}, "ID"),
		cmpopts.IgnoreFields(category{}, "ID"),
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
				Title:    "A New Post",
				Text:     "Text of new post",
				Tags:     []string{},
				Category: &category{Name: "A Category"},
			},
			{
				Title: "Another New Post",
				Text:  "Text of other new post",
				Tags:  []string{},
			},
		},
	}

	newAuth := addMultipleAuthorFromRef(t, []*author{auth}, executeRequest)[0]
	requireAuthor(t, newAuth.ID, newAuth, executeRequest)

	anotherCountry := addCountry(t, executeRequest)

	patchSet := &author{
		Posts: []*post{
			{
				Title:    "Creating in an update",
				Text:     "Text of new post",
				Category: newAuth.Posts[0].Category,
				Tags:     []string{},
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
		Query: `mutation updateAuthor($id: ID!, $set: AuthorPatch!, $remove: AuthorPatch!) {
			updateAuthor(
				input: {
					filter: {id: [$id]},
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
						tags
						category {
							id
							name
						}
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
	requireNoGQLErrors(t, gqlResponse)

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

func testMultipleMutations(t *testing.T) {
	newCountry := addCountry(t, postExecutor)

	auth1 := &author{
		Name:    "New Author1",
		Country: newCountry,
		Posts: []*post{
			{
				Title: "A New Post",
				Text:  "Text of new post",
				Tags:  []string{},
			},
			{
				Title: "Another New Post",
				Text:  "Text of other new post",
				Tags:  []string{},
			},
		},
	}

	auth2 := &author{
		Name:    "New Author2",
		Country: newCountry,
		Posts: []*post{
			{
				Title: "A Wonder Post",
				Text:  "Text of wonder post",
				Tags:  []string{},
			},
			{
				Title: "Another Wonder Post",
				Text:  "Text of other wonder post",
				Tags:  []string{},
			},
		},
	}

	expectedAuthors := []*author{auth1, auth2}
	newAuths := addMultipleAuthorFromRef(t, expectedAuthors, postExecutor)

	for _, auth := range newAuths {
		postSort := func(i, j int) bool {
			return auth.Posts[i].Title < auth.Posts[j].Title
		}
		sort.Slice(auth.Posts, postSort)
	}

	for i := range expectedAuthors {
		for j := range expectedAuthors[i].Posts {
			expectedAuthors[i].Posts[j].PostID = newAuths[i].Posts[j].PostID
		}
	}

	for i := range newAuths {
		requireAuthor(t, newAuths[i].ID, expectedAuthors[i], postExecutor)
		require.Equal(t, len(newAuths[i].Posts), 2)
		for j := range newAuths[i].Posts {
			expectedAuthors[i].Posts[j].Author = &author{
				ID:      newAuths[i].ID,
				Name:    expectedAuthors[i].Name,
				Dob:     expectedAuthors[i].Dob,
				Country: expectedAuthors[i].Country,
			}
			requirePost(t, newAuths[i].Posts[j].PostID, expectedAuthors[i].Posts[j],
				true, postExecutor)
		}
	}

	cleanUp(t,
		[]*country{newCountry},
		newAuths,
		append(newAuths[0].Posts, newAuths[1].Posts...))
}

func addMultipleAuthorFromRef(t *testing.T, newAuthor []*author,
	executeRequest requestExecutor) []*author {
	addAuthorParams := &GraphQLParams{
		Query: `mutation addAuthor($author: [AddAuthorInput!]!) {
			addAuthor(input: $author) {
			  	author {
					id
					name
					reputation
					country {
						id
				  		name
					}
					posts(order: { asc: title }) {
						postID
						title
						text
						tags
						category {
							id
							name
						}
					}
			  	}
			}
		}`,
		Variables: map[string]interface{}{"author": newAuthor},
	}

	gqlResponse := executeRequest(t, graphqlURL, addAuthorParams)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		AddAuthor struct {
			Author []*author
		}
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	for i := range result.AddAuthor.Author {
		requireUID(t, result.AddAuthor.Author[i].ID)
	}

	authorSort := func(i, j int) bool {
		return result.AddAuthor.Author[i].Name < result.AddAuthor.Author[j].Name
	}
	sort.Slice(result.AddAuthor.Author, authorSort)
	if diff := cmp.Diff(newAuthor, result.AddAuthor.Author, ignoreOpts()...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddAuthor.Author

}

func deepXIDMutations(t *testing.T) {
	deepXIDTest(t, postExecutor)
}

func deepXIDTest(t *testing.T, executeRequest requestExecutor) {
	newCountry := &country{
		Name: "A Country",
		States: []*state{
			{Name: "Alphabet", Code: "ABC"},
			{Name: "A State", Code: "XYZ"},
		},
	}

	// mutations get run serially, each in their own transaction, so the addState
	// sets up the "XZY" xid that's used by the following mutation.
	addCountryParams := &GraphQLParams{
		Query: `mutation addCountry($input: AddCountryInput!) {
			addState(input: [{ xcode: "XYZ", name: "A State" }]) {
				state { id xcode name }
			}

			addCountry(input: [$input])
			{
				country {
					id
					name
					states(order: { asc: xcode }) {
						id
						xcode
						name
					}
				}
			}
		}`,
		Variables: map[string]interface{}{"input": newCountry},
	}

	gqlResponse := executeRequest(t, graphqlURL, addCountryParams)
	requireNoGQLErrors(t, gqlResponse)

	var addResult struct {
		AddState struct {
			State []*state
		}
		AddCountry struct {
			Country []*country
		}
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &addResult)
	require.NoError(t, err)

	require.NotNil(t, addResult)
	require.NotNil(t, addResult.AddState)
	require.NotNil(t, addResult.AddCountry)

	// because the two mutations are linked by an XID, the addCountry mutation shouldn't
	// have created a new state for "XYZ", so the UIDs should be the same
	require.Equal(t, addResult.AddState.State[0].ID, addResult.AddCountry.Country[0].States[1].ID)

	if diff := cmp.Diff(newCountry, addResult.AddCountry.Country[0], ignoreOpts()...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	patchSet := &country{
		States: []*state{{Code: "DEF", Name: "Definitely A State"}},
	}

	patchRemove := &country{
		States: []*state{{Code: "XYZ"}},
	}

	expectedCountry := &country{
		Name:   "A Country",
		States: []*state{newCountry.States[0], patchSet.States[0]},
	}

	updateCountryParams := &GraphQLParams{
		Query: `mutation updateCountry($id: ID!, $set: CountryPatch!, $remove: CountryPatch!) {
			addState(input: [{ xcode: "DEF", name: "Definitely A State" }]) {
				state { id }
			}

			updateCountry(
				input: {
					filter: {id: [$id]},
					set: $set,
					remove: $remove
				}
			) {
				country {
					id
					name
					states(order: { asc: xcode }) {
						id
						xcode
						name
					}
				}
			}
		}`,
		Variables: map[string]interface{}{
			"id":     addResult.AddCountry.Country[0].ID,
			"set":    patchSet,
			"remove": patchRemove,
		},
	}

	gqlResponse = executeRequest(t, graphqlURL, updateCountryParams)
	requireNoGQLErrors(t, gqlResponse)

	var updResult struct {
		AddState struct {
			State []*state
		}
		UpdateCountry struct {
			Country []*country
		}
	}
	err = json.Unmarshal([]byte(gqlResponse.Data), &updResult)
	require.NoError(t, err)
	require.Len(t, updResult.UpdateCountry.Country, 1)

	if diff :=
		cmp.Diff(expectedCountry, updResult.UpdateCountry.Country[0], ignoreOpts()...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	requireCountry(t, addResult.AddCountry.Country[0].ID, expectedCountry, true, executeRequest)

	// The "XYZ" state should have its country set back to null like it was before it was
	// linked to the country
	requireState(t, addResult.AddState.State[0].ID, addResult.AddState.State[0], executeRequest)

	// No need to cleanup states ATM because, beyond this test,
	// there's no queries that rely on them
	cleanUp(t, []*country{addResult.AddCountry.Country[0]}, []*author{}, []*post{})
}

func addPost(t *testing.T, authorID, countryID string,
	executeRequest requestExecutor) *post {

	addPostParams := &GraphQLParams{
		Query: `mutation addPost($post: AddPostInput!) {
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
			getPost(postID: $id) {
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
			"id": []string{newCountry.ID, anotherCountry.ID},
		}
		newName := "updated name"
		updateCountry(t, filter, newName, true)
		newCountry.Name = newName
		anotherCountry.Name = newName

		requireCountry(t, newCountry.ID, newCountry, false, postExecutor)
		requireCountry(t, anotherCountry.ID, anotherCountry, false, postExecutor)
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
		requireCountry(t, newCountry.ID, newCountry, false, postExecutor)
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
		requireCountry(t, newCountry.ID, newCountry, false, postExecutor)
		requireCountry(t, anotherCountry.ID, anotherCountry, false, postExecutor)
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
		requireCountry(t, newCountry.ID, newCountry, false, postExecutor)
		requireCountry(t, anotherCountry.ID, anotherCountry, false, postExecutor)
	})

	cleanUp(t, []*country{newCountry, anotherCountry}, []*author{}, []*post{})
}

func updateRemove(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	newAuthor := addAuthor(t, newCountry.ID, postExecutor)
	newPost := addPost(t, newAuthor.ID, newCountry.ID, postExecutor)

	filter := map[string]interface{}{
		"postID": []string{newPost.PostID},
	}
	remPatch := map[string]interface{}{
		"text":        "This post is just a test.",
		"isPublished": nil,
		"tags":        []string{"test", "notatag"},
		"numLikes":    999,
	}

	updateParams := &GraphQLParams{
		Query: `mutation updPost($filter: PostFilter!, $rem: PostPatch!) {
			updatePost(input: { filter: $filter, remove: $rem }) {
				post {
					text
					isPublished
					tags
					numLikes
				}
			}
		}`,
		Variables: map[string]interface{}{"filter": filter, "rem": remPatch},
	}

	gqlResponse := updateParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

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
	requireNoGQLErrors(t, gqlResponse)

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
					"id": []string{countries[0].ID, countries[1].ID},
				},
			},
			FilterCountries: map[string]interface{}{
				"id": []string{countries[1].ID},
			},
			Expected:  1,
			Countries: []*country{&countries[0], &countries[1]},
		},

		"ID Filter": {
			Filter: map[string]interface{}{
				"id": []string{countries[2].ID},
			},
			FilterCountries: map[string]interface{}{
				"id": []string{countries[2].ID, countries[3].ID},
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
			requireNoGQLErrors(t, gqlResponse)

			var result struct {
				UpdateCountry struct {
					Country []*country
					NumUids int
				}
			}

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.NoError(t, err)

			require.Equal(t, len(result.UpdateCountry.Country), test.Expected)
			for i := 0; i < test.Expected; i++ {
				require.Equal(t, result.UpdateCountry.Country[i].Name, "updatedValue")
			}

			for _, country := range test.Countries {
				requireCountry(t, country.ID, country, false, postExecutor)
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
		filter := map[string]interface{}{"id": []string{country.ID, anotherCountry.ID}}
		deleteCountry(t, filter, deleteCountryExpected, nil)
	})

	t.Run("check Country is deleted", func(t *testing.T) {
		requireCountry(t, country.ID, nil, false, postExecutor)
		requireCountry(t, anotherCountry.ID, nil, false, postExecutor)
	})
}

func deleteMutationWithSingleID(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	anotherCountry := addCountry(t, postExecutor)
	t.Run("delete Country", func(t *testing.T) {
		deleteCountryExpected := `{"deleteCountry" : { "msg": "Deleted" } }`
		filter := map[string]interface{}{"id": []string{newCountry.ID}}
		deleteCountry(t, filter, deleteCountryExpected, nil)
	})

	// In this case anotherCountry shouldn't be deleted.
	t.Run("check Country is deleted", func(t *testing.T) {
		requireCountry(t, newCountry.ID, nil, false, postExecutor)
		requireCountry(t, anotherCountry.ID, anotherCountry, false, postExecutor)
	})
	cleanUp(t, []*country{anotherCountry}, nil, nil)
}

func deleteMutationByName(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	anotherCountry := addCountry(t, postExecutor)
	anotherCountry.Name = "New country"
	filter := map[string]interface{}{
		"id": []string{anotherCountry.ID},
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
		requireCountry(t, newCountry.ID, nil, false, postExecutor)
		requireCountry(t, anotherCountry.ID, anotherCountry, false, postExecutor)
	})
	cleanUp(t, []*country{anotherCountry}, nil, nil)
}

func addMutationReferences(t *testing.T) {
	addMutationUpdatesRefs(t, postExecutor)
	addMutationUpdatesRefsXID(t, postExecutor)
}

func addMutationUpdatesRefs(t *testing.T, executeRequest requestExecutor) {
	newCountry := addCountry(t, executeRequest)
	newAuthor := addAuthor(t, newCountry.ID, executeRequest)
	newPost := addPost(t, newAuthor.ID, newCountry.ID, executeRequest)

	// adding this author with a reference to the existing post changes both the
	// post and the author it was originally linked to.
	addAuthorParams := &GraphQLParams{
		Query: `mutation addAuthor($author: AddAuthorInput!) {
			addAuthor(input: [$author]) {
			  	author { id }
			}
		}`,
		Variables: map[string]interface{}{"author": map[string]interface{}{
			"name":  "Test Author",
			"posts": []interface{}{newPost},
		}},
	}
	gqlResponse := executeRequest(t, graphqlURL, addAuthorParams)
	requireNoGQLErrors(t, gqlResponse)

	var addResult struct {
		AddAuthor struct {
			Author []*author
		}
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &addResult)
	require.NoError(t, err)

	// The original author no longer has newPost in its list of posts
	newAuthor.Posts = []*post{}
	requireAuthor(t, newAuthor.ID, newAuthor, executeRequest)

	cleanUp(t,
		[]*country{newCountry},
		[]*author{newAuthor, addResult.AddAuthor.Author[0]},
		[]*post{newPost})
}

func addMutationUpdatesRefsXID(t *testing.T, executeRequest requestExecutor) {
	newCountry := &country{
		Name: "A Country",
		States: []*state{
			{Name: "Alphabet", Code: "ABC"},
		},
	}

	// The addCountry2 mutation should also remove the state "ABC" from country1's states list
	addCountryParams := &GraphQLParams{
		Query: `mutation addCountry($input: AddCountryInput!) {
			addCountry1: addCountry(input: [$input]) {
				country { id }
			}
			addCountry2: addCountry(input: [$input]) {
				country {
					id
					states {
						id
					}
				}
			}
		}`,
		Variables: map[string]interface{}{"input": newCountry},
	}

	gqlResponse := executeRequest(t, graphqlURL, addCountryParams)
	requireNoGQLErrors(t, gqlResponse)

	var addResult struct {
		AddCountry1 struct {
			Country []*country
		}
		AddCountry2 struct {
			Country []*country
		}
	}

	err := json.Unmarshal([]byte(gqlResponse.Data), &addResult)
	require.NoError(t, err)

	// Country1 doesn't have "ABC" in it's states list
	requireCountry(t, addResult.AddCountry1.Country[0].ID,
		&country{Name: "A Country", States: []*state{}},
		true, executeRequest)

	// Country 2 has the state
	requireCountry(t, addResult.AddCountry2.Country[0].ID,
		&country{Name: "A Country", States: []*state{{Name: "Alphabet", Code: "ABC"}}},
		true, executeRequest)

	cleanUp(t, []*country{addResult.AddCountry1.Country[0], addResult.AddCountry2.Country[0]}, nil,
		nil)
}

func updateMutationReferences(t *testing.T) {
	updateMutationUpdatesRefs(t, postExecutor)
	updateMutationUpdatesRefsXID(t, postExecutor)
}

func updateMutationUpdatesRefs(t *testing.T, executeRequest requestExecutor) {
	newCountry := addCountry(t, executeRequest)
	newAuthor := addAuthor(t, newCountry.ID, executeRequest)
	newPost := addPost(t, newAuthor.ID, newCountry.ID, executeRequest)
	newAuthor2 := addAuthor(t, newCountry.ID, executeRequest)

	// update author2 to steal newPost from author1 ... the post should get removed
	// from author1's post list
	updateAuthorParams := &GraphQLParams{
		Query: `mutation updateAuthor($id: ID!, $set: AuthorPatch!) {
			updateAuthor(
				input: {
					filter: {id: [$id]},
					set: $set
				}
			) {
			  	author { id }
			}
		}`,
		Variables: map[string]interface{}{
			"id":  newAuthor2.ID,
			"set": map[string]interface{}{"posts": []interface{}{newPost}},
		},
	}
	gqlResponse := executeRequest(t, graphqlURL, updateAuthorParams)
	requireNoGQLErrors(t, gqlResponse)

	// The original author no longer has newPost in its list of posts
	newAuthor.Posts = []*post{}
	requireAuthor(t, newAuthor.ID, newAuthor, executeRequest)

	// It's in author2
	newAuthor2.Posts = []*post{{
		PostID: newPost.PostID,
		Title:  newPost.Title,
		Text:   newPost.Text,
		Tags:   newPost.Tags,
	}}
	requireAuthor(t, newAuthor2.ID, newAuthor2, executeRequest)

	cleanUp(t,
		[]*country{newCountry},
		[]*author{newAuthor, newAuthor2},
		[]*post{newPost})
}

func updateMutationUpdatesRefsXID(t *testing.T, executeRequest requestExecutor) {

	newCountry := &country{
		Name: "Testland",
		States: []*state{
			{Name: "Alphabet", Code: "ABC"},
		},
	}

	addCountryParams := &GraphQLParams{
		Query: `mutation addCountry($input: AddCountryInput!) {
			addCountry(input: [$input]) {
				country { id }
			}
		}`,
		Variables: map[string]interface{}{"input": newCountry},
	}

	gqlResponse := executeRequest(t, graphqlURL, addCountryParams)
	requireNoGQLErrors(t, gqlResponse)

	var addResult struct {
		AddCountry struct {
			Country []*country
		}
	}

	err := json.Unmarshal([]byte(gqlResponse.Data), &addResult)
	require.NoError(t, err)

	newCountry2 := addCountry(t, executeRequest)

	// newCountry has state ABC, now let's update newCountry2 to take it
	// and check that it's gone from newCountry

	updateCountryParams := &GraphQLParams{
		Query: `mutation updateCountry($id: ID!, $set: CountryPatch!) {
			updateCountry(
				input: {
					filter: {id: [$id]},
					set: $set
				}
			) {
				country { id }
			}
		}`,
		Variables: map[string]interface{}{
			"id":  newCountry2.ID,
			"set": map[string]interface{}{"states": newCountry.States},
		},
	}

	gqlResponse = executeRequest(t, graphqlURL, updateCountryParams)
	requireNoGQLErrors(t, gqlResponse)

	// newCountry doesn't have "ABC" in it's states list
	requireCountry(t, addResult.AddCountry.Country[0].ID,
		&country{Name: "Testland", States: []*state{}},
		true, executeRequest)

	// newCountry2 has the state
	requireCountry(t, newCountry2.ID,
		&country{Name: "Testland", States: []*state{{Name: "Alphabet", Code: "ABC"}}},
		true, executeRequest)

	cleanUp(t, []*country{addResult.AddCountry.Country[0], newCountry2}, nil, nil)
}

func deleteMutationReferences(t *testing.T) {
	deleteMutationSingleReference(t, postExecutor)
	deleteMutationMultipleReferences(t, postExecutor)
}

func deleteMutationSingleReference(t *testing.T, executeRequest requestExecutor) {

	newCountry := &country{
		Name: "A Country",
		States: []*state{
			{Name: "Alphabet", Code: "ABC"},
		},
	}

	addCountryParams := &GraphQLParams{
		Query: `mutation addCountry($input: AddCountryInput!) {
			addCountry(input: [$input]) {
				country {
					id
					states {
						id
					}
				}
			}
		}`,
		Variables: map[string]interface{}{"input": newCountry},
	}

	gqlResponse := executeRequest(t, graphqlURL, addCountryParams)
	requireNoGQLErrors(t, gqlResponse)

	var addResult struct {
		AddCountry struct {
			Country []*country
		}
	}

	err := json.Unmarshal([]byte(gqlResponse.Data), &addResult)
	require.NoError(t, err)

	filter := map[string]interface{}{"id": []string{addResult.AddCountry.Country[0].ID}}
	deleteCountry(t, filter, `{"deleteCountry" : { "msg": "Deleted" } }`, nil)

	// the state doesn't belong to a country
	getCatParams := &GraphQLParams{
		Query: `query getState($id: ID!) {
			getState(id: $id) {
				country { id }
			}
		}`,
		Variables: map[string]interface{}{"id": addResult.AddCountry.Country[0].States[0].ID},
	}
	gqlResponse = getCatParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	require.JSONEq(t, `{"getState":{"country":null}}`, string(gqlResponse.Data))
}

func deleteMutationMultipleReferences(t *testing.T, executeRequest requestExecutor) {
	newCountry := addCountry(t, executeRequest)
	newAuthor := addAuthor(t, newCountry.ID, executeRequest)
	newPost := addPost(t, newAuthor.ID, newCountry.ID, executeRequest)
	newCategory := addCategory(t, executeRequest)

	updateParams := &GraphQLParams{
		Query: `mutation updPost($filter: PostFilter!, $set: PostPatch!) {
			updatePost(input: { filter: $filter, set: $set }) {
				post { postID category { id } }
			}
		}`,
		Variables: map[string]interface{}{
			"filter": map[string]interface{}{"postID": []string{newPost.PostID}},
			"set":    map[string]interface{}{"category": newCategory}},
	}

	gqlResponse := updateParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	// show that this post is in the author's posts
	newAuthor.Posts = []*post{{
		PostID:   newPost.PostID,
		Title:    newPost.Title,
		Text:     newPost.Text,
		Tags:     newPost.Tags,
		Category: newCategory,
	}}
	requireAuthor(t, newAuthor.ID, newAuthor, executeRequest)

	deletePost(t, newPost.PostID, `{"deletePost" : { "msg": "Deleted" } }`, nil)

	// the post isn't in the author's list of posts
	newAuthor.Posts = []*post{}
	requireAuthor(t, newAuthor.ID, newAuthor, executeRequest)

	// the category doesn't have any posts
	getCatParams := &GraphQLParams{
		Query: `query getCategory($id: ID!) {
			getCategory(id: $id) {
				posts { postID }
			}
		}`,
		Variables: map[string]interface{}{"id": newCategory.ID},
	}
	gqlResponse = getCatParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	require.JSONEq(t, `{"getCategory":{"posts":[]}}`, string(gqlResponse.Data))

	// the post is already deleted
	cleanUp(t, []*country{newCountry}, []*author{newAuthor}, nil)
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
				"id": []string{authorID},
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
			"postID": []string{postID},
		}},
	}

	gqlResponse := deletePostParams.ExecuteAsPost(t, graphqlURL)

	require.JSONEq(t, deletePostExpected, string(gqlResponse.Data))

	if diff := cmp.Diff(expectedErrors, gqlResponse.Errors); diff != "" {
		t.Errorf("errors mismatch (-want +got):\n%s", diff)
	}
}

func deleteWrongID(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	newAuthor := addAuthor(t, newCountry.ID, postExecutor)

	expectedData := `{ "deleteCountry": {
		"msg": "Deleted",
		"numUids": 0
	} }`

	filter := map[string]interface{}{"id": []string{newAuthor.ID}}
	deleteCountryParams := &GraphQLParams{
		Query: `mutation deleteCountry($filter: CountryFilter!) {
			deleteCountry(filter: $filter) {
				msg
				numUids
			}
		}`,
		Variables: map[string]interface{}{"filter": filter},
	}

	gqlResponse := deleteCountryParams.ExecuteAsPost(t, graphqlURL)
	require.JSONEq(t, expectedData, string(gqlResponse.Data))

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
				"id": []string{newCountry.ID}}, "name2": "Testland2"},
	}
	multiMutationExpected := `{
		"add1": { "country": [{ "id": "_UID_", "name": "Testland1" }] },
		"deleteCountry" : { "msg": "Deleted" },
		"add2": { "country": [{ "id": "_UID_", "name": "Testland2" }] }
	}`

	gqlResponse := multiMutationParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

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
		requireCountry(t, newCountry.ID, nil, false, postExecutor)
	})

	cleanUp(t, append(result.Add1.Country, result.Add2.Country...), []*author{}, []*post{})
}

func testSelectionInAddObject(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	newAuth := addAuthor(t, newCountry.ID, postExecutor)

	post1 := &post{
		Title:  "Test1",
		Author: newAuth,
	}

	post2 := &post{
		Title:  "Test2",
		Author: newAuth,
	}

	cases := map[string]struct {
		Filter   map[string]interface{}
		First    int
		Offset   int
		Sort     map[string]interface{}
		Expected []*post
	}{
		"Pagination": {
			First:  1,
			Offset: 1,
			Sort: map[string]interface{}{
				"desc": "title",
			},
			Expected: []*post{post1},
		},
		"Filter": {
			Filter: map[string]interface{}{
				"title": map[string]interface{}{
					"anyoftext": "Test1",
				},
			},
			Expected: []*post{post1},
		},
		"Sort": {
			Sort: map[string]interface{}{
				"desc": "title",
			},
			Expected: []*post{post2, post1},
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			addPostParams := &GraphQLParams{
				Query: `mutation addPost($posts: [AddPostInput!]!, $filter:
					PostFilter, $first: Int, $offset: Int, $sort: PostOrder) {
				addPost(input: $posts) {
				  post (first:$first, offset:$offset, filter:$filter, order:$sort){
					postID
					title
				  }
				}
			}`,
				Variables: map[string]interface{}{
					"posts":  []*post{post1, post2},
					"first":  test.First,
					"offset": test.Offset,
					"sort":   test.Sort,
					"filter": test.Filter,
				},
			}

			gqlResponse := postExecutor(t, graphqlURL, addPostParams)
			requireNoGQLErrors(t, gqlResponse)
			var result struct {
				AddPost struct {
					Post []*post
				}
			}

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.NoError(t, err)

			opt := cmpopts.IgnoreFields(post{}, "PostID", "Author")
			if diff := cmp.Diff(test.Expected, result.AddPost.Post, opt); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}

			cleanUp(t, []*country{}, []*author{}, result.AddPost.Post)
		})

	}

	cleanUp(t, []*country{newCountry}, []*author{newAuth}, []*post{})

}

func mutationEmptyDelete(t *testing.T) {
	// Try to delete a node that doesn't exists.
	updatePostParams := &GraphQLParams{
		Query: `mutation{
			updatePost(input:{
				filter:{title:{allofterms:"Random"}},
				remove:{author:{name:"Non Existent"}}
		  }) {
		    post {
		    title
		    }
		  }
		}`,
	}

	gqlResponse := updatePostParams.ExecuteAsPost(t, graphqlURL)
	require.NotNil(t, gqlResponse.Errors)
	require.Equal(t, gqlResponse.Errors[0].Error(), "couldn't rewrite mutation updatePost"+
		" because failed to rewrite mutation payload because id is not provided")
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
		Query: `mutation addPost($post: AddPostInput!) {
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
			filter := map[string]interface{}{"id": []string{country.ID}}
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
		Query: `mutation addStarship($starship: AddStarshipInput!) {
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
	requireNoGQLErrors(t, gqlResponse)

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
		Query: `mutation addHuman($human: AddHumanInput!) {
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
	requireNoGQLErrors(t, gqlResponse)

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
		Query: `mutation addDroid($droid: AddDroidInput!) {
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
	requireNoGQLErrors(t, gqlResponse)

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
				"id": []string{id},
			},
			"set": map[string]interface{}{
				"name": "Han Solo",
			},
		}},
	}

	gqlResponse := updateCharacterParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)
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
			  id
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

		expected := fmt.Sprintf(`{
			"queryCharacter": [
			  {
				"id": "%s",
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
				"id": "%s",
				"name": "R2-D2",
				"appearsIn": ["EMPIRE"],
				"primaryFunction": "Robot"
			  }
			]
		  }`, humanID, droidID)

		testutil.CompareJSON(t, expected, string(gqlResponse.Data))
	})

	t.Run("test query characters by name", func(t *testing.T) {
		queryCharacterByNameParams := &GraphQLParams{
			Query: `query {
		queryCharacter(filter: { name: { eq: "Han Solo" } }) {
		  id
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

		expected := fmt.Sprintf(`{
		"queryCharacter": [
		  {
			"id": "%s",
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
	  }`, humanID)
		testutil.CompareJSON(t, expected, string(gqlResponse.Data))
	})

	t.Run("test query all humans", func(t *testing.T) {
		queryHumanParams := &GraphQLParams{
			Query: `query {
		queryHuman {
		  id
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

		expected := fmt.Sprintf(`{
		"queryHuman": [
		  {
			"id": "%s",
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
	  }`, humanID)
		testutil.CompareJSON(t, expected, string(gqlResponse.Data))
	})

	t.Run("test query humans by name", func(t *testing.T) {
		queryHumanParamsByName := &GraphQLParams{
			Query: `query {
		queryHuman(filter: { name: { eq: "Han Solo" } }) {
		  id
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

		expected := fmt.Sprintf(`{
		"queryHuman": [
		  {
			"id": "%s",
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
	  }`, humanID)

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
				"id": []string{starshipID},
			},
			"humanFilter": map[string]interface{}{
				"id": []string{humanID},
			},
			"droidFilter": map[string]interface{}{
				"id": []string{droidID},
			},
		},
	}
	multiMutationExpected := `{
	"deleteStarship": { "msg": "Deleted" },
	"deleteHuman" : { "msg": "Deleted" },
	"deleteDroid": { "msg": "Deleted" }
}`

	gqlResponse := multiMutationParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

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
				country {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"id": uid},
	}
	gqlResponse := executeRequest(t, graphqlURL, params)
	requireNoGQLErrors(t, gqlResponse)

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
		Query: `mutation addState($xcode: String!, $name: String!) {
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
	requireNoGQLErrors(t, gqlResponse)

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
		Query: `mutation addState($xcode: String!, $name: String!) {
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

func addMultipleMutationWithOneError(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	newAuth := addAuthor(t, newCountry.ID, postExecutor)

	badAuth := &author{
		ID: "0x0",
	}

	goodPost := &post{
		Title:       "Test Post",
		Text:        "This post is just a test.",
		IsPublished: true,
		NumLikes:    1000,
		Author:      newAuth,
	}

	badPost := &post{
		Title:       "Test Post",
		Text:        "This post is just a test.",
		IsPublished: true,
		NumLikes:    1000,
		Author:      badAuth,
	}

	anotherGoodPost := &post{
		Title:       "Another Test Post",
		Text:        "This is just another post",
		IsPublished: true,
		NumLikes:    1000,
		Author:      newAuth,
	}

	addPostParams := &GraphQLParams{
		Query: `mutation addPost($posts: [AddPostInput!]!) {
			addPost(input: $posts) {
			  post {
				postID
				title
				author {
					id
				}
			  }
			}
		}`,
		Variables: map[string]interface{}{"posts": []*post{goodPost, badPost,
			anotherGoodPost}},
	}

	gqlResponse := postExecutor(t, graphqlURL, addPostParams)

	addPostExpected := fmt.Sprintf(`{ "addPost": {
		"post": [{
			"title": "Text Post",
			"author": {
				"id": "%s"
			}
		}, {
			"title": "Another Test Post",
			"author": {
				"id": "%s"
			}
		}]
	} }`, newAuth.ID, newAuth.ID)

	var expected, result struct {
		AddPost struct {
			Post []*post
		}
	}
	err := json.Unmarshal([]byte(addPostExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	require.Contains(t, gqlResponse.Errors[0].Error(),
		`couldn't rewrite query for mutation addPost because ID "0x0" isn't a Author`)

	cleanUp(t, []*country{newCountry}, []*author{newAuth}, result.AddPost.Post)
}

func addMovie(t *testing.T, executeRequest requestExecutor) *movie {
	addMovieParams := &GraphQLParams{
		Query: `mutation addMovie($name: String!) {
			addMovie(input: [{ name: $name }]) {
				movie {
					id
					name
					director {
						name
					}
				}
			}
		}`,
		Variables: map[string]interface{}{"name": "Testmovie"},
	}
	addMovieExpected := `
		{ "addMovie": { "movie": [{ "id": "_UID_", "name": "Testmovie", "director": [] }] } }`

	gqlResponse := executeRequest(t, graphqlURL, addMovieParams)
	requireNoGQLErrors(t, gqlResponse)

	var expected, result struct {
		AddMovie struct {
			Movie []*movie
		}
	}
	err := json.Unmarshal([]byte(addMovieExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	require.Equal(t, len(result.AddMovie.Movie), 1)
	requireUID(t, result.AddMovie.Movie[0].ID)

	// Always ignore the ID of the object that was just created.  That ID is
	// minted by Dgraph.
	opt := cmpopts.IgnoreFields(movie{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	return result.AddMovie.Movie[0]
}

func cleanupMovieAndDirector(t *testing.T, movieID, directorID string) {
	// Delete everything
	multiMutationParams := &GraphQLParams{
		Query: `mutation cleanup($movieFilter: MovieFilter!, $dirFilter: MovieDirectorFilter!) {
		deleteMovie(filter: $movieFilter) { msg }
		deleteMovieDirector(filter: $dirFilter) { msg }
	}`,
		Variables: map[string]interface{}{
			"movieFilter": map[string]interface{}{
				"id": []string{movieID},
			},
			"dirFilter": map[string]interface{}{
				"id": []string{directorID},
			},
		},
	}
	multiMutationExpected := `{
	"deleteMovie": { "msg": "Deleted" },
	"deleteMovieDirector" : { "msg": "Deleted" }
}`

	gqlResponse := multiMutationParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	testutil.CompareJSON(t, multiMutationExpected, string(gqlResponse.Data))
}

func addMutationWithReverseDgraphEdge(t *testing.T) {
	// create movie
	// create movie director and link the movie
	// query for movie and movie director along reverse edge, we should be able to get the director

	newMovie := addMovie(t, postExecutor)

	addMovieDirectorParams := &GraphQLParams{
		Query: `mutation addMovieDirector($dir: [AddMovieDirectorInput!]!) {
			addMovieDirector(input: $dir) {
			  moviedirector {
				id
				name
			  }
			}
		}`,
		Variables: map[string]interface{}{"dir": []map[string]interface{}{{
			"name":     "Spielberg",
			"directed": []map[string]interface{}{{"id": newMovie.ID}},
		}}},
	}

	addMovieDirectorExpected := `{ "addMovieDirector": { "movieDirector": [{ "id": "_UID_", "name": "Spielberg" }] } }`

	gqlResponse := postExecutor(t, graphqlURL, addMovieDirectorParams)
	requireNoGQLErrors(t, gqlResponse)

	var expected, result struct {
		AddMovieDirector struct {
			MovieDirector []*director
		}
	}
	err := json.Unmarshal([]byte(addMovieDirectorExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	require.Equal(t, len(result.AddMovieDirector.MovieDirector), 1)
	movieDirectorID := result.AddMovieDirector.MovieDirector[0].ID
	requireUID(t, movieDirectorID)

	// Always ignore the ID of the object that was just created.  That ID is
	// minted by Dgraph.
	opt := cmpopts.IgnoreFields(director{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	getMovieParams := &GraphQLParams{
		Query: `query getMovie($id: ID!) {
			getMovie(id: $id) {
				name
				director {
					name
				}
			}
		}`,
		Variables: map[string]interface{}{
			"id": newMovie.ID,
		},
	}

	gqlResponse = getMovieParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)
	expectedResponse := `{"getMovie":{"name":"Testmovie","director":[{"name":"Spielberg"}]}}`
	require.Equal(t, expectedResponse, string(gqlResponse.Data))

	cleanupMovieAndDirector(t, newMovie.ID, movieDirectorID)
}

func testNumUids(t *testing.T) {
	newCountry := addCountry(t, postExecutor)

	auth := &author{
		Name:    "New Author",
		Country: newCountry,
		Posts: []*post{
			{
				Title:    "A New Post for testing numUids",
				Text:     "Text of new post",
				Tags:     []string{},
				Category: &category{Name: "A Category"},
			},
			{
				Title: "Another New Post for testing numUids",
				Text:  "Text of other new post",
				Tags:  []string{},
			},
		},
	}

	addAuthorParams := &GraphQLParams{
		Query: `mutation addAuthor($author: [AddAuthorInput!]!) {
			addAuthor(input: $author) {
				numUids
				author {
					id
					posts {
						postID
					}
				}
			}
		}`,
		Variables: map[string]interface{}{"author": []*author{auth}},
	}

	var result struct {
		AddAuthor struct {
			Author  []*author
			NumUids int
		}
	}

	gqlResponse := postExecutor(t, graphqlURL, addAuthorParams)
	requireNoGQLErrors(t, gqlResponse)

	t.Run("Test numUID in add", func(t *testing.T) {
		err := json.Unmarshal([]byte(gqlResponse.Data), &result)
		require.NoError(t, err)
		require.Equal(t, result.AddAuthor.NumUids, 4)
	})

	t.Run("Test numUID in update", func(t *testing.T) {
		updatePostParams := &GraphQLParams{
			Query: `mutation updatePosts($posts: UpdatePostInput!) {
			updatePost(input: $posts) {
				numUids
			}
		}`,
			Variables: map[string]interface{}{"posts": map[string]interface{}{
				"filter": map[string]interface{}{
					"title": map[string]interface{}{
						"anyofterms": "numUids",
					},
				},
				"set": map[string]interface{}{
					"numLikes": 999,
				},
			}},
		}

		gqlResponse = postExecutor(t, graphqlURL, updatePostParams)
		requireNoGQLErrors(t, gqlResponse)

		var updateResult struct {
			UpdatePost struct {
				Post    []*post
				NumUids int
			}
		}

		err := json.Unmarshal([]byte(gqlResponse.Data), &updateResult)
		require.NoError(t, err)
		require.Equal(t, updateResult.UpdatePost.NumUids, 2)
	})

	t.Run("Test numUID in delete", func(t *testing.T) {
		deleteAuthorParams := &GraphQLParams{
			Query: `mutation deleteItems($authorFilter: AuthorFilter!,
			$postFilter: PostFilter!) {

			deleteAuthor(filter: $authorFilter) {
				numUids
			}

			deletePost(filter: $postFilter) {
				numUids
				msg
			}
		}`,
			Variables: map[string]interface{}{
				"postFilter": map[string]interface{}{
					"title": map[string]interface{}{
						"anyofterms": "numUids",
					},
				},
				"authorFilter": map[string]interface{}{
					"id": []string{result.AddAuthor.Author[0].ID},
				},
			},
		}
		gqlResponse = postExecutor(t, graphqlURL, deleteAuthorParams)
		requireNoGQLErrors(t, gqlResponse)

		var deleteResult struct {
			DeleteAuthor struct {
				Msg     string
				NumUids int
			}
			DeletePost struct {
				Msg     string
				NumUids int
			}
		}

		err := json.Unmarshal([]byte(gqlResponse.Data), &deleteResult)
		require.NoError(t, err)
		require.Equal(t, deleteResult.DeleteAuthor.NumUids, 1)
		require.Equal(t, deleteResult.DeleteAuthor.Msg, "")
		require.Equal(t, deleteResult.DeletePost.NumUids, 2)
		require.Equal(t, deleteResult.DeletePost.Msg, "Deleted")
	})

	cleanUp(t, []*country{newCountry}, result.AddAuthor.Author,
		result.AddAuthor.Author[0].Posts)
}

func checkUser(t *testing.T, userObj, expectedObj *user) {
	checkUserParams := &GraphQLParams{
		Query: `query checkUserPassword($name: String!, $pwd: String!) {
			checkUserPassword(name: $name, password: $pwd) { name }
		}`,
		Variables: map[string]interface{}{
			"name": userObj.Name,
			"pwd":  userObj.Password,
		},
	}

	gqlResponse := checkUserParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		CheckUserPasword *user `json:"checkUserPassword,omitempty"`
	}

	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.Nil(t, err)

	opt := cmpopts.IgnoreFields(user{}, "Password")
	if diff := cmp.Diff(expectedObj, result.CheckUserPasword, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func deleteUser(t *testing.T, userObj user) {
	deletePostParams := &GraphQLParams{
		Query: `mutation deleteUser($filter: UserFilter!) {
			deleteUser(filter: $filter) { msg }
		}`,
		Variables: map[string]interface{}{"filter": map[string]interface{}{
			"name": map[string]interface{}{
				"eq": userObj.Name,
			},
		}},
	}

	gqlResponse := deletePostParams.ExecuteAsPost(t, graphqlURL)

	requireNoGQLErrors(t, gqlResponse)
	require.JSONEq(t, `{"deleteUser": {"msg": "Deleted"}}`, string(gqlResponse.Data))
}

func passwordTest(t *testing.T) {
	newUser := &user{
		Name:     "Test User",
		Password: "password",
	}

	addUserParams := &GraphQLParams{
		Query: `mutation addUser($user: [AddUserInput!]!) {
			addUser(input: $user) {
				user {
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"user": []*user{newUser}},
	}

	updateUserParams := &GraphQLParams{
		Query: `mutation addUser($user: UpdateUserInput!) {
			updateUser(input: $user) {
				user {
					name
				}
			}
		}`,
		Variables: map[string]interface{}{"user": map[string]interface{}{
			"filter": map[string]interface{}{
				"name": map[string]interface{}{
					"eq": newUser.Name,
				},
			},
			"set": map[string]interface{}{
				"password": "password_new",
			},
		}},
	}

	t.Run("Test add and update user", func(t *testing.T) {
		gqlResponse := postExecutor(t, graphqlURL, addUserParams)
		requireNoGQLErrors(t, gqlResponse)
		require.Equal(t, `{"addUser":{"user":[{"name":"Test User"}]}}`,
			string(gqlResponse.Data))

		checkUser(t, newUser, newUser)
		checkUser(t, &user{Name: "Test User", Password: "Wrong Pass"}, nil)

		gqlResponse = postExecutor(t, graphqlURL, updateUserParams)
		requireNoGQLErrors(t, gqlResponse)
		require.Equal(t, `{"updateUser":{"user":[{"name":"Test User"}]}}`,
			string(gqlResponse.Data))
		checkUser(t, newUser, nil)
		updatedUser := &user{Name: newUser.Name, Password: "password_new"}
		checkUser(t, updatedUser, updatedUser)
	})

	deleteUser(t, *newUser)
}
