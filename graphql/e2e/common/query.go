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
	"io/ioutil"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
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

	gqlResponse := getCountryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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

func touchedUidsHeader(t *testing.T) {
	query := &GraphQLParams{
		Query: `query {
			queryCountry {
				name
			}
		}`,
	}
	req, err := query.CreateGQLPost(GraphqlURL)
	require.NoError(t, err)

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)

	// confirm that the header value is a non-negative integer
	touchedUidsInHeader, err := strconv.ParseUint(resp.Header.Get("Graphql-TouchedUids"), 10, 64)
	require.NoError(t, err)

	// confirm that the value in header is same as the value in body
	var gqlResp GraphQLResponse
	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &gqlResp))
	require.Equal(t, touchedUidsInHeader, uint64(gqlResp.Extensions["touched_uids"].(float64)))
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

	gqlResponse := queryCountry.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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

	gqlResponse := queryCountry.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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

	gqlResponse := queryCountry.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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

	gqlResponse := queryCountry.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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
		map[string]interface{}{"title": map[string]interface {
		}{"alloftext": "Introducing GraphQL in Dgraph"}},
	}
	for _, filter := range testCases {
		getCountryParams := &GraphQLParams{
			Query:     query,
			Variables: map[string]interface{}{"filter": filter},
		}

		gqlResponse := getCountryParams.ExecuteAsPost(t, GraphqlURL)
		RequireNoGQLErrors(t, gqlResponse)

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
	queryPostParams := &GraphQLParams{
		Query: `query {
			queryPost (filter: {title : { regexp : "/Introducing.*$/" }} ) {
			    title
			}
		}`,
	}

	gqlResponse := queryPostParams.ExecuteAsPost(t, GraphqlURL)
	require.NotNil(t, gqlResponse.Errors)

	expected := `Field "regexp" is not defined by type StringFullTextFilter_StringTermFilter`
	require.Contains(t, gqlResponse.Errors[0].Error(), expected)
}

