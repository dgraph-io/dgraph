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
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/dgraph-io/dgo/y"
	"github.com/dgraph-io/dgraph/testutil"
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
  query {
    me(func: eq(email, "email@company.io")) {
      v as uid
    }
  }

  mutation {
    set {
      uid(v) <name> "Wrong" .
      uid(v) <email> "email@company.io" .
    }
  }
}`
	keys, preds, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
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
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Wrong")

	// mutation with correct name
	m2 := `
upsert {
  query {
    me(func: eq(email, "email@company.io")) {
      v as uid
    }
  }

  mutation {
    set {
      uid(v) <name> "Ashish" .
    }
  }
}`
	keys, preds, _, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)
	require.True(t, len(keys) == 0)
	require.True(t, contains(preds, "name"))

	// query should return correct name
	res, _, err = queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Ashish")
}

func TestUpsertNoCloseBracketRDF(t *testing.T) {
	SetupBankExample(t)

	m1 := `
upsert {
  query {
    u as var(func: has(amount)) {
      amt as amount
    }
    me() {
      max_amt as max(val(amt))
    }
  }

  mutation {
    set {
      uid(u) <amount> val(max_amt .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.Contains(t, err.Error(), "Expected ')' while reading function found: '.'")
}

func TestUpsertNoCloseBracketJSON(t *testing.T) {
	SetupBankExample(t)

	m1 := `
{
  "query": "{ u as var(func: has(amount)) { amt as amount} me () {  updated_amt as math(amt+1)}}",
  "set": [
    {
      "uid": "uid(u)",
      "amount": "val(updated_amt"
    }
  ]
}
`
	_, _, _, err := mutationWithTs(m1, "application/json", false, true, 0)
	require.Contains(t, err.Error(), "brackets are not closed properly")
}

func TestUpsertExampleJSON(t *testing.T) {
	SetupBankExample(t)

	m1 := `
{
  "query": "{ u as var(func: has(amount)) { amt as amount} me () {  updated_amt as math(amt+1)}}",
  "set": [
    {
      "uid": "uid(u)",
      "amount": "val(updated_amt)"
    }
  ]
}
`
	_, _, _, err := mutationWithTs(m1, "application/json", false, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: has(name)) {
    name
    amount
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	expectedRes := `
{
  "data": {
    "q": [{
       "name": "user3",
       "amount": 1001.000000
     }, {
       "name": "user1",
       "amount": 11.000000
     }, {
       "name": "user2",
       "amount": 101.000000
     }]
   }
}`
	testutil.CompareJSON(t, res, expectedRes)
}

func TestUpsertExample0JSON(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`email: string @index(exact) .`))

	// Mutation with wrong name
	m1 := `
{
  "query": "{me(func: eq(email, \"email@company.io\")) {v as uid}}",
  "set": [
    {
      "uid": "uid(v)",
      "name": "Wrong"
    },
    {
      "uid": "uid(v)",
      "email": "email@company.io"
    }
  ]
}`
	keys, _, _, err := mutationWithTs(m1, "application/json", false, true, 0)
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
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Wrong")

	// mutation with correct name
	m2 := `
{
  "query": "{me(func: eq(email, \"email@company.io\")) {v as uid}}",
  "set": [
    {
      "uid": "uid(v)",
      "name": "Ashish"
    }
  ]
}`
	keys, preds, _, err := mutationWithTs(m2, "application/json", false, true, 0)
	require.NoError(t, err)
	require.True(t, len(keys) == 0)
	require.True(t, contains(preds, "name"))

	// query should return correct name
	res, _, err = queryWithTs(q1, "application/graphql+-", "", 0)
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

  mutation {
    set {
      _:user1 <age> "45" .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.Contains(t, err.Error(), "upsert query block has no variables")
}

func TestUpsertWithFragment(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
age: int @index(int) .
friend: uid @reverse .`))

	m1 := `
upsert {
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

  mutation {
    set {
      uid(variable) <age> "45" .
    }
  }
}`
	keys, preds, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)
	require.True(t, 0 == len(keys))
	require.True(t, contains(preds, "age"))

	// Ensure that another run works too
	keys, preds, _, err = mutationWithTs(m1, "application/rdf", false, true, 0)
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
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
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

  mutation {
    set {
      uid(42) <age> "45" .
      uid(variable) <age> "45" .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
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

  mutation {
    set {
      uid(var2) <age> "45" .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
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
	_, _, _, err := mutationWithTs(m0, "application/rdf", false, true, 0)
	require.NoError(t, err)

	m1 := `
upsert {
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

  mutation {
    set {
      uid(	u) <oldest> "true" .
    }
  }
}`
	_, _, _, err = mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: has(oldest)) {
    name@en
    age
    oldest
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "user3")
	require.Contains(t, res, "56")
	require.Contains(t, res, "true")

	m2 := `
upsert {
  query {
    user1(func: eq(name@en, "user1")) {
      u1 as uid
    }
  }

  mutation {
    delete {
      uid (u1) <name> * .
    }
  }
}`
	_, _, _, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q2 := `
{
  q(func: eq(name@en, "user1")) {
    name@en
    age
  }
}`
	res, _, err = queryWithTs(q2, "application/graphql+-", "", 0)
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
	_, _, _, err := mutationWithTs(m0, "application/rdf", false, true, 0)
	require.NoError(t, err)

	m1 := `
upsert {
  query {
    user1(func: eq(name@en, "user1")) {
      u1 as uid
    }

    user2(func: eq(name@en, "user2")) {
      u2 as uid
    }
  }

  mutation {
    set {
      uid ( u1 ) <friend> uid ( u2 ) .
    }
  }
}`
	_, _, _, err = mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: eq(name@en, "user1")) {
    friend {
      name@en
    }
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "user2")

	m2 := `
upsert {
  query {
    user1(func: eq(name@en, "user1")) {
      u1 as uid
    }

    user2(func: eq(name@en, "user2")) {
      u2 as uid
    }
  }

  mutation {
    delete {
      uid (u1) <friend> uid ( u2 ) .
    }
  }
}`
	_, _, _, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q2 := `
{
  q(func: eq(name@en, "user1")) {
    friend {
      name@en
    }
  }
}`
	res, _, err = queryWithTs(q2, "application/graphql+-", "", 0)
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
	_, _, _, err := mutationWithTs(m0, "application/rdf", false, true, 0)
	require.NoError(t, err)

	m1 := `
{
  "query": "{var(func: has(age)) {a as age} oldest(func: uid(a), orderdesc: val(a), first: 1) {u as uid}}",
  "set": [
    {
      "uid": "uid(u)",
      "oldest": "true"
    }
  ]
}`
	_, _, _, err = mutationWithTs(m1, "application/json", false, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: has(oldest)) {
    name@en
    age
    oldest
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "user3")
	require.Contains(t, res, "56")
	require.Contains(t, res, "true")

	m2 := `
{
  "query": "{user1(func: eq(name@en, \"user1\")) {u1 as uid}}",
  "delete": [
    {
      "uid": "uid (u1)",
      "name": null
    }
  ]
}`
	_, _, _, err = mutationWithTs(m2, "application/json", false, true, 0)
	require.NoError(t, err)

	q2 := `
{
  q(func: eq(name@en, "user1")) {
    name@en
    age
  }
}`
	res, _, err = queryWithTs(q2, "application/graphql+-", "", 0)
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
	_, _, _, err := mutationWithTs(m0, "application/rdf", false, true, 0)
	require.NoError(t, err)

	m1 := `
{
  "query": "{user1(func: eq(name@en, \"user1\")) {u1 as uid} user2(func: eq(name@en, \"user2\")) {u2 as uid}}",
  "set": [
      {
        "uid": "uid(u1)",
        "friend": "uid  (u2 ) "
    }
  ]
}`
	_, _, _, err = mutationWithTs(m1, "application/json", false, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: eq(name@en, "user1")) {
    friend {
      name@en
    }
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "user2")

	m3 := `
{
  "query": "{user1(func: eq(name@en, \"user1\")) {u1 as uid} user2(func: eq(name@en, \"user2\")) {u2 as uid}}",
  "delete": [
    {
      "uid": "uid (u1)",
      "friend": "uid ( u2 )"
    }
  ]
}`
	_, _, _, err = mutationWithTs(m3, "application/json", false, true, 0)
	require.NoError(t, err)

	q3 := `
{
  q(func: eq(name@en, "user1")) {
    friend {
      name@en
    }
  }
}`
	res, _, err = queryWithTs(q3, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.NotContains(t, res, "user2")
}

func TestUpsertBlankNodeWithVar(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`name: string @index(exact) @lang .`))

	m := `
upsert {
  query {
    users(func: eq(name, "user1")) {
      u as uid
    }
  }

  mutation {
    set {
      uid(u) <name> "user1" .
      _:u <name> "user2" .
    }
  }
}`
	_, _, _, err := mutationWithTs(m, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q := `
{
  users(func: has(name)) {
    uid
    name
  }
}`
	res, _, err := queryWithTs(q, "application/graphql+-", "", 0)
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
}`
	doUpsert := func(wg *sync.WaitGroup) {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			err := y.ErrAborted
			for err != nil && strings.Contains(err.Error(), "Transaction has been aborted. Please retry") {
				_, _, _, err = mutationWithTs(m, "application/rdf", false, true, 0)
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
	res, _, err := queryWithTs(q, "application/graphql+-", "", 0)
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
	testutil.CompareJSON(t, expected, res)
}

func TestUpsertDeleteNonExistent(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
email: string @index(exact) @upsert .
name: string @index(exact) @lang .
friend: uid @reverse .`))

	m := `
upsert {
  query {
    user1(func: eq(name@en, "user1")) {
      u1 as uid
    }

    user2(func: eq(name@en, "user2")) {
      u2 as uid
    }
  }

  mutation {
    delete {
      uid (u1) <friend> uid ( u2 ) .
    }
  }
}`
	_, _, _, err := mutationWithTs(m, "application/rdf", false, true, 0)
	require.NoError(t, err)
}

func TestConditionalUpsertExample0(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`email: string @index(exact) .`))

	// Mutation with wrong name
	m1 := `
upsert {
  query {
    me(func: eq(email, "email@company.io")) {
      v as uid
    }
  }

  mutation @if(eq(len(v), 0)) {
    set {
      uid(v) <name> "Wrong" .
      uid(v) <email> "email@company.io" .
    }
  }
}`
	keys, preds, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)
	require.True(t, len(keys) == 0)
	require.True(t, contains(preds, "email"))
	require.True(t, contains(preds, "name"))

	// Trying again, should be a NOOP
	_, _, _, err = mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	// query should return the wrong name
	q1 := `
{
  q(func: has(email)) {
    uid
    name
    email
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Wrong")

	// mutation with correct name
	m2 := `
upsert {
  query {
    me(func: eq(email, "email@company.io")) {
      v as uid
    }
  }

  mutation @if(eq(len(v), 1)) {
    set {
      uid(v) <name> "Ashish" .
    }
  }
}`
	keys, preds, _, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)
	require.True(t, len(keys) == 0)
	require.True(t, contains(preds, "name"))

	// query should return correct name
	res, _, err = queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Ashish")
}

func TestConditionalUpsertExample0JSON(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`email: string @index(exact) .`))

	// Mutation with wrong name
	m1 := `
{
  "query": "{me(func: eq(email, \"email@company.io\")) {v as uid}}",
  "cond": " @if(eq(len(v), 0)) ",
  "set": [
    {
      "uid": "uid(v)",
      "name": "Wrong"
    },
    {
      "uid": "uid(v)",
      "email": "email@company.io"
    }
  ]
}`
	keys, _, _, err := mutationWithTs(m1, "application/json", false, true, 0)
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
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Wrong")

	// mutation with correct name
	m2 := `
{
  "query": "{me(func: eq(email, \"email@company.io\")) {v as uid}}",
  "cond": "@if(eq(len(v), 1))",
  "set": [
    {
      "uid": "uid(v)",
      "name": "Ashish"
    }
  ]
}`
	keys, preds, _, err := mutationWithTs(m2, "application/json", false, true, 0)
	require.NoError(t, err)
	require.True(t, len(keys) == 0)
	require.True(t, contains(preds, "name"))

	// query should return correct name
	res, _, err = queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "Ashish")
}

func populateCompanyData(t *testing.T) {
	require.NoError(t, alterSchema(`
email: string @index(exact) .
works_for: string @index(exact) .
works_with: [uid] .`))

	m1 := `
{
  set {
    _:user1 <name> "user1" .
    _:user1 <email> "user1@company1.io" .
    _:user1 <works_for> "company1" .

    _:user2 <name> "user2" .
    _:user2 <email> "user2@company1.io" .
    _:user2 <works_for> "company1" .

    _:user3 <name> "user3" .
    _:user3 <email> "user3@company2.io" .
    _:user3 <works_for> "company2" .

    _:user4 <name> "user4" .
    _:user4 <email> "user4@company2.io" .
    _:user4 <works_for> "company2" .
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)
}

func TestUpsertMultiValue(t *testing.T) {
	require.NoError(t, dropAll())
	populateCompanyData(t)

	// add color to all employees of company1
	m2 := `
upsert {
  query {
    me(func: eq(works_for, "company1")) {
      u as uid
    }
  }

  mutation {
    set {
      uid(u) <color> "red" .
    }
  }
}`
	keys, preds, _, err := mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)
	require.True(t, len(keys) == 0)
	require.True(t, contains(preds, "color"))
	require.False(t, contains(preds, "works_for"))

	q2 := `
{
  q(func: eq(works_for, "%s")) {
    name
    works_for
    color
    works_with
  }
}`
	res, _, err := queryWithTs(fmt.Sprintf(q2, "company1"), "application/graphql+-", "", 0)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"data":{"q":[{"name":"user1","works_for":"company1","color":"red"},`+
		`{"name":"user2","works_for":"company1","color":"red"}]}}`, res)

	// delete color for employess of company1 and set color for employees of company2
	m3 := `
upsert {
  query {
    c1 as var(func: eq(works_for, "company1"))
    c2 as var(func: eq(works_for, "company2"))
  }

  mutation @if(le(len(c1), 100) AND lt(len(c2), 100)) {
    delete {
      uid(c1) <color> * .
    }

    set {
      uid(c2) <color> "blue" .
    }
  }
}`
	_, _, _, err = mutationWithTs(m3, "application/rdf", false, true, 0)
	require.NoError(t, err)

	// The following mutation should have no effect on the state of the database
	m4 := `
upsert {
  query {
    c1 as var(func: eq(works_for, "company1"))
    c2 as var(func: eq(works_for, "company2"))
  }

  mutation @if(gt(len(c1), 2) OR ge(len(c2), 3)) {
    delete {
      uid(c1) <color> * .
    }

    set {
      uid(c2) <color> "blue" .
    }
  }
}`
	_, _, _, err = mutationWithTs(m4, "application/rdf", false, true, 0)
	require.NoError(t, err)

	res, _, err = queryWithTs(fmt.Sprintf(q2, "company1"), "application/graphql+-", "", 0)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"data":{"q":[{"name":"user1","works_for":"company1"},`+
		`{"name":"user2","works_for":"company1"}]}}`, res)

	res, _, err = queryWithTs(fmt.Sprintf(q2, "company2"), "application/graphql+-", "", 0)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"data":{"q":[{"name":"user3","works_for":"company2","color":"blue"},`+
		`{"name":"user4","works_for":"company2","color":"blue"}]}}`, res)
}

func TestUpsertMultiValueEdge(t *testing.T) {
	require.NoError(t, dropAll())
	populateCompanyData(t)

	// All employees of company1 now works with all employees of company2
	m1 := `
upsert {
  query {
    c1 as var(func: eq(works_for, "company1"))
    c2 as var(func: eq(works_for, "company2"))
  }

  mutation @if(eq(len(c1), 2) AND eq(len(c2), 2)) {
    set {
      uid(c1) <works_with> uid(c2) .
      uid(c2) <works_with> uid(c1) .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: eq(works_for, "%s")) {
    name
    works_with {
      name
    }
  }
}`
	res, _, err := queryWithTs(fmt.Sprintf(q1, "company1"), "application/graphql+-", "", 0)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"data":{"q":[{"name":"user2","works_with":[{"name":"user3"},{"name":"user4"}]},`+
		`{"name":"user1","works_with":[{"name":"user3"},{"name":"user4"}]}]}}`, res)

	res, _, err = queryWithTs(fmt.Sprintf(q1, "company2"), "application/graphql+-", "", 0)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"data":{"q":[{"name":"user3","works_with":[{"name":"user1"},{"name":"user2"}]},`+
		`{"name":"user4","works_with":[{"name":"user1"},{"name":"user2"}]}]}}`, res)

	// user1 and user3 do not work with each other anymore
	m2 := `
upsert {
  query {
    u1 as var(func: eq(email, "user1@company1.io"))
    u3 as var(func: eq(email, "user3@company2.io"))
  }

  mutation @if(eq(len(u1), 1) AND eq(len(u3), 1)) {
    delete {
      uid (u1) <works_with> uid (u3) .
      uid (u3) <works_with> uid (u1) .
    }
  }
}`
	_, _, _, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)

	res, _, err = queryWithTs(fmt.Sprintf(q1, "company1"), "application/graphql+-", "", 0)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"data":{"q":[{"name":"user1","works_with":[{"name":"user4"}]},`+
		`{"name":"user2","works_with":[{"name":"user4"},{"name":"user3"}]}]}}`, res)

	res, _, err = queryWithTs(fmt.Sprintf(q1, "company2"), "application/graphql+-", "", 0)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"data":{"q":[{"name":"user3","works_with":[{"name":"user2"}]},`+
		`{"name":"user4","works_with":[{"name":"user1"},{"name":"user2"}]}]}}`, res)
}

