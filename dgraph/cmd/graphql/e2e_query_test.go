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
	"math/rand"
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
			queryAuthor(filter: { name: { eq: "Ann Author" } }) {
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
		&author{Name: "Ann Author", Dob: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)}}
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

func TestDeepFilter(t *testing.T) {
	getAuthorParams := &GraphQLParams{
		Query: `query {
			queryAuthor(filter: { name: { eq: "Ann Other Author" } }) {
				name
				posts(filter: { title: { anyofterms: "GraphQL" } }) {
					title
				}
			}
		}`,
	}

	gqlResponse := getAuthorParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var result struct {
		QueryAuthor []*author
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.QueryAuthor))

	expected := &author{
		Name:  "Ann Other Author",
		Posts: []post{{Title: "GraphQL in Dgraph"}},
	}

	if diff := cmp.Diff(expected, result.QueryAuthor[0]); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
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

// TestManyTestQueriesWithErrorQueries runs multiple queries in the one block with
// an error.  Internally, the GraphQL server should run those concurrently, and
// an error in one query should not affect the results of any others.
func TestQueriesWithError(t *testing.T) {
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

	// make one random query fail
	shouldFail := rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(posts))

	var bld strings.Builder
	bld.WriteString("query {\n")
	for idx, p := range posts {
		bld.WriteString(fmt.Sprintf("  query%v : ", idx))
		if idx == shouldFail {
			bld.WriteString(fmt.Sprintf(getPattern, "Not_An_ID"))
		} else {
			bld.WriteString(fmt.Sprintf(getPattern, p.PostID))
		}
	}
	bld.WriteString("}")

	queryParams := &GraphQLParams{
		Query: bld.String(),
	}

	gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
	require.Len(t, gqlResponse.Errors, 1, "expected 1 error from malformed query")

	var result map[string]*post
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	for idx, expectedPost := range posts {
		resultPost := result[fmt.Sprintf("query%v", idx)]
		if idx == shouldFail {
			require.Nil(t, resultPost, "expected this query to fail and return nil")
		} else {
			if diff := cmp.Diff(expectedPost, resultPost); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		}
	}
}
