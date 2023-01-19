/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/x"
)

func TestBigMathValue(t *testing.T) {
	s1 := testSchema + "\n money: int .\n"
	setSchema(s1)
	triples := `
		_:user1 <money> "48038396025285290" .
	`
	require.NoError(t, addTriplesToCluster(triples))

	t.Run("div", func(t *testing.T) {
		q1 := `
	{
		q(func: has(money)) {
			f as money
			g: math(f/2)
		}
	}
	`

		js := processQueryNoErr(t, q1)
		require.JSONEq(t, `{"data":{"q":[
		{"money":48038396025285290,
		"g":24019198012642645}
	]}}`, js)

	})

	t.Run("add", func(t *testing.T) {
		q1 := `
	{
		q(func: has(money)) {
			f as money
			g: math(2+f)
		}
	}
	`

		js := processQueryNoErr(t, q1)
		require.JSONEq(t, `{"data":{"q":[
		{"money":48038396025285290,
		"g":48038396025285292}
	]}}`, js)

	})

	t.Run("sub", func(t *testing.T) {
		q1 := `
	{
		q(func: has(money)) {
			f as money
			g: math(f-2)
		}
	}
	`

		js := processQueryNoErr(t, q1)
		require.JSONEq(t, `{"data":{"q":[
		{"money":48038396025285290,
		"g":48038396025285288}
	]}}`, js)

	})
}

func TestFloatConverstion(t *testing.T) {
	t.Run("Convert up to float", func(t *testing.T) {
		query := `
	{
		me as var(func: eq(name, "Michonne"))
		var(func: uid(me)) {
			friend {
				x as age
			}
			x2 as sum(val(x))
			c as count(friend)
		}

		me(func: uid(me)) {
			ceilAge: math(ceil((1.0*x2)/c))
		}
	}
	`
		js := processQueryNoErr(t, query)
		require.JSONEq(t, `{"data": {"me":[{"ceilAge":14.000000}]}}`, js)
	})

	t.Run("Int aggregation only", func(t *testing.T) {
		query := `
	{
		me as var(func: eq(name, "Michonne"))
		var(func: uid(me)) {
			friend {
				x as age
			}
			x2 as sum(val(x))
			c as count(friend)
		}

		me(func: uid(me)) {
			ceilAge: math(ceil(x2/c))
		}
	}
	`
		js := processQueryNoErr(t, query)
		require.JSONEq(t, `{"data": {"me":[{"ceilAge":13.000000}]}}`, js)
	})

}

