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
	"sort"
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

	countrySort := func(i, j int) bool {
		return result.QueryCountry[i].Name < result.QueryCountry[j].Name
	}
	sort.Slice(result.QueryCountry, countrySort)

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
		&country{Name: "Angola"},
		&country{Name: "Bangladesh"},
		&country{Name: "Mozambique"},
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	sort.Slice(result.QueryCountry, func(i, j int) bool {
		return result.QueryCountry[i].Name < result.QueryCountry[j].Name
	})

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
			&country{Name: "Angola"},
			&country{Name: "Bangladesh"},
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
		Posts: []post{{Title: "Learning GraphQL in Dgraph"}},
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

func TestDateFilters(t *testing.T) {
	cases := map[string]struct {
		Filter   interface{}
		Expected []*author
	}{
		"less than": {
			Filter:   map[string]interface{}{"dob": map[string]interface{}{"lt": "2000-01-01"}},
			Expected: []*author{{Name: "Ann Other Author"}}},
		"less or equal": {
			Filter:   map[string]interface{}{"dob": map[string]interface{}{"le": "2000-01-01"}},
			Expected: []*author{{Name: "Ann Author"}, {Name: "Ann Other Author"}}},
		"equal": {
			Filter:   map[string]interface{}{"dob": map[string]interface{}{"eq": "2000-01-01"}},
			Expected: []*author{{Name: "Ann Author"}}},
		"greater or equal": {
			Filter:   map[string]interface{}{"dob": map[string]interface{}{"ge": "2000-01-01"}},
			Expected: []*author{{Name: "Ann Author"}, {Name: "Three Author"}}},
		"greater than": {
			Filter:   map[string]interface{}{"dob": map[string]interface{}{"gt": "2000-01-01"}},
			Expected: []*author{{Name: "Three Author"}}},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			authorTest(t, test.Filter, test.Expected)
		})
	}
}

func TestFloatFilters(t *testing.T) {
	cases := map[string]struct {
		Filter   interface{}
		Expected []*author
	}{
		"less than": {
			Filter:   map[string]interface{}{"reputation": map[string]interface{}{"lt": 8.9}},
			Expected: []*author{{Name: "Ann Author"}}},
		"less or equal": {
			Filter:   map[string]interface{}{"reputation": map[string]interface{}{"le": 8.9}},
			Expected: []*author{{Name: "Ann Author"}, {Name: "Ann Other Author"}}},
		"equal": {
			Filter:   map[string]interface{}{"reputation": map[string]interface{}{"eq": 8.9}},
			Expected: []*author{{Name: "Ann Other Author"}}},
		"greater or equal": {
			Filter:   map[string]interface{}{"reputation": map[string]interface{}{"ge": 8.9}},
			Expected: []*author{{Name: "Ann Other Author"}, {Name: "Three Author"}}},
		"greater than": {
			Filter:   map[string]interface{}{"reputation": map[string]interface{}{"gt": 8.9}},
			Expected: []*author{{Name: "Three Author"}}},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			authorTest(t, test.Filter, test.Expected)
		})
	}
}

func authorTest(t *testing.T, filter interface{}, expected []*author) {
	queryParams := &GraphQLParams{
		Query: `query filterVariable($filter: AuthorFilter) {
			queryAuthor(filter: $filter, order: { asc: name }) {
				name
			}
		}`,
		Variables: map[string]interface{}{"filter": filter},
	}

	gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		QueryAuthor []*author
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result.QueryAuthor); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

