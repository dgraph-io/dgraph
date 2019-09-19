/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	facetSetupDone = false
)

func populateClusterWithFacets() {
	// Return immediately if the setup has been performed already.
	if facetSetupDone {
		return
	}

	triples := `
		<25> <name> "Daryl Dixon" .
		<31> <name> "Andrea" .
		<33> <name> "Michale" .
		<320> <name> "Test facet"@en (type = "Test facet with lang") .

		<31> <friend> <24> .

		<33> <schools> <2433> .

		<1> <gender> "female" .
		<23> <gender> "male" .

		<202> <model> "Prius" (type = "Electric") .
	`

	friendFacets1 := "(since = 2006-01-02T15:04:05)"
	friendFacets2 := "(since = 2005-05-02T15:04:05, close = true, family = false, age = 33)"
	friendFacets3 := "(since = 2004-05-02T15:04:05, close = true, family = true, tag = \"Domain3\")"
	friendFacets4 := "(since = 2007-05-02T15:04:05, close = false, family = true, tag = 34)"
	friendFacets5 := "(games = \"football basketball chess tennis\", close = false, age = 35)"
	friendFacets6 := "(games = \"football basketball hockey\", close = false)"

	triples += fmt.Sprintf("<1> <friend> <23> %s .\n", friendFacets1)
	triples += fmt.Sprintf("<1> <friend> <24> %s .\n", friendFacets3)
	triples += fmt.Sprintf("<1> <friend> <25> %s .\n", friendFacets4)
	triples += fmt.Sprintf("<1> <friend> <31> %s .\n", friendFacets1)
	triples += fmt.Sprintf("<1> <friend> <101> %s .\n", friendFacets2)
	triples += fmt.Sprintf("<23> <friend> <1> %s .\n", friendFacets1)
	triples += fmt.Sprintf("<31> <friend> <1> %s .\n", friendFacets5)
	triples += fmt.Sprintf("<31> <friend> <25> %s .\n", friendFacets6)

	nameFacets := "(origin = \"french\")"
	triples += fmt.Sprintf("<1> <name> \"Michonne\" %s .\n", nameFacets)
	triples += fmt.Sprintf("<23> <name> \"Rick Grimes\" %s .\n", nameFacets)
	triples += fmt.Sprintf("<24> <name> \"Glenn Rhee\" %s .\n", nameFacets)

	addTriplesToCluster(triples)

	// Mark the setup as done so that the next tests do not have to perform it.
	facetSetupDone = true
}