func TestDeleteAndReadIndex(t *testing.T) {
	// Add new predicate with several indices.
	s1 := testSchema + "\n numerology: string @index(exact, term, fulltext) .\n"
	setSchema(s1)
	triples := `
		<0x666> <numerology> "This number is evil"  .
		<0x777> <numerology> "This number is good"  .
	`
	require.NoError(t, addTriplesToCluster(triples))

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

func TestDeleteAndReadCount(t *testing.T) {
	// Add new predicate with count index.
	s1 := testSchema + "\n numerology: string @count .\n"
	setSchema(s1)
	triples := `
		<0x666> <numerology> "This number is evil"  .
		<0x777> <numerology> "This number is good"  .
	`
	require.NoError(t, addTriplesToCluster(triples))

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

func TestDeleteAndReadReverse(t *testing.T) {
	// Add new predicate with a reverse edge.
	s1 := testSchema + "\n child_pred: uid @reverse .\n"
	setSchema(s1)
	triples := `<0x666> <child_pred> <0x777>  .`
	require.NoError(t, addTriplesToCluster(triples))

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

func TestSchemaUpdateNoConflict(t *testing.T) {
	// Verify schema is as expected for the predicate with noconflict directive.
	q1 := `schema(pred: [noconflict_pred]) { }`
	js := processQueryNoErr(t, q1)
	require.JSONEq(t, `{
		"data": {
			"schema": [{
				"predicate": "noconflict_pred",
				"type": "string",
				"no_conflict": true
			}]
		}
	}`, js)

	// Verify schema is as expected for the predicate without noconflict directive.
	q1 = `schema(pred: [name]) { }`
	js = processQueryNoErr(t, q1)
	require.JSONEq(t, `{
		"data": {
			"schema": [{
				"predicate": "name",
				"type": "string",
				"index": true,
				"tokenizer": ["term", "exact", "trigram"],
				"count": true,
				"lang": true
			}]
		}
	}`, js)
}

func TestNoConflictQuery1(t *testing.T) {
	schema := `
		type node {
		name_noconflict: string
		child: uid
		}

		name_noconflict: string @noconflict .
		child: uid .
	`
	setSchema(schema)

	type node struct {
		ID    string `json:"uid"`
		Name  string `json:"name_noconflict"`
		Child *node  `json:"child"`
	}

	child := node{ID: "_:blank-0", Name: "child"}
	js, err := json.Marshal(child)
	require.NoError(t, err)

	res, err := client.NewTxn().Mutate(context.Background(),
		&api.Mutation{SetJson: js, CommitNow: true})
	require.NoError(t, err)

	in := []node{}
	for i := 0; i < 5; i++ {
		in = append(in, node{ID: "_:blank-0", Name: fmt.Sprintf("%d", i+1),
			Child: &node{ID: res.GetUids()["blank-0"]}})
	}

	errChan := make(chan error)
	for i := range in {
		go func(n node) {
			js, err := json.Marshal(n)
			require.NoError(t, err)

			_, err = client.NewTxn().Mutate(context.Background(),
				&api.Mutation{SetJson: js, CommitNow: true})
			errChan <- err
		}(in[i])
	}

	errs := []error{}
	for i := 0; i < len(in); i++ {
		errs = append(errs, <-errChan)
	}

	for _, e := range errs {
		assert.NoError(t, e)
	}
}

func TestNoConflictQuery2(t *testing.T) {
	schema := `
		type node {
		name_noconflict: string
		address_conflict: string
		child: uid
		}

		name_noconflict: string @noconflict .
		address_conflict: string .
		child: uid .
	`
	setSchema(schema)

	type node struct {
		ID      string `json:"uid"`
		Name    string `json:"name_noconflict"`
		Child   *node  `json:"child"`
		Address string `json:"address_conflict"`
	}

	child := node{ID: "_:blank-0", Name: "child", Address: "dgraph labs"}
	js, err := json.Marshal(child)
	require.NoError(t, err)

	res, err := client.NewTxn().Mutate(context.Background(),
		&api.Mutation{SetJson: js, CommitNow: true})
	require.NoError(t, err)

	in := []node{}
	for i := 0; i < 5; i++ {
		in = append(in, node{ID: "_:blank-0", Name: fmt.Sprintf("%d", i+1),
			Child: &node{ID: res.GetUids()["blank-0"]}})
	}

	errChan := make(chan error)
	for i := range in {
		go func(n node) {
			js, err := json.Marshal(n)
			require.NoError(t, err)

			_, err = client.NewTxn().Mutate(context.Background(),
				&api.Mutation{SetJson: js, CommitNow: true})
			errChan <- err
		}(in[i])
	}

	errs := []error{}
	for i := 0; i < len(in); i++ {
		errs = append(errs, <-errChan)
	}

	hasError := false
	for _, e := range errs {
		if e != nil {
			hasError = true
			require.Contains(t, e.Error(), "Transaction has been aborted. Please retry")
		}
	}
	x.AssertTrue(hasError)
}

func TestDropPredicate(t *testing.T) {
	// Add new predicate with several indices.
	s1 := testSchema + "\n numerology: string @index(term) .\n"
	setSchema(s1)
	triples := `
		<0x666> <numerology> "This number is evil"  .
		<0x777> <numerology> "This number is good"  .
	`
	require.NoError(t, addTriplesToCluster(triples))

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
		{"make":"Ford","model":"Focus","year":2008, "~previous_model": [{"uid":"0xc9"}]},
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
		{"name": "Car", "make":"Toyota","model":"Prius", "model@jp":"プリウス", "year":2009,
			"owner": [{"uid": "0xcb"}]}]}}`, js)
}

func TestTypeExpandExplicitType(t *testing.T) {
	query := `{
		q(func: eq(make, "Toyota")) {
			expand(Object) {
				uid
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q":[{"name":"Car", "owner": [{"uid": "0xcb"}]}]}}`, js)
}

func TestTypeExpandMultipleExplicitTypes(t *testing.T) {
	query := `{
		q(func: eq(make, "Toyota")) {
			expand(CarModel, Object) {
				uid
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q":[
		{"name": "Car", "make":"Toyota","model":"Prius", "model@jp":"プリウス", "year":2009,
			"owner": [{"uid": "0xcb"}]}]}}`, js)
}

func TestTypeFilterAtExpand(t *testing.T) {
	query := `{
		q(func: eq(make, "Toyota")) {
			expand(_all_) @filter(type(Person)) {
				owner_name
				uid
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"q":[{"owner": [{"owner_name": "Owner of Prius", "uid": "0xcb"}]}]}}`, js)
}

func TestTypeFilterAtExpandEmptyResults(t *testing.T) {
	query := `{
		q(func: eq(make, "Toyota")) {
			expand(_all_) @filter(type(Animal)) {
				owner_name
				uid
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q":[]}}`, js)
}