func TestUpsertEdgeWithBlankNode(t *testing.T) {
	require.NoError(t, dropAll())
	populateCompanyData(t)

	// Add a new employee who works with every employee in company2
	m1 := `
upsert {
  query {
    c1 as var(func: eq(works_for, "company1"))
    c2 as var(func: eq(works_for, "company2"))
  }

  mutation @if(lt(len(c1), 3)) {
    set {
      _:user5 <name> "user5" .
      _:user5 <email> "user5@company1.io" .
      _:user5 <works_for> "company1" .
      _:user5 <works_with> uid(c2) .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: eq(email, "user5@company1.io")) {
    name
    email
    works_for
    works_with {
      name
    }
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"data":{"q":[{"name":"user5","email":"user5@company1.io",`+
		`"works_for":"company1","works_with":[{"name":"user3"},{"name":"user4"}]}]}}`, res)
}

func TestConditionalUpsertWithFilterErr(t *testing.T) {
	require.NoError(t, dropAll())
	populateCompanyData(t)

	m1 := `
upsert {
  query {
    me(func: eq(email, "email@company.io")) {
      v as uid
    }
  }

  mutation @filter(eq(len(v), 0)) {
    set {
      uid(v) <name> "Wrong" .
      uid(v) <email> "email@company.io" .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.Contains(t, err.Error(), "Expected @if, found [@filter]")
}

func TestConditionalUpsertMissingAtErr(t *testing.T) {
	require.NoError(t, dropAll())
	populateCompanyData(t)

	m1 := `
upsert {
  query {
    me(func: eq(email, "email@company.io")) {
      v as uid
    }
  }

  mutation if(eq(len(v), 0)) {
    set {
      uid(v) <name> "Wrong" .
      uid(v) <email> "email@company.io" .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.Contains(t, err.Error(), `Unrecognized character inside mutation: U+0028 '('`)
}

func TestConditionalUpsertDoubleIfErr(t *testing.T) {
	require.NoError(t, dropAll())
	populateCompanyData(t)

	m1 := `
upsert {
  query {
    me(func: eq(email, "email@company.io")) {
      v as uid
    }
  }

  mutation @if(eq(len(v), 0)) @if(eq(len(v), 0)) {
    set {
      uid(v) <name> "Wrong" .
      uid(v) <email> "email@company.io" .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.Contains(t, err.Error(), "Expected { at the start of block")
}

func TestConditionalUpsertMissingRightRoundErr(t *testing.T) {
	require.NoError(t, dropAll())
	populateCompanyData(t)

	m1 := `
upsert {
  query {
    me(func: eq(email, "email@company.io")) {
      v as uid
    }
  }

  mutation @if(eq(len(v), 0) {
    set {
      uid(v) <name> "Wrong" .
      uid(v) <email> "email@company.io" .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.Contains(t, err.Error(), "Matching brackets not found")
}

func TestUpsertDeleteOnlyYourPost(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
name: string @index(exact) .
content: string @index(exact) .`))

	m1 := `
{
  set {
    _:user1 <name> "user1" .
    _:user2 <name> "user2" .
    _:user3 <name> "user3" .
    _:user4 <name> "user4" .

    _:post1 <content> "post1" .
    _:post1 <author> _:user1 .

    _:post2 <content> "post2" .
    _:post2 <author> _:user1 .

    _:post3 <content> "post3" .
    _:post3 <author> _:user2 .

    _:post4 <content> "post4" .
    _:post4 <author> _:user3 .

    _:post5 <content> "post5" .
    _:post5 <author> _:user3 .

    _:post6 <content> "post6" .
    _:post6 <author> _:user3 .
  }
}`

	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	// user2 trying to delete the post4
	m2 := `
upsert {
  query {
    var(func: eq(content, "post4")) {
      p4 as uid
      author {
        n3 as name
      }
    }

    u2 as var(func: eq(val(n3), "user2"))
  }

  mutation @if(eq(len(u2), 1)) {
    delete {
      uid(p4) <content> * .
      uid(p4) <author> * .
    }
  }
}`
	_, _, _, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)

	// post4 must still exist
	q2 := `
{
  post(func: eq(content, "post4")) {
    content
  }
}`
	res, _, err := queryWithTs(q2, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.Contains(t, res, "post4")

	// user3 deleting the post4
	m3 := `
upsert {
  query {
    var(func: eq(content, "post4")) {
      p4 as uid
      author {
        n3 as name
      }
    }

    u4 as var(func: eq(val(n3), "user3"))
  }

  mutation @if(eq(len(u4), 1)) {
    delete {
      uid(p4) <content> * .
      uid(p4) <author> * .
    }
  }
}`
	_, _, _, err = mutationWithTs(m3, "application/rdf", false, true, 0)
	require.NoError(t, err)

	// post4 shouldn't exist anymore
	res, _, err = queryWithTs(q2, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.NotContains(t, res, "post4")
}

func TestUpsertMultiTypeUpdate(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
name: string @index(exact) .
branch: string .
age: int .
active: bool .
openDate: dateTime .
password: password .
loc: geo .
amount: float .`))

	m1 := `
{
  set {
    _:user1 <name> "user1" .
    _:user1 <branch> "Fuller Street, San Francisco" .
    _:user1 <amount> "10" .
    _:user1 <age> "30" .
    _:user1 <active> "1" .
    _:user1 <openDate> "1980-01-01" .
    _:user1 <password> "password" .
    _:user1 <loc> "{'type':'Point','coordinates':[-122.4220186,37.772318]}"^^<geo:geojson> .

    _:user2 <name> "user2" .
    _:user2 <branch> "Fuller Street, San Francisco" .
    _:user2 <amount> "10" .
    _:user2 <age> "30" .
    _:user2 <active> "1" .
    _:user2 <openDate> "1980-01-01" .
    _:user2 <password> "password" .
    _:user2 <loc> "{'type':'Point','coordinates':[-122.4220186,37.772318]}"^^<geo:geojson> .

    _:user3 <name> "user3" .
    _:user3 <branch> "Fuller Street, San Francisco" .
    _:user3 <amount> "10" .
    _:user3 <age> "30" .
    _:user3 <active> "1" .
    _:user3 <password> "password" .
    _:user3 <loc> "{'type':'Point','coordinates':[-122.4220186,37.772318]}"^^<geo:geojson> .
  }
}`

	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: has(branch)) {
    name
    branch
    amount
    age
    active
    openDate
    password
    loc
  }
}`
	expectedRes, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)

	m2 := `
upsert {
  query {
    u as var(func: has(amount)) {
      amt as amount
      n as name
      b as branch
      a as age
      ac as active
      open as openDate
      pass as password
      l as loc
    }
  }

  mutation {
    set {
      uid(u ) <amount> val(amt) .
      uid(u) <name> val (n) .
      uid(u) <branch> val( b) .
      uid(u) <age> val(a) .
      uid(u) <active> val(ac) .
      uid(u) <openDate> val(open) .
      uid(u) <password> val(pass) .
      uid(u) <loc> val(l) .
    }
  }
}
  `

	// This test is to ensure that all the types are being
	// parsed correctly by the val function.
	// User3 doesn't have all the fields. This test also ensures
	// that val works when not all records have the values
	_, _, _, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)

	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	testutil.CompareJSON(t, res, expectedRes)
}

func TestValInSubject(t *testing.T) {
	m3 := `
upsert {
  query {
    u as var(func: has(amount)) {
      amt as amount
    }
  }
  mutation {
    set {
      val(amt) <amount> 1 .
    }
  }
}
`

	_, _, _, err := mutationWithTs(m3, "application/rdf", false, true, 0)
	require.Contains(t, err.Error(), "while lexing val(amt) <amount> 1")
}

func TestUpperCaseFunctionErrorMsg(t *testing.T) {
	m1 := `
upsert {
  query {
    u as var(func: has(amount)) {
      amt as amount
    }
  }
  mutation {
    set {
      uid(u) <amount> VAL(amt) .
    }
  }
}
`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.Contains(t, err.Error(), "Invalid input: V at lexText")

	m2 := `
upsert {
  query {
    u as var(func: has(amount)) {
      amt as amount
    }
  }
  mutation {
    set {
      UID(u) <amount> val(amt) .
    }
  }
}
`
	_, _, _, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.Contains(t, err.Error(), "Invalid input: U at lexText")
}

func TestBigFloat(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
name: string @index(exact) .
branch: string .
amount: bigfloat .`))

	m1 := `
{
  set {
    _:user1 <name> "user1" .
    _:user1 <amount> "10.00000000000000000001" .
  }
}`

	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q1 := `
{
  q(func: has(name)) {
    name
    amount
  }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)

	expectedRes := `{"data":{"q":[{"name":"user1","amount":10.00000000000000000001}]}}`
	require.EqualValues(t, expectedRes, res)
}

func SetupBankExample(t *testing.T) string {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
name: string @index(exact) .
branch: string .
amount: float .`))

	m1 := `
{
  set {
    _:user1 <name> "user1" .
    _:user1 <amount> "10" .

    _:user2 <name> "user2" .
    _:user2 <amount> "100" .

    _:user3 <name> "user3" .
    _:user3 <amount> "1000" .
  }
}`

	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)
	q1 := `
{
  q(func: has(name)) {
    name
    amount
  }
}`
	expectedRes, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)

	return expectedRes
}

