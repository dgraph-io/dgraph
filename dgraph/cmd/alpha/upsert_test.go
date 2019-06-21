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

package alpha

import (
	"strings"
	"sync"
	"testing"

	"github.com/dgraph-io/dgraph/z"

	"github.com/dgraph-io/dgo/y"
	"github.com/stretchr/testify/require"
)

// contains checks whether given element is contained
// in any of the elements of the given list of strings.
func contains(ps []string, p string) bool {
	var res bool
	for _, v := range ps {
		res = res || strings.Contains(v, p)
	}

	return res
}

func TestUpsertExample0(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`email: string @index(exact) .`))

	// Mutation with wrong name
	m1 := `
upsert {
  mutation {
    set {
      uid(v) <name> "Wrong" .
      uid(v) <email> "ashish@dgraph.io" .
    }
  }

  query {
    me(func: eq(email, "ashish@dgraph.io")) {
      v as uid
    }
  }
}`
	keys, preds, _, err := mutationWithTs(m1, "application/rdf", false, true, true, 0)
	require.NoError(t, err)
	require.True(t, len(keys) == 0)
	require.True(t, contains(preds, "email"))
	require.True(t, contains(preds, "name"))

	// query should return the wrong name
	q1 := `
{
  q(func: has(email)) {
    uid
    name
    email
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Wrong")

	// mutation with correct name
	m2 := `
