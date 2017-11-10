/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package query

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func populateGraphWithFacets(t *testing.T) {
	x.AssertTrue(ps != nil)
	// So, user we're interested in has uid: 1.
	// She has 5 friends: 23, 24, 25, 31, and 101
	friendFacets1 := map[string]string{"since": "2006-01-02T15:04:05"}
	friendFacets2 := map[string]string{
		"since": "2005-05-02T15:04:05", "close": "true", "family": "false", "age": "33"}
	friendFacets3 := map[string]string{
		"since": "2004-05-02T15:04:05", "close": "true", "family": "true", "tag": "\"Domain3\""}
	friendFacets4 := map[string]string{
		"since": "2007-05-02T15:04:05", "close": "false", "family": "true", "tag": "34"}
	addEdgeToUID(t, "friend", 1, 23, friendFacets1)
	addEdgeToUID(t, "friend", 1, 24, friendFacets3)
	addEdgeToUID(t, "friend", 1, 25, friendFacets4)
	addEdgeToUID(t, "friend", 1, 31, friendFacets1)
	addEdgeToUID(t, "friend", 1, 101, friendFacets2)
	addEdgeToUID(t, "friend", 31, 24, nil)
	addEdgeToUID(t, "friend", 23, 1, friendFacets1)
	addEdgeToUID(t, "schools", 33, 2433, nil)

	friendFacets5 := map[string]string{
		"games": `"football basketball chess tennis"`, "close": "false", "age": "35"}
	friendFacets6 := map[string]string{
		"games": `"football basketball hockey"`, "close": "false"}

	addEdgeToUID(t, "friend", 31, 1, friendFacets5)
	addEdgeToUID(t, "friend", 31, 25, friendFacets6)

	nameFacets := map[string]string{"origin": `"french"`}
	// Now let's add a few properties for the main user.
	addEdgeToValue(t, "name", 1, "Michonne", nameFacets)
	addEdgeToValue(t, "gender", 1, "female", nil)

	// Now let's add a name for each of the friends, except 101.
	addEdgeToTypedValue(t, "name", 23, types.StringID, []byte("Rick Grimes"), nameFacets)
	addEdgeToValue(t, "gender", 23, "male", nil)
	addEdgeToValue(t, "name", 24, "Glenn Rhee", nameFacets)
	addEdgeToValue(t, "name", 25, "Daryl Dixon", nil)

	addEdgeToValue(t, "name", 31, "Andrea", nil)

	addEdgeToValue(t, "name", 33, "Michale", nil)
	// missing name for 101 -- no name edge and no facets.

	addEdgeToLangValue(t, "name", 320, "Test facet", "en",
		map[string]string{"type": `"Test facet with lang"`})

	time.Sleep(5 * time.Millisecond)
}

// teardownGraphWithFacets removes friend edges otherwise tests in query_test.go
// are affected by populateGraphWithFacets.
func teardownGraphWithFacets(t *testing.T) {
	delEdgeToUID(t, "friend", 1, 23)
	delEdgeToUID(t, "friend", 1, 24)
	delEdgeToUID(t, "friend", 1, 25)
	delEdgeToUID(t, "friend", 1, 31)
	delEdgeToUID(t, "friend", 1, 101)
	delEdgeToUID(t, "friend", 31, 24)
	delEdgeToUID(t, "friend", 23, 1)
	delEdgeToUID(t, "friend", 31, 1)
	delEdgeToUID(t, "friend", 31, 25)
	delEdgeToUID(t, "schools", 33, 2433)
	delEdgeToLangValue(t, "name", 320, "Test facet", "en")
}

func TestRetrieveFacetsSimple(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(func: uid(0x1)) {
				name @facets
				gender @facets
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name|origin":"french","name":"Michonne","gender":"female"}]}}`,
		js)
}

func TestOrderFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// to see how friend @facets are positioned in output.
	query := `
		{
			me(func: uid(1)) {
				friend @facets(orderasc:since) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"friend":[{"name":"Glenn Rhee","friend|since":"2004-05-02T15:04:05Z"},{"friend|since":"2005-05-02T15:04:05Z"},{"name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"name":"Daryl Dixon","friend|since":"2007-05-02T15:04:05Z"}]}]}}`,
		js)
}

func TestOrderdescFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// to see how friend @facets are positioned in output.
	query := `
		{
			me(func: uid(1)) {
				friend @facets(orderdesc:since) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"friend":[{"name":"Daryl Dixon","friend|since":"2007-05-02T15:04:05Z"},{"name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"friend|since":"2005-05-02T15:04:05Z"},{"name":"Glenn Rhee","friend|since":"2004-05-02T15:04:05Z"}]}]}}`,
		js)
}