func TestUpsertSanityCheck(t *testing.T) {
	expectedRes := SetupBankExample(t)

	// Checking for error when some wrong field is being used
	m1 := `
upsert {
  query {
    u as var(func: has(amount)) {
      amt as nofield
    }
  }

  mutation {
    set {
      uid(u) <amount> val(amt) .
    }
  }
}`

	q1 := `
{
  q(func: has(name)) {
    name
    amount
  }
}`

	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	testutil.CompareJSON(t, res, expectedRes)
}

func TestUpsertDeleteWrongValue(t *testing.T) {
	expectedRes := SetupBankExample(t)

	// Checking that delete and val should only
	// delete if the value of variable matches
	m1 := `
upsert {
  query {
    u as var(func: has(amount)) {
      amt as amount
    }
    me() {
      updated_amt as  math(amt+1)
    }
  }
  mutation {
    delete {
      uid(u) <amount> val(updated_amt) .
    }
  }
}`

	q1 := `
{
  q(func: has(name)) {
    name
    amount
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	// There should be no change
	testutil.CompareJSON(t, res, expectedRes)
}

func TestUpsertDeleteRightValue(t *testing.T) {
	SetupBankExample(t)
	// Checking Bulk Delete in Val
	m1 := `
upsert {
  query {
    u as var(func: has(amount)) {
      amt as amount
    }
  }

  mutation {
    delete {
      uid(u) <amount> val(amt) .
    }
  }
}
`
	q1 := `
{
  q(func: has(name)) {
    name
    amount
  }
}`

	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.NotContains(t, res, "amount")
}

func TestUpsertBuldUpdateValue(t *testing.T) {
	SetupBankExample(t)

	// Resetting the val in upsert to check if the
	// values are not switched and the interest is added
	m1 := `
upsert {
  query {
    u as var(func: has(amount)) {
      amt as amount
    }
    me () {
      updated_amt as math(amt+1)
    }
  }

  mutation {
    set {
      uid(u) <amount> val(updated_amt) .
    }
  }
}
  `

	q1 := `
{
  q(func: has(name)) {
    name
    amount
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	expectedRes := `
{
  "data": {
    "q": [{
       "name": "user3",
       "amount": 1001.000000
     }, {
       "name": "user1",
       "amount": 11.000000
     }, {
       "name": "user2",
       "amount": 101.000000
     }]
   }
}`
	require.NoError(t, err)
	testutil.CompareJSON(t, res, expectedRes)

}

func TestAggregateValBuldUpdate(t *testing.T) {
	SetupBankExample(t)
	q1 := `
{
  q(func: has(name)) {
    name
    amount
  }
}`

	// Checking support for bulk update values
	// to aggregate variable in upsert
	m1 := `
upsert {
  query {
    u as var(func: has(amount)) {
      amt as amount
    }
    me() {
      max_amt as max(val(amt))
    }
  }

  mutation {
    set {
      uid(u) <amount> val(max_amt) .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	expectedRes := `
{
  "data": {
    "q": [{
       "name": "user3",
       "amount": 1000.000000
     }, {
       "name": "user1",
       "amount": 1000.000000
     }, {
       "name": "user2",
       "amount": 1000.000000
     }]
   }
}`
	require.NoError(t, err)
	testutil.CompareJSON(t, res, expectedRes)
}

func TestUpsertEmptyUID(t *testing.T) {
	SetupBankExample(t)
	m1 := `
upsert {
  query {
    var(func: has(amount)) {
      amt as amount
    }
    me() {
      max_amt as max(val(amt))
    }
    v as var(func: eq(name, "Michael")) {
      amount
    }
  }

  mutation {
    set {
      uid(v) <amount> val(max_amt) .
    }
  }
}`
	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)
}

func TestUpsertBulkUpdateBranch(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
name: string @index(exact) .
branch: string .
amount: float .`))

	m1 := `
{
  set {
    _:user1 <name> "user1" .
    _:user1 <branch> "Fuller Street, San Francisco" .
    _:user1 <amount> "10" .

    _:user2 <name> "user2" .
    _:user2 <branch> "Fuller Street, San Francisco" .
    _:user2 <amount> "100" .

    _:user3 <name> "user3" .
    _:user3 <branch> "Fuller Street, San Francisco" .
    _:user3 <amount> "1000" .
  }
}`

	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	// Bulk Update: update everyone's branch
	m2 := `
upsert {
  query {
    u as var(func: has(branch))
  }

  mutation {
    set {
      uid(u) <branch> "Fuller Street, SF" .
    }
  }
}`
	_, _, _, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q2 := `
{
  q(func: has(branch)) {
    name
    branch
    amount
  }
}`
	res, _, err := queryWithTs(q2, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.NotContains(t, res, "San Francisco")
	require.Contains(t, res, "user1")
	require.Contains(t, res, "user2")
	require.Contains(t, res, "user3")
	require.Contains(t, res, "Fuller Street, SF")
	// Bulk Delete: delete everyone's branch
	m3 := `
upsert {
  query {
    u as var(func: has(branch))
  }

  mutation {
    delete {
      uid(u) <branch> * .
    }
  }
}`
	_, _, _, err = mutationWithTs(m3, "application/rdf", false, true, 0)
	require.NoError(t, err)

	res, _, err = queryWithTs(q2, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.NotContains(t, res, "San Francisco")
	require.NotContains(t, res, "Fuller Street, SF")
}

func TestDeleteCountIndex(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`
<game_answer>: uid @count @reverse .
<name>: int @index(int) .`))

	m1 := `
{
set {
  _:1 <game_answer> _:2 .
  _:1 <name> "1" .
  _:2 <game_answer> _:3 .
  _:2 <name> "2" .
  _:4 <game_answer> _:2 .
  _:3 <name> "3" .
  _:4 <name> "4" .
}
}`

	_, _, _, err := mutationWithTs(m1, "application/rdf", false, true, 0)
	require.NoError(t, err)

	m2 := `
upsert {
  query {
    u3 as var(func: eq(name, "3"))
    u2 as var(func: eq(name, "2"))
  }
  mutation {
    delete {
        uid(u2) <game_answer> uid(u3) .
    }
  }
}`
	_, _, _, err = mutationWithTs(m2, "application/rdf", false, true, 0)
	require.NoError(t, err)

	q1 := `
{
    me(func: eq(count(~game_answer), 1)) {
      name
      count(~game_answer)
    }
}`
	res, _, err := queryWithTs(q1, "application/graphql+-", "", 0)
	require.NoError(t, err)
	require.NotContains(t, res, "count(~game_answer)")
}