upsert {
  mutation {
    set {
      uid(v) <name> "Ashish" .
    }
  }

  query {
    me(func: eq(email, "ashish@dgraph.io")) {
      v as uid
    }
  }
}`
	keys, preds, _, err = mutationWithTs(m2, "application/rdf", false, true, true, 0)
	require.NoError(t, err)
	require.True(t, len(keys) == 0)
	require.True(t, contains(preds, "name"))

	// query should return correct name
	res, _, err = queryWithTs(q1, "application/graphql+-", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Ashish")
}

func TestUpsertExample0JSON(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`email: string @index(exact) .`))

	// Mutation with wrong name
	m1 := `
{
  "query": "{me(func: eq(email, \"ashish@dgraph.io\")) {v as uid}}",
  "set": [
    {
      "uid": "uid(v)",
      "name": "Wrong"
    },
    {
      "uid": "uid(v)",
      "email": "ashish@dgraph.io"
    }
  ]
}`
	keys, _, _, err := mutationWithTs(m1, "application/json", false, true, true, 0)
	require.NoError(t, err)
	require.True(t, len(keys) == 0)

	// query should return the wrong name
	q1 := `
{
  q(func: has(email)) {
    uid
    name
    email
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Wrong")

	// mutation with correct name
	m2 := `
{
  "query": "{me(func: eq(email, \"ashish@dgraph.io\")) {v as uid}}",
  "set": [
    {
      "uid": "uid(v)",
      "name": "Ashish"
    }
  ]
}`
	keys, preds, _, err := mutationWithTs(m2, "application/json", false, true, true, 0)
	require.NoError(t, err)
	require.True(t, len(keys) == 0)
	require.True(t, contains(preds, "name"))

	// query should return correct name
	res, _, err = queryWithTs(q1, "application/graphql+-", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Ashish")
}

func TestUpsertNoVarErr(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
age: int @index(int) .
friend: uid @reverse .`))

	m1 := `
upsert {
  mutation {
    set {
      _:user1 <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) {
      ...fragmentA
      friend {
        ...fragmentA
        age
      }
    }
  }

  fragment fragmentA {
    uid
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, true, 0)
	require.Contains(t, err.Error(), "upsert query op has no variables")
}

func TestUpsertWithFragment(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
age: int @index(int) .
friend: uid @reverse .`))

	m1 := `
upsert {
  mutation {
    set {
      uid(variable) <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) {
      friend {
        ...fragmentA
      }
    }
  }

  fragment fragmentA {
    variable as uid
  }
}`
	keys, preds, _, err := mutationWithTs(m1, "application/rdf", false, true, true, 0)
	require.NoError(t, err)
	require.True(t, 0 == len(keys))
	require.True(t, contains(preds, "age"))

	// Ensure that another run works too
	keys, preds, _, err = mutationWithTs(m1, "application/rdf", false, true, true, 0)
	require.NoError(t, err)
	require.True(t, 0 == len(keys))
	require.True(t, contains(preds, "age"))
}

func TestUpsertInvalidErr(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
age: int @index(int) .
name: string @index(exact) .
friend: uid @reverse .`))

	m1 := `
{
  set {
    uid(variable) <age> "45" .
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, true, 0)
	require.Contains(t, err.Error(), "invalid syntax")
}

func TestUpsertUndefinedVarErr(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
age: int @index(int) .
name: string @index(exact) .
friend: uid @reverse .`))

	m1 := `
upsert {
  mutation {
    set {
      uid(42) <age> "45" .
      uid(variable) <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) {
      friend {
        ...fragmentA
      }
    }
  }

  fragment fragmentA {
    variable as uid
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, true, 0)
	require.Contains(t, err.Error(), "Some variables are used but not defined")
	require.Contains(t, err.Error(), "Defined:[variable]")
	require.Contains(t, err.Error(), "Used:[42 variable]")
}

func TestUpsertUnusedVarErr(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
age: int @index(int) .
name: string @index(exact) .
friend: uid @reverse .`))

	m1 := `
upsert {
  mutation {
    set {
      uid(var2) <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) {
      var2 as uid
      friend {
        ...fragmentA
      }
    }
  }

  fragment fragmentA {
    var1 as uid
    name
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, true, 0)
	require.Contains(t, err.Error(), "Some variables are defined but not used")
	require.Contains(t, err.Error(), "Defined:[var1 var2]")
	require.Contains(t, err.Error(), "Used:[var2]")
}

func TestUpsertExampleNode(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
age: int @index(int) .
name: string @index(exact) @lang .
friend: uid @reverse .`))

	m0 := `
{
  set {
    _:user1 <age> "23" .
    _:user1 <name@en> "user1" .
    _:user2 <age> "34" .
    _:user2 <name@en> "user2" .
    _:user3 <age> "56" .
    _:user3 <name@en> "user3" .
  }
}`
	_, _, _, err := mutationWithTs(m0, "application/rdf", false, true, true, 0)
	require.NoError(t, err)

	m1 := `
upsert {
  mutation {
    set {
      uid(	u) <oldest> "true" .
    }
  }

  query {
    var(func: has(age)) {
      a as age
    }

    oldest(func: uid(a), orderdesc: val(a), first: 1) {
      u as uid
      name
      age
    }
  }
}`
	_, _, _, err = mutationWithTs(m1, "application/rdf", false, true, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: has(oldest)) {
    name@en
    age
    oldest
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", 0)
	require.NoError(t, err)
	require.Contains(t, res, "user3")
	require.Contains(t, res, "56")
	require.Contains(t, res, "true")

	m2 := `
upsert {
  mutation {
    delete {
      uid (u1) <name> * .
    }
  }

  query {
    user1(func: eq(name@en, "user1")) {
      u1 as uid
    }
  }
}`
	_, _, _, err = mutationWithTs(m2, "application/rdf", false, true, true, 0)
	require.NoError(t, err)

	q2 := `
{
  q(func: eq(name@en, "user1")) {
    name@en
    age
  }
}`
	res, _, err = queryWithTs(q2, "application/graphql+-", 0)
	require.NoError(t, err)
	require.NotContains(t, res, "user1")
}