func TestFilterAtSameLevelOnUIDWithExpand(t *testing.T) {
	query := `{
		q(func: eq(name, "Michonne")) {
			expand(_all_)
			friend @filter(eq(alive, true)){
				expand(_all_)
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"q":[{"name":"Michonne","gender":"female","alive":true,
	"friend":[{"gender":"male","alive":true,"name":"Rick Grimes"}]}]}}`, js)
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
				"name": "name"
			  },
			  {
				"name": "expand"
			  },
			  {
				"name": "Shoreline Amphitheater"
			  },
			  {
				"name": "School B"
			  },
			  {
				"name": "School A"
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
				"name": "San Mateo School District"
			  },
			  {
				"name": "San Mateo High School"
			  },
			  {
				"name": "San Mateo County"
			  },
			  {
				"name": "San Carlos Airport"
			  },
			  {
				"name": "San Carlos"
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
				"name": "A"
			  },
			  {
				"name": "Alex"
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
				"name": "Alice"
			  },
			  {
				"name": "Alice\""
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

// This test is not working currently, but start working after
// PR https://github.com/dgraph-io/dgraph/pull/4316 is merged.
// func TestHasFirstLangPredicate(t *testing.T) {
// 	query := `{
// 		q(func:has(name@lang), orderasc: name, first:5) {
// 			 name@lang
// 		 }
// 	 }`
// 	js := processQueryNoErr(t, query)
// 	require.JSONEq(t, `{
// 		{
// 			"data":{
// 				"q":[
// 					{
// 						"name@en":"Alex"
// 					},
// 					{
// 						"name@en":"Amit"
// 					},
// 					{
// 						"name@en":"Andrew"
// 					},
// 					{
// 						"name@en":"Artem Tkachenko"
// 					},
// 					{
// 						"name@en":"European badger"
// 					}
// 				]
// 			}
// 		}`, js)
// }

func TestHasCountPredicateWithLang(t *testing.T) {
	query := `{
		q(func:has(name@en), first: 11) {
			 count(uid)
		 }
	 }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
			"data":{
				"q":[
					{
						"count":11
					}
				]
			}
	}`, js)
}

func TestRegExpVariable(t *testing.T) {
	query := `
		query {
			q (func: has(name)) @filter( regexp(name, /King*/) ) {
				name
			}
		}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [{
				"name": "King Lear"
			}]
		}
	}`, js)
}

func TestRegExpVariableReplacement(t *testing.T) {
	query := `
		query all($regexp_query: string = "/King*/" ) {
			q (func: has(name)) @filter( regexp(name, $regexp_query) ) {
				name
			}
		}`

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [{
				"name": "King Lear"
			}]
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

func TestCountUIDWithOneUID(t *testing.T) {
	query := `{
		q(func: uid(1)) {
			count(uid)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": [{"count": 1}]}}`, js)
}

