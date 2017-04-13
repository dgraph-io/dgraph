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

	"github.com/gogo/protobuf/proto"
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
		"since": "2004-05-02T15:04:05", "close": "true", "family": "true"}
	friendFacets4 := map[string]string{
		"since": "2007-05-02T15:04:05", "close": "false", "family": "true"}
	addEdgeToUID(t, "friend", 1, 23, friendFacets1)
	addEdgeToUID(t, "friend", 1, 24, friendFacets3)
	addEdgeToUID(t, "friend", 1, 25, friendFacets4)
	addEdgeToUID(t, "friend", 1, 31, friendFacets1)
	addEdgeToUID(t, "friend", 1, 101, friendFacets2)
	addEdgeToUID(t, "friend", 31, 24, nil)
	addEdgeToUID(t, "friend", 33, 24, nil)
	addEdgeToUID(t, "friend", 23, 1, friendFacets1)

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
	delEdgeToUID(t, "friend", 33, 24)
	delEdgeToUID(t, "friend", 23, 1)
	delEdgeToUID(t, "friend", 31, 1)
	delEdgeToUID(t, "friend", 31, 25)
}

func TestRetrieveFacetsSimple(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(id:0x1) {
				name @facets
				gender @facets
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"@facets":{"name":{"origin":"french"}},"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestRetrieveFacetsAsVars(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// to see how friend @facets are positioned in output.
	query := `
		{
			var(id:0x1) {
				f as friend {
				 a as	name @facets
				}
			}

			me(id: var(f), orderasc: var(a)) {
				name
				var(a)
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Rick Grimes","var[a]":"french"},{"name":"Glenn Rhee","var[a]":"french"}]}`,
		js)
}

func TestRetrieveFacetsUidValues(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// to see how friend @facets are positioned in output.
	query := `
		{
			me(id:0x1) {
				friend @facets {
					name @facets
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"@facets":{"_":{"since":"2006-01-02T15:04:05Z"},"name":{"origin":"french"}},"name":"Rick Grimes"},{"@facets":{"_":{"close":true,"family":true,"since":"2004-05-02T15:04:05Z"},"name":{"origin":"french"}},"name":"Glenn Rhee"},{"@facets":{"_":{"close":false,"family":true,"since":"2007-05-02T15:04:05Z"}},"name":"Daryl Dixon"},{"@facets":{"_":{"since":"2006-01-02T15:04:05Z"}},"name":"Andrea"},{"@facets":{"_":{"age":33,"close":true,"family":false,"since":"2005-05-02T15:04:05Z"}}}]}]}`,
		js)
}

func TestRetrieveFacetsAll(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(id:0x1) {
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
		`{"me":[{"@facets":{"name":{"origin":"french"}},"friend":[{"@facets":{"_":{"since":"2006-01-02T15:04:05Z"},"name":{"origin":"french"}},"gender":"male","name":"Rick Grimes"},{"@facets":{"_":{"close":true,"family":true,"since":"2004-05-02T15:04:05Z"},"name":{"origin":"french"}},"name":"Glenn Rhee"},{"@facets":{"_":{"close":false,"family":true,"since":"2007-05-02T15:04:05Z"}},"name":"Daryl Dixon"},{"@facets":{"_":{"since":"2006-01-02T15:04:05Z"}},"name":"Andrea"},{"@facets":{"_":{"age":33,"close":true,"family":false,"since":"2005-05-02T15:04:05Z"}}}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestFacetsNotInQuery(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(id:0x1) {
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
		`{"me":[{"friend":[{"gender":"male","name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}`,
		js)
}

func TestSubjectWithNoFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// id 33 does not have any facets associated with name and friend
	query := `
		{
			me(id:0x21) {
				name @facets
				friend @facets {
					name
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Glenn Rhee"}],"name":"Michale"}]}`,
		js)
}

func TestFetchingFewFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// only 1 friend of 1 has facet : "close" and she/he has no name
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(close) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Rick Grimes"},{"@facets":{"_":{"close":true}},"name":"Glenn Rhee"},{"@facets":{"_":{"close":false}},"name":"Daryl Dixon"},{"name":"Andrea"},{"@facets":{"_":{"close":true}}}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsSortOrder(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// order of facets in gql query should not matter.
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(family, close) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Rick Grimes"},{"@facets":{"_":{"close":true,"family":true}},"name":"Glenn Rhee"},{"@facets":{"_":{"close":false,"family":true}},"name":"Daryl Dixon"},{"name":"Andrea"},{"@facets":{"_":{"close":true,"family":false}}}],"name":"Michonne"}]}`,
		js)
}

func TestUnknownFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// uknown facets should be ignored.
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(unknownfacets1, unknownfacets2) {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsMutation(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	delEdgeToUID(t, "friend", 1, 24) // Delete friendship between Michonne and Glenn
	friendFacets := map[string]string{"since": "2001-11-10", "close": "false", "family": "false"}
	addEdgeToUID(t, "friend", 1, 101, friendFacets) // and 101 is not close friend now.
	query := `
		{
			me(id:0x1) {
				name
				friend @facets {
					name
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"@facets":{"_":{"since":"2006-01-02T15:04:05Z"}},"name":"Rick Grimes"},{"@facets":{"_":{"close":false,"family":true,"since":"2007-05-02T15:04:05Z"}},"name":"Daryl Dixon"},{"@facets":{"_":{"since":"2006-01-02T15:04:05Z"}},"name":"Andrea"},{"@facets":{"_":{"close":false,"family":false,"since":"2001-11-10T00:00:00Z"}}}],"name":"Michonne"}]}`,
		js)
}

func TestToProtoFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(id:0x1) {
				name @facets
				friend @facets {
					name @facets
				}
			}
		}
	`
	pb := processToPB(t, query, map[string]string{}, true)
	require.EqualValues(t,
		`attribute: "_root_"