func TestUpsertExampleEdge(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
age: int @index(int) .
name: string @index(exact) @lang .
friend: uid @reverse .`))

	m0 := `
{
  set {
    _:user1 <age> "23" .
    _:user1 <name@en> "user1" .
    _:user2 <age> "34" .
    _:user2 <name@en> "user2" .
    _:user3 <age> "56" .
    _:user3 <name@en> "user3" .
  }
}`
	_, _, _, err := mutationWithTs(m0, "application/rdf", false, true, true, 0)
	require.NoError(t, err)

	m1 := `
upsert {
  mutation {
    set {
      uid ( u1 ) <friend> uid ( u2 ) .
    }
  }

  query {
    user1(func: eq(name@en, "user1")) {
      u1 as uid
    }

    user2(func: eq(name@en, "user2")) {
      u2 as uid
    }
  }
}`
	_, _, _, err = mutationWithTs(m1, "application/rdf", false, true, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: eq(name@en, "user1")) {
    friend {
      name@en
    }
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", 0)
	require.NoError(t, err)
	require.Contains(t, res, "user2")

	m2 := `
upsert {
  mutation {
    delete {
      uid (u1) <friend> uid ( u2 ) .
    }
  }

  query {
    user1(func: eq(name@en, "user1")) {
      u1 as uid
    }

    user2(func: eq(name@en, "user2")) {
      u2 as uid
    }
  }
}`
	_, _, _, err = mutationWithTs(m2, "application/rdf", false, true, true, 0)
	require.NoError(t, err)

	q2 := `
{
  q(func: eq(name@en, "user1")) {
    friend {
      name@en
    }
  }
}`
	res, _, err = queryWithTs(q2, "application/graphql+-", 0)
	require.NoError(t, err)
	require.NotContains(t, res, "user2")
}

func TestUpsertExampleNodeJSON(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
age: int @index(int) .
name: string @index(exact) @lang .
friend: uid @reverse .`))

	m0 := `
{
  set {
    _:user1 <age> "23" .
    _:user1 <name@en> "user1" .
    _:user2 <age> "34" .
    _:user2 <name@en> "user2" .
    _:user3 <age> "56" .
    _:user3 <name@en> "user3" .
  }
}`
	_, _, _, err := mutationWithTs(m0, "application/rdf", false, true, true, 0)
	require.NoError(t, err)

	m1 := `
{
  "set": [
    {
      "uid": "uid(u)",
      "oldest": "true"
    }
  ],

  "query": "{var(func: has(age)) {a as age} oldest(func: uid(a), orderdesc: val(a), first: 1) {u as uid}}"
}`
	_, _, _, err = mutationWithTs(m1, "application/json", false, true, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: has(oldest)) {
    name@en
    age
    oldest
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", 0)
	require.NoError(t, err)
	require.Contains(t, res, "user3")
	require.Contains(t, res, "56")
	require.Contains(t, res, "true")

	m2 := `
{
  "delete": [
    {
      "uid": "uid (u1)",
      "name": null
    }
  ],

  "query": "{user1(func: eq(name@en, \"user1\")) {u1 as uid}}"
}`
	_, _, _, err = mutationWithTs(m2, "application/json", false, true, true, 0)
	require.NoError(t, err)

	q2 := `
{
  q(func: eq(name@en, "user1")) {
    name@en
    age
  }
}`
	res, _, err = queryWithTs(q2, "application/graphql+-", 0)
	require.NoError(t, err)
	require.NotContains(t, res, "user1")
}

