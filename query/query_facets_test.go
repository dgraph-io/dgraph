/*
 * Copyright 2017 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
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
	// logrus.SetLevel(logrus.DebugLevel)
	// So, user we're interested in has uid: 1.
	// She has 5 friends: 23, 24, 25, 31, and 101
	friendFacets1 := map[string]string{"since": "12-01-1991"}
	friendFacets2 := map[string]string{"since": "11-10-2001", "close": "true", "family": "false"}
	addEdgeToUID(t, "friend", 1, 23, friendFacets1)
	addEdgeToUID(t, "friend", 1, 24, friendFacets1)
	addEdgeToUID(t, "friend", 1, 25, friendFacets1)
	addEdgeToUID(t, "friend", 1, 31, friendFacets1)
	addEdgeToUID(t, "friend", 1, 101, friendFacets2)
	addEdgeToUID(t, "friend", 31, 24, nil)
	addEdgeToUID(t, "friend", 23, 1, friendFacets1)

	nameFacets := map[string]string{"origin": "french"}
	// Now let's add a few properties for the main user.
	addEdgeToValue(t, "name", 1, "Michonne", nameFacets)
	addEdgeToValue(t, "gender", 1, "female", nil)

	// Now let's add a name for each of the friends, except 101.
	addEdgeToTypedValue(t, "name", 23, types.StringID, []byte("Rick Grimes"), nameFacets)
	addEdgeToValue(t, "gender", 23, "male", nil)
	addEdgeToValue(t, "name", 24, "Glenn Rhee", nil)
	addEdgeToValue(t, "name", 25, "Daryl Dixon", nil)
	addEdgeToValue(t, "name", 31, "Andrea", nil)
	// missing name for 101 -- no name edge and no facets.

	time.Sleep(5 * time.Millisecond)
}

func TestRetrieveFacetsSimple(t *testing.T) {
	populateGraphWithFacets(t)
	query := `
		{
			me(id:0x1) {
				name @facets
			}
		}
	`

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"name": "Michonne", "@facets": {"name": {"origin": "french"}}}]}`,
		js)
}

func TestRetrieveFacetsAll(t *testing.T) {
	populateGraphWithFacets(t)
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

	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"@facets":{"name":{"origin":"french"}},"friend":[{"@facets":{"_":{"since":"12-01-1991"},"name":{"origin":"french"}},"name":"Rick Grimes"},{"@facets":{"_":{"since":"12-01-1991"}},"name":"Glenn Rhee"},{"@facets":{"_":{"since":"12-01-1991"}},"name":"Daryl Dixon"},{"@facets":{"_":{"since":"12-01-1991"}},"name":"Andrea"},{"@facets":{"_":{"close":true,"family":false,"since":"11-10-2001"}}}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsNotInQuery(t *testing.T) {
	populateGraphWithFacets(t)
	query := `
		{
			me(id:0x1) {
				name
				friend {
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

func TestSubjectWithNoFacets(t *testing.T) {
	populateGraphWithFacets(t)
	// id 31 does not have any facets associated with name and friend
	query := `
		{
			me(id:0x1f) {
				name @facets
				friend @facets {
					name
				}
			}
		}
	`
	js := processToFastJSON(t, query)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Glenn Rhee"}],"name":"Andrea"}]}`,
		js)
}

func TestFetchingFewFacets(t *testing.T) {
	populateGraphWithFacets(t)
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
		`{"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"},{"@facets":{"_":{"close":true}}}],"name":"Michonne"}]}`,
		js)
}

func TestFacetsSortOrder(t *testing.T) {
	populateGraphWithFacets(t)
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
	x.Printf(js)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"},{"@facets":{"_":{"close":true,"family":false}}}],"name":"Michonne"}]}`,
		js)
}

func TestUnknownFacets(t *testing.T) {
	populateGraphWithFacets(t)
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
	x.Printf(js)
	require.JSONEq(t,
		`{"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"}]}`,
		js)
}

func TestToProtoFacets(t *testing.T) {
	populateGraphWithFacets(t)
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
	pb := processToPB(t, query, true)
	require.EqualValues(t,
		`attribute: "_root_"
children: <
  uid: 1
  attribute: "me"
  properties: <
    prop: "name"
    raw_value: <
      str_val: "Michonne"
    >
  >
  properties: <
    prop: "@facets"
    node_value: <
      attribute: "facets"
      properties: <
        prop: "name"
        node_value: <
          attribute: "name"
          properties: <
            prop: "origin"
            raw_value: <
              str_val: "french"
            >
          >
        >
      >
    >
  >
  children: <
    uid: 23
    attribute: "friend"
    properties: <
      prop: "name"
      raw_value: <
        str_val: "Rick Grimes"
      >
    >
    properties: <
      prop: "@facets"
      node_value: <
        attribute: "facets"
        properties: <
          prop: "name"
          node_value: <
            attribute: "name"
            properties: <
              prop: "origin"
              raw_value: <
                str_val: "french"
              >
            >
          >
        >
        properties: <
          prop: "_"
          node_value: <
            attribute: "friend"
            properties: <
              prop: "since"
              raw_value: <
                str_val: "12-01-1991"
              >
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
      raw_value: <
        str_val: "Glenn Rhee"
      >
    >
    properties: <
      prop: "@facets"
      node_value: <
        attribute: "_"
        properties: <
          prop: "_"
          node_value: <
            attribute: "friend"
            properties: <
              prop: "since"
              raw_value: <
                str_val: "12-01-1991"
              >
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
      raw_value: <
        str_val: "Daryl Dixon"
      >
    >
    properties: <
      prop: "@facets"
      node_value: <
        attribute: "_"
        properties: <
          prop: "_"
          node_value: <
            attribute: "friend"
            properties: <
              prop: "since"
              raw_value: <
                str_val: "12-01-1991"
              >
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
      raw_value: <
        str_val: "Andrea"
      >
    >
    properties: <
      prop: "@facets"
      node_value: <
        attribute: "_"
        properties: <
          prop: "_"
          node_value: <
            attribute: "friend"
            properties: <
              prop: "since"
              raw_value: <
                str_val: "12-01-1991"
              >
            >
          >
        >
      >
    >
  >
  children: <
    uid: 101
    attribute: "friend"
    properties: <
      prop: "@facets"
      node_value: <
        attribute: "_"
        properties: <
          prop: "_"
          node_value: <
            attribute: "friend"
            properties: <
              prop: "close"
              raw_value: <
                bool_val: true
              >
            >
            properties: <
              prop: "family"
              raw_value: <
                bool_val: false
              >
            >
            properties: <
              prop: "since"
              raw_value: <
                str_val: "11-10-2001"
              >
            >
          >
        >
      >
    >
  >
>
`,
		proto.MarshalTextString(pb))
}
