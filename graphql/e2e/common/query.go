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

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
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

// This test checks that all the different combinations of
// request sending compressed / uncompressed query and receiving
// compressed / uncompressed result.
func gzipCompression(t *testing.T) {
	r := []bool{false, true}
	for _, acceptGzip := range r {
		for _, gzipEncoding := range r {
			t.Run(fmt.Sprintf("TestQueryByType acceptGzip=%t gzipEncoding=%t",
				acceptGzip, gzipEncoding), func(t *testing.T) {

				queryByTypeWithEncoding(t, acceptGzip, gzipEncoding)
			})
		}
	}
}

func queryByType(t *testing.T) {
	queryByTypeWithEncoding(t, true, true)
}

func queryByTypeWithEncoding(t *testing.T, acceptGzip, gzipEncoding bool) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry {
				name
			}
		}`,
		acceptGzip:   acceptGzip,
		gzipEncoding: gzipEncoding,
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

func uidAlias(t *testing.T) {
	queryCountry := &GraphQLParams{
		Query: `query {
			queryCountry(order: { asc: name }) {
				uid: name
			}
		}`,
	}
	type countryUID struct {
		UID string
	}

	gqlResponse := queryCountry.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		QueryCountry []*countryUID
	}
	expected.QueryCountry = []*countryUID{
		&countryUID{UID: "Angola"},
		&countryUID{UID: "Bangladesh"},
		&countryUID{UID: "Mozambique"},
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func orderAtRoot(t *testing.T) {
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

func pageAtRoot(t *testing.T) {
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

func regExp(t *testing.T) {
	queryCountryByRegExp(t, "/[Aa]ng/",
		[]*country{
			&country{Name: "Angola"},
			&country{Name: "Bangladesh"},
		})
}

func multipleSearchIndexes(t *testing.T) {
	query := `query queryPost($filter: PostFilter){
		  queryPost (filter: $filter) {
		      title
		  }
	}`

	testCases := []interface{}{
		map[string]interface{}{"title": map[string]interface{}{"anyofterms": "Introducing"}},
		map[string]interface{}{"title": map[string]interface{}{"alloftext": "Introducing GraphQL in Dgraph"}},
	}
	for _, filter := range testCases {
		getCountryParams := &GraphQLParams{
			Query:     query,
			Variables: map[string]interface{}{"filter": filter},
		}

		gqlResponse := getCountryParams.ExecuteAsPost(t, graphqlURL)
		require.Nil(t, gqlResponse.Errors)

		var expected, result struct {
			QueryPost []*post
		}

		expected.QueryPost = []*post{
			&post{Title: "Introducing GraphQL in Dgraph"},
		}
		err := json.Unmarshal([]byte(gqlResponse.Data), &result)
		require.NoError(t, err)

		if diff := cmp.Diff(expected, result); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	}
}

func multipleSearchIndexesWrongField(t *testing.T) {
	getCountryParams := &GraphQLParams{
		Query: `query {
			queryPost (filter: {title : { regexp : "/Introducing.*$/" }} ) {
			    title
			}
		}`,
	}

	gqlResponse := getCountryParams.ExecuteAsPost(t, graphqlURL)
	require.NotNil(t, gqlResponse.Errors)

	expected := `Field "regexp" is not defined by type StringFullTextFilter_StringTermFilter`
	require.Contains(t, gqlResponse.Errors[0].Error(), expected)
}

func hashSearch(t *testing.T) {
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

func deepFilter(t *testing.T) {
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

// manyQueries runs multiple queries in the one block.  Internally, the GraphQL
// server should run those concurrently, but the results should be returned in the
// requested order.  This makes sure those many test runs are reassembled correctly.
func manyQueries(t *testing.T) {
	posts := allPosts(t)

	getPattern := `getPost(id: "%s") {
		postID
		title
		text
		tags
		isPublished
		postType
		numLikes
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

func queryOrderAtRoot(t *testing.T) {
	posts := allPosts(t)

	answers := make([]*post, 2)
	for _, p := range posts {
		if p.NumLikes == 77 {
			answers[0] = p
		} else if p.NumLikes == 100 {
			answers[1] = p
		}
	}

	filter := map[string]interface{}{
		"ids": []string{answers[0].PostID, answers[1].PostID},
	}

	orderLikesDesc := map[string]interface{}{
		"desc": "numLikes",
	}

	orderLikesAsc := map[string]interface{}{
		"asc": "numLikes",
	}

	var result, expected struct {
		QueryPost []*post
	}

	cases := map[string]struct {
		Order    map[string]interface{}
		First    int
		Offset   int
		Expected []*post
	}{
		"orderAsc": {
			Order:    orderLikesAsc,
			First:    2,
			Offset:   0,
			Expected: []*post{answers[0], answers[1]},
		},
		"orderDesc": {
			Order:    orderLikesDesc,
			First:    2,
			Offset:   0,
			Expected: []*post{answers[1], answers[0]},
		},
		"first": {
			Order:    orderLikesDesc,
			First:    1,
			Offset:   0,
			Expected: []*post{answers[1]},
		},
		"offset": {
			Order:    orderLikesDesc,
			First:    2,
			Offset:   1,
			Expected: []*post{answers[0]},
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			getParams := &GraphQLParams{
				Query: `query queryPost($filter: PostFilter, $order: PostOrder,
		$first: Int, $offset: Int) {
			queryPost(
			  filter: $filter,
			  order: $order,
			  first: $first,
			  offset: $offset) {
				postID
				title
				text
				tags
				isPublished
				postType
				numLikes
			}
		}
		`,
				Variables: map[string]interface{}{
					"filter": filter,
					"order":  test.Order,
					"first":  test.First,
					"offset": test.Offset,
				},
			}

			gqlResponse := getParams.ExecuteAsPost(t, graphqlURL)
			require.Nil(t, gqlResponse.Errors)

			expected.QueryPost = test.Expected
			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.NoError(t, err)

			require.Equal(t, len(result.QueryPost), len(expected.QueryPost))
			if diff := cmp.Diff(expected, result); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}

}

// queriesWithError runs multiple queries in the one block with
// an error.  Internally, the GraphQL server should run those concurrently, and
// an error in one query should not affect the results of any others.
func queriesWithError(t *testing.T) {
	posts := allPosts(t)

	getPattern := `getPost(id: "%s") {
		postID
		title
		text
		tags
		isPublished
		postType
		numLikes
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

func dateFilters(t *testing.T) {
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

func floatFilters(t *testing.T) {
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

func intFilters(t *testing.T) {
	cases := map[string]struct {
		Filter   interface{}
		Expected []*post
	}{
		"less than": {
			Filter: map[string]interface{}{"numLikes": map[string]interface{}{"lt": 87}},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Random post"}}},
		"less or equal": {
			Filter: map[string]interface{}{"numLikes": map[string]interface{}{"le": 87}},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Learning GraphQL in Dgraph"},
				{Title: "Random post"}}},
		"equal": {
			Filter:   map[string]interface{}{"numLikes": map[string]interface{}{"eq": 87}},
			Expected: []*post{{Title: "Learning GraphQL in Dgraph"}}},
		"greater or equal": {
			Filter: map[string]interface{}{"numLikes": map[string]interface{}{"ge": 87}},
			Expected: []*post{
				{Title: "Introducing GraphQL in Dgraph"},
				{Title: "Learning GraphQL in Dgraph"}}},
		"greater than": {
			Filter:   map[string]interface{}{"numLikes": map[string]interface{}{"gt": 87}},
			Expected: []*post{{Title: "Introducing GraphQL in Dgraph"}}},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			postTest(t, test.Filter, test.Expected)
		})
	}
}

func booleanFilters(t *testing.T) {
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

func termFilters(t *testing.T) {
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

func fullTextFilters(t *testing.T) {
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

func stringExactFilters(t *testing.T) {
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

func scalarListFilters(t *testing.T) {

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

func skipDirective(t *testing.T) {
	getAuthorParams := &GraphQLParams{
		Query: `query ($skipPost: Boolean!, $skipName: Boolean!) {
			queryAuthor(filter: { name: { eq: "Ann Other Author" } }) {
				name @skip(if: $skipName)
				dob
				reputation
				posts @skip(if: $skipPost) {
					title
				}
			}
		}`,
		Variables: map[string]interface{}{
			"skipPost": true,
			"skipName": false,
		},
	}

	gqlResponse := getAuthorParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	expected := `{"queryAuthor":[{"name":"Ann Other Author",
		"dob":"1988-01-01T00:00:00Z","reputation":8.9}]}`
	require.JSONEq(t, expected, string(gqlResponse.Data))
}

func includeDirective(t *testing.T) {
	getAuthorParams := &GraphQLParams{
		Query: `query ($includeName: Boolean!, $includePost: Boolean!) {
			queryAuthor(filter: { name: { eq: "Ann Other Author" } }) {
			  name @include(if: $includeName)
			  dob
			  posts @include(if: $includePost) {
				title
			  }
			}
		  }`,
		Variables: map[string]interface{}{
			"includeName": true,
			"includePost": false,
		},
	}

	gqlResponse := getAuthorParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	expected := `{"queryAuthor":[{"name":"Ann Other Author","dob":"1988-01-01T00:00:00Z"}]}`
	require.JSONEq(t, expected, string(gqlResponse.Data))
}

func includeAndSkipDirective(t *testing.T) {
	getAuthorParams := &GraphQLParams{
		Query: `query ($includeFalse: Boolean!, $skipTrue: Boolean!, $includeTrue: Boolean!,
			$skipFalse: Boolean!) {
			queryAuthor (filter: { name: { eq: "Ann Other Author" } }) {
			  dob @include(if: $includeFalse) @skip(if: $skipFalse)
			  reputation @include(if: $includeFalse) @skip(if: $skipTrue)
			  name @include(if: $includeTrue) @skip(if: $skipFalse)
			  posts(filter: { title: { anyofterms: "GraphQL" } }, first: 10)
			    @include(if: $includeTrue) @skip(if: $skipTrue) {
				title
				tags
			  }
			}
		  }`,
		Variables: map[string]interface{}{
			"includeFalse": false,
			"includeTrue":  true,
			"skipFalse":    false,
			"skipTrue":     true,
		},
	}

	gqlResponse := getAuthorParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	expected := `{"queryAuthor":[{"name":"Ann Other Author"}]}`
	require.JSONEq(t, expected, string(gqlResponse.Data))
}

func queryByMultipleIds(t *testing.T) {
	posts := allPosts(t)
	ids := make([]string, 0, len(posts))
	for _, post := range posts {
		ids = append(ids, post.PostID)
	}

	queryParams := &GraphQLParams{
		Query: `query queryPost($filter: PostFilter) {
			queryPost(filter: $filter) {
				postID
				title
				text
				tags
				numLikes
				isPublished
				postType
			}
		}`,
		Variables: map[string]interface{}{"filter": map[string]interface{}{
			"ids": ids,
		}},
	}

	gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		QueryPost []*post
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)
	if diff := cmp.Diff(posts, result.QueryPost); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func enumFilter(t *testing.T) {
	posts := allPosts(t)

	queryParams := &GraphQLParams{
		Query: `query queryPost($filter: PostFilter) {
			queryPost(filter: $filter) {
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

	facts := make([]*post, 0, len(posts))
	questions := make([]*post, 0, len(posts))
	for _, post := range posts {
		if post.PostType == "Fact" {
			facts = append(facts, post)
		}
		if post.PostType == "Question" {
			questions = append(questions, post)
		}
	}

	cases := map[string]struct {
		Filter   interface{}
		Expected []*post
	}{
		"Hash Filter test": {
			Filter: map[string]interface{}{
				"postType": map[string]interface{}{
					"eq": "Fact",
				},
			},
			Expected: facts,
		},

		"Regexp Filter test": {
			Filter: map[string]interface{}{
				"postType": map[string]interface{}{
					"regexp": "/(Fact)|(Question)/",
				},
			},
			Expected: append(questions, facts...),
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			queryParams.Variables = map[string]interface{}{"filter": test.Filter}

			gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
			requireNoGQLErrors(t, gqlResponse)

			var result struct {
				QueryPost []*post
			}

			postSort := func(i, j int) bool {
				return result.QueryPost[i].Title < result.QueryPost[j].Title
			}
			testSort := func(i, j int) bool {
				return test.Expected[i].Title < test.Expected[j].Title
			}

			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			sort.Slice(result.QueryPost, postSort)
			sort.Slice(test.Expected, testSort)

			require.NoError(t, err)
			if diff := cmp.Diff(test.Expected, result.QueryPost); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})

	}
}

func defaultEnumFilter(t *testing.T) {
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)
	droidID := addDroid(t)
	updateCharacter(t, humanID)

	t.Run("test query enum default index on appearsIn", func(t *testing.T) {
		queryCharacterParams := &GraphQLParams{
			Query: `query {
				queryCharacter (filter: {
					appearsIn: {
						eq: EMPIRE
					}
				}) {
					name
					appearsIn
				}
			}`,
		}

		gqlResponse := queryCharacterParams.ExecuteAsPost(t, graphqlURL)
		requireNoGQLErrors(t, gqlResponse)

		expected := `{
		"queryCharacter": [
		  {
			"name":"Han Solo",
			"appearsIn": ["EMPIRE"]
		  },
		  {
			"name": "R2-D2",
			"appearsIn": ["EMPIRE"]
		  }
		]
	  }`
		testutil.CompareJSON(t, expected, string(gqlResponse.Data))
	})

	cleanupStarwars(t, newStarship.ID, humanID, droidID)
}

func queryByMultipleInvalidIds(t *testing.T) {
	queryParams := &GraphQLParams{
		Query: `query queryPost($filter: PostFilter) {
			queryPost(filter: $filter) {
				postID
				title
				text
				tags
				numLikes
				isPublished
				postType
			}
		}`,
		Variables: map[string]interface{}{"filter": map[string]interface{}{
			"ids": []string{"foo", "bar"},
		}},
	}
	// Since the ids are invalid and can't be converted to uint64, the query sent to Dgraph should
	// have func: uid() at root and should return 0 results.

	gqlResponse := queryParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	require.Equal(t, `{"queryPost":[]}`, string(gqlResponse.Data))
	var result struct {
		QueryPost []*post
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)
	require.Equal(t, 0, len(result.QueryPost))
}

func getStateByXid(t *testing.T) {
	getStateParams := &GraphQLParams{
		Query: `{
			getState(xcode: "nsw") {
				name
			}
		}`,
	}

	gqlResponse := getStateParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
	require.Equal(t, `{"getState":{"name":"NSW"}}`, string(gqlResponse.Data))
}

func getStateWithoutArgs(t *testing.T) {
	getStateParams := &GraphQLParams{
		Query: `{
			getState {
				name
			}
		}`,
	}

	gqlResponse := getStateParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
	require.JSONEq(t, `{"getState":null}`, string(gqlResponse.Data))
}

func getStateByBothXidAndUid(t *testing.T) {
	getStateParams := &GraphQLParams{
		Query: `{
			getState(xcode: "nsw", id: "0x1") {
				name
			}
		}`,
	}

	gqlResponse := getStateParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
	require.JSONEq(t, `{"getState":null}`, string(gqlResponse.Data))
}

func queryStateByXid(t *testing.T) {
	getStateParams := &GraphQLParams{
		Query: `{
			queryState(filter: { xcode: { eq: "nsw"}}) {
				name
			}
		}`,
	}

	gqlResponse := getStateParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
	require.Equal(t, `{"queryState":[{"name":"NSW"}]}`, string(gqlResponse.Data))
}

func queryStateByXidRegex(t *testing.T) {
	getStateParams := &GraphQLParams{
		Query: `{
			queryState(filter: { xcode: { regexp: "/n/"}}) {
				name
			}
		}`,
	}

	gqlResponse := getStateParams.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)
	testutil.CompareJSON(t, `{"queryState":[{"name":"Nusa"},{"name": "NSW"}]}`, string(gqlResponse.Data))
}

func multipleOperations(t *testing.T) {
	params := &GraphQLParams{
		Query: `query sortCountryByNameDesc {
			queryCountry(order: { desc: name }, first: 1) {
				name
			}
		}

		query sortCountryByNameAsc {
			queryCountry(order: { asc: name }, first: 1) {
				name
			}
		}
		`,
		OperationName: "sortCountryByNameAsc",
	}

	gqlResponse := params.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	var expected, result struct {
		QueryCountry []*country
	}
	expected.QueryCountry = []*country{{Name: "Angola"}}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	params.OperationName = "sortCountryByNameDesc"
	gqlResponse = params.ExecuteAsPost(t, graphqlURL)
	require.Nil(t, gqlResponse.Errors)

	expected.QueryCountry = []*country{{Name: "Mozambique"}}
	err = json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func multipleOperationsWithIncorrectOperationName(t *testing.T) {
	params := &GraphQLParams{
		Query: `query sortCountryByNameDesc {
			queryCountry(order: { desc: name }, first: 1) {
				name
			}
		}

		query sortCountryByNameAsc {
			queryCountry(order: { asc: name }, first: 1) {
				name
			}
		}
		`,
	}

	gqlResponse := params.ExecuteAsPost(t, graphqlURL)
	require.NotNil(t, gqlResponse.Errors)
	require.Equal(t, "unable to find operation to resolve: []", gqlResponse.Errors[0].Error())

	params.OperationName = "sortCountryByName"
	gqlResponse = params.ExecuteAsPost(t, graphqlURL)
	require.NotNil(t, gqlResponse.Errors)
	require.Equal(t, "unable to find operation to resolve: [sortCountryByName]",
		gqlResponse.Errors[0].Error())
}
