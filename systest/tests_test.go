package main

import "testing"

func TestGoldenData(t *testing.T) {
}

func TestHelloWorld(t *testing.T) {
	s := newSuite(t, `
		name: string @index(term) .
	`, `
		_:pj <name> "Peter Jackson" .
		_:pp <name> "Peter Pan" .
	`)
	defer s.cleanup()

	t.Run("Pan and Jackson", s.singleQuery(`
		q(func: anyofterms(name, "Peter")) {
			name
		}
	`, `
		"q": [
			{ "name": "Peter Pan" },
			{ "name": "Peter Jackson" }
		]
	`))

	t.Run("Pan only", s.singleQuery(`
		q(func: anyofterms(name, "Pan")) {
			name
		}
	`, `
		"q": [
			{ "name": "Peter Pan" }
		]
	`))

	t.Run("Jackson only", s.singleQuery(`
		q(func: anyofterms(name, "Jackson")) {
			name
		}
	`, `
		"q": [
			{ "name": "Peter Jackson" }
		]
	`))
}

func TestCountIndex(t *testing.T) {
	s := newSuite(t, `
		name: string @index(exact) .
		friend: uid @count @reverse .
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

	t.Run("All queries", s.multiQuery(`
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
	{"data": {
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
		"has_4_rev_friends": [
			{ "name": "Bob" }
		],
		"has_5_rev_friends": [
			{ "name": "Carol" }
		]
	}}
	`))
}
