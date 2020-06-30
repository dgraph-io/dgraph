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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	facetSetupDone = false
)

func populateClusterWithFacets() error {
	// Return immediately if the setup has been performed already.
	if facetSetupDone {
		return nil
	}

	triples := `
		<1> <name> "Michelle"@en (origin = "french") .
		<25> <name> "Daryl Dixon" .
		<25> <alt_name> "Daryl Dick" .
		<31> <name> "Andrea" .
		<31> <alt_name> "Andy" .
		<33> <name> "Michale" .
		<34> <name> "Roger" .
		<320> <name> "Test facet"@en (type = "Test facet with lang") .
		<14000> <name> "Andrew" (kind = "official") .

		<31> <friend> <24> .

		<33> <schools> <2433> .

		<1> <gender> "female" .
		<23> <gender> "male" .

		<202> <model> "Prius" (type = "Electric") .

		<14000> <language> "english" (proficiency = "advanced") .
		<14000> <language> "hindi" (proficiency = "intermediate") .
		<14000> <language> "french" (proficiency = "novice") .

		<14000> <dgraph.type> "Speaker" .
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

	nameFacets := "(origin = \"french\", dummy = true)"
	nameFacets1 := "(origin = \"spanish\", dummy = false, isNick = true)"
	triples += fmt.Sprintf("<1> <name> \"Michonne\" %s .\n", nameFacets)
	triples += fmt.Sprintf("<23> <name> \"Rick Grimes\" %s .\n", nameFacets)
	triples += fmt.Sprintf("<24> <name> \"Glenn Rhee\" %s .\n", nameFacets)
	triples += fmt.Sprintf("<1> <alt_name> \"Michelle\" %s .\n", nameFacets)
	triples += fmt.Sprintf("<1> <alt_name> \"Michelin\" %s .\n", nameFacets1)
	triples += fmt.Sprintf("<12000> <name> \"Harry\"@en %s .\n", nameFacets)
	triples += fmt.Sprintf("<12000> <alt_name> \"Potter\" %s .\n", nameFacets1)

	bossFacet := "(company = \"company1\")"
	triples += fmt.Sprintf("<1> <boss> <34> %s .\n", bossFacet)

	friendFacets7 := "(since=2006-01-02T15:04:05, fastfriend=true, score=100, from=\"delhi\")"
	friendFacets8 := "(since=2007-01-02T15:04:05, fastfriend=false, score=100)"
	friendFacets9 := "(since=2008-01-02T15:04:05, fastfriend=true, score=200, from=\"bengaluru\")"
	triples += fmt.Sprintf("<33> <friend> <25> %s .\n", friendFacets7)
	triples += fmt.Sprintf("<33> <friend> <31> %s .\n", friendFacets8)
	triples += fmt.Sprintf("<33> <friend> <34> %s .\n", friendFacets9)

	triples += fmt.Sprintf("<34> <friend> <31> %s .\n", friendFacets8)
	triples += fmt.Sprintf("<34> <friend> <25> %s .\n", friendFacets9)

	err := addTriplesToCluster(triples)

	// Mark the setup as done so that the next tests do not have to perform it.
	facetSetupDone = true
	return err
}

func TestFacetsVarAllofterms(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{
			me(func: uid(31)) {
				name
				friend @facets(allofterms(games, "football basketball hockey")) {
					name
					uid
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"name":"Daryl Dixon","uid":"0x19"}],"name":"Andrea"}]}}`,
		js)
}

func TestFacetsWithVarEq(t *testing.T) {
	populateClusterWithFacets()
	// find family of 1
	query := `
		query works($family : bool = true){
			me(func: uid(1)) {
				name
				friend @facets(eq(family, $family)) {
					name
					uid
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19", "name": "Daryl Dixon"}],"name":"Michonne"}]}}`,
		js)
}

