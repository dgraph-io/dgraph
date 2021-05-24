/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func TestReindexTerm(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`name: string .`))

	m1 := `{
    set {
      _:u1 <name> "Ab Bc" .
      _:u2 <name> "Bc Cd" .
      _:u3 <name> "Cd Da" .
    }
  }`
	_, err := mutationWithTs(mutationInp{body: m1, typ: "application/rdf", commitNow: true})
	require.NoError(t, err)

	// perform re-indexing
	err = x.RetryUntilSuccess(3, time.Second, func() error {
		return alterSchema(`name: string @index(term) .`)
	})
	require.NoError(t, err)

	q1 := `{
      q(func: anyofterms(name, "bc")) {
        name
      }
    }`
	res, _, err := queryWithTs(queryInp{body: q1, typ: "application/dql"})
	require.NoError(t, err)
	require.Contains(t, res, `{"name":"Ab Bc"}`)
	require.Contains(t, res, `{"name":"Bc Cd"}`)
}

func TestReindexLang(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`name: string @lang .`))

	m1 := `{
    set {
      <10111> <name>	"Runtime"@en	.
      <10032> <name>	"Runtime"@en	.
      <10240> <name>	"Хавьер Перес Гробет"@ru	.
      <10231> <name>	"結婚って、幸せですか THE MOVIE"@ja	.
    }
  }`
	_, err := mutationWithTs(mutationInp{body: m1, typ: "application/rdf", commitNow: true})
	require.NoError(t, err)

	// perform re-indexing
	err = x.RetryUntilSuccess(3, time.Second, func() error {
		return alterSchema(`name: string @lang @index(exact) .`)
	})
	require.NoError(t, err)

	q1 := `{
    q(func: eq(name@en, "Runtime")) {
      uid
      name@en
    }
  }`
	res, _, err := queryWithTs(queryInp{body: q1, typ: "application/dql"})
	require.NoError(t, err)
	require.JSONEq(t, `{
    "data": {
      "q": [
        {
          "uid": "0x2730",
          "name@en": "Runtime"
        },
        {
          "uid": "0x277f",
          "name@en": "Runtime"
        }
      ]
    }
  }`, res)

	// adding another triplet
	m2 := `{ set { <10400> <name>	"Runtime"@en	. }}`
	_, err = mutationWithTs(mutationInp{body: m2, typ: "application/rdf", commitNow: true})
	require.NoError(t, err)

	res, _, err = queryWithTs(queryInp{body: q1, typ: "application/dql"})
	require.NoError(t, err)
	require.JSONEq(t, `{
    "data": {
      "q": [
        {
          "uid": "0x2730",
          "name@en": "Runtime"
        },
        {
          "uid": "0x277f",
          "name@en": "Runtime"
        },
        {
          "uid": "0x28a0",
          "name@en": "Runtime"
        }
      ]
    }
  }`, res)
}

func TestReindexReverseCount(t *testing.T) {
	require.NoError(t, dropAll())
	require.NoError(t, alterSchema(`value: [uid] .`))

	m1 := `{
    set {
      <1> <value>	<4>	.
      <1> <value>	<5>	.
      <1> <value>	<6>	.
      <1> <value>	<7>	.
      <1> <value>	<8>	.
      <2> <value>	<4>	.
      <2> <value>	<5>	.
      <2> <value>	<6>	.
      <3> <value>	<5>	.
      <3> <value>	<6>	.
    }
  }`
	_, err := mutationWithTs(mutationInp{body: m1, typ: "application/rdf", commitNow: true})
	require.NoError(t, err)

	// perform re-indexing
	err = x.RetryUntilSuccess(3, time.Second, func() error {
		return alterSchema(`value: [uid] @count @reverse .`)
	})
	require.NoError(t, err)

	q1 := `{
    q(func: eq(count(~value), "3")) {
      uid
    }
  }`
	res, _, err := queryWithTs(queryInp{body: q1, typ: "application/dql"})
	require.NoError(t, err)
	require.JSONEq(t, `{
    "data": {
      "q": [
        {
          "uid": "0x5"
        },
        {
          "uid": "0x6"
        }
      ]
    }
  }`, res)

	// adding another triplet
	m2 := `{ set { <9> <value>	<4>	. }}`
	_, err = mutationWithTs(mutationInp{body: m2, typ: "application/rdf", commitNow: true})
	require.NoError(t, err)

	res, _, err = queryWithTs(queryInp{body: q1, typ: "application/dql"})
	require.NoError(t, err)
	require.JSONEq(t, `{
    "data": {
      "q": [
        {
          "uid": "0x4"
        },
        {
          "uid": "0x5"
        },
        {
          "uid": "0x6"
        }
      ]
    }
  }`, res)
}

func checkSchema(t *testing.T, query, key string) {
	N := 10
	for i := 0; i < N; i++ {
		res, _, err := queryWithTs(queryInp{body: query, typ: "application/dql"})
		require.NoError(t, err)
		if strings.Contains(res, key) {
			return
		}
		time.Sleep(time.Second)

		if i == N-1 {
			t.Fatalf("expected %v, got schema: %v", key, res)
		}
	}
}

func TestBgIndexSchemaReverse(t *testing.T) {
	require.NoError(t, dropAll())
	q1 := `schema(pred: [value]) {}`
	require.NoError(t, alterSchemaInBackground(`value: [uid] .`))
	checkSchema(t, q1, "list")
	require.NoError(t, alterSchemaInBackground(`value: [uid] @count @reverse .`))
	checkSchema(t, q1, "reverse")
}

func TestBgIndexSchemaTokenizers(t *testing.T) {
	require.NoError(t, dropAll())
	q1 := `schema(pred: [value]) {}`
	require.NoError(t, alterSchemaInBackground(`value: string @index(fulltext, hash) .`))
	checkSchema(t, q1, "fulltext")
	require.NoError(t, alterSchemaInBackground(`value: string @index(term, hash) @upsert .`))
	checkSchema(t, q1, "term")
}

func TestBgIndexSchemaCount(t *testing.T) {
	require.NoError(t, dropAll())
	q1 := `schema(pred: [value]) {}`
	require.NoError(t, alterSchemaInBackground(`value: [uid] @count .`))
	checkSchema(t, q1, "count")
	require.NoError(t, alterSchemaInBackground(`value: [uid] @reverse .`))
	checkSchema(t, q1, "reverse")
}

func TestBgIndexSchemaReverseAndCount(t *testing.T) {
	require.NoError(t, dropAll())
	q1 := `schema(pred: [value]) {}`
	require.NoError(t, alterSchemaInBackground(`value: [uid] @reverse .`))
	checkSchema(t, q1, "reverse")
	require.NoError(t, alterSchemaInBackground(`value: [uid] @count .`))
	checkSchema(t, q1, "count")
}