func TestRetrieveFacetsAsVars(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// to see how friend @facets are positioned in output.
	query := `
		{
			var(func: uid(0x1)) {
				friend @facets(a as since)
			}

			me(func: uid( 23)) {
				name
				val(a)
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Rick Grimes","val(a)":"2006-01-02T15:04:05Z"}]}}`,
		js)
}

func TestRetrieveFacetsUidValues(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// to see how friend @facets are positioned in output.
	query := `
		{
			me(func: uid(0x1)) {
				friend @facets {
					name @facets
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"friend":[{"name|origin":"french","name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"name|origin":"french","name":"Glenn Rhee","friend|close":true,"friend|family":true,"friend|since":"2004-05-02T15:04:05Z","friend|tag":"Domain3"},{"name":"Daryl Dixon","friend|close":false,"friend|family":true,"friend|since":"2007-05-02T15:04:05Z","friend|tag":34},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"friend|age":33,"friend|close":true,"friend|family":false,"friend|since":"2005-05-02T15:04:05Z"}]}]}}`,
		js)
}

func TestRetrieveFacetsAll(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(func: uid(0x1)) {
				name @facets
				friend @facets {
					name @facets
					gender @facets
				}
				gender @facets
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name|origin":"french","name":"Michonne","friend":[{"name|origin":"french","name":"Rick Grimes","gender":"male","friend|since":"2006-01-02T15:04:05Z"},{"name|origin":"french","name":"Glenn Rhee","friend|close":true,"friend|family":true,"friend|since":"2004-05-02T15:04:05Z","friend|tag":"Domain3"},{"name":"Daryl Dixon","friend|close":false,"friend|family":true,"friend|since":"2007-05-02T15:04:05Z","friend|tag":34},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"friend|age":33,"friend|close":true,"friend|family":false,"friend|since":"2005-05-02T15:04:05Z"}],"gender":"female"}]}}`,
		js)
}

func TestFacetsNotInQuery(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(func: uid(0x1)) {
				name
				gender
				friend {
					name
					gender
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"gender":"male","name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestSubjectWithNoFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// id 33 does not have any facets associated with name and school
	query := `
		{
			me(func: uid(0x21)) {
				name @facets
				school @facets {
					name
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michale"}]}}`,
		js)
}

func TestFetchingFewFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// only 1 friend of 1 has facet : "close" and she/he has no name
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(close) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name":"Michonne","friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee","friend|close":true},{"name":"Daryl Dixon","friend|close":false},{"name":"Andrea"},{"friend|close":true}]}]}}`,
		js)
}

func TestFetchingNoFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// TestFetchingFewFacets but without the facet.  Returns no facets.
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets() {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsSortOrder(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// order of facets in gql query should not matter.
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(family, close) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name":"Michonne","friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee","friend|close":true,"friend|family":true},{"name":"Daryl Dixon","friend|close":false,"friend|family":true},{"name":"Andrea"},{"friend|close":true,"friend|family":false}]}]}}`,
		js)
}

func TestUnknownFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// uknown facets should be ignored.
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(unknownfacets1, unknownfacets2) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsMutation(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	delEdgeToUID(t, "friend", 1, 24) // Delete friendship between Michonne and Glenn
	friendFacets := map[string]string{"since": "2001-11-10T00:00:00Z", "close": "false", "family": "false"}
	addEdgeToUID(t, "friend", 1, 101, friendFacets) // and 101 is not close friend now.
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name":"Michonne","friend":[{"name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"name":"Daryl Dixon","friend|close":false,"friend|family":true,"friend|since":"2007-05-02T15:04:05Z","friend|tag":34},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"friend|close":false,"friend|family":false,"friend|since":"2001-11-10T00:00:00Z"}]}]}}`,
		js)
}

func TestFacetsFilterSimple(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find close friends of 1
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(eq(close, true)) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	// 0x65 does not have name.
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterSimple2(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find close friends of 1
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(eq(tag, "Domain3")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterSimple3(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find close friends of 1
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(eq(tag, "34")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x19","name":"Daryl Dixon"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterOr(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find close or family friends of 1
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(eq(close, true) OR eq(family, true)) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	// 0x65 (101) does not have name.
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"},{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterAnd(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// unknown filters do not have any effect on results.
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(eq(close, true) AND eq(family, false)) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterle(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find friends of 1 below 36 years of age.
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(le(age, 35)) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterge(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find friends of 1 above 32 years of age.
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(ge(age, 33)) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterAndOrle(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find close or family friends of 1 before 2007-01-10
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(eq(close, true) OR eq(family, true) AND le(since, "2007-01-10")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	// 0x65 (101) does not have name.
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterAndOrge2(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find close or family friends of 1 after 2007-01-10
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(eq(close, false) OR eq(family, true) AND ge(since, "2007-01-10")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x19","name":"Daryl Dixon"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterNotAndOrgeMutuallyExclusive(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find Not (close or family friends of 1 after 2007-01-10)
	// Mutually exclusive of above result : TestFacetsFilterNotAndOrge
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(not (eq(close, false) OR eq(family, true) AND ge(since, "2007-01-10"))) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x17","name":"Rick Grimes"},{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x1f","name":"Andrea"},{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterUnknownFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// unknown facets should filter out edges.
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(ge(dob, "2007-01-10")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterUnknownOrKnown(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// unknown filters with OR do not have any effect on results
	query := `
		{
			me(func: uid(0x1)) {
				name
				friend @facets(ge(dob, "2007-01-10") OR eq(family, true)) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterallofterms(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(func: uid(31)) {
				name
				friend @facets(allofterms(games, "football chess tennis")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Michonne","uid":"0x1"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAllofMultiple(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(func: uid(31)) {
				name
				friend @facets(allofterms(games, "football basketball")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Michonne","uid":"0x1"}, {"name":"Daryl Dixon","uid":"0x19"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAllofNone(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// nothing matches in allofterms
	query := `
		{
			me(func: uid(31)) {
				name
				friend @facets(allofterms(games, "football chess tennis cricket")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilteranyofterms(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(func: uid(31)) {
				name
				friend @facets(anyofterms(games, "tennis cricket")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x1","name":"Michonne"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAnyofNone(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(func: uid(31)) {
				name
				friend @facets(anyofterms(games, "cricket")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAllofanyofterms(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(func: uid(31)) {
				name
				friend @facets(allofterms(games, "basketball hockey") OR anyofterms(games, "chess")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x1","name":"Michonne"},{"uid":"0x19","name":"Daryl Dixon"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAllofAndanyofterms(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(func: uid(31)) {
				name
				friend @facets(allofterms(games, "hockey") AND anyofterms(games, "football basketball")) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x19","name":"Daryl Dixon"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAtValueFail(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// facet filtering is not supported at value level.
	query := `
	{
		me(func: uid(1)) {
			friend {
				name @facets(eq(origin, "french"))
			}
		}
	}
`

	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

func TestFacetsFilterAndRetrieval(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// Close should not be retrieved.. only used for filtering.
	query := `
		{
			me(func: uid(1)) {
				name
				friend @facets(eq(close, true)) @facets(family) {
					name
					uid
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name":"Michonne","friend":[{"name":"Glenn Rhee","uid":"0x18","friend|family":true},{"uid":"0x65","friend|family":false}]}]}}`,
		js)
}

func TestFacetWithLang(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(func: uid(320)) {
				name@en @facets
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name@en|type":"Test facet with lang","name@en":"Test facet"}]}}`, js)
}

func TestFilterUidFacetMismatch(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
	{
		me(func: uid(0x1)) {
			friend @filter(uid(24, 101)) @facets {
				name
			}
		}
	}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"data":{"me":[{"friend":[{"name":"Glenn Rhee","friend|close":true,"friend|family":true,"friend|since":"2004-05-02T15:04:05Z","friend|tag":"Domain3"},{"friend|age":33,"friend|close":true,"friend|family":false,"friend|since":"2005-05-02T15:04:05Z"}]}]}}`, js)
}

func TestRecurseFacetOrder(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
    {
		recurse(func: uid(1), depth: 2) {
			friend @facets(orderdesc: since)
			uid
			name
		}
	}
  `
	js := processToFastJSON(t, query)
	require.JSONEq(t, `{"data":{"recurse":[{"friend":[{"uid":"0x19","name":"Daryl Dixon","friend|since":"2007-05-02T15:04:05Z"},{"friend":[{"friend|since":"2006-01-02T15:04:05Z"}],"uid":"0x17","name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"uid":"0x1f","name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"uid":"0x65","friend|since":"2005-05-02T15:04:05Z"},{"uid":"0x18","name":"Glenn Rhee","friend|since":"2004-05-02T15:04:05Z"}],"uid":"0x1","name":"Michonne"}]}}`, js)

	query = `
    {
		recurse(func: uid(1), depth: 2) {
			friend @facets(orderasc: since)
			uid
			name
		}
	}
  `
	js = processToFastJSON(t, query)
	require.JSONEq(t, `{"data":{"recurse":[{"friend":[{"uid":"0x18","name":"Glenn Rhee","friend|since":"2004-05-02T15:04:05Z"},{"uid":"0x65","friend|since":"2005-05-02T15:04:05Z"},{"friend":[{"friend|since":"2006-01-02T15:04:05Z"}],"uid":"0x17","name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"uid":"0x1f","name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"uid":"0x19","name":"Daryl Dixon","friend|since":"2007-05-02T15:04:05Z"}],"uid":"0x1","name":"Michonne"}]}}`, js)
}
