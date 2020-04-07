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

package custom_logic

import (
	"testing"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/stretchr/testify/require"
)

const (
	alphaURL      = "http://localhost:8180/graphql"
	alphaAdminURL = "http://localhost:8180/admin"
	customTypes   = `type MovieDirector @remote {
		id: ID!
		name: String!
		directed: [Movie]
	}

	type Movie @remote {
		id: ID!
		name: String!
		director: [MovieDirector]
	}
`
)

func updateSchema(t *testing.T, sch string) {
	add := &common.GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					schema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": sch},
	}
	addResult := add.ExecuteAsPost(t, alphaAdminURL)
	require.Nil(t, addResult.Errors)
}

func TestCustomGetQuery(t *testing.T) {
	schema := customTypes + `
	type Query {
        myFavoriteMovies(id: ID!, name: String!, num: Int): [Movie] @custom(http: {
                url: "http://mock:8888/favMovies/$id?name=$name&num=$num",
                method: "GET"
        })
	}`
	updateSchema(t, schema)

	query := `
	query {
		myFavoriteMovies(id: "0x123", name: "Author", num: 10) {
			id
			name
			director {
				id
				name
			}
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)

	expected := `{"myFavoriteMovies":[{"id":"0x3","name":"Star Wars","director":[{"id":"0x4","name":"George Lucas"}]},{"id":"0x5","name":"Star Trek","director":[{"id":"0x6","name":"J.J. Abrams"}]}]}`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomPostQuery(t *testing.T) {
	schema := customTypes + `
	type Query {
        myFavoriteMoviesPost(id: ID!, name: String!, num: Int): [Movie] @custom(http: {
                url: "http://mock:8888/favMoviesPost/$id?name=$name&num=$num",
                method: "POST"
        })
	}`
	updateSchema(t, schema)

	query := `
	query {
		myFavoriteMoviesPost(id: "0x123", name: "Author", num: 10) {
			id
			name
			director {
				id
				name
			}
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)

	expected := `{"myFavoriteMoviesPost":[{"id":"0x3","name":"Star Wars","director":[{"id":"0x4","name":"George Lucas"}]},{"id":"0x5","name":"Star Trek","director":[{"id":"0x6","name":"J.J. Abrams"}]}]}`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomQueryShouldForwardHeaders(t *testing.T) {
	schema := customTypes + `
	type Query {
        verifyHeaders(id: ID!): [Movie] @custom(http: {
                url: "http://mock:8888/verifyHeaders",
				method: "GET",
				forwardHeaders: ["X-App-Token", "X-User-Id"]
        })
	}`
	updateSchema(t, schema)

	query := `
	query {
		verifyHeaders(id: "0x123") {
			id
			name
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
		Headers: map[string][]string{
			"X-App-Token":   []string{"app-token"},
			"X-User-Id":     []string{"123"},
			"Random-header": []string{"random"},
		},
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)
	expected := `{"verifyHeaders":[{"id":"0x3","name":"Star Wars"}]}`
	require.Equal(t, expected, string(result.Data))
}

func TestCustomPostMutation(t *testing.T) {
	schema := customTypes + `
	input MovieDirectorInput {
		id: ID
		name: String
		directed: [MovieInput]
	}
	input MovieInput {
		id: ID
		name: String
		director: [MovieDirectorInput]
	}
	type Mutation {
        createMyFavouriteMovies(input: [MovieInput!]): [Movie] @custom(http: {
			url: "http://mock:8888/favMoviesCreate",
			method: "POST",
			body: "{ movies: $input}"
        })
	}`
	updateSchema(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation createMovies($movs: [MovieInput!]) {
			createMyFavouriteMovies(input: $movs) {
				id
				name
				director {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{
			"movs": []interface{}{
				map[string]interface{}{
					"name":     "Mov1",
					"director": []interface{}{map[string]interface{}{"name": "Dir1"}},
				},
				map[string]interface{}{"name": "Mov2"},
			}},
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)

	expected := `
	{
      "createMyFavouriteMovies": [
        {
          "id": "0x1",
          "name": "Mov1",
          "director": [
            {
              "id": "0x2",
              "name": "Dir1"
            }
          ]
        },
        {
          "id": "0x3",
          "name": "Mov2",
          "director": []
        }
      ]
    }`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomPatchMutation(t *testing.T) {
	schema := customTypes + `
	input MovieDirectorInput {
		id: ID
		name: String
		directed: [MovieInput]
	}
	input MovieInput {
		id: ID
		name: String
		director: [MovieDirectorInput]
	}
	type Mutation {
        updateMyFavouriteMovie(id: ID!, input: MovieInput!): Movie @custom(http: {
			url: "http://mock:8888/favMoviesUpdate/$id",
			method: "PATCH",
			body: "$input"
        })
	}`
	updateSchema(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation updateMovies($id: ID!, $mov: MovieInput!) {
			updateMyFavouriteMovie(id: $id, input: $mov) {
				id
				name
				director {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{
			"id": "0x1",
			"mov": map[string]interface{}{
				"name":     "Mov1",
				"director": []interface{}{map[string]interface{}{"name": "Dir1"}},
			}},
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)

	expected := `
	{
      "updateMyFavouriteMovie": {
        "id": "0x1",
        "name": "Mov1",
        "director": [
          {
            "id": "0x2",
            "name": "Dir1"
          }
        ]
      }
    }`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomMutationShouldForwardHeaders(t *testing.T) {
	schema := customTypes + `
	type Mutation {
        deleteMyFavouriteMovie(id: ID!): Movie @custom(http: {
			url: "http://mock:8888/favMoviesDelete/$id",
			method: "DELETE",
			forwardHeaders: ["X-App-Token", "X-User-Id"]
        })
	}`
	updateSchema(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation {
			deleteMyFavouriteMovie(id: "0x1") {
				id
				name
			}
		}`,
		Headers: map[string][]string{
			"X-App-Token":   {"app-token"},
			"X-User-Id":     {"123"},
			"Random-header": {"random"},
		},
	}

	result := params.ExecuteAsPost(t, alphaURL)
	require.Nil(t, result.Errors)

	expected := `
	{
      "deleteMyFavouriteMovie": {
        "id": "0x1",
        "name": "Mov1"
      }
    }`
	require.JSONEq(t, expected, string(result.Data))
}