func TestUpsertExampleEdgeJSON(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
age: int @index(int) .
name: string @index(exact) @lang .
friend: uid @reverse .`))

	m0 := `
{
  set {
    _:user1 <age> "23" .
    _:user1 <name@en> "user1" .
    _:user2 <age> "34" .
    _:user2 <name@en> "user2" .
    _:user3 <age> "56" .
    _:user3 <name@en> "user3" .
  }
}`
	_, _, _, err := mutationWithTs(m0, "application/rdf", false, true, true, 0)
	require.NoError(t, err)

	m1 := `
{
  "set": [
      {
        "uid": "uid(u1)",
        "friend": "uid  (u2 ) "
    }
  ],

  "query": "{user1(func: eq(name@en, \"user1\")) {u1 as uid} user2(func: eq(name@en, \"user2\")) {u2 as uid}}"
}`
	_, _, _, err = mutationWithTs(m1, "application/json", false, true, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: eq(name@en, "user1")) {
    friend {
      name@en
    }
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", 0)
	require.NoError(t, err)
	require.Contains(t, res, "user2")

	m3 := `
{
  "delete": [
    {
      "uid": "uid (u1)",
      "friend": "uid ( u2 )"
    }
  ],

  "query": "{user1(func: eq(name@en, \"user1\")) {u1 as uid} user2(func: eq(name@en, \"user2\")) {u2 as uid}}"
}`
	_, _, _, err = mutationWithTs(m3, "application/json", false, true, true, 0)
	require.NoError(t, err)

	q3 := `
{
  q(func: eq(name@en, "user1")) {
    friend {
      name@en
    }
  }
}`
	res, _, err = queryWithTs(q3, "application/graphql+-", 0)
	require.NoError(t, err)
	require.NotContains(t, res, "user2")
}

func TestUpsertBlankNodeWithVar(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`name: string @index(exact) @lang .`))

	m := `
upsert {
  mutation {
    set {
      uid(u) <name> "user1" .
      _:u <name> "user2" .
    }
  }

  query {
    users(func: eq(name, "user1")) {
      u as uid
    }
  }
}`
	_, _, _, err := mutationWithTs(m, "application/rdf", false, true, true, 0)
	require.NoError(t, err)

	q := `
{
  users(func: has(name)) {
    uid
    name
  }
}`
	res, _, err := queryWithTs(q, "application/graphql+-", 0)
	require.NoError(t, err)
	require.Contains(t, res, "user1")
	require.Contains(t, res, "user2")
}

func TestUpsertParallel(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
email: string @index(exact) @upsert .
name: string @index(exact) @lang .
friend: uid @reverse .`))

	m := `
upsert {
  mutation {
    set {
      uid(u1) <email> "user1@dgraph.io" .
      uid(u1) <name> "user1" .
      uid(u2) <email> "user2@dgraph.io" .
      uid(u2) <name> "user2" .
      uid(u1) <friend> uid(u2) .
    }

    delete {
      uid(u3) <friend> uid(u1) .
      uid(u1) <friend> uid(u3) .
      uid(u3) <name> * .
    }
  }

  query {
    user1(func: eq(email, "user1@dgraph.io")) {
      u1 as uid
    }

    user2(func: eq(email, "user2@dgraph.io")) {
      u2 as uid
    }

    user3(func: eq(email, "user3@dgraph.io")) {
      u3 as uid
    }
  }
}`
	doUpsert := func(wg *sync.WaitGroup) {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			err := y.ErrAborted
			for err != nil && strings.Contains(err.Error(), "Transaction has been aborted. Please retry") {
				_, _, _, err = mutationWithTs(m, "application/rdf", false, true, true, 0)
			}

			require.NoError(t, err)
		}
	}

	// 10 routines each doing parallel upsert 10 times
	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go doUpsert(&wg)
	}
	wg.Wait()

	q := `
{
  user1(func: eq(email, "user1@dgraph.io")) {
    name
    email
    friend {
      name
      email
    }
  }
}`
	res, _, err := queryWithTs(q, "application/graphql+-", 0)
	require.NoError(t, err)
	expected := `
{
  "data": {
    "user1": [
      {
        "name": "user1",
        "email": "user1@dgraph.io",
        "friend": {
          "name": "user2",
          "email": "user2@dgraph.io"
        }
      }
    ]
  }
}`
	z.CompareJSON(t, expected, res)
}

func TestUpsertDeleteNonExistent(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
email: string @index(exact) @upsert .
name: string @index(exact) @lang .
friend: uid @reverse .`))

	m := `
upsert {
  mutation {
    delete {
      uid (u1) <friend> uid ( u2 ) .
    }
  }

  query {
    user1(func: eq(name@en, "user1")) {
      u1 as uid
    }

    user2(func: eq(name@en, "user2")) {
      u2 as uid
    }
  }
}`
	_, _, _, err := mutationWithTs(m, "application/rdf", false, true, true, 0)
	require.NoError(t, err)
}