func TestFacetWithVarLe(t *testing.T) {
	populateClusterWithFacets()

	query := `
		query works($age : int = 35) {
			me(func: uid(0x1)) {
				name
				friend @facets(le(age, $age)) {
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

func TestFacetWithVarGt(t *testing.T) {
	populateClusterWithFacets()

	query := `
		query works($age : int = "32") {
			me(func: uid(0x1)) {
				name
				friend @facets(gt(age, $age)) {
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
		`{"data":{"me":[{"name|origin":"french","name|dummy":true,"name":"Michonne",
			"gender":"female"}]}}`, js)
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
	require.JSONEq(t, `
		{
		  "data": {
		    "me": [
		      {
		        "friend": [
		          {
		            "name": "Glenn Rhee",
		            "friend|since": "2004-05-02T15:04:05Z"
		          },
		          {
		            "name": "Rick Grimes",
		            "friend|since": "2006-01-02T15:04:05Z"
		          },
		          {
		            "name": "Andrea",
		            "friend|since": "2006-01-02T15:04:05Z"
		          },
		          {
		            "name": "Daryl Dixon",
		            "friend|since": "2007-05-02T15:04:05Z"
		          }
		        ]
		      }
		    ]
		  }
		}
	`, js)
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
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "friend": [
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|since": "2007-05-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Rick Grimes",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Glenn Rhee",
		                        "friend|since": "2004-05-02T15:04:05Z"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
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
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "friend": [
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|since": "2007-05-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Rick Grimes",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Glenn Rhee",
		                        "friend|since": "2004-05-02T15:04:05Z"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
}

func TestFacetsMultipleOrderby(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{
			me(func: uid(33)) {
				name
				friend @facets(orderasc:score, orderdesc:since) {
					name
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "name": "Michale",
		                "friend": [
		                    {
		                        "name": "Andrea",
		                        "friend|score": 100,
		                        "friend|since": "2007-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|score": 100,
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Roger",
		                        "friend|score": 200,
		                        "friend|since": "2008-01-02T15:04:05Z"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
}

func TestFacetsMultipleOrderbyMultipleUIDs(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{
			me(func: uid(33, 34)) {
				name
				friend @facets(orderdesc:since, orderasc:score) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "name": "Michale",
		                "friend": [
		                    {
		                        "name": "Roger",
		                        "friend|score": 200,
		                        "friend|since": "2008-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|score": 100,
		                        "friend|since": "2007-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|score": 100,
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    }
		                ]
		            },
		            {
		                "name": "Roger",
		                "friend": [
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|score": 200,
		                        "friend|since": "2008-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|score": 100,
		                        "friend|since": "2007-01-02T15:04:05Z"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
}

func TestFacetsMultipleOrderbyNonsortableFacet(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{
			me(func: uid(33)) {
				name
				friend @facets(orderasc:score, orderasc:fastfriend) {
					name
				}
			}
		}
	`

	js := processQueryNoErr(t, query)
	// Since fastfriend is of bool type, it is not sortable.
	// Hence result should be sorted by score.
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "name": "Michale",
		                "friend": [
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|fastfriend": true,
		                        "friend|score": 100
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|fastfriend": false,
		                        "friend|score": 100
		                    },
		                    {
		                        "name": "Roger",
		                        "friend|fastfriend": true,
		                        "friend|score": 200
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
}

func TestFacetsMultipleOrderbyAllFacets(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{
			me(func: uid(33)) {
				name
				friend @facets(fastfriend, from, orderdesc:score, orderasc:since) {
					name
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "name": "Michale",
		                "friend": [
		                    {
		                        "name": "Roger",
		                        "friend|fastfriend": true,
		                        "friend|from": "bengaluru",
		                        "friend|score": 200,
		                        "friend|since": "2008-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|fastfriend": true,
		                        "friend|from": "delhi",
		                        "friend|score": 100,
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|fastfriend": false,
		                        "friend|score": 100,
		                        "friend|since": "2007-01-02T15:04:05Z"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
}

// This test tests multiple order by on facets where some facets in not present in all records.
func TestFacetsMultipleOrderbyMissingFacets(t *testing.T) {
	populateClusterWithFacets()
	query := `
		{
			me(func: uid(33)) {
				name
				friend @facets(orderasc:from, orderdesc:since) {
					name
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "name": "Michale",
		                "friend": [
		                    {
		                        "name": "Roger",
		                        "friend|from": "bengaluru",
		                        "friend|since": "2008-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|from": "delhi",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|since": "2007-01-02T15:04:05Z"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
}

func TestRetrieveFacetsAsVars(t *testing.T) {
	populateClusterWithFacets()
	// to see how friend @facets are positioned in output.
	query := `
		{
			var(func: uid(0x1)) {
				friend @facets(a as since)
			}

			me(func: uid(23)) {
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
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "friend": [
		                    {
		                        "name|dummy": true,
		                        "name|origin": "french",
		                        "name": "Rick Grimes",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name|dummy": true,
		                        "name|origin": "french",
		                        "name": "Glenn Rhee",
		                        "friend|close": true,
		                        "friend|family": true,
		                        "friend|since": "2004-05-02T15:04:05Z",
		                        "friend|tag": "Domain3"
		                    },
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|close": false,
		                        "friend|family": true,
		                        "friend|since": "2007-05-02T15:04:05Z",
		                        "friend|tag": 34
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
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
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "name|dummy": true,
		                "name|origin": "french",
		                "name": "Michonne",
		                "friend": [
		                    {
		                        "name|dummy": true,
		                        "name|origin": "french",
		                        "name": "Rick Grimes",
		                        "gender": "male",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name|dummy": true,
		                        "name|origin": "french",
		                        "name": "Glenn Rhee",
		                        "friend|close": true,
		                        "friend|family": true,
		                        "friend|since": "2004-05-02T15:04:05Z",
		                        "friend|tag": "Domain3"
		                    },
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|close": false,
		                        "friend|family": true,
		                        "friend|since": "2007-05-02T15:04:05Z",
		                        "friend|tag": 34
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    }
		                ],
		                "gender": "female"
		            }
		        ]
		    }
		}
	`, js)
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
	require.JSONEq(t, `
		{
			"data":{
				"me":[
					{
						"name":"Michonne",
						"friend":[
							{
								"name":"Rick Grimes"
							},
							{
								"name":"Glenn Rhee",
								"friend|close": true
							},
							{
								"name":"Daryl Dixon",
								"friend|close": false
							},
							{
								"name":"Andrea"
							}
						]
					}
				]
			}
		}
	`, js)
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
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "name": "Michonne",
		                "friend": [
		                    {
		                        "name": "Rick Grimes"
		                    },
		                    {
		                        "name": "Glenn Rhee",
		                        "friend|close": true,
		                        "friend|family": true
		                    },
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|close": false,
		                        "friend|family": true
		                    },
		                    {
		                        "name": "Andrea"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
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
	require.NoError(t,
		addTriplesToCluster(fmt.Sprintf(`<1> <friend> <101> %s .`, friendFacets)))
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
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "name": "Michonne",
		                "friend": [
		                    {
		                        "name": "Rick Grimes",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|close": false,
		                        "friend|family": true,
		                        "friend|since": "2007-05-02T15:04:05Z",
		                        "friend|tag": 34
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
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

func TestFacetsFilterAtValueBasic(t *testing.T) {
	populateClusterWithFacets()
	query := `
	{
		me(func: has(name)) {
			name @facets(eq(origin, "french"))
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name": "Michonne"}, {"name":"Rick Grimes"}, {"name": "Glenn Rhee"}]}}`,
		js)
}

func TestFacetsFilterAtValueListType(t *testing.T) {
	populateClusterWithFacets()
	query := `
	{
		me(func: has(name)) {
			alt_name @facets(eq(origin, "french"))
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alt_name": ["Michelle"]}]}}`, js)
}

func TestFacetsFilterAtValueComplex1(t *testing.T) {
	populateClusterWithFacets()
	query := `
	{
		me(func: has(name)) {
			name @facets(eq(origin, "french") AND eq(dummy, true))
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name": "Michonne"}, {"name":"Rick Grimes"}, {"name": "Glenn Rhee"}]}}`,
		js)
}

func TestFacetsFilterAtValueComplex2(t *testing.T) {
	populateClusterWithFacets()
	query := `
	{
		me(func: has(name)) {
			name @facets(eq(origin, "french") AND eq(dummy, false))
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
}

func TestFacetsFilterAtValueWithLangs(t *testing.T) {
	populateClusterWithFacets()
	query := `
	{
		me(func: has(name)) {
			name@en @facets(eq(origin, "french"))
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@en": "Michelle"}]}}`, js)
}

func TestFacetsFilterAtValueWithBadLang(t *testing.T) {
	populateClusterWithFacets()
	query := `
	{
		me(func: has(name)) {
			name@hi @facets(eq(origin, "french"))
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
}

func TestFacetsFilterAtValueWithFacet(t *testing.T) {
	populateClusterWithFacets()
	query := `
	{
		me(func: has(name)) {
			name @facets(eq(origin, "french")) @facets(origin)
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name": "Michonne", "name|origin": "french"},
			{"name": "Rick Grimes", "name|origin": "french"},
			{"name": "Glenn Rhee", "name|origin": "french"}]}}`, js)
}

func TestFacetsFilterAtValueWithFacetAndLangs(t *testing.T) {
	populateClusterWithFacets()
	query := `
	{
		me(func: has(name)) {
			name@en @facets(eq(origin, "french")) @facets(origin)
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@en": "Michelle", "name@en|origin": "french"}]}}`, js)
}

func TestFacetsFilterAtValueWithDifferentFacet(t *testing.T) {
	populateClusterWithFacets()
	query := `
	{
		me(func: has(name)) {
			name @facets(eq(dummy, "true")) @facets(origin)
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name": "Michonne", "name|origin": "french"},
			{"name": "Rick Grimes", "name|origin": "french"},
			{"name": "Glenn Rhee", "name|origin": "french"}]}}`, js)
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
	require.JSONEq(t, `
		{
			"data":{
				"me":[
					{
						"name":"Michonne",
						"friend":[
							{
								"name":"Glenn Rhee",
								"uid":"0x18",
								"friend|family": true
							},
							{
								"uid":"0x65",
								"friend|family": false
							}
						]
					}
				]
			}
		}
	`, js)
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
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "friend": [
		                    {
		                        "name": "Glenn Rhee",
		                        "friend|close": true,
		                        "friend|family": true,
		                        "friend|since": "2004-05-02T15:04:05Z",
		                        "friend|tag": "Domain3"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
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
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "friend": [
		                    {
		                        "uid": "0x19",
		                        "name": "Daryl Dixon",
		                        "friend|since": "2007-05-02T15:04:05Z"
		                    },
		                    {
		                        "uid": "0x17",
		                        "name": "Rick Grimes",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "uid": "0x1f",
		                        "name": "Andrea",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "uid": "0x65",
		                        "friend|since": "2005-05-02T15:04:05Z"
		                    },
		                    {
		                        "uid": "0x18",
		                        "name": "Glenn Rhee",
		                        "friend|since": "2004-05-02T15:04:05Z"
		                    }
		                ],
		                "uid": "0x1",
		                "name": "Michonne"
		            }
		        ]
		    }
		}
	`, js)

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
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "friend": [
		                    {
		                        "uid": "0x18",
		                        "name": "Glenn Rhee",
		                        "friend|since": "2004-05-02T15:04:05Z"
		                    },
		                    {
		                        "uid": "0x65",
		                        "friend|since": "2005-05-02T15:04:05Z"
		                    },
		                    {
		                        "uid": "0x17",
		                        "name": "Rick Grimes",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "uid": "0x1f",
		                        "name": "Andrea",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "uid": "0x19",
		                        "name": "Daryl Dixon",
		                        "friend|since": "2007-05-02T15:04:05Z"
		                    }
		                ],
		                "uid": "0x1",
		                "name": "Michonne"
		            }
		        ]
		    }
		}
	`, js)
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
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "o": "french",
		                "name": "Michonne",
		                "friend": [
		                    {
		                        "o": "french",
		                        "name": "Rick Grimes",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "o": "french",
		                        "name": "Glenn Rhee",
		                        "friend|family": true,
		                        "friend|since": "2004-05-02T15:04:05Z",
		                        "tagalias": "Domain3"
		                    },
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|family": true,
		                        "friend|since": "2007-05-02T15:04:05Z",
		                        "tagalias": 34
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
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
	require.JSONEq(t, `
		{
			"data":{
				"me2":[

				],
				"me":[
					{
						"name":"Rick Grimes",
						"val(a)":"2006-01-02T15:04:05Z"
					}
				]
			}
		}
	`, js)
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
		{"name": "Car", "make":"Toyota","model":"Prius", "model@jp":"プリウス",
			"model|type":"Electric", "year":2009, "owner": [{"uid": "0xcb"}]}]}}`, js)
}

func TestFacetsCascadeScalarPredicate(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(1, 23)) @cascade {
			name @facets
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
	{
		"data": {
			"q": [
				{
					"name|dummy": true,
					"name|origin": "french",
					"name": "Michonne"
				},
				{
					"name|dummy": true,
					"name|origin": "french",
					"name": "Rick Grimes"
				}
			]
		}
	}
	`, js)
}

func TestFacetsCascadeUIDPredicate(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(1, 23, 24)) @cascade {
			name @facets
			friend {
				name @facets
			}
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
	{
		"data": {
			"q": [
				{
					"name|dummy": true,
					"name|origin": "french",
					"name": "Michonne",
					"friend": [
						{
							"name|dummy": true,
							"name|origin": "french",
							"name": "Rick Grimes"
						},
						{
							"name|dummy": true,
							"name|origin": "french",
							"name": "Glenn Rhee"
						},
						{
							"name": "Daryl Dixon"
						},
						{
							"name": "Andrea"
						}
					]
				},
				{
					"name|dummy": true,
					"name|origin": "french",
					"name": "Rick Grimes",
					"friend": [
						{
							"name|dummy": true,
							"name|origin": "french",
							"name": "Michonne"
						}
					]
				}
			]
		}
	}
	`, js)
}

func TestFacetsNestedCascade(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(1, 23)) {
			name @facets
			friend @cascade {
				name @facets
			}
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
	{
		"data": {
			"q": [
				{
					"name|dummy": true,
					"name|origin": "french",
					"name": "Michonne",
					"friend": [
						{
							"name|dummy": true,
							"name|origin": "french",
							"name": "Rick Grimes"
						},
						{
							"name|dummy": true,
							"name|origin": "french",
							"name": "Glenn Rhee"
						},
						{
							"name": "Daryl Dixon"
						},
						{
							"name": "Andrea"
						}
					]
				},
				{
					"name|dummy": true,
					"name|origin": "french",
					"name": "Rick Grimes",
					"friend": [
						{
							"name|dummy": true,
							"name|origin": "french",
							"name": "Michonne"
						}
					]
				}
			]
		}
	}
	`, js)
}

func TestFacetsCascadeWithFilter(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(1, 23)) @filter(eq(name, "Michonne")) @cascade {
			name @facets
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
	{
		"data": {
			"q": [
				{
					"name|dummy": true,
					"name|origin": "french",
					"name": "Michonne"
				}
			]
		}
	}`, js)
}

func TestFacetUIDPredicate(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(0x1)) {
			name
			boss @facets {
				name
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
			"data":{
				"q":[
					{
						"name":"Michonne",
						"boss":{
							"name":"Roger",
							"boss|company":"company1"
						}
					}
				]
			}
		}
	`, js)
}

func TestFacetUIDListPredicate(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(0x1)) {
			name
			friend @facets(since) {
				name
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
		    "data": {
		        "q": [
		            {
		                "name": "Michonne",
		                "friend": [
		                    {
		                        "name": "Rick Grimes",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Glenn Rhee",
		                        "friend|since": "2004-05-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Daryl Dixon",
		                        "friend|since": "2007-05-02T15:04:05Z"
		                    },
		                    {
		                        "name": "Andrea",
		                        "friend|since": "2006-01-02T15:04:05Z"
		                    }
		                ]
		            }
		        ]
		    }
		}
	`, js)
}

func TestFacetValueListPredicate(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(1, 12000)) {
			name@en @facets
			alt_name @facets
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
			"data":{
				"q":[
					{
						"name@en|origin":"french",
						"name@en":"Michelle",
						"alt_name|dummy":{
							"0":true,
							"1":false
						},
						"alt_name|origin":{
							"0":"french",
							"1":"spanish"
						},
						"alt_name|isNick":{
							"1":true
						},
						"alt_name":[
							"Michelle",
							"Michelin"
						]
					},
					{
						"name@en|dummy":true,
						"name@en|origin":"french",
						"name@en":"Harry",
						"alt_name|dummy":{
							"0":false
						},
						"alt_name|isNick":{
							"0":true
						},
						"alt_name|origin":{
							"0":"spanish"
						},
						"alt_name":[
							"Potter"
						]
					}
				]
			}
		}
	`, js)
}

func TestFacetUIDPredicateWithNormalize(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(0x1)) @normalize {
			name: name
			from: boss @facets {
				boss: name
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
			"data": {
				"q": [
					{
						"boss": "Roger",
						"from|company": "company1",
						"name": "Michonne"
					}
				]
			}
		}
	`, js)
}

func TestFacetUIDListPredicateWithNormalize(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(0x1)) @normalize {
			name: name
			friend @facets(since) {
				friend_name: name
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
			"data": {
				"q": [
					{
						"friend_name": "Rick Grimes",
						"friend|since": "2006-01-02T15:04:05Z",
						"name": "Michonne"
					},
					{
						"friend_name": "Glenn Rhee",
						"friend|since": "2004-05-02T15:04:05Z",
						"name": "Michonne"
					},
					{
						"friend_name": "Daryl Dixon",
						"friend|since": "2007-05-02T15:04:05Z",
						"name": "Michonne"
					},
					{
						"friend_name": "Andrea",
						"friend|since": "2006-01-02T15:04:05Z",
						"name": "Michonne"
					}
				]
			}
		}
	`, js)
}

func TestNestedFacetUIDListPredicateWithNormalize(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(0x1)) @normalize {
			name: name
			friend @facets(since) @normalize {
				friend_name: name @facets
				friend @facets(close)  {
					friend_name_level2: name
				}
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
			"data": {
				"q": [
					{
						"friend_name": "Rick Grimes",
						"friend_name_level2": "Michonne",
						"friend_name|dummy": true,
						"friend_name|origin": "french",
						"friend|since": "2006-01-02T15:04:05Z",
						"name": "Michonne"
					},
					{
						"friend_name": "Glenn Rhee",
						"friend_name|dummy": true,
						"friend_name|origin": "french",
						"friend|since": "2004-05-02T15:04:05Z",
						"name": "Michonne"
					},
					{
						"friend_name": "Daryl Dixon",
						"friend|since": "2007-05-02T15:04:05Z",
						"name": "Michonne"
					},
					{
						"friend_name": "Andrea",
						"friend_name_level2": "Michonne",
						"friend|close": false,
						"friend|since": "2006-01-02T15:04:05Z",
						"name": "Michonne"
					},
					{
						"friend_name": "Andrea",
						"friend_name_level2": "Glenn Rhee",
						"friend|since": "2006-01-02T15:04:05Z",
						"name": "Michonne"
					},
					{
						"friend_name": "Andrea",
						"friend_name_level2": "Daryl Dixon",
						"friend|close": false,
						"friend|since": "2006-01-02T15:04:05Z",
						"name": "Michonne"
					}
				]
			}
		}
	`, js)
}

func TestFacetValuePredicateWithNormalize(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(1, 12000)) @normalize {
			eng_name: name@en @facets
			alt_name: alt_name @facets
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
			"data":{
				"q":[
					{
						"eng_name|origin":"french",
						"eng_name":"Michelle",
						"alt_name|dummy":{
							"0":true,
							"1":false
						},
						"alt_name|origin":{
							"0":"french",
							"1":"spanish"
						},
						"alt_name|isNick":{
							"1":true
						},
						"alt_name":[
							"Michelle",
							"Michelin"
						]
					},
					{
						"eng_name|dummy":true,
						"eng_name|origin":"french",
						"eng_name":"Harry",
						"alt_name|dummy":{
							"0":false
						},
						"alt_name|isNick":{
							"0":true
						},
						"alt_name|origin":{
							"0":"spanish"
						},
						"alt_name":[
							"Potter"
						]
					}
				]
			}
		}
	`, js)
}

func TestFacetValueListPredicateSingleFacet(t *testing.T) {
	populateClusterWithFacets()
	query := `{
		q(func: uid(0x1)) {
			alt_name @facets(origin)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
			"data":{
				"q":[
					{
						"alt_name|origin":{
							"0":"french",
							"1":"spanish"
						},
						"alt_name":[
							"Michelle",
							"Michelin"
						]
					}
				]
			}
		}
	`, js)
}

func TestFacetsWithExpand(t *testing.T) {
	populateClusterWithFacets()

	query := `{
		q(func: uid(14000)) {
			dgraph.type
			expand(_all_)
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
	{
		"data": {
			"q": [
				{
					"dgraph.type": [
						"Speaker"
					],
					"name|kind": "official",
					"name": "Andrew",
					"language|proficiency": {
						"0": "novice",
						"1": "intermediate",
						"2": "advanced"
					},
					"language": [
						"french",
						"hindi",
						"english"
					]
				}
			]
		}
	}`, js)
}

func TestCountFacetsFilteringUidListPredicate(t *testing.T) {
	populateClusterWithFacets()

	query := `{
		q(func: uid(1, 33)) {
			name
			filtered_count: count(friend) @facets(eq(since, "2006-01-02T15:04:05"))
			full_count: count(friend)
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
	{
		"data": {
			"q": [
				{
					"name": "Michonne",
					"filtered_count": 2,
					"full_count": 5
				},
				{
					"name": "Michale",
					"filtered_count": 1,
					"full_count": 3
				}
			]
		}
	}`, js)
}

func TestCountFacetsFilteringUidPredicate(t *testing.T) {
	populateClusterWithFacets()

	query := `{
		q(func: uid(1, 33)) {
			name
			filtered_count: count(boss) @facets(eq(company, "company1"))
			full_count: count(boss)
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
	{
		"data": {
			"q": [
				{
					"name": "Michonne",
					"filtered_count": 1,
					"full_count": 1
				},
				{
					"name": "Michale",
					"filtered_count": 0,
					"full_count": 0
				}
			]
		}
	}`, js)
}

func TestCountFacetsFilteringScalarPredicate(t *testing.T) {
	populateClusterWithFacets()

	query := `{
		q(func: uid(1, 23)) {
			name
			french_origin_count: count(name) @facets(eq(origin, "french"))
			french_spanish_count: count(name) @facets(eq(origin, "spanish"))
			full_count: count(name)
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
	{
		"data": {
			"q": [
				{
					"name": "Michonne",
					"french_origin_count": 1,
					"french_spanish_count": 0,
					"full_count": 1
				},
				{
					"name": "Rick Grimes",
					"french_origin_count": 1,
					"french_spanish_count": 0,
					"full_count": 1
				}
			]
		}
	}`, js)
}

func TestCountFacetsFilteringScalarListPredicate(t *testing.T) {
	populateClusterWithFacets()

	query := `{
		q(func: uid(1, 12000)) {
			name
			alt_name
			filtered_count: count(alt_name) @facets(eq(origin, "french"))
			full_count: count(alt_name)
		}
	}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
	{
		"data": {
			"q": [
				{
					"name": "Michonne",
					"alt_name": [
						"Michelle",
						"Michelin"
					],
					"filtered_count": 1,
					"full_count": 2
				},
				{
					"alt_name": [
						"Potter"
					],
					"filtered_count": 0,
					"full_count": 1
				}
			]
		}
	}`, js)
}