children: <
  uid: 1
  attribute: "me"
  properties: <
    prop: "name"
    value: <
      str_val: "Michonne"
    >
  >
  children: <
    uid: 23
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Rick Grimes"
      >
    >
    children: <
      attribute: "@facets"
      children: <
        attribute: "name"
        properties: <
          prop: "origin"
          value: <
            str_val: "french"
          >
        >
      >
      children: <
        attribute: "_"
        children: <
          attribute: "friend"
          properties: <
            prop: "since"
            value: <
              str_val: "2006-01-02T15:04:05Z"
            >
          >
        >
      >
    >
  >
  children: <
    uid: 24
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Glenn Rhee"
      >
    >
    children: <
      attribute: "@facets"
      children: <
        attribute: "name"
        properties: <
          prop: "origin"
          value: <
            str_val: "french"
          >
        >
      >
      children: <
        attribute: "_"
        children: <
          attribute: "friend"
          properties: <
            prop: "close"
            value: <
              bool_val: true
            >
          >
          properties: <
            prop: "family"
            value: <
              bool_val: true
            >
          >
          properties: <
            prop: "since"
            value: <
              str_val: "2004-05-02T15:04:05Z"
            >
          >
        >
      >
    >
  >
  children: <
    uid: 25
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Daryl Dixon"
      >
    >
    children: <
      attribute: "@facets"
      children: <
        attribute: "_"
        children: <
          attribute: "friend"
          properties: <
            prop: "close"
            value: <
              bool_val: false
            >
          >
          properties: <
            prop: "family"
            value: <
              bool_val: true
            >
          >
          properties: <
            prop: "since"
            value: <
              str_val: "2007-05-02T15:04:05Z"
            >
          >
        >
      >
    >
  >
  children: <
    uid: 31
    attribute: "friend"
    properties: <
      prop: "name"
      value: <
        str_val: "Andrea"
      >
    >
    children: <
      attribute: "@facets"
      children: <
        attribute: "_"
        children: <
          attribute: "friend"
          properties: <
            prop: "since"
            value: <
              str_val: "2006-01-02T15:04:05Z"
            >
          >
        >
      >
    >
  >
  children: <
    uid: 101
    attribute: "friend"
    children: <
      attribute: "@facets"
      children: <
        attribute: "_"
        children: <
          attribute: "friend"
          properties: <
            prop: "age"
            value: <
              int_val: 33
            >
          >
          properties: <
            prop: "close"
            value: <
              bool_val: true
            >
          >
          properties: <
            prop: "family"
            value: <
              bool_val: false
            >
          >
          properties: <
            prop: "since"
            value: <
              str_val: "2005-05-02T15:04:05Z"
            >
          >
        >
      >
    >
  >
  children: <
    attribute: "@facets"
    children: <
      attribute: "name"
      properties: <
        prop: "origin"
        value: <
          str_val: "french"
        >
      >
    >
  >
