/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
)

func lambdaOnTypeField(t *testing.T) {
	query := `
		query {
			queryAuthor {
				name
				bio
				rank
			}
		}`
	params := &GraphQLParams{Query: query}
	resp := params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	expectedResponse := `{
		"queryAuthor": [
			{
				"name":"Three Author",
				"bio":"My name is Three Author and I was born on 2001-01-01T00:00:00Z.",
				"rank":1
			},
			{
				"name":"Ann Author",
				"bio":"My name is Ann Author and I was born on 2000-01-01T00:00:00Z.",
				"rank":3
			},
			{
				"name":"Ann Other Author",
				"bio":"My name is Ann Other Author and I was born on 1988-01-01T00:00:00Z.",
				"rank":2
			}
		]
	}`
	testutil.CompareJSON(t, expectedResponse, string(resp.Data))
}

func lambdaOnInterfaceField(t *testing.T) {
	starship := addStarship(t)
	humanID := addHuman(t, starship.ID)
	droidID := addDroid(t)

	// when querying bio on Character (interface) we should get the bio constructed by the lambda
	// registered on Character.bio
	query := `
		query {
			queryCharacter {
				name
				bio
			}
		}`
	params := &GraphQLParams{Query: query}
	resp := params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	expectedResponse := `{
		"queryCharacter": [
			{
				"name":"Han",
				"bio":"My name is Han."
			},
			{
				"name":"R2-D2",
				"bio":"My name is R2-D2."
			}
		]
	}`
	testutil.CompareJSON(t, expectedResponse, string(resp.Data))

	// TODO: this should work. At present there is a bug with @custom on interface field resolved
	// through a fragment on one of its types. We need to fix that first, then uncomment this test.

	// when querying bio on Human & Droid (type) we should get the bio constructed by the lambda
	// registered on Human.bio and Droid.bio respectively
	/*query = `
		query {
			queryCharacter {
				name
				... on Human {
					bio
				}
				... on Droid {
					bio
				}
			}
		}`
	params = &GraphQLParams{Query: query}
	resp = params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	expectedResponse = `{
		"queryCharacter": [
			{
				"name":"Han",
				"bio":"My name is Han. I have 10 credits."
			},
			{
				"name":"R2-D2",
				"bio":"My name is R2-D2. My primary function is Robot."
			}
		]
	}`
	testutil.CompareJSON(t, expectedResponse, string(resp.Data))*/

	// cleanup
	cleanupStarwars(t, starship.ID, humanID, droidID)
}

func lambdaOnQueryUsingDql(t *testing.T) {
	query := `
		query {
			authorsByName(name: "Ann Author") {
				name
				dob
				reputation
			}
		}`
	params := &GraphQLParams{Query: query}
	resp := params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	expectedResponse := `{
		"authorsByName": [
			{
				"name":"Ann Author",
				"dob":"2000-01-01T00:00:00Z",
				"reputation":6.6
			}
		]
	}`
	testutil.CompareJSON(t, expectedResponse, string(resp.Data))
}

func lambdaOnMutationUsingGraphQL(t *testing.T) {
	// first, add the author using @lambda
	query := `
		mutation {
			newAuthor(name: "Lambda")
		}`
	params := &GraphQLParams{Query: query}
	resp := params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	// let's get the author ID of the newly added author as returned by lambda
	var addResp struct {
		AuthorID string `json:"newAuthor"`
	}
	require.NoError(t, json.Unmarshal(resp.Data, &addResp))

	// now, lets query the same author and verify that its reputation was set as 3.0 by lambda func
	query = `
		query ($id: ID!){
			getAuthor(id: $id) {
				name
				reputation
			}
		}`
	params = &GraphQLParams{Query: query, Variables: map[string]interface{}{"id": addResp.AuthorID}}
	resp = params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	expectedResponse := `{
		"getAuthor": {
				"name":"Lambda",
				"reputation":3.0
			}
	}`
	testutil.CompareJSON(t, expectedResponse, string(resp.Data))

	// cleanup
	deleteAuthors(t, []string{addResp.AuthorID}, nil)
}
