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

package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeleteAndReaddIndex(t *testing.T) {
	// Add new predicate with several indices.
	s1 := testSchema + "\n numerology: string @index(exact, term, fulltext) .\n"
	setSchema(s1)
	triples := `
		<0x666> <numerology> "This number is evil"  .
		<0x777> <numerology> "This number is good"  .
	`
	addTriplesToCluster(triples)

	// Verify fulltext index works as expected.
	q1 := `
	{
		me(func: anyoftext(numerology, "numbers")) {
			uid
			numerology
		}
	}`
	js := processQueryNoErr(t, q1)
	require.JSONEq(t, `{"data": {"me": [
		{"uid": "0x666", "numerology": "This number is evil"},
		{"uid": "0x777", "numerology": "This number is good"}
	]}}`, js)

	// Remove the fulltext index and verify the previous query is no longer supported.
	s2 := testSchema + "\n numerology: string @index(exact, term) .\n"
	setSchema(s2)
	_, err := processQuery(context.Background(), t, q1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Attribute numerology is not indexed with type fulltext")

	// Verify term index still works as expected.
	q2 := `
	{
		me(func: anyofterms(numerology, "number")) {
			uid
			numerology
		}
	}`
	js = processQueryNoErr(t, q2)
	require.JSONEq(t, `{"data": {"me": [
		{"uid": "0x666", "numerology": "This number is evil"},
		{"uid": "0x777", "numerology": "This number is good"}
	]}}`, js)

	// Re-add index and verify that the original query works again.
	setSchema(s1)
	js = processQueryNoErr(t, q1)
	require.JSONEq(t, `{"data": {"me": [
		{"uid": "0x666", "numerology": "This number is evil"},
		{"uid": "0x777", "numerology": "This number is good"}
	]}}`, js)

	// Finally, drop the predicate and restore schema.
	dropPredicate("numerology")
	setSchema(testSchema)
}

func TestDeleteAndReaddCount(t *testing.T) {
	// Add new predicate with count index.
	s1 := testSchema + "\n numerology: string @count .\n"
	setSchema(s1)
	triples := `
		<0x666> <numerology> "This number is evil"  .
		<0x777> <numerology> "This number is good"  .
	`
	addTriplesToCluster(triples)

	// Verify count index works as expected.
	q1 := `
	{
		me(func: gt(count(numerology), 0)) {
			uid
			numerology
		}
	}`
	js := processQueryNoErr(t, q1)
	require.JSONEq(t, `{"data": {"me": [
		{"uid": "0x666", "numerology": "This number is evil"},
		{"uid": "0x777", "numerology": "This number is good"}
	]}}`, js)

	// Remove the count index and verify the previous query is no longer supported.
	s2 := testSchema + "\n numerology: string .\n"
	setSchema(s2)
	_, err := processQuery(context.Background(), t, q1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Need @count directive in schema for attr: numerology")

	// Re-add count index and verify that the original query works again.
	setSchema(s1)
	js = processQueryNoErr(t, q1)
	require.JSONEq(t, `{"data": {"me": [
		{"uid": "0x666", "numerology": "This number is evil"},
		{"uid": "0x777", "numerology": "This number is good"}
	]}}`, js)

	// Finally, drop the predicate and restore schema.
	dropPredicate("numerology")
	setSchema(testSchema)
}

func TestDeleteAndReaddReverse(t *testing.T) {
	// Add new predicate with a reverse edge.
	s1 := testSchema + "\n child_pred: uid @reverse .\n"
	setSchema(s1)
	triples := `<0x666> <child_pred> <0x777>  .`
	addTriplesToCluster(triples)

	// Verify reverse edges works as expected.
	q1 := `
	{
		me(func: uid(0x777)) {
			~child_pred {
				uid
			}
		}
	}`
	js := processQueryNoErr(t, q1)
	require.JSONEq(t, `{"data": {"me": [{"~child_pred": [{"uid": "0x666"}]}]}}`, js)

	// Remove the reverse edges and verify the previous query is no longer supported.
	s2 := testSchema + "\n child_pred: uid .\n"
	setSchema(s2)
	_, err := processQuery(context.Background(), t, q1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Predicate child_pred doesn't have reverse edge")

	// Re-add reverse edges and verify that the original query works again.
	setSchema(s1)
	js = processQueryNoErr(t, q1)
	require.JSONEq(t, `{"data": {"me": [{"~child_pred": [{"uid": "0x666"}]}]}}`, js)

	// Finally, drop the predicate and restore schema.
	dropPredicate("child_pred")
	setSchema(testSchema)
}

func TestDropPredicate(t *testing.T) {
	// Add new predicate with several indices.
	s1 := testSchema + "\n numerology: string @index(term) .\n"
	setSchema(s1)
	triples := `
		<0x666> <numerology> "This number is evil"  .
		<0x777> <numerology> "This number is good"  .
	`
	addTriplesToCluster(triples)

	// Verify queries work as expected.
	q1 := `
	{
		me(func: anyofterms(numerology, "number")) {
			uid
			numerology
		}
	}`
	js := processQueryNoErr(t, q1)
	require.JSONEq(t, `{"data": {"me": [
		{"uid": "0x666", "numerology": "This number is evil"},
		{"uid": "0x777", "numerology": "This number is good"}
	]}}`, js)

	// Finally, drop the predicate and verify the query no longer works because
	// the index was dropped when all the data for that predicate was deleted.
	dropPredicate("numerology")
	_, err := processQuery(context.Background(), t, q1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Attribute numerology is not indexed with type term")

	// Finally, restore the schema.
	setSchema(testSchema)
}

func TestNestedExpandAll(t *testing.T) {
	query := `{
		q(func: has(node)) {
			uid
			expand(_all_) {
				uid
				node {
					uid
					expand(_all_)
				}
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {
    "q": [
      {
        "uid": "0x2b5c",
        "name": "expand",
        "node": [
          {
            "uid": "0x2b5c",
            "node": [
              {
                "uid": "0x2b5c",
                "name": "expand"
              }
            ]
          }
        ]
      }
    ]}}`, js)
}

func TestNoResultsFilter(t *testing.T) {
	query := `{
		q(func: has(nonexistent_pred)) @filter(le(name, "abc")) {
			uid
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": []}}`, js)
}

func TestNoResultsPagination(t *testing.T) {
	query := `{
		q(func: has(nonexistent_pred), first: 50) {
			uid
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": []}}`, js)
}

func TestNoResultsGroupBy(t *testing.T) {
	query := `{
		q(func: has(nonexistent_pred)) @groupby(name) {
			count(uid)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {}}`, js)
}