>
`,
		proto.MarshalTextString(pb[0]))
}

func TestFacetsFilterSimple(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find close friends of 1
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(eq(close, true)) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	// 0x65 does not have name.
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x65"}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsFilterOr(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find close or family friends of 1
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(eq(close, true) OR eq(family, true)) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	// 0x65 (101) does not have name.
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x19","name":"Daryl Dixon"},{"_uid_":"0x65"}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsFilterAnd(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// unknown filters do not have any effect on results.
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(eq(close, true) AND eq(family, false)) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x65"}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsFilterLeq(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find friends of 1 below 36 years of age.
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(leq(age, 35)) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x65"}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsFilterGeq(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find friends of 1 above 32 years of age.
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(geq(age, 33)) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x65"}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsFilterAndOrLeq(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find close or family friends of 1 before 2007-01-10
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(eq(close, true) OR eq(family, true) AND leq(since, "2007-01-10")) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	// 0x65 (101) does not have name.
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x65"}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsFilterAndOrGeq2(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find close or family friends of 1 after 2007-01-10
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(eq(close, false) OR eq(family, true) AND geq(since, "2007-01-10")) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x19","name":"Daryl Dixon"}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsFilterNotAndOrGeqMutuallyExclusive(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// find Not (close or family friends of 1 after 2007-01-10)
	// Mutually exclusive of above result : TestFacetsFilterNotAndOrGeq
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(not (eq(close, false) OR eq(family, true) AND geq(since, "2007-01-10"))) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x17","name":"Rick Grimes"},{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x1f","name":"Andrea"},{"_uid_":"0x65"}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsFilterUnknownFacets(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// unknown facets should filter out edges.
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(geq(dob, "2007-01-10")) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Michonne"}]}`,
		js)
}

func TestFacetsFilterUnknownOrKnown(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// unknown filters with OR do not have any effect on results
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(geq(dob, "2007-01-10") OR eq(family, true)) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x18","name":"Glenn Rhee"},{"_uid_":"0x19","name":"Daryl Dixon"}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsFilterFail1(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// integer overflow error is propagated to stop the query.
	query := `
		{
			me(id:0x1) {
				name
				friend @facets(geq(age, 111111111111111111118888888)) {
					name
					_uid_
				}
			}
		}
	`

	_, err := processToFastJsonReq(t, query)
	require.Error(t, err)
}

func TestFacetsFilterallofterms(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(id:31) {
				name
				friend @facets(allofterms(games, "football chess tennis")) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Michonne","_uid_":"0x1"}],"name":"Andrea"}]}`,
		js)
}

func TestFacetsFilterAllofMultiple(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(id:31) {
				name
				friend @facets(allofterms(games, "football basketball")) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Michonne","_uid_":"0x1"}, {"name":"Daryl Dixon","_uid_":"0x19"}],"name":"Andrea"}]}`,
		js)
}

func TestFacetsFilterAllofNone(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// nothing matches in allofterms
	query := `
		{
			me(id:31) {
				name
				friend @facets(allofterms(games, "football chess tennis cricket")) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Andrea"}]}`,
		js)
}

func TestFacetsFilteranyofterms(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(id:31) {
				name
				friend @facets(anyofterms(games, "tennis cricket")) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x1","name":"Michonne"}],"name":"Andrea"}]}`,
		js)
}

func TestFacetsFilterAnyofNone(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(id:31) {
				name
				friend @facets(anyofterms(games, "cricket")) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name":"Andrea"}]}`,
		js)
}

func TestFacetsFilterAllofanyofterms(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(id:31) {
				name
				friend @facets(allofterms(games, "basketball hockey") OR anyofterms(games, "chess")) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x1","name":"Michonne"},{"_uid_":"0x19","name":"Daryl Dixon"}],"name":"Andrea"}]}`,
		js)
}

func TestFacetsFilterAllofAndanyofterms(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	query := `
		{
			me(id:31) {
				name
				friend @facets(allofterms(games, "hockey") AND anyofterms(games, "football basketball")) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"_uid_":"0x19","name":"Daryl Dixon"}],"name":"Andrea"}]}`,
		js)
}

func TestFacetsFilterAtValueFail(t *testing.T) {
	populateGraphWithFacets(t)
	defer teardownGraphWithFacets(t)
	// facet filtering is not supported at value level.
	query := `
	{
		me(id:1) {
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
			me(id:1) {
				name
				friend @facets(eq(close, true)) @facets(family) {
					name
					_uid_
				}
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"@facets":{"_":{"family":true}},"_uid_":"0x18","name":"Glenn Rhee"},{"@facets":{"_":{"family":false}},"_uid_":"0x65"}],"name":"Michonne"}]}`,
		js)
}