// FIXME: Int is currently not working in API.  It's because it doesn't deserialize properly.
// We just need to look at the expected type and make sure it's an in.  There's a card in Asana.
// func TestIntFilters(t *testing.T) {
// 	cases := map[string]struct {
// 		Filter   interface{}
// 		Expected []*post
// 	}{
// 		"less than": {
// 			Filter: map[string]interface{}{"numLikes": map[string]interface{}{"lt": 87}},
// 			Expected: []*post{
// 				{Title: "GraphQL in Dgraph doco"},
// 				{Title: "Random post"}}},
// 		"less or equal": {
// 			Filter: map[string]interface{}{"numLikes": map[string]interface{}{"le": 87}},
// 			Expected: []*post{
// 				{Title: "GraphQL in Dgraph doco"},
// 				{Title: "Learning GraphQL in Dgraph"},
// 				{Title: "Random post"}}},
// 		"equal": {
// 			Filter:   map[string]interface{}{"numLikes": map[string]interface{}{"eq": 87}},
// 			Expected: []*post{{Title: "Learning GraphQL in Dgraph"}}},
// 		"greater or equal": {
// 			Filter: map[string]interface{}{"numLikes": map[string]interface{}{"ge": 87}},
// 			Expected: []*post{
// 				{Title: "Introducing GraphQL in Dgraph"},
// 				{Title: "Learning GraphQL in Dgraph"}}},
// 		"greater than": {
// 			Filter:   map[string]interface{}{"numLikes": map[string]interface{}{"gt": 87}},
// 			Expected: []*post{{Title: "Introducing GraphQL in Dgraph"}}},
// 	}

// 	for name, test := range cases {
// 		t.Run(name, func(t *testing.T) {
// 			postTest(t, test.Filter, test.Expected)
// 		})
// 	}
// }

func TestBooleanFilters(t *testing.T) {
	cases := map[string]struct {
		Filter   interface{}
		Expected []*post
	}{
		"true": {
			Filter: map[string]interface{}{"isPublished": true},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Introducing GraphQL in Dgraph"},
				{Title: "Learning GraphQL in Dgraph"}}},
		"false": {
			Filter:   map[string]interface{}{"isPublished": false},
			Expected: []*post{{Title: "Random post"}}},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			postTest(t, test.Filter, test.Expected)
		})
	}
}

func TestTermFilters(t *testing.T) {
	cases := map[string]struct {
		Filter   interface{}
		Expected []*post
	}{
		"all of terms": {
			Filter: map[string]interface{}{
				"title": map[string]interface{}{"allofterms": "GraphQL Dgraph"}},
			Expected: []*post{
				{Title: "Introducing GraphQL in Dgraph"},
				{Title: "Learning GraphQL in Dgraph"}}},
		"any of terms": {
			Filter: map[string]interface{}{
				"title": map[string]interface{}{"anyofterms": "GraphQL Dgraph"}},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Introducing GraphQL in Dgraph"},
				{Title: "Learning GraphQL in Dgraph"}}},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			postTest(t, test.Filter, test.Expected)
		})
	}
}

func TestFullTextFilters(t *testing.T) {
	cases := map[string]struct {
		Filter   interface{}
		Expected []*post
	}{
		"all of text": {
			Filter: map[string]interface{}{
				"text": map[string]interface{}{"alloftext": "learn GraphQL"}},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Learning GraphQL in Dgraph"}}},
		"any of text": {
			Filter: map[string]interface{}{
				"text": map[string]interface{}{"anyoftext": "learn GraphQL"}},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Introducing GraphQL in Dgraph"},
				{Title: "Learning GraphQL in Dgraph"}}},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			postTest(t, test.Filter, test.Expected)
		})
	}
}

func TestStringExactFilters(t *testing.T) {
	cases := map[string]struct {
		Filter   interface{}
		Expected []*post
	}{
		"less than": {
			Filter:   map[string]interface{}{"topic": map[string]interface{}{"lt": "GraphQL"}},
			Expected: []*post{{Title: "GraphQL doco"}}},
		"less or equal": {
			Filter: map[string]interface{}{"topic": map[string]interface{}{"le": "GraphQL"}},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Introducing GraphQL in Dgraph"}}},
		"equal": {
			Filter:   map[string]interface{}{"topic": map[string]interface{}{"eq": "GraphQL"}},
			Expected: []*post{{Title: "Introducing GraphQL in Dgraph"}}},
		"greater or equal": {
			Filter: map[string]interface{}{"topic": map[string]interface{}{"ge": "GraphQL"}},
			Expected: []*post{
				{Title: "Introducing GraphQL in Dgraph"},
				{Title: "Learning GraphQL in Dgraph"},
				{Title: "Random post"}}},
		"greater than": {
			Filter: map[string]interface{}{"topic": map[string]interface{}{"gt": "GraphQL"}},
			Expected: []*post{
				{Title: "Learning GraphQL in Dgraph"},
				{Title: "Random post"}}},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			postTest(t, test.Filter, test.Expected)
		})
	}
}