func hashSearch(t *testing.T) {
	queryAuthorParams := &GraphQLParams{
		Query: `query {
			queryAuthor(filter: { name: { eq: "Ann Author" } }) {
				name
				dob
			}
		}`,
	}

	gqlResponse := queryAuthorParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	var expected, result struct {
		QueryAuthor []*author
	}
	dob := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	expected.QueryAuthor = []*author{{Name: "Ann Author", Dob: &dob}}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func allPosts(t *testing.T) []*post {
	queryPostParams := &GraphQLParams{
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
	gqlResponse := queryPostParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	var result struct {
		QueryPost []*post
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	require.Equal(t, 4, len(result.QueryPost))

	return result.QueryPost
}

func inFilter(t *testing.T) {
	addStateParams := &GraphQLParams{
		Query: `mutation addState($name1: String!, $code1: String!, $name2: String!, $code2: String! ) {
			addState(input: [{name: $name1, xcode: $code1},{name: $name2, xcode: $code2}]) {
				state {
					xcode
					name
				}
			}
		}`,

		Variables: map[string]interface{}{
			"name1": "A State",
			"code1": "abc",
			"name2": "B State",
			"code2": "def",
		},
	}

	gqlResponse := addStateParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	updateStateParams := &GraphQLParams{
		Query: `mutation{
			updateState(input: { 
				filter: {
					xcode: { in: ["abc", "def"]}},
				set: { 
					capital: "Common Capital"} }){
				state{
					xcode
					name
					capital
				}
			}
		  }`,
	}
	gqlResponse = updateStateParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	getStateParams := &GraphQLParams{
		Query: `query{
			queryState(filter: {xcode: {in: ["abc", "def"]}}){
				xcode
				name
				capital
			}
		}`,
	}

	gqlResponse = getStateParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	var result struct {
		QueryState []*state
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)
	require.Equal(t, 2, len(result.QueryState))
	queriedResult := map[string]*state{}
	queriedResult[result.QueryState[0].Name] = result.QueryState[0]
	queriedResult[result.QueryState[1].Name] = result.QueryState[1]

	state1 := &state{
		Name:    "A State",
		Code:    "abc",
		Capital: "Common Capital",
	}
	state2 := &state{
		Name:    "B State",
		Code:    "def",
		Capital: "Common Capital",
	}

	if diff := cmp.Diff(state1, queriedResult[state1.Name]); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	if diff := cmp.Diff(state2, queriedResult[state2.Name]); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	deleteFilter := map[string]interface{}{"xcode": map[string]interface{}{"in": []string{"abc", "def"}}}
	deleteGqlType(t, "State", deleteFilter, 2, nil)
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

	gqlResponse := getAuthorParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	var result struct {
		QueryAuthor []*author
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.QueryAuthor))

	expected := &author{
		Name:  "Ann Other Author",
		Posts: []*post{{Title: "Learning GraphQL in Dgraph"}},
	}

	if diff := cmp.Diff(expected, result.QueryAuthor[0]); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func deepHasFilter(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	newAuthor := addAuthor(t, newCountry.ID, postExecutor)
	newPost1 := addPostWithNullText(t, newAuthor.ID, newCountry.ID, postExecutor)
	newPost2 := addPost(t, newAuthor.ID, newCountry.ID, postExecutor)
	getAuthorParams := &GraphQLParams{
		Query: `query {
			queryAuthor(filter: { name: { eq: "Test Author" } }) {
				name
				posts(filter: {not :{ has : text } }) {
					title
				}
			}
		}`,
	}

	gqlResponse := getAuthorParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	var result struct {
		QueryAuthor []*author
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.QueryAuthor))

	expected := &author{
		Name:  "Test Author",
		Posts: []*post{{Title: "No text"}},
	}

	if diff := cmp.Diff(expected, result.QueryAuthor[0]); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
	cleanUp(t, []*country{newCountry}, []*author{newAuthor}, []*post{newPost1, newPost2})
}

// manyQueries runs multiple queries in the one block.  Internally, the GraphQL
// server should run those concurrently, but the results should be returned in the
// requested order.  This makes sure those many test runs are reassembled correctly.
func manyQueries(t *testing.T) {
	posts := allPosts(t)

	getPattern := `getPost(postID: "%s") {
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
	x.Check2(bld.WriteString("query {\n"))
	for idx, p := range posts {
		x.Check2(bld.WriteString(fmt.Sprintf("  query%v : ", idx)))
		x.Check2(bld.WriteString(fmt.Sprintf(getPattern, p.PostID)))
	}
	x.Check2(bld.WriteString("}"))

	queryParams := &GraphQLParams{
		Query: bld.String(),
	}

	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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
		"postID": []string{answers[0].PostID, answers[1].PostID},
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

			gqlResponse := getParams.ExecuteAsPost(t, GraphqlURL)
			RequireNoGQLErrors(t, gqlResponse)

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

	getPattern := `getPost(postID: "%s") {
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
	x.Check2(bld.WriteString("query {\n"))
	for idx, p := range posts {
		x.Check2(bld.WriteString(fmt.Sprintf("  query%v : ", idx)))
		if idx == shouldFail {
			x.Check2(bld.WriteString(fmt.Sprintf(getPattern, "Not_An_ID")))
		} else {
			x.Check2(bld.WriteString(fmt.Sprintf(getPattern, p.PostID)))
		}
	}
	x.Check2(bld.WriteString("}"))

	queryParams := &GraphQLParams{
		Query: bld.String(),
	}

	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlURL)
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

	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	var result struct {
		QueryAuthor []*author
	}
	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	if diff := cmp.Diff(expected, result.QueryAuthor); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func int32Filters(t *testing.T) {
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

func hasFilters(t *testing.T) {
	newCountry := addCountry(t, postExecutor)
	newAuthor := addAuthor(t, newCountry.ID, postExecutor)
	newPost := addPostWithNullText(t, newAuthor.ID, newCountry.ID, postExecutor)

	Filter := map[string]interface{}{"has": "text"}
	Expected := []*post{
		{Title: "GraphQL doco"},
		{Title: "Introducing GraphQL in Dgraph"},
		{Title: "Learning GraphQL in Dgraph"},
		{Title: "Random post"}}

	postTest(t, Filter, Expected)
	cleanUp(t, []*country{newCountry}, []*author{newAuthor}, []*post{newPost})
}

func int64Filters(t *testing.T) {
	cases := map[string]struct {
		Filter   interface{}
		Expected []*post
	}{
		"less than": {
			Filter: map[string]interface{}{"numViews": map[string]interface{}{"lt": 274877906944}},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Random post"}}},
		"less or equal": {
			Filter: map[string]interface{}{"numViews": map[string]interface{}{"le": 274877906944}},
			Expected: []*post{
				{Title: "GraphQL doco"},
				{Title: "Learning GraphQL in Dgraph"},
				{Title: "Random post"}}},
		"equal": {
			Filter:   map[string]interface{}{"numViews": map[string]interface{}{"eq": 274877906944}},
			Expected: []*post{{Title: "Learning GraphQL in Dgraph"}}},
		"greater or equal": {
			Filter: map[string]interface{}{"numViews": map[string]interface{}{"ge": 274877906944}},
			Expected: []*post{
				{Title: "Introducing GraphQL in Dgraph"},
				{Title: "Learning GraphQL in Dgraph"}}},
		"greater than": {
			Filter:   map[string]interface{}{"numViews": map[string]interface{}{"gt": 274877906944}},
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

	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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

	gqlResponse := getAuthorParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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

	gqlResponse := getAuthorParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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

	gqlResponse := getAuthorParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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
			"postID": ids,
		}},
	}

	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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

			gqlResponse := queryParams.ExecuteAsPost(t, GraphqlURL)
			RequireNoGQLErrors(t, gqlResponse)

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

func queryApplicationGraphQl(t *testing.T) {
	getCountryParams := &GraphQLParams{
		Query: `query queryCountry {
			queryCountry {
				name
			}
		}`,
	}

	gqlResponse := getCountryParams.ExecuteAsPostApplicationGraphql(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	expected := `{
	"queryCountry": [
          { "name": "Angola"},
          { "name": "Bangladesh"},
          { "name": "Mozambique"}
        ]
}`
	testutil.CompareJSON(t, expected, string(gqlResponse.Data))

}

func queryTypename(t *testing.T) {
	getCountryParams := &GraphQLParams{
		Query: `query queryCountry {
			queryCountry {
				name
				__typename
			}
		}`,
	}

	gqlResponse := getCountryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	expected := `{
	"queryCountry": [
          {
                "name": "Angola",
                "__typename": "Country"
          },
          {
                "name": "Bangladesh",
                "__typename": "Country"
          },
          {
                "name": "Mozambique",
                "__typename": "Country"
          }
        ]
}`
	testutil.CompareJSON(t, expected, string(gqlResponse.Data))

}

func queryNestedTypename(t *testing.T) {
	getCountryParams := &GraphQLParams{
		Query: `query {
			queryAuthor(filter: { name: { eq: "Ann Author" } }) {
				name
				dob
				posts {
					title
					__typename
				}
			}
		}`,
	}

	gqlResponse := getCountryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	expected := `{
	"queryAuthor": [
	  {
		"name": "Ann Author",
		"dob": "2000-01-01T00:00:00Z",
		"posts": [
		  {
			"title": "Introducing GraphQL in Dgraph",
			"__typename": "Post"
		  },
		  {
			"title": "GraphQL doco",
			"__typename": "Post"
		  }
		]
	  }
	]
}`
	testutil.CompareJSON(t, expected, string(gqlResponse.Data))
}

func typenameForInterface(t *testing.T) {
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)
	droidID := addDroid(t)
	updateCharacter(t, humanID)

	t.Run("test __typename for interface types", func(t *testing.T) {
		queryCharacterParams := &GraphQLParams{
			Query: `query {
				queryCharacter (filter: {
					appearsIn: {
						eq: [EMPIRE]
					}
				}) {
					name
					__typename
					... on Human {
						totalCredits
			                }
					... on Droid {
						primaryFunction
			                }
				}
			}`,
		}

		expected := `{
		"queryCharacter": [
		  {
			"name":"Han Solo",
			"__typename": "Human",
			"totalCredits": 10
		  },
		  {
			"name": "R2-D2",
			"__typename": "Droid",
			"primaryFunction": "Robot"
		  }
		]
	  }`

		gqlResponse := queryCharacterParams.ExecuteAsPost(t, GraphqlURL)
		RequireNoGQLErrors(t, gqlResponse)
		testutil.CompareJSON(t, expected, string(gqlResponse.Data))
	})

	cleanupStarwars(t, newStarship.ID, humanID, droidID)
}

func queryOnlyTypename(t *testing.T) {

	newCountry1 := addCountry(t, postExecutor)
	newCountry2 := addCountry(t, postExecutor)
	newCountry3 := addCountry(t, postExecutor)

	getCountryParams := &GraphQLParams{
		Query: `query {
			queryCountry(filter: { name: {eq: "Testland"}}) {
				__typename
			}
		}`,
	}

	gqlResponse := getCountryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	expected := `{
	"queryCountry": [
         {
               "__typename": "Country"
         },
         {
               "__typename": "Country"
         },
         {
               "__typename": "Country"
         }

       ]
}`

	require.JSONEq(t, expected, string(gqlResponse.Data))
	cleanUp(t, []*country{newCountry1, newCountry2, newCountry3}, []*author{}, []*post{})
}

func querynestedOnlyTypename(t *testing.T) {

	newCountry := addCountry(t, postExecutor)
	newAuthor := addAuthor(t, newCountry.ID, postExecutor)
	newPost1 := addPost(t, newAuthor.ID, newCountry.ID, postExecutor)
	newPost2 := addPost(t, newAuthor.ID, newCountry.ID, postExecutor)
	newPost3 := addPost(t, newAuthor.ID, newCountry.ID, postExecutor)

	getCountryParams := &GraphQLParams{
		Query: `query {
			queryAuthor(filter: { name: { eq: "Test Author" } }) {
				posts {
					__typename
				}
			}
		}`,
	}

	gqlResponse := getCountryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	expected := `{
	"queryAuthor": [
	  {
		"posts": [
		  {
			"__typename": "Post"
		  },
                  {
			"__typename": "Post"
		  },
		  {

			"__typename": "Post"
		  }
		]
	  }
	]
}`
	require.JSONEq(t, expected, string(gqlResponse.Data))
	cleanUp(t, []*country{newCountry}, []*author{newAuthor}, []*post{newPost1, newPost2, newPost3})
}

func onlytypenameForInterface(t *testing.T) {
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)
	droidID := addDroid(t)
	updateCharacter(t, humanID)

	t.Run("test __typename for interface types", func(t *testing.T) {
		queryCharacterParams := &GraphQLParams{
			Query: `query {
				queryCharacter (filter: {
					appearsIn: {
						eq: [EMPIRE]
					}
				}) {


					... on Human {
						__typename
			                }
					... on Droid {
						__typename
			                }
				}
			}`,
		}

		expected := `{
		"queryCharacter": [
		  {
                      "__typename": "Human"
		  },
		  {
	             "__typename": "Droid"
		  }
		]
	  }`

		gqlResponse := queryCharacterParams.ExecuteAsPost(t, GraphqlURL)
		RequireNoGQLErrors(t, gqlResponse)
		testutil.CompareJSON(t, expected, string(gqlResponse.Data))
	})

	cleanupStarwars(t, newStarship.ID, humanID, droidID)
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
						eq: [EMPIRE]
					}
				}) {
					name
					appearsIn
				}
			}`,
		}

		gqlResponse := queryCharacterParams.ExecuteAsPost(t, GraphqlURL)
		RequireNoGQLErrors(t, gqlResponse)

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
			"postID": []string{"foo", "bar"},
		}},
	}
	// Since the ids are invalid and can't be converted to uint64, the query sent to Dgraph should
	// have func: uid() at root and should return 0 results.

	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

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

	gqlResponse := getStateParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)
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

	gqlResponse := getStateParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)
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

	gqlResponse := getStateParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)
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

	gqlResponse := getStateParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)
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

	gqlResponse := getStateParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)
	testutil.CompareJSON(t, `{"queryState":[{"name":"Nusa"},{"name": "NSW"}]}`,
		string(gqlResponse.Data))
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

	cases := []struct {
		name          string
		operationName string
		expectedError string
		expected      []*country
	}{
		{
			"second query name as operation name",
			"sortCountryByNameAsc",
			"",
			[]*country{{Name: "Angola"}},
		},
		{
			"first query name as operation name",
			"sortCountryByNameDesc",
			"",
			[]*country{{Name: "Mozambique"}},
		},
		{
			"operation name doesn't exist",
			"sortCountryByName",
			"Supplied operation name sortCountryByName isn't present in the request.",
			nil,
		},
		{
			"operation name is empty",
			"",
			"Operation name must by supplied when query has more than 1 operation.",
			nil,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			params.OperationName = test.operationName
			gqlResponse := params.ExecuteAsPost(t, GraphqlURL)
			if test.expectedError != "" {
				require.NotNil(t, gqlResponse.Errors)
				require.Equal(t, test.expectedError, gqlResponse.Errors[0].Error())
				return
			}
			RequireNoGQLErrors(t, gqlResponse)

			var expected, result struct {
				QueryCountry []*country
			}
			expected.QueryCountry = test.expected
			err := json.Unmarshal([]byte(gqlResponse.Data), &result)
			require.NoError(t, err)

			if diff := cmp.Diff(expected, result); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func queryPostWithAuthor(t *testing.T) {
	queryPostParams := &GraphQLParams{
		Query: `query {
			queryPost (filter: {title : { anyofterms : "Introducing" }} ) {
				title
				author {
					name
				}
			}
		}`,
	}

	gqlResponse := queryPostParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)
	testutil.CompareJSON(t,
		`{"queryPost":[{"title":"Introducing GraphQL in Dgraph","author":{"name":"Ann Author"}}]}`,
		string(gqlResponse.Data))
}

func queriesHaveExtensions(t *testing.T) {
	query := &GraphQLParams{
		Query: `query {
			queryPost {
				title
			}
		}`,
	}

	touchedUidskey := "touched_uids"
	gqlResponse := query.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)
	require.Contains(t, gqlResponse.Extensions, touchedUidskey)
	require.Greater(t, int(gqlResponse.Extensions[touchedUidskey].(float64)), 0)
}

func queryWithAlias(t *testing.T) {
	queryPostParams := &GraphQLParams{
		Query: `query {
			post : queryPost (filter: {title : { anyofterms : "Introducing" }} ) {
				type : __typename
				title
				postTitle : title
				postAuthor : author {
					theName : name
				}
			}
		}`,
	}

	gqlResponse := queryPostParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)
	testutil.CompareJSON(t,
		`{
			"post": [ {
				"type": "Post",
				"title": "Introducing GraphQL in Dgraph",
				"postTitle": "Introducing GraphQL in Dgraph",
				"postAuthor": { "theName": "Ann Author" }}]}`,
		string(gqlResponse.Data))
}

func DgraphDirectiveWithSpecialCharacters(t *testing.T) {
	mutation := &GraphQLParams{
		Query: `
		mutation {
			addMessage(input : [{content : "content1", author: "author1"}]) {
				message {
					content
					author
				}
			}
		}`,
	}
	result := `{"addMessage":{"message":[{"content":"content1","author":"author1"}]}}`
	gqlResponse := mutation.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)
	require.JSONEq(t, result, string(gqlResponse.Data))

	queryParams := &GraphQLParams{
		Query: `
		query {
			queryMessage {
				content
				author
			}
		}`,
	}
	result = `{"queryMessage":[{"content":"content1","author":"author1"}]}`
	gqlResponse = queryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)
	require.JSONEq(t, result, string(gqlResponse.Data))
}

func queryWithCascade(t *testing.T) {
	// for testing normal and parameterized @cascade with get by ID and filter queries on multiple levels
	authors := addMultipleAuthorFromRef(t, []*author{
		{
			Name:       "George",
			Reputation: 4.5,
			Posts:      []*post{{Title: "A show about nothing", Text: "Got ya!", Tags: []string{}}},
		}, {
			Name:       "Jerry",
			Reputation: 4.6,
			Country:    &country{Name: "outer Galaxy2"},
			Posts:      []*post{{Title: "Outside", Tags: []string{}}},
		}, {
			Name:    "Kramer",
			Country: &country{Name: "outer space2"},
			Posts:   []*post{{Title: "Ha! Cosmo Kramer", Text: "Giddy up!", Tags: []string{}}},
		},
	}, postExecutor)
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)
	authorIds := []string{authors[0].ID, authors[1].ID, authors[2].ID}
	postIds := []string{authors[0].Posts[0].PostID, authors[1].Posts[0].PostID,
		authors[2].Posts[0].PostID}
	countryIds := []string{authors[1].Country.ID, authors[2].Country.ID}
	getAuthorByIdQuery := `query ($id: ID!) {
							  getAuthor(id: $id) @cascade {
								reputation
								posts {
								  text
								}
							  }
							}`

	// for testing @cascade with get by XID queries
	states := []*state{
		{Name: "California", Code: "CA", Capital: "Sacramento"},
		{Name: "Texas", Code: "TX"},
	}
	addStateParams := GraphQLParams{
		Query: `mutation ($input: [AddStateInput!]!) {
					addState(input: $input) {
						numUids
					}
				}`,
		Variables: map[string]interface{}{"input": states},
	}
	resp := addStateParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)
	testutil.CompareJSON(t, `{"addState":{"numUids":2}}`, string(resp.Data))
	getStateByXidQuery := `query ($xid: String!) {
							  getState(xcode: $xid) @cascade {
								xcode
								capital
							  }
							}`

	tcases := []struct {
		name      string
		query     string
		variables map[string]interface{}
		respData  string
	}{
		{
			name:      "@cascade on get by ID query returns null",
			query:     getAuthorByIdQuery,
			variables: map[string]interface{}{"id": authors[1].ID},
			respData:  `{"getAuthor": null}`,
		}, {
			name:      "@cascade on get by ID query returns author",
			query:     getAuthorByIdQuery,
			variables: map[string]interface{}{"id": authors[0].ID},
			respData: `{
							"getAuthor": {
								"reputation": 4.5,
								"posts": [{
									"text": "Got ya!"
								}]
							}
						}`,
		}, {
			name:      "@cascade on get by XID query returns null",
			query:     getStateByXidQuery,
			variables: map[string]interface{}{"xid": states[1].Code},
			respData:  `{"getState": null}`,
		}, {
			name:      "@cascade on get by XID query returns state",
			query:     getStateByXidQuery,
			variables: map[string]interface{}{"xid": states[0].Code},
			respData: `{
							"getState": {
								"xcode": "CA",
								"capital": "Sacramento"
							}
						}`,
		}, {
			name: "@cascade on filter query",
			query: `query ($ids: [ID!]) {
					  queryAuthor(filter: {id: $ids}) @cascade {
						reputation
						posts {
						  text
						}
					  }
					}`,
			variables: map[string]interface{}{"ids": authorIds},
			respData: `{
							"queryAuthor": [{
								"reputation": 4.5,
								"posts": [{
									"text": "Got ya!"
								}]
							}]
						}`,
		}, {
			name: "@cascade on query field",
			query: `query ($ids: [ID!]) {
					  queryAuthor(filter: {id: $ids}) {
						reputation
						posts @cascade {
						  title
						  text
						}
					  }
					}`,
			variables: map[string]interface{}{"ids": authorIds},
			respData: `{
							"queryAuthor": [{
								"reputation": 4.5,
								"posts": [{
									"title": "A show about nothing",
									"text": "Got ya!"
								}]
							},{
								"reputation": 4.6,
								"posts": []
							},{
								"reputation": null,
								"posts": [{
									"title": "Ha! Cosmo Kramer",
									"text": "Giddy up!"
								}]
							}]
						}`,
		},
		{
			name: "parameterized cascade with argument at outer level only",
			query: `query ($ids: [ID!]) {
						queryAuthor(filter: {id: $ids})  @cascade(fields:["name"]) {
							reputation
							name
							country {
								name
							}
						}
					}`,
			variables: map[string]interface{}{"ids": authorIds},
			respData: `{
						  "queryAuthor": [
							{
							  "reputation": 4.6,
							  "name": "Jerry",
							  "country": {
								"name": "outer Galaxy2"
							  }
							},
							{
							  "name": "Kramer",
							  "reputation": null,
							  "country": {
								"name": "outer space2"
							  }
							},
							{
							  "reputation": 4.5,
							  "name": "George",
							  "country": null
							}
						  ]
						}`,
		},
		{
			name: "parameterized cascade only at inner level ",
			query: `query ($ids: [ID!]) {
						queryAuthor(filter: {id: $ids})  {
							reputation
							name
							posts @cascade(fields:["text"]) {
								title
								text
							}
						}
					}`,
			variables: map[string]interface{}{"ids": authorIds},
			respData: `{
						  "queryAuthor": [
							{
							  "reputation": 4.5,
							  "name": "George",
							  "posts": [
								{
								  "title": "A show about nothing",
								  "text": "Got ya!"
								}
							  ]
							},
							{
							  "name": "Kramer",
							  "reputation": null,
							  "posts": [
								{
								  "title": "Ha! Cosmo Kramer",
								  "text": "Giddy up!"
								}
							  ]
							},
							{
							  "name": "Jerry",
							  "reputation": 4.6,
							  "posts": []
							}
						  ]
						}`,
		},
		{
			name: "parameterized cascade at all levels ",
			query: `query ($ids: [ID!]) {
						queryAuthor(filter: {id: $ids}) @cascade(fields:["reputation","name"]) {
							reputation
							name
							dob
							posts @cascade(fields:["text"]) {
								title
								text
							}
						}
					}`,
			variables: map[string]interface{}{"ids": authorIds},
			respData: `{
						  "queryAuthor": [
							{
							  "reputation": 4.5,
							  "name": "George",
							  "dob": null,
							  "posts": [
								{
								  "title": "A show about nothing",
								  "text": "Got ya!"
								}
							  ]
							},
							{
							  "dob": null,
							  "name": "Jerry",
							  "posts": [],
							  "reputation": 4.6
							}
						  ]
						}`,
		},
		{
			name: "parameterized cascade on ID type ",
			query: `query ($ids: [ID!]) {
						queryAuthor(filter: {id: $ids}) @cascade(fields:["reputation","id"]) {
							reputation
							name
							dob
						}
					}`,
			variables: map[string]interface{}{"ids": authorIds},
			respData: `{
						  "queryAuthor": [
							{
							  "reputation": 4.5,
							  "name": "George",
							  "dob": null
							},
							{
							  "dob": null,
							  "name": "Jerry",
							  "reputation": 4.6
							}
						  ]
						}`,
		},
		{
			name: "parameterized cascade on field of interface ",
			query: `query  {
						queryHuman() @cascade(fields:["name"]) {
							name
							totalCredits
						}
					}`,
			respData: `{
						  "queryHuman": [
							{
							  "name": "Han",
							  "totalCredits": 10
							}
						  ]
						}`,
		},
		{
			name: "parameterized cascade on interface ",
			query: `query {
						queryCharacter (filter: { appearsIn: { eq: [EMPIRE] } }) @cascade(fields:["appearsIn"]){	
							name
							appearsIn
						} 
					}`,
			respData: `{
						  "queryCharacter": [
							{
							  "name": "Han",
							  "appearsIn": [
								"EMPIRE"
							  ]
							}
						  ]
						}`,
		},
	}

	for _, tcase := range tcases {
		t.Run(tcase.name, func(t *testing.T) {
			params := &GraphQLParams{
				Query:     tcase.query,
				Variables: tcase.variables,
			}
			resp := params.ExecuteAsPost(t, GraphqlURL)
			RequireNoGQLErrors(t, resp)
			testutil.CompareJSON(t, tcase.respData, string(resp.Data))
		})
	}

	// cleanup
	deleteAuthors(t, authorIds, nil)
	deleteCountry(t, map[string]interface{}{"id": countryIds}, len(countryIds), nil)
	deleteGqlType(t, "Post", map[string]interface{}{"postID": postIds}, len(postIds), nil)
	deleteState(t, getXidFilter("xcode", []string{states[0].Code, states[1].Code}), len(states),
		nil)
	cleanupStarwars(t, newStarship.ID, humanID, "")
}

func queryGeoNearFilter(t *testing.T) {
	addHotelParams := &GraphQLParams{
		Query: `
		mutation addHotel($hotels: [AddHotelInput!]!) {
		  addHotel(input: $hotels) {
			hotel {
			  name
			  location {
				latitude
				longitude
			  }
			}
		  }
		}`,
		Variables: map[string]interface{}{"hotels": []interface{}{
			map[string]interface{}{
				"name": "Taj Hotel 1",
				"location": map[string]interface{}{
					"latitude":  11.11,
					"longitude": 22.22,
				},
			},
			map[string]interface{}{
				"name": "Taj Hotel 2",
				"location": map[string]interface{}{
					"latitude":  33.33,
					"longitude": 22.22,
				},
			},
			map[string]interface{}{
				"name": "Taj Hotel 3",
				"location": map[string]interface{}{
					"latitude":  11.11,
					"longitude": 33.33,
				},
			},
		},
		},
	}
	gqlResponse := addHotelParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	queryHotel := &GraphQLParams{
		Query: `
			query {
			  queryHotel(filter: { location: { near: { distance: 100, coordinate: { latitude: 11.11, longitude: 22.22} } } }) {
				name
				location {
				  latitude
				  longitude
				}
			  }
			}`,
	}
	gqlResponse = queryHotel.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	queryHotelExpected := `
	{
		"queryHotel":[{
			"name" : "Taj Hotel 1",
			"location" : {
				"latitude" : 11.11,
				"longitude" : 22.22
			}
		}]
	}`
	testutil.CompareJSON(t, queryHotelExpected, string(gqlResponse.Data))
	// Cleanup
	deleteGqlType(t, "Hotel", map[string]interface{}{}, 3, nil)
}
