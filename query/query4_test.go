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
	// "encoding/json"
	"fmt"
	"math/big"
	// "strings"
	"testing"

	"github.com/stretchr/testify/require"
	// "google.golang.org/grpc/metadata"
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
		{"make":"Ford","model":"Focus","year":2008, "~previous_model": [{"uid":"0xc9"}]},
		{"make":"Ford","model":"Focus","year":2009, "previous_model": {"uid":"0xc8"}}
	]}}`, js)
}

func TestTypeExpandForward(t *testing.T) {
	query := `{
		q(func: eq(make, "Ford")) {
			expand(_forward_) {
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

func TestTypeExpandReverse(t *testing.T) {
	query := `{
		q(func: eq(make, "Ford")) {
			expand(_reverse_) {
				uid
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q":[
		{"~previous_model": [{"uid":"0xc9"}]}
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

func TestBigFloatTypeTokenizer(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "10.0000000000000000000123"  .
		<0x777> <amount> "10.0000000000000000000124"  .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: eq(amount, "10.0000000000000000000124")) {
			uid
			amount
		}
	}`
	js := processQueryNoErr(t, q1)
	require.JSONEq(t, `{"data":{"me":[{"uid":"0x777","amount":10.0000000000000000000124}]}}`,
		js)
}

func TestBigFloatCeil(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "2.1"  .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: eq(amount, "2.1")) {
			uid
			amount as amount
			amt : math(ceil(amount))
		}
	}`
	js := processQueryNoErr(t, q1)
	float2 := *new(big.Float).SetPrec(200).SetFloat64(2.1)
	ceil2 := *new(big.Float).SetPrec(200).SetFloat64(3)
	expectedRes := fmt.Sprintf(`{"data": {"me":[{"uid":"0x666", "amount":%s, "amt":%s}]}}`,
		float2.Text('f', 200), ceil2.Text('f', 200))
	require.JSONEq(t, js, expectedRes)
}

func TestBigFloatFloor(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "2.1"  .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: eq(amount, "2.1")) {
			uid
			amount as amount
			amt : math(floor(amount))
		}
	}`
	js := processQueryNoErr(t, q1)
	float2 := *new(big.Float).SetPrec(200).SetFloat64(2.1)
	floor2 := *new(big.Float).SetPrec(200).SetFloat64(2)
	expectedRes := fmt.Sprintf(`{"data": {"me":[{"uid":"0x666", "amount":%s, "amt":%s}]}}`,
		float2.Text('f', 200), floor2.Text('f', 200))
	require.JSONEq(t, js, expectedRes)
}

func TestBigFloatSqrt(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "2"  .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: eq(amount, "2")) {
			uid
			amount as amount
			amt : math(sqrt(amount))
		}
	}`
	js := processQueryNoErr(t, q1)
	float2 := *new(big.Float).SetPrec(200).SetFloat64(2)
	sqrt2 := *new(big.Float).SetPrec(200).Sqrt(&float2)
	expectedRes := fmt.Sprintf(`{"data": {"me":[{"uid":"0x666", "amount":%s, "amt":%s}]}}`,
		float2.Text('f', 200), sqrt2.Text('f', 200))
	require.JSONEq(t, js, expectedRes)
}

func TestBigFloatSort(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "100"  .
		<0x124> <amount> "99.1231231233" .
		<0x777> <amount> "99" .
		<0x888> <amount> "99.0000000000000000000001" .
		<0x123> <amount> "123123.123123123132" .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: has(amount), orderasc: amount) {
			uid
		}
	}`
	js := processQueryNoErr(t, q1)
	expectedRes := `{"data":{"me":[{"uid":"0x777"},{"uid":"0x888"}` +
		`,{"uid":"0x124"},{"uid":"0x666"},{"uid":"0x123"}]}}`
	require.JSONEq(t, js, expectedRes)
}

func TestBigFloatMax(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "100"  .
		<0x124> <amount> "99.1231231233" .
		<0x777> <amount> "99" .
		<0x888> <amount> "99.0000000000000000000001" .
		<0x123> <amount> "123123.123123123132" .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: has(amount)) {
			uid
			amount as amount
		}
		q() {
			max_amt : max(val(amount))
		}
	}`
	js := processQueryNoErr(t, q1)
	require.Contains(t, js, `"max_amt":123123.123123123132`)
}

func TestBigFloatSum(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "100"  .
		<0x124> <amount> "99.1231231233" .
		<0x777> <amount> "99" .
		<0x888> <amount> "99.0000000000000000000001" .
		<0x123> <amount> "123123.123123123132" .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: has(amount)) {
			uid
			amount as amount
		}
		q() {
			sum_amt : sum(val(amount))
		}
	}`
	js := processQueryNoErr(t, q1)
	require.Contains(t, js, `"sum_amt":123520.2462462464320000000001`)
}

func TestBigFloatAvg(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "100"  .
		<0x124> <amount> "99.1231231233" .
		<0x777> <amount> "99" .
		<0x888> <amount> "99.0000000000000000000001" .
		<0x123> <amount> "123123.123123123132" .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: has(amount)) {
			uid
			amount as amount
		}
		q() {
			avg_amt : avg(val(amount))
		}
	}`
	js := processQueryNoErr(t, q1)
	require.Contains(t, js, `"avg_amt":24704.04924924928640000000002`)
}

func TestBigFloatLt(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "100"  .
		<0x124> <amount> "99.1231231233" .
		<0x777> <amount> "99" .
		<0x888> <amount> "99.0000000000000000000001" .
		<0x123> <amount> "123123.123123123132" .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: has(amount)) @filter(lt(amount, 100)){
			uid
		}
	}`
	js := processQueryNoErr(t, q1)
	expectedRes := `{"data":{"me":[{"uid":"0x124"},{"uid":"0x777"},{"uid":"0x888"}]}}`
	require.JSONEq(t, js, expectedRes)
}

func TestBigFloatGt(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "100"  .
		<0x124> <amount> "99.1231231233" .
		<0x777> <amount> "99" .
		<0x888> <amount> "99.0000000000000000000001" .
		<0x123> <amount> "123123.123123123132" .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: has(amount)) @filter(ge(amount, 100)){
			uid
		}
	}`
	js := processQueryNoErr(t, q1)
	expectedRes := `{"data":{"me":[{"uid":"0x123"},{"uid":"0x666"}]}}`
	require.JSONEq(t, js, expectedRes)
}

func TestBigFloatConnectingFilters(t *testing.T) {
	s1 := testSchema + "\n amount: bigfloat @index(bigfloat) .\n"

	setSchema(s1)
	triples := `
		<0x666> <amount> "100"  .
		<0x124> <amount> "99.1231231233" .
		<0x777> <amount> "99" .
		<0x888> <amount> "99.0000000000000000000001" .
		<0x123> <amount> "123123.123123123132" .
	`
	addTriplesToCluster(triples)

	q1 := `
	{
		me(func: has(amount)) @filter(gt(amount, 99.1231231233) AND lt(amount, 1000)) {
			uid
		}
	}`
	js := processQueryNoErr(t, q1)
	expectedRes := `{"data":{"me":[{"uid":"0x666"}]}}`
	require.JSONEq(t, js, expectedRes)
}