func TestCountUIDWithMultipleUIDs(t *testing.T) {
	query := `{
		q(func: uid(1, 2, 3)) {
			count(uid)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": [{"count": 3}]}}`, js)
}

func TestCountUIDWithPredicate(t *testing.T) {
	query := `{
		q(func: uid(1, 2, 3)) {
			name
			count(uid)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
				{
					"count": 3
				},
				{
					"name": "Michonne"
				},
				{
					"name": "King Lear"
				},
				{
					"name": "Margaret"
				}
			]
		}
	}`, js)
}

func TestCountUIDWithAlias(t *testing.T) {
	query := `{
		q(func: uid(1, 2, 3)) {
			total: count(uid)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": [{"total": 3}]}}`, js)
}

func TestCountUIDWithVar(t *testing.T) {
	query := `{
		var(func: uid(1, 2, 3)) {
			total as count(uid)
		}

		q(func: uid(total)) {
			count(uid)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": [{"count": 1}]}}`, js)
}

func TestCountUIDWithParentAlias(t *testing.T) {
	query := `{
		total1 as var(func: uid(1, 2, 3)) {
			total2 as count(uid)
		}

		q1(func: uid(total1)) {
			count(uid)
		}

		q2(func: uid(total2)) {
			count(uid)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q1": [{"count": 3}], "q2": [{"count": 1}]}}`, js)
}

func TestCountUIDWithMultipleCount(t *testing.T) {
	query := `{
		q(func: uid(1, 2, 3)) {
			count(uid)
			count(uid)
		}
	}`
	_, err := processQuery(context.Background(), t, query)
	require.Contains(t, err.Error(), "uidcount not allowed multiple times in same sub-query")
}

func TestCountUIDWithMultipleCountAndAlias(t *testing.T) {
	query := `{
		q(func: uid(1, 2, 3)) {
			total1: count(uid)
			total2: count(uid)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": [{"total1": 3},{"total2": 3}]}}`, js)
}

func TestCountUIDWithMultipleCountAndAliasAndPredicate(t *testing.T) {
	query := `{
		q(func: uid(1, 2, 3)) {
			name
			total1: count(uid)
			total2: count(uid)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
				{
					"total1": 3
				},
				{
					"total2": 3
				},
				{
					"name": "Michonne"
				},
				{
					"name": "King Lear"
				},
				{
					"name": "Margaret"
				}
			]
		}
	}`, js)
}

func TestCountUIDNested(t *testing.T) {
	query := `{
		q(func: uid(1, 2, 3)) {
			total1: count(uid)
			total2: count(uid)
			friend {
				name
				count(uid)
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
				{
					"total1": 3
				},
				{
					"total2": 3
				},
				{
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
						},
						{
							"count": 5
						}
					]
				}
			]
		}
	}`, js)
}

func TestCountUIDNestedMultiple(t *testing.T) {
	query := `{
		q(func: has(friend)) {
			count(uid)
			friend {
				name
				count(uid)
				friend {
					name
					count(uid)
				}
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
			"q": [
				{
					"count": 3
				},
				{
					"friend": [
						{
							"name": "Rick Grimes",
							"friend": [
								{
									"name": "Michonne"
								},
								{
									"count": 1
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
							"name": "Andrea",
							"friend": [
								{
									"name": "Glenn Rhee"
								},
								{
									"count": 1
								}
							]
						},
						{
							"count": 5
						}
					]
				},
				{
					"friend": [
						{
							"name": "Michonne",
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
								},
								{
									"count": 5
								}
							]
						},
						{
							"count": 1
						}
					]
				},
				{
					"friend": [
						{
							"name": "Glenn Rhee"
						},
						{
							"count": 1
						}
					]
				}
			]
		}
	}`, js)
}

func TestNumUids(t *testing.T) {
	query := `{
        me(func:has(name), first:10){
          name
         friend{
           name
          }
        }
      }`
	metrics := processQueryForMetrics(t, query)
	require.Equal(t, metrics.NumUids["friend"], uint64(10))
	require.Equal(t, metrics.NumUids["name"], uint64(16))
	require.Equal(t, metrics.NumUids["_total"], uint64(26))
}