func TestNoResultsOrder(t *testing.T) {
	query := `{
		q(func: has(nonexistent_pred), orderasc: name) {
			uid
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": []}}`, js)
}

func TestNoResultsCount(t *testing.T) {
	query := `{
		q(func: has(nonexistent_pred)) {
			uid
			count(friend)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": []}}`, js)
}

func TestTypeExpandAll(t *testing.T) {
	query := `{
		q(func: eq(make, "Ford")) {
			expand(_all_) {
				uid
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q":[
		{"make":"Ford","model":"Focus","year":2008},
		{"make":"Ford","model":"Focus","year":2009, "previous_model": {"uid":"0xc8"}}
	]}}`, js)
}

func TestTypeExpandLang(t *testing.T) {
	query := `{
		q(func: eq(make, "Toyota")) {
			expand(_all_) {
				uid
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q":[
		{"make":"Toyota","model":"Prius", "model@jp":"プリウス", "year":2009}]}}`, js)
}

// Test Related to worker based pagination.

func TestHasOrderDesc(t *testing.T) {
	query := `{
		q(func:has(name), orderdesc: name, first:5) {
			 name
		 }
	 }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": ""
			  },
			  {
				"name": ""
			  },
			  {
				"name": "Badger"
			  },
			  {
				"name": "name"
			  },
			  {
				"name": "expand"
			  }
			]
		  }
	}`, js)
}
func TestHasOrderDescOffset(t *testing.T) {
	query := `{
		q(func:has(name), orderdesc: name, first:5, offset: 5) {
			 name
		 }
	 }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Shoreline Amphitheater"
			  },
			  {
				"name": "School B"
			  },
			  {
				"name": "School A"
			  },
			  {
				"name": "San Mateo School District"
			  },
			  {
				"name": "San Mateo High School"
			  }
			]
		  }
	}`, js)
}

func TestHasOrderAsc(t *testing.T) {
	query := `{
		q(func:has(name), orderasc: name, first:5) {
			 name
		 }
	 }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": ""
			  },
			  {
				"name": ""
			  },
			  {
				"name": "Alex"
			  },
			  {
				"name": "Alice"
			  },
			  {
				"name": "Alice"
			  }
			]
		  }
	}`, js)
}

func TestHasOrderAscOffset(t *testing.T) {
	query := `{
		q(func:has(name), orderasc: name, first:5, offset: 5) {
			 name
		 }
	 }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Alice"
			  },
			  {
				"name": "Alice"
			  },
			  {
				"name": "Alice"
			  },
			  {
				"name": "Alice\""
			  },
			  {
				"name": "Andre"
			  }
			]
		  }
	}`, js)
}

func TestHasFirst(t *testing.T) {
	query := `{
		q(func:has(name),first:5) {
			 name
		 }
	 }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Michonne"
			  },
			  {
				"name": "King Lear"
			  },
			  {
				"name": "Margaret"
			  },
			  {
				"name": "Leonard"
			  },
			  {
				"name": "Garfield"
			  }
			]
		  }
	}`, js)
}

func TestHasFirstOffset(t *testing.T) {
	query := `{
		q(func:has(name),first:5, offset: 5) {
			 name
		 }
	 }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Bear"
			  },
			  {
				"name": "Nemo"
			  },
			  {
				"name": "name"
			  },
			  {
				"name": "Rick Grimes"
			  },
			  {
				"name": "Glenn Rhee"
			  }
			]
		  }
	}`, js)
}