func TestRetrieveFacetsSimple(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{
			me(func: uid(0x1)) {
				name @facets
				gender @facets
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name|origin":"french","name":"Michonne","gender":"female"}]}}`,
		js)
}

func TestOrderFacets(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"friend":[{"name":"Glenn Rhee","friend|since":"2004-05-02T15:04:05Z"},{"friend|since":"2005-05-02T15:04:05Z"},{"name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"name":"Daryl Dixon","friend|since":"2007-05-02T15:04:05Z"}]}]}}`,
		js)
}

func TestOrderdescFacets(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"friend":[{"name":"Daryl Dixon","friend|since":"2007-05-02T15:04:05Z"},{"name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"friend|since":"2005-05-02T15:04:05Z"},{"name":"Glenn Rhee","friend|since":"2004-05-02T15:04:05Z"}]}]}}`,
		js)
}

func TestOrderdescFacetsWithFilters(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{

			var(func: uid(1)) {
				f as friend
			}

			me(func: uid(1)) {
				friend @filter(uid(f)) @facets(orderdesc:since) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"friend":[{"name":"Daryl Dixon","friend|since":"2007-05-02T15:04:05Z"},{"name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"friend|since":"2005-05-02T15:04:05Z"},{"name":"Glenn Rhee","friend|since":"2004-05-02T15:04:05Z"}]}]}}`,
		js)
}

func TestRetrieveFacetsAsVars(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Rick Grimes","val(a)":"2006-01-02T15:04:05Z"}]}}`,
		js)
}

func TestRetrieveFacetsUidValues(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"friend":[{"name|origin":"french","name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"name|origin":"french","name":"Glenn Rhee","friend|close":true,"friend|family":true,"friend|since":"2004-05-02T15:04:05Z","friend|tag":"Domain3"},{"name":"Daryl Dixon","friend|close":false,"friend|family":true,"friend|since":"2007-05-02T15:04:05Z","friend|tag":34},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"friend|age":33,"friend|close":true,"friend|family":false,"friend|since":"2005-05-02T15:04:05Z"}]}]}}`,
		js)
}

func TestRetrieveFacetsAll(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name|origin":"french","name":"Michonne","friend":[{"name|origin":"french","name":"Rick Grimes","gender":"male","friend|since":"2006-01-02T15:04:05Z"},{"name|origin":"french","name":"Glenn Rhee","friend|close":true,"friend|family":true,"friend|since":"2004-05-02T15:04:05Z","friend|tag":"Domain3"},{"name":"Daryl Dixon","friend|close":false,"friend|family":true,"friend|since":"2007-05-02T15:04:05Z","friend|tag":34},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"friend|age":33,"friend|close":true,"friend|family":false,"friend|since":"2005-05-02T15:04:05Z"}],"gender":"female"}]}}`,
		js)
}

func TestFacetsNotInQuery(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"gender":"male","name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestSubjectWithNoFacets(t *testing.T) {
	populateClusterWithFacets()
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michale"}]}}`,
		js)
}

func TestFetchingFewFacets(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name":"Michonne","friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee","friend|close":true},{"name":"Daryl Dixon","friend|close":false},{"name":"Andrea"},{"friend|close":true}]}]}}`,
		js)
}

func TestFetchingNoFacets(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsSortOrder(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name":"Michonne","friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee","friend|close":true,"friend|family":true},{"name":"Daryl Dixon","friend|close":false,"friend|family":true},{"name":"Andrea"},{"friend|close":true,"friend|family":false}]}]}}`,
		js)
}

func TestUnknownFacets(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsMutation(t *testing.T) {
	populateClusterWithFacets()

	// Delete friendship between Michonne and Glenn
	deleteTriplesInCluster("<1> <friend> <24> .")
	friendFacets := "(since = 2001-11-10T00:00:00Z, close = false, family = false)"
	// 101 is not close friend now.
	addTriplesToCluster(fmt.Sprintf(`<1> <friend> <101> %s .`, friendFacets))
	// This test messes with the test setup, so set facetSetupDone to false so
	// the next test redoes the setup.
	facetSetupDone = false

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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name":"Michonne","friend":[{"name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"name":"Daryl Dixon","friend|close":false,"friend|family":true,"friend|since":"2007-05-02T15:04:05Z","friend|tag":34},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"friend|close":false,"friend|family":false,"friend|since":"2001-11-10T00:00:00Z"}]}]}}`,
		js)
}

func TestFacetsFilterSimple(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	// 0x65 does not have name.
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterSimple2(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterSimple3(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x19","name":"Daryl Dixon"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterOr(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	// 0x65 (101) does not have name.
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"},{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterAnd(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterle(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterge(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterAndOrle(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	// 0x65 (101) does not have name.
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterAndOrge2(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x19","name":"Daryl Dixon"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterNotAndOrgeMutuallyExclusive(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x17","name":"Rick Grimes"},{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x1f","name":"Andrea"},{"uid":"0x65"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterUnknownFacets(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterUnknownOrKnown(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetsFilterallofterms(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Michonne","uid":"0x1"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAllofMultiple(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Michonne","uid":"0x1"}, {"name":"Daryl Dixon","uid":"0x19"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAllofNone(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilteranyofterms(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x1","name":"Michonne"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAnyofNone(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAllofanyofterms(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x1","name":"Michonne"},{"uid":"0x19","name":"Daryl Dixon"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAllofAndanyofterms(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x19","name":"Daryl Dixon"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsFilterAtValueFail(t *testing.T) {
	populateClusterWithFacets()
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

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestFacetsFilterAndRetrieval(t *testing.T) {
	populateClusterWithFacets()
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name":"Michonne","friend":[{"name":"Glenn Rhee","uid":"0x18","friend|family":true},{"uid":"0x65","friend|family":false}]}]}}`,
		js)
}

