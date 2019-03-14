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

package main

import (
	"os"
	"testing"
	"time"
)

// TODO: This test was used just to make sure some really basic examples work.
// It can be deleted once the remainder of the tests have been setup.
func TestHelloWorld(t *testing.T) {
	s := newSuite(t, `
		name: string @index(term) .
	`, `
		_:pj <name> "Peter Jackson" .
		_:pp <name> "Peter Pan" .
	`)
	defer s.cleanup()

	t.Run("Pan and Jackson", s.testCase(`
		{q(func: anyofterms(name, "Peter")) {
			name
		}}
	`, `
		{"q": [
			{ "name": "Peter Pan" },
			{ "name": "Peter Jackson" }
		]}
	`))

	t.Run("Pan only", s.testCase(`
		{q(func: anyofterms(name, "Pan")) {
			name
		}}
	`, `
		{"q": [
			{ "name": "Peter Pan" }
		]}
	`))

	t.Run("Jackson only", s.testCase(`
		{q(func: anyofterms(name, "Jackson")) {
			name
		}}
	`, `
		{"q": [
			{ "name": "Peter Jackson" }
		]}
	`))
}

func TestFacets(t *testing.T) {
	s := newSuite(t, `
		name: string @index(exact) .
		boss: uid @reverse .
	`, `
		_:alice <name> "Alice" (middle_initial="J") .
		_:bob   <name> "Bob"   (middle_initial="M") .
		_:bob   <boss> _:alice (since=2017-04-26)   .
	`)
	defer s.cleanup()

	t.Run("facet on terminal edge", s.testCase(`
		{q(func: eq(name, "Alice")) {
			name @facets(middle_initial)
		}}
	`, `
		{"q": [ {
			"name": "Alice",
			"name|middle_initial": "J"
		} ]}
	`))

	t.Run("facets on fwd uid edge", s.testCase(`
		{q(func: eq(name, "Bob")) {
			boss @facets(since)
		}}
	`, `
		{"q": [ {
			"boss": {
				"boss|since": "2017-04-26T00:00:00Z"
			}
		} ]}
	`))

	t.Run("facets on rev uid edge", s.testCase(`
		{q(func: eq(name, "Alice")) {
			~boss @facets(since)
		}}
	`, `
		{"q": [ {
			"~boss": [ {
				"~boss|since": "2017-04-26T00:00:00Z"
			} ]
		} ]}
	`))
}