func TestHasFirstFilter(t *testing.T) {
	query := `{
		q(func:has(name), first: 1, offset:2)@filter(lt(age, 25)) {
			 name
		 }
	 }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Daryl Dixon"
			  }
			]
		  }
	}`, js)
}

func TestHasFilterOrderOffset(t *testing.T) {
	query := `{
		q(func:has(name), first: 2, offset:2, orderasc: name)@filter(gt(age, 20)) {
			 name
		 }
	 }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
			  {
				"name": "Alice"
			  },
			  {
				"name": "Bob"
			  }
			]
		  }
	}`, js)
}
func TestCascadeSubQuery1(t *testing.T) {
	query := `
	{
		me(func: uid(0x01)) {
			name
			full_name
			gender
			friend @cascade {
				name
				full_name
				friend {
					name
					full_name
					dob
					age
				}
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
					"full_name": "Michonne's large name for hashing",
					"gender": "female"
				}
			]
		}
	}`, js)
}

func TestCascadeSubQuery2(t *testing.T) {
	query := `
	{
		me(func: uid(0x01)) {
			name
			full_name
			gender
			friend {
				name
				full_name
				friend @cascade {
					name
					full_name
					dob
					age
				}
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
						"full_name": "Michonne's large name for hashing",
						"gender": "female",
						"friend": [
							{
								"name": "Rick Grimes",
								"friend": [
									{
										"name": "Michonne",
										"full_name": "Michonne's large name for hashing",
										"dob": "1910-01-01T00:00:00Z",
										"age": 38
									}
								]
							},
							{
								"name": "Glenn Rhee"
							},
							{
								"name": "Daryl Dixon"
							},
							{
								"name": "Andrea"
							}
						]
					}
				]
			}
		}`, js)
}

func TestCascadeRepeatedMultipleLevels(t *testing.T) {
	// It should have result same as applying @cascade at the top level friend predicate.
	query := `
	{
		me(func: uid(0x01)) {
			name
			full_name
			gender
			friend @cascade {
				name
				full_name
				friend @cascade {
					name
					full_name
					dob
					age
				}
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
					"full_name": "Michonne's large name for hashing",
					"gender": "female"
				}
			]
		}
	}`, js)
}

func TestCascadeSubQueryWithFilter(t *testing.T) {
	query := `
	{
		me(func: uid(0x01)) {
			name
			full_name
			gender
			friend {
				name
				full_name
				friend @cascade @filter(gt(age, 40)) {
					name
					full_name
					dob
					age
				}
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
						"full_name": "Michonne's large name for hashing",
						"gender": "female",
						"friend": [
							{
								"name": "Rick Grimes"
							},
							{
								"name": "Glenn Rhee"
							},
							{
								"name": "Daryl Dixon"
							},
							{
								"name": "Andrea"
							}
						]
					}
				]
			}
		}`, js)
}

func TestCascadeSubQueryWithVars1(t *testing.T) {
	query := `
	{
		him(func: uid(0x01)) {
			L as friend {
				B as friend @cascade {
					name
				}
			}
		}

		me(func: uid(L, B)) {
			name
		}
	}
`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
			"data": {
				"him": [
					{
						"friend": [
							{
								"friend": [
									{
										"name": "Michonne"
									}
								]
							},
							{
								"friend": [
									{
										"name": "Glenn Rhee"
									}
								]
							}
						]
					}
				],
				"me": [
					{
						"name": "Michonne"
					},
					{
						"name": "Rick Grimes"
					},
					{
						"name": "Glenn Rhee"
					},
					{
						"name": "Daryl Dixon"
					},
					{
						"name": "Andrea"
					}
				]
			}
		}`, js)
}

func TestCascadeSubQueryWithVars2(t *testing.T) {
	query := `
	{
		var(func: uid(0x01)) {
			L as friend  @cascade {
				B as friend
			}
		}

		me(func: uid(L, B)) {
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
						"name": "Michonne"
					},
					{
						"name": "Rick Grimes"
					},
					{
						"name": "Glenn Rhee"
					},
					{
						"name": "Andrea"
					}
				]
			}
		}`, js)
}

func TestCascadeSubQueryMultiUid(t *testing.T) {
	query := `
	{
		me(func: uid(0x01, 0x02, 0x03)) {
			name
			full_name
			gender
			friend @cascade {
				name
				full_name
				friend {
					name
					full_name
					dob
					age
				}
			}
		}
	}
`
	js := processQueryNoErr(t, query)
	// Friends of Michonne who don't have full_name predicate associated with them are filtered.
	require.JSONEq(t, `
		{
			"data": {
				"me": [
					{
						"name": "Michonne",
						"full_name": "Michonne's large name for hashing",
						"gender": "female"
					},
					{
						"name": "King Lear"
					},
					{
						"name": "Margaret"
					}
				]
			}
		}
	`, js)
}
