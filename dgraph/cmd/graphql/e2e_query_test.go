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
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func queryCountryByRegExp(t *testing.T, regexp string, expectedCountries []*country) {
	getCountryParams := &GraphQLParams{
		Query: `query queryCountry($regexp: String!) {
			queryCountry(filter: { name: { regexp: $regexp } }) {
				name
			}
		}`,
		Variables: map[string]interface{}{"regexp": regexp},
	}

	gqlResponse := getCountryParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		QueryCountry []*country
	}
	expected.QueryCountry = expectedCountries
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestQueryByType(t *testing.T) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry {
				name
			}
		}`,
	}

	gqlResponse := queryCountry.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		QueryCountry []*country
	}
	expected.QueryCountry = []*country{
		&country{Name: "Bangladesh"},
		&country{Name: "Mozambique"},
		&country{Name: "Angola"},
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestOrderAtRoot(t *testing.T) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry(order: { asc: name }) {
				name
			}
		}`,
	}

	gqlResponse := queryCountry.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		QueryCountry []*country
	}
	expected.QueryCountry = []*country{
		&country{Name: "Angola"},
		&country{Name: "Bangladesh"},
		&country{Name: "Mozambique"},
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestPageAtRoot(t *testing.T) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry(order: { desc: name }, first: 2, offset: 1) {
				name
			}
		}`,
	}

	gqlResponse := queryCountry.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		QueryCountry []*country
	}
	expected.QueryCountry = []*country{
		&country{Name: "Bangladesh"},
		&country{Name: "Angola"},
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestRegExp(t *testing.T) {
	queryCountryByRegExp(t, "/[Aa]ng/",
		[]*country{
			&country{Name: "Bangladesh"},
			&country{Name: "Angola"},
		})
}

func TestHashSearch(t *testing.T) {
	getCountryParams := &GraphQLParams{
		Query: `query {
			queryAuthor(filter: { name: { eq: "An Author" } }) {
				name
				dob
			}
		}`,
	}

	gqlResponse := getCountryParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		QueryAuthor []*author
	}
	expected.QueryAuthor = []*author{
		&author{Name: "An Author", Dob: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)}}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func allPosts(t *testing.T) []*post {
	queryAuthorParams := &GraphQLParams{
		Query: `query {
			queryPost {
				postID
				title
				text
				tags
				numLikes
				isPublished
				postType
			}
		}`,
	}
	gqlResponse := queryAuthorParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		QueryPost []*post
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	require.Equal(t, 4, len(result.QueryPost))

	return result.QueryPost
}

// TestManyQueries runs multiple queries in the one block.  Internally, the GraphQL
// server should run those concurrently, but the results should be returned in the
// requested order.  This makes sure those many test runs are reassembled correctly.
func TestManyQueries(t *testing.T) {
	posts := allPosts(t)

	getPattern := `getPost(id: "%s") {
		postID
		title
		text
		tags
		isPublished
		postType
	}
	`

	var bld strings.Builder
	bld.WriteString("query {\n")
	for idx, p := range posts {
		bld.WriteString(fmt.Sprintf("  query%v : ", idx))
		bld.WriteString(fmt.Sprintf(getPattern, p.PostID))
	}
	bld.WriteString("}")

	queryParams := &GraphQLParams{
		Query: bld.String(),
	}

	gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var result map[string]*post
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	for idx, expectedPost := range posts {
		resultPost := result[fmt.Sprintf("query%v", idx)]
		if diff := cmp.Diff(expectedPost, resultPost); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	}
}