func TestCountIndex(t *testing.T) {
	s := newSuite(t, `
		name: string @index(exact) .
		friend: [uid] @count @reverse .
	`, `
		_:alice <friend> _:bob   .
		_:alice <friend> _:carol .
		_:alice <friend> _:dave  .

		_:bob   <friend> _:carol .

		_:carol <friend> _:bob   .
		_:carol <friend> _:dave  .

		_:erin  <friend> _:bob   .
		_:erin  <friend> _:carol .

		_:frank <friend> _:carol .
		_:frank <friend> _:dave  .
		_:frank <friend> _:erin  .

		_:grace <friend> _:alice .
		_:grace <friend> _:bob   .
		_:grace <friend> _:carol .
		_:grace <friend> _:dave  .
		_:grace <friend> _:erin  .
		_:grace <friend> _:frank .

		_:alice <name> "Alice" .
		_:bob   <name> "Bob" .
		_:carol <name> "Carol" .
		_:erin  <name> "Erin" .
		_:frank <name> "Frank" .
		_:grace <name> "Grace" .
	`)
	defer s.cleanup()

	// Ensures that the index keys are written to disk after commit.
	time.Sleep(time.Second)
	t.Run("All queries", s.testCase(`
	{
		alice_friend_count(func: eq(name, "Alice")) {
			count(friend),
		},
		bob_friend_count(func: eq(name, "Bob")) {
			count(friend),
		},
		carol_friend_count(func: eq(name, "Carol")) {
			count(friend),
		},
		erin_friend_count(func: eq(name, "Erin")) {
			count(friend),
		},
		frank_friend_count(func: eq(name, "Frank")) {
			count(friend),
		},
		grace_friend_count(func: eq(name, "Grace")) {
			count(friend),
		},

		has_1_friend(func: has(friend)) @filter(eq(count(friend), 1)) {
			name
		},
		has_2_friends(func: has(friend)) @filter(eq(count(friend), 2)) {
			name
		},
		has_3_friends(func: has(friend)) @filter(eq(count(friend), 3)) {
			name
		},
		has_4_friends(func: has(friend)) @filter(eq(count(friend), 4)) {
			name
		},
		has_5_friends(func: has(friend)) @filter(eq(count(friend), 5)) {
			name
		},
		has_6_friends(func: has(friend)) @filter(eq(count(friend), 6)) {
			name
		},

		has_1_rev_friend(func: has(friend)) @filter(eq(count(~friend), 1)) {
			name
		},
		has_2_rev_friends(func: has(friend)) @filter(eq(count(~friend), 2)) {
			name
		},
		has_3_rev_friends(func: has(friend)) @filter(eq(count(~friend), 3)) {
			name
		},
		has_4_rev_friends(func: has(friend)) @filter(eq(count(~friend), 4)) {
			name
		},
		has_5_rev_friends(func: has(friend)) @filter(eq(count(~friend), 5)) {
			name
		},
		has_6_rev_friends(func: has(friend)) @filter(eq(count(~friend), 6)) {
			name
		},
	}
	`, `
	{
		"alice_friend_count": [
			{ "count(friend)": 3 }
		],
		"bob_friend_count": [
			{ "count(friend)": 1 }
		],
		"carol_friend_count": [
			{ "count(friend)": 2 }
		],
		"erin_friend_count": [
			{ "count(friend)": 2 }
		],
		"frank_friend_count": [
			{ "count(friend)": 3 }
		],
		"grace_friend_count": [
			{ "count(friend)": 6 }
		],

		"has_1_friend": [
			{ "name": "Bob" }
		],
		"has_2_friends": [
			{ "name": "Carol" },
			{ "name": "Erin" }
		],
		"has_3_friends": [
			{ "name": "Alice" },
			{ "name": "Frank" }
		],
		"has_4_friends": [],
		"has_5_friends": [],
		"has_6_friends": [
			{ "name": "Grace" }
		],

		"has_1_rev_friend": [
			{ "name": "Alice" },
			{ "name": "Frank" }
		],
		"has_2_rev_friends": [
			{ "name": "Erin" }
		],
		"has_3_rev_friends": [
		],
		"has_4_rev_friends": [
			{ "name": "Bob" }
		],
		"has_5_rev_friends": [
			{ "name": "Carol" }
		],
		"has_6_rev_friends": []
	}
	`))
}

// TODO: Fix this later.
func DONOTRUNTestGoldenData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	s := newSuiteFromFile(t,
		os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/goldendata.schema"),
		os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/goldendata.rdf.gz"),
	)
	defer s.cleanup()

	err := matchExportCount(matchExport{
		expectedRDF:    1120879,
		expectedSchema: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("basic", s.testCase(`
		{pj_films(func:allofterms(name@en,"Peter Jackson")) {
			director.film (orderasc: name@en, first: 10) {
				name@en
			}
		}}
	`, `
		{"pj_films": [ { "director.film": [
			{ "name@en": "Bad Taste" },
			{ "name@en": "Heavenly Creatures" },
			{ "name@en": "Forgotten Silver" },
			{ "name@en": "Dead Alive" },
			{ "name@en": "The Adventures of Tintin: Prisoners of the Sun" },
			{ "name@en": "Crossing the Line" },
			{ "name@en": "Meet the Feebles" },
			{ "name@en": "King Kong" },
			{ "name@en": "The Frighteners" },
			{ "name@en": "Gollum's Acceptance Speech" }
		] } ]}
	`))

	// TODO: Add the test cases from contrib/goldendata-queries.sh The tests
	// there use grep to find the number of leaf nodes in the queries, so the
	// queries will have to be modified.

	// TODO: Add tests similar to those in
	// https://docs.dgraph.io/query-language/. These test most of the main
	// functionality of dgraph.
}
