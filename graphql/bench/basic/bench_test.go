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

package basic

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/stretchr/testify/require"
)

const (
	graphqlURL = "http://localhost:8080/graphql"
)

func BenchmarkSingleLevel_QueryRestaurant(b *testing.B) {
	query := `
	query($offset: Int) {
	  queryRestaurant (first: 100, offset: $offset) {
		id
		name
	  }
	}
	`

	params := &common.GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"offset": 0},
	}

	offset := 0
	for i := 0; i < b.N; i++ {
		gqlResponse := params.ExecuteAsPost(b, graphqlURL)
		require.Nilf(b, gqlResponse.Errors, "%+v", gqlResponse.Errors)
		offset += 100
		params.Variables["offset"] = offset
	}
}

func BenchmarkMultiLevel_QueryRestaurant(b *testing.B) {
	query := `
	query($offset: Int) {
	  queryRestaurant (first: 100, offset: $offset) {
		id
		name
		cuisines(first: 100) {
			id
			name
			dishes(first: 100) {
				id
				name
			}
		}
	  }
	}
	`

	params := &common.GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"offset": 0},
	}

	offset := 0
	for i := 0; i < b.N; i++ {
		gqlResponse := params.ExecuteAsPost(b, graphqlURL)
		require.Nilf(b, gqlResponse.Errors, "%+v", gqlResponse.Errors)
		offset += 100
		params.Variables["offset"] = offset
	}
}

type res struct {
	QueryRestaurant []struct {
		ID string
	}
}

func getRestaurantIDs(b *testing.B) res {
	query := `
	query {
	  queryRestaurant (first: 10000) {
		id
	  }
	}`

	params := &common.GraphQLParams{
		Query: query,
	}

	gqlResponse := params.ExecuteAsPost(b, graphqlURL)
	require.Nilf(b, gqlResponse.Errors, "%+v", gqlResponse.Errors)

	var r res
	err := json.Unmarshal(gqlResponse.Data, &r)
	require.NoError(b, err)

	require.Greater(b, len(r.QueryRestaurant), 0)
	return r
}

func BenchmarkSingleLevel_GetRestaurant(b *testing.B) {
	r := getRestaurantIDs(b)

	getQuery := `
	query($id: ID!) {
		getRestaurant(id: $id) {
			id
			name
		}
	}
	`

	params := &common.GraphQLParams{
		Query: getQuery,
		Variables: map[string]interface{}{
			"id": r.QueryRestaurant[rand.Intn(len(r.QueryRestaurant))].ID,
		},
	}

	for i := 0; i < b.N; i++ {
		gqlResponse := params.ExecuteAsPost(b, graphqlURL)
		require.Nilf(b, gqlResponse.Errors, "%+v", gqlResponse.Errors)
		params.Variables["id"] = r.QueryRestaurant[rand.Intn(len(r.QueryRestaurant))].ID
	}
}

func BenchmarkMultiLevel_GetRestaurant(b *testing.B) {
	r := getRestaurantIDs(b)

	getQuery := `
	query($id: ID!) {
		getRestaurant(id: $id) {
			id
			name
			cuisines(first: 100) {
				id
				name
				dishes(first: 100) {
					id
					name
				}
			}
		}
	}
	`

	params := &common.GraphQLParams{
		Query: getQuery,
		Variables: map[string]interface{}{
			"id": r.QueryRestaurant[rand.Intn(len(r.QueryRestaurant))].ID,
		},
	}

	for i := 0; i < b.N; i++ {
		gqlResponse := params.ExecuteAsPost(b, graphqlURL)
		require.Nilf(b, gqlResponse.Errors, "%+v", gqlResponse.Errors)
		params.Variables["id"] = r.QueryRestaurant[rand.Intn(len(r.QueryRestaurant))].ID
	}
}