func TestFacetWithLang(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{
			me(func: uid(320)) {
				name@en @facets
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name@en|type":"Test facet with lang","name@en":"Test facet"}]}}`, js)
}

func TestFilterUidFacetMismatch(t *testing.T) {
	populateClusterWithFacets()
	query := `
	{
		me(func: uid(0x1)) {
			friend @filter(uid(24, 101)) @facets {
				name
			}
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"friend":[{"name":"Glenn Rhee","friend|close":true,"friend|family":true,"friend|since":"2004-05-02T15:04:05Z","friend|tag":"Domain3"},{"friend|age":33,"friend|close":true,"friend|family":false,"friend|since":"2005-05-02T15:04:05Z"}]}]}}`, js)
}

func TestRecurseFacetOrder(t *testing.T) {
	populateClusterWithFacets()
	query := `
    {
		me(func: uid(1)) @recurse(depth: 2) {
			friend @facets(orderdesc: since)
			uid
			name
		}
	}
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"friend":[
			{"uid":"0x19","name":"Daryl Dixon","friend|since":"2007-05-02T15:04:05Z"},
			{"uid":"0x17","name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},
			{"uid":"0x1f","name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},
			{"uid":"0x65","friend|since":"2005-05-02T15:04:05Z"},
			{"uid":"0x18","name":"Glenn Rhee","friend|since":"2004-05-02T15:04:05Z"}],
		"uid":"0x1","name":"Michonne"}]}}`, js)

	query = `
    {
		me(func: uid(1)) @recurse(depth: 2) {
			friend @facets(orderasc: since)
			uid
			name
		}
	}
  `
	js = processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"friend":[
			{"uid":"0x18","name":"Glenn Rhee","friend|since":"2004-05-02T15:04:05Z"},
			{"uid":"0x65","friend|since":"2005-05-02T15:04:05Z"},
			{"uid":"0x17","name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},
			{"uid":"0x1f","name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},
			{"uid":"0x19","name":"Daryl Dixon","friend|since":"2007-05-02T15:04:05Z"}],
		"uid":"0x1","name":"Michonne"}]}}`, js)
}

func TestFacetsAlias(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{
			me(func: uid(0x1)) {
				name @facets(o: origin)
				friend @facets(family, tagalias: tag, since) {
					name @facets(o: origin)
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.Equal(t, `{"data":{"me":[{"o":"french","name":"Michonne","friend":[{"o":"french","name":"Rick Grimes","friend|since":"2006-01-02T15:04:05Z"},{"o":"french","name":"Glenn Rhee","friend|family":true,"friend|since":"2004-05-02T15:04:05Z","tagalias":"Domain3"},{"name":"Daryl Dixon","friend|family":true,"friend|since":"2007-05-02T15:04:05Z","tagalias":34},{"name":"Andrea","friend|since":"2006-01-02T15:04:05Z"},{"friend|family":false,"friend|since":"2005-05-02T15:04:05Z"}]}]}}`, js)
}

func TestFacetsAlias2(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{
			me2(func: uid(0x1)) {
				friend @facets(f: family, a as since, orderdesc: tag, close)
			}

			me(func: uid(23)) {
				name
				val(a)
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me2":[{"friend":[{"friend|close":true,"f":false,"friend|since":"2005-05-02T15:04:05Z"},{"friend|since":"2006-01-02T15:04:05Z"},{"friend|since":"2006-01-02T15:04:05Z"},{"friend|close":true,"f":true,"friend|since":"2004-05-02T15:04:05Z","friend|tag":"Domain3"},{"friend|close":false,"f":true,"friend|since":"2007-05-02T15:04:05Z","friend|tag":34}]}],"me":[{"name":"Rick Grimes", "val(a)":"2006-01-02T15:04:05Z"}]}}`, js)
}

func TestTypeExpandFacets(t *testing.T) {
	query := `{
		q(func: eq(make, "Toyota")) {
			expand(_all_) {
				uid
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q":[
		{"make":"Toyota","model":"Prius", "model@jp":"プリウス", "model|type":"Electric",
			"year":2009}]}}`, js)
}