func TestScalarListFilters(t *testing.T) {

	// tags is a list of strings with @search(by: exact).  So all the filters
	// lt, le, ... mean "is there something in the list that's lt 'Dgraph'", etc.

	cases := map[string]struct {
		Filter   interface{}
		Expected []*post
	}{
		"less than": {
			Filter:   map[string]interface{}{"tags": map[string]interface{}{"lt": "Dgraph"}},
			Expected: []*post{{Title: "Introducing GraphQL in Dgraph"}}},
		"less or equal": {
			Filter: map[string]interface{}{"tags": map[string]interface{}{"le": "Dgraph"}},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Introducing GraphQL in Dgraph"},
				{Title: "Learning GraphQL in Dgraph"}}},
		"equal": {
			Filter:   map[string]interface{}{"tags": map[string]interface{}{"eq": "Database"}},
			Expected: []*post{{Title: "Introducing GraphQL in Dgraph"}}},
		"greater or equal": {
			Filter: map[string]interface{}{"tags": map[string]interface{}{"ge": "Dgraph"}},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Introducing GraphQL in Dgraph"},
				{Title: "Learning GraphQL in Dgraph"},
				{Title: "Random post"}}},
		"greater than": {
			Filter:   map[string]interface{}{"tags": map[string]interface{}{"gt": "GraphQL"}},
			Expected: []*post{{Title: "Random post"}}},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			postTest(t, test.Filter, test.Expected)
		})
	}
}

func postTest(t *testing.T, filter interface{}, expected []*post) {
	queryParams := &GraphQLParams{
		Query: `query filterVariable($filter: PostFilter) {
			queryPost(filter: $filter, order: { asc: title }) {
				title
			}
		}`,
		Variables: map[string]interface{}{"filter": filter},
	}

	gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		QueryPost []*post
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result.QueryPost); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func TestSkipDirective(t *testing.T) {
	getAuthorParams := &GraphQLParams{
		Query: `query ($skipTrue: Boolean!, $skipFalse: Boolean!) {
			queryAuthor(filter: { name: { eq: "Ann Other Author" } }) {
				name @skip(if: $skipFalse)
				dob
				reputation
				posts @skip(if: $skipTrue) {
					title
				}
			}
		}`,
		Variables: map[string]interface{}{
			"skipTrue":  true,
			"skipFalse": false,
		},
	}

	gqlResponse := getAuthorParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	expected := `{"queryAuthor":[{"name":"Ann Other Author",
		"dob":"1988-01-01T00:00:00Z","reputation":8.9}]}`
	require.JSONEq(t, expected, string(gqlResponse.Data))
}

func TestIncludeDirective(t *testing.T) {
	getAuthorParams := &GraphQLParams{
		Query: `query ($includeTrue: Boolean!, $includeFalse: Boolean!) {
			queryAuthor(filter: { name: { eq: "Ann Other Author" } }) {
			  name @include(if: $includeTrue)
			  dob
			  posts @include(if: $includeFalse) {
				title
			  }
			}
		  }`,
		Variables: map[string]interface{}{
			"includeTrue":  true,
			"includeFalse": false,
		},
	}

	gqlResponse := getAuthorParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	expected := `{"queryAuthor":[{"name":"Ann Other Author","dob":"1988-01-01T00:00:00Z"}]}`
	require.JSONEq(t, expected, string(gqlResponse.Data))
}
