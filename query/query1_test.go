/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

func TestSchemaBlock2(t *testing.T) {
	query := `
		schema(pred: name) {
			index
			reverse
			type
			tokenizer
			count
		}
	`
	actual := processSchemaQuery(t, query)
	expected := []*api.SchemaNode{
		{Predicate: "name",
			Type:      "string",
			Index:     true,
			Tokenizer: []string{"term", "exact", "trigram"},
			Count:     true}}
	checkSchemaNodes(t, expected, actual)
}

func TestSchemaBlock3(t *testing.T) {
	query := `
		schema(pred: age) {
			index
			reverse
			type
			tokenizer
			count
		}
	`
	actual := processSchemaQuery(t, query)
	expected := []*api.SchemaNode{{Predicate: "age",
		Type:      "int",
		Index:     true,
		Tokenizer: []string{"int"},
		Count:     false}}
	checkSchemaNodes(t, expected, actual)
}

func TestSchemaBlock4(t *testing.T) {
	query := `
		schema(pred: [age, genre, random]) {
			index
			reverse
			type
			tokenizer
		}
	`
	actual := processSchemaQuery(t, query)
	expected := []*api.SchemaNode{
		{Predicate: "genre",
			Type:    "uid",
			Reverse: true}, {Predicate: "age",
			Type:      "int",
			Index:     true,
			Tokenizer: []string{"int"}}}
	checkSchemaNodes(t, expected, actual)
}

func TestSchemaBlock5(t *testing.T) {
	query := `
		schema(pred: name) {
		}
	`
	actual := processSchemaQuery(t, query)
	expected := []*api.SchemaNode{
		{Predicate: "name",
			Type:      "string",
			Index:     true,
			Tokenizer: []string{"term", "exact", "trigram"},
			Count:     true,
			Lang:      true,
		}}
	checkSchemaNodes(t, expected, actual)
}

// Duplicate implemention as in cmd/dgraph/main_test.go
// TODO: Change the implementation in cmd/dgraph to test for network failure
type raftServer struct {
}

func (c *raftServer) Echo(ctx context.Context, in *api.Payload) (*api.Payload, error) {
	return in, nil
}

func (c *raftServer) RaftMessage(ctx context.Context, in *api.Payload) (*api.Payload, error) {
	return &api.Payload{}, nil
}

func (c *raftServer) JoinCluster(ctx context.Context, in *pb.RaftContext) (*api.Payload, error) {
	return &api.Payload{}, nil
}

func updateMaxPending() {
	for mp := range maxPendingCh {
		posting.Oracle().ProcessDelta(&pb.OracleDelta{
			MaxAssigned: mp,
		})
	}

}

func TestFilterNonIndexedPredicateFail(t *testing.T) {

	// filtering on non indexing predicate fails
	query := `
		{
			me(func: uid(0x01)) {
				friend @filter(le(survival_rate, 30)) {
					uid
					name
					age
				}
			}
		}
	`
	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestMultipleSamePredicateInBlockFail(t *testing.T) {

	// name is asked for two times..
	query := `
		{
			me(func: uid(0x01)) {
				name
				friend {
					age
				}
				name
			}
		}
	`
	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestMultipleSamePredicateInBlockFail2(t *testing.T) {

	// age is asked for two times..
	query := `
		{
			me(func: uid(0x01)) {
				friend {
					age
					age
				}
				name
			}
		}
	`
	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestMultipleSamePredicateInBlockFail3(t *testing.T) {

	// friend is asked for two times..
	query := `
		{
			me(func: uid(0x01)) {
				friend {
					age
				}
				friend {
					name
				}
				name
			}
		}
	`
	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestXidInvalidJSON(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				_xid_
				gender
				alive
				friend {
					_xid_
					random
					name
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"_xid_":"mich","alive":true,"friend":[{"name":"Rick Grimes"},{"_xid_":"g\"lenn","name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
	m := make(map[string]interface{})
	err := json.Unmarshal([]byte(js), &m)
	require.NoError(t, err)
}

func TestToJSONReverseNegativeFirst(t *testing.T) {

	query := `
		{
			me(func: allofterms(name, "Andrea")) {
				name
				~friend (first: -1) {
					name
					gender
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea","~friend":[{"gender":"female","name":"Michonne"}]},{"name":"Andrea With no friends"}]}}`,
		js)
}

func TestToFastJSONOrderLang(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				friend(first:2, orderdesc: alias@en:de:.) {
					alias
				}
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"alias":"Zambo Alice"},{"alias":"John Oliver"}]}]}}`,
		js)
}

func TestBoolIndexEqRoot1(t *testing.T) {

	query := `
		{
			me(func: eq(alive, true)) {
				name
				alive
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":true,"name":"Michonne"},{"alive":true,"name":"Rick Grimes"}]}}`,
		js)
}

func TestBoolIndexEqRoot2(t *testing.T) {

	query := `
		{
			me(func: eq(alive, false)) {
				name
				alive
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":false,"name":"Daryl Dixon"},{"alive":false,"name":"Andrea"}]}}`,
		js)
}

func TestBoolIndexgeRoot(t *testing.T) {

	q := `
		{
			me(func: ge(alive, true)) {
				name
				alive
				friend {
					name
					alive
				}
			}
		}`

	_, err := processToFastJson(t, q)
	require.NotNil(t, err)
}

func TestBoolIndexEqChild(t *testing.T) {

	query := `
		{
			me(func: eq(alive, true)) {
				name
				alive
				friend @filter(eq(alive, false)) {
					name
					alive
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":true,"friend":[{"alive":false,"name":"Daryl Dixon"},{"alive":false,"name":"Andrea"}],"name":"Michonne"},{"alive":true,"name":"Rick Grimes"}]}}`,
		js)
}

func TestBoolSort(t *testing.T) {

	q := `
		{
			me(func: anyofterms(name, "Michonne Andrea Rick"), orderasc: alive) {
				name
				alive
			}
		}
	`

	_, err := processToFastJson(t, q)
	require.NotNil(t, err)
}

func TestStringEscape(t *testing.T) {

	query := `
		{
			me(func: uid(2301)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Alice\""}]}}`,
		js)
}

func TestJSONQueryVariables(t *testing.T) {

	q := `query test ($a: int = 1) {
		me(func: uid(0x01)) {
			name
			gender
			friend(first: $a) {
				name
			}
		}
	}`
	js, err := processToFastJsonCtxVars(t, q, defaultContext(), map[string]string{"$a": "2"})
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}],"gender":"female","name":"Michonne"}]}}`, js)
}

func TestOrderDescFilterCount(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				friend(first:2, orderdesc: age) @filter(eq(alias, "Zambo Alice")) {
					alias
				}
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"alias":"Zambo Alice"}]}]}}`,
		js)
}

func TestHashTokEq(t *testing.T) {

	query := `
		{
			me(func: eq(full_name, "Michonne's large name for hashing")) {
				full_name
				alive
				friend {
					name
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":true,"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"full_name":"Michonne's large name for hashing"}]}}`,
		js)
}

func TestHashTokGeqErr(t *testing.T) {

	query := `
		{
			me(func: ge(full_name, "Michonne's large name for hashing")) {
				full_name
				alive
				friend {
					name
				}
			}
		}
	`
	res, _ := gql.Parse(gql.Request{Str: query})
	queryRequest := QueryRequest{Latency: &Latency{}, GqlQuery: &res}
	err := queryRequest.ProcessQuery(defaultContext())
	require.Error(t, err)
}

func TestNameNotIndexed(t *testing.T) {

	query := `
		{
			me(func: eq(noindex_name, "Michonne's name not indexed")) {
				full_name
				alive
				friend {
					name
				}
			}
		}
	`
	res, _ := gql.Parse(gql.Request{Str: query})
	queryRequest := QueryRequest{Latency: &Latency{}, GqlQuery: &res}
	err := queryRequest.ProcessQuery(defaultContext())
	require.Error(t, err)
}

func TestMultipleMinMax(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				friend {
					x as age
					n as name
				}
				min(val(x))
				max(val(x))
				min(val(n))
				max(val(n))
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"age":15,"name":"Rick Grimes"},{"age":15,"name":"Glenn Rhee"},{"age":17,"name":"Daryl Dixon"},{"age":19,"name":"Andrea"}],"max(val(n))":"Rick Grimes","max(val(x))":19,"min(val(n))":"Andrea","min(val(x))":15}]}}`,
		js)
}

func TestDuplicateAlias(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				friend {
					x as age
				}
				a: min(val(x))
				a: max(val(x))
			}
		}`
	res, _ := gql.Parse(gql.Request{Str: query})
	queryRequest := QueryRequest{Latency: &Latency{}, GqlQuery: &res}
	err := queryRequest.ProcessQuery(defaultContext())
	require.Error(t, err)
}

func TestGraphQLId(t *testing.T) {

	q := `query test ($a: string = 1) {
		me(func: uid($a)) {
			name
			gender
			friend(first: 1) {
				name
			}
		}
	}`
	js, err := processToFastJsonCtxVars(t, q, defaultContext(), map[string]string{"$a": "[1, 31]"})
	require.NoError(t, err)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Rick Grimes"}],"gender":"female","name":"Michonne"},{"friend":[{"name":"Glenn Rhee"}],"name":"Andrea"}]}}`, js)
}

func TestDebugUid(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				friend {
					name
					friend
				}
			}
		}`
	ctx := context.WithValue(defaultContext(), DebugKey, "true")
	buf, err := processToFastJsonCtxVars(t, query, ctx, nil)
	require.NoError(t, err)
	var mp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(buf), &mp))
	resp := mp["data"].(map[string]interface{})["me"]
	body, err := json.Marshal(resp)
	require.NoError(t, err)
	require.JSONEq(t, `[{"friend":[{"name":"Rick Grimes","uid":"0x17"},{"name":"Glenn Rhee","uid":"0x18"},{"name":"Daryl Dixon","uid":"0x19"},{"name":"Andrea","uid":"0x1f"}],"name":"Michonne","uid":"0x1"}]`, string(body))
}

func TestUidAlias(t *testing.T) {

	query := `
		{
			me(func: uid(0x1)) {
				id: uid
				alive
				friend {
					uid: uid
					name
				}
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":true,"friend":[{"name":"Rick Grimes","uid":"0x17"},{"name":"Glenn Rhee","uid":"0x18"},{"name":"Daryl Dixon","uid":"0x19"},{"name":"Andrea","uid":"0x1f"},{"uid":"0x65"}],"id":"0x1"}]}}`,
		js)
}

func TestCountAtRoot(t *testing.T) {

	query := `
        {
            me(func: gt(count(friend), 0)) {
				count(uid)
			}
        }
        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"count": 3}]}}`, js)
}

func TestCountAtRoot2(t *testing.T) {

	query := `
        {
                me(func: anyofterms(name, "Michonne Rick Andrea")) {
			count(uid)
		}
        }
        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"count": 4}]}}`, js)
}

func TestCountAtRoot3(t *testing.T) {

	query := `
        {
		me(func:anyofterms(name, "Michonne Rick Daryl")) {
			name
			count(uid)
			count(friend)
			friend {
				name
				count(uid)
			}
		}
        }
        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"count":3},{"count(friend)":5,"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"},{"count":5}],"name":"Michonne"},{"count(friend)":1,"friend":[{"name":"Michonne"},{"count":1}],"name":"Rick Grimes"},{"count(friend)":0,"name":"Daryl Dixon"}]}}`, js)
}

func TestCountAtRootWithAlias4(t *testing.T) {

	query := `
	{
                me(func:anyofterms(name, "Michonne Rick Daryl")) @filter(le(count(friend), 2)) {
			personCount: count(uid)
		}
        }
        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": [{"personCount": 2}]}}`, js)
}

func TestCountAtRoot5(t *testing.T) {

	query := `
	{
		me(func: uid(1)) {
			f as friend {
				name
			}
		}
		MichonneFriends(func: uid(f)) {
			count(uid)
		}
	}


        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"MichonneFriends":[{"count":5}],"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}]}}`, js)
}

func TestHasFuncAtRoot(t *testing.T) {

	query := `
	{
		me(func: has(friend)) {
			name
			friend {
				count(uid)
			}
		}
	}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"count":5}],"name":"Michonne"},{"friend":[{"count":1}],"name":"Rick Grimes"},{"friend":[{"count":1}],"name":"Andrea"}]}}`, js)
}

func TestHasFuncAtRootWithAfter(t *testing.T) {

	query := `
	{
		me(func: has(friend), after: 0x01) {
			uid
			name
			friend {
				count(uid)
			}
		}
	}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"count":1}],"name":"Rick Grimes","uid":"0x17"},{"friend":[{"count":1}],"name":"Andrea","uid":"0x1f"}]}}`, js)
}

func TestHasFuncAtRootFilter(t *testing.T) {

	query := `
	{
		me(func: anyofterms(name, "Michonne Rick Daryl")) @filter(has(friend)) {
			name
			friend {
				count(uid)
			}
		}
	}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"count":5}],"name":"Michonne"},{"friend":[{"count":1}],"name":"Rick Grimes"}]}}`, js)
}

func TestHasFuncAtChild1(t *testing.T) {

	query := `
	{
		me(func: has(school)) {
			name
			friend @filter(has(scooter)) {
				name
			}
		}
	}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}}`, js)
}

func TestHasFuncAtChild2(t *testing.T) {

	query := `
	{
		me(func: has(school)) {
			name
			friend @filter(has(alias)) {
				name
				alias
			}
		}
	}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"alias":"Zambo Alice","name":"Rick Grimes"},{"alias":"John Alice","name":"Glenn Rhee"},{"alias":"Bob Joe","name":"Daryl Dixon"},{"alias":"Allan Matt","name":"Andrea"},{"alias":"John Oliver"}],"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"friend":[{"alias":"John Alice","name":"Glenn Rhee"}],"name":"Andrea"}]}}`, js)
}

func TestHasFuncAtRoot2(t *testing.T) {

	query := `
	{
		me(func: has(name@en)) {
			name@en
		}
	}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name@en":"Alex"},{"name@en":"Amit"},{"name@en":"Andrew"},{"name@en":"European badger"},{"name@en":"Honey badger"},{"name@en":"Honey bee"},{"name@en":"Artem Tkachenko"}]}}`, js)
}

func getSubGraphs(t *testing.T, query string) (subGraphs []*SubGraph) {
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	for _, block := range res.Query {
		subGraph, err := ToSubGraph(ctx, block)
		require.NoError(t, err)
		require.NotNil(t, subGraph)

		subGraphs = append(subGraphs, subGraph)
	}

	return subGraphs
}

// simplest case
func TestGetAllPredicatesSimple(t *testing.T) {
	query := `
	{
		me(func: uid(0x1)) {
			name
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 1, len(predicates))
	require.Equal(t, "name", predicates[0])
}

// recursive SubGraph traversal; predicates should be unique
func TestGetAllPredicatesUnique(t *testing.T) {
	query := `
	{
		me(func: uid(0x1)) {
			name
			friend {
				name
				age
			}
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 3, len(predicates))
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "friend")
	require.Contains(t, predicates, "age")
}

// gather predicates from functions and filters
func TestGetAllPredicatesFunctions(t *testing.T) {
	query := `
	{
		me(func:anyofterms(name, "Alice")) @filter(le(age, 30)) {
			alias
			friend @filter(eq(school, 5000)) {
				alias
				follow
			}
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 6, len(predicates))
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "age")
	require.Contains(t, predicates, "alias")
	require.Contains(t, predicates, "friend")
	require.Contains(t, predicates, "school")
	require.Contains(t, predicates, "follow")
}

// gather predicates from functions and filters
func TestGetAllPredicatesFunctions2(t *testing.T) {
	query := `
	{
		me(func:anyofterms(name, "Alice")) @filter(le(age, 30)) {
			alias
			friend @filter(uid(123, 5000)) {
				alias
				follow
			}
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 5, len(predicates))
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "age")
	require.Contains(t, predicates, "alias")
	require.Contains(t, predicates, "friend")
	require.Contains(t, predicates, "follow")
}

// gather predicates from order
func TestGetAllPredicatesOrdering(t *testing.T) {
	query := `
	{
		me(func:anyofterms(name, "Alice"), orderasc: age) {
			name
			friend(orderdesc: alias) {
				name
			}
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 4, len(predicates))
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "age")
	require.Contains(t, predicates, "friend")
	require.Contains(t, predicates, "alias")
}

// gather predicates from multiple query blocks (and var)
func TestGetAllPredicatesVars(t *testing.T) {
	query := `
	{
		IDS as var(func:anyofterms(name, "Alice"), orderasc: age) {}

		me(func: uid(IDS)) {
			alias
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 3, len(predicates))
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "age")
	require.Contains(t, predicates, "alias")
}

// gather predicates from groupby
func TestGetAllPredicatesGroupby(t *testing.T) {
	query := `
	{
		me(func: uid(1)) {
			friend @groupby(age) {
				count(uid)
			}
			name
		}
	}
	`

	subGraphs := getSubGraphs(t, query)

	predicates := GetAllPredicates(subGraphs)
	require.NotNil(t, predicates)
	require.Equal(t, 4, len(predicates))
	require.Contains(t, predicates, "uid")
	require.Contains(t, predicates, "name")
	require.Contains(t, predicates, "age")
	require.Contains(t, predicates, "friend")
}

func TestMathVarCrash(t *testing.T) {

	query := `
		{
			f(func: anyofterms(name, "Rick Michonne Andrea")) {
				age as age
				a as math(age *2)
				val(a)
			}
		}
	`
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	queryRequest := QueryRequest{Latency: &Latency{}, GqlQuery: &res}
	err = queryRequest.ProcessQuery(defaultContext())
	require.Error(t, err)
}

func TestMathVarAlias(t *testing.T) {

	query := `
		{
			f(func: anyofterms(name, "Rick Michonne Andrea")) {
				ageVar as age
				a: math(ageVar *2)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"f":[{"a":76.000000,"age":38},{"a":30.000000,"age":15},{"a":38.000000,"age":19}]}}`, js)
}

func TestMathVarAlias2(t *testing.T) {

	query := `
		{
			f as me(func: anyofterms(name, "Rick Michonne Andrea")) {
				ageVar as age
				doubleAge: a as math(ageVar *2)
			}

			me2(func: uid(f)) {
				val(a)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"age":38,"doubleAge":76.000000},{"age":15,"doubleAge":30.000000},{"age":19,"doubleAge":38.000000}],"me2":[{"val(a)":76.000000},{"val(a)":30.000000},{"val(a)":38.000000}]}}`, js)
}

func TestMathVar3(t *testing.T) {

	query := `
		{
			f as me(func: anyofterms(name, "Rick Michonne Andrea")) {
				ageVar as age
				a as math(ageVar *2)
			}

			me2(func: uid(f)) {
				val(a)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"age":38,"val(a)":76.000000},{"age":15,"val(a)":30.000000},{"age":19,"val(a)":38.000000}],"me2":[{"val(a)":76.000000},{"val(a)":30.000000},{"val(a)":38.000000}]}}`, js)
}

func TestMultipleEquality(t *testing.T) {

	query := `
	{
		me(func: eq(name, ["Rick Grimes"])) {
			name
			friend {
				name
			}
		}
	}


        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Michonne"}],"name":"Rick Grimes"}]}}`, js)
}

func TestMultipleEquality2(t *testing.T) {

	query := `
	{
		me(func: eq(name, ["Badger", "Bobby", "Matt"])) {
			name
			friend {
				name
			}
		}
	}

        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Matt"},{"name":"Badger"}]}}`, js)
}

func TestMultipleEquality3(t *testing.T) {

	query := `
	{
		me(func: eq(dob, ["1910-01-01", "1909-05-05"])) {
			name
			friend {
				name
			}
		}
	}

        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestMultipleEquality4(t *testing.T) {

	query := `
	{
		me(func: eq(dob, ["1910-01-01", "1909-05-05"])) {
			name
			friend @filter(eq(name, ["Rick Grimes", "Andrea"])) {
				name
			}
		}
	}

        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Andrea"}],"name":"Michonne"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestMultipleEquality5(t *testing.T) {

	query := `
	{
		me(func: eq(name@en, ["Honey badger", "Honey bee"])) {
			name@en
		}
	}

        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name@en":"Honey badger"},{"name@en":"Honey bee"}]}}`, js)
}

func TestMultipleGtError(t *testing.T) {

	query := `
	{
		me(func: gt(name, ["Badger", "Bobby"])) {
			name
			friend {
				name
			}
		}
	}

  `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	queryRequest := QueryRequest{Latency: &Latency{}, GqlQuery: &res}
	err = queryRequest.ProcessQuery(defaultContext())
	require.Error(t, err)
}

func TestMultipleEqQuote(t *testing.T) {

	query := `
	{
		me(func: eq(name, ["Alice\"", "Michonne"])) {
			name
			friend {
				name
			}
		}
	}
`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"},{"name":"Alice\""}]}}`, js)
}

func TestMultipleEqInt(t *testing.T) {

	query := `
	{
		me(func: eq(age, [15, 17, 38])) {
			name
			friend {
				name
			}
		}
	}
`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]},{"name":"Rick Grimes","friend":[{"name":"Michonne"}]},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"}]}}`, js)
}

func TestUidFunction(t *testing.T) {

	query := `
	{
		me(func: uid(23, 1, 24, 25, 31)) {
			name
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}}`, js)
}

func TestUidFunctionInFilter(t *testing.T) {

	query := `
	{
		me(func: uid(23, 1, 24, 25, 31))  @filter(uid(1, 24)) {
			name
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestUidFunctionInFilter2(t *testing.T) {

	query := `
	{
		me(func: uid(23, 1, 24, 25, 31)) {
			name
			# Filtering only Michonne and Rick.
			friend @filter(uid(23, 1)) {
				name
			}
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","friend":[{"name":"Rick Grimes"}]},{"name":"Rick Grimes","friend":[{"name":"Michonne"}]},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}}`, js)
}

func TestUidFunctionInFilter3(t *testing.T) {

	query := `
	{
		me(func: anyofterms(name, "Michonne Andrea")) @filter(uid(1)) {
			name
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"}]}}`, js)
}

func TestUidFunctionInFilter4(t *testing.T) {

	query := `
	{
		me(func: anyofterms(name, "Michonne Andrea")) @filter(not uid(1, 31)) {
			name
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Andrea With no friends"}]}}`, js)
}

func TestUidInFunction(t *testing.T) {

	query := `
	{
		me(func: uid(1, 23, 24)) @filter(uid_in(friend, 23)) {
			name
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"}]}}`, js)
}

func TestUidInFunction1(t *testing.T) {

	query := `
	{
		me(func: UID(1, 23, 24)) @filter(uid_in(school, 5000)) {
			name
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Glenn Rhee"}]}}`, js)
}

func TestUidInFunction2(t *testing.T) {

	query := `
	{
		me(func: uid(1, 23, 24)) {
			friend @filter(uid_in(school, 5000)) {
				name
			}
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"}]},{"friend":[{"name":"Michonne"}]}]}}`,
		js)
}

func TestUidInFunctionAtRoot(t *testing.T) {

	query := `
	{
		me(func: uid_in(school, 5000)) {
				name
		}
	}`

	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := defaultContext()
	qr := QueryRequest{Latency: &Latency{}, GqlQuery: &res}
	err = qr.ProcessQuery(ctx)
	require.Error(t, err)
}

func TestBinaryJSON(t *testing.T) {
	query := `
	{
		me(func: uid(1)) {
			name
			bin_data
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","bin_data":"YmluLWRhdGE="}]}}`, js)
}

func TestReflexive(t *testing.T) {

	query := `
	{
		me(func:anyofterms(name, "Michonne Rick Daryl")) @ignoreReflex {
			name
			friend {
				name
				friend {
					name
				}
			}
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"friend":[{"name":"Glenn Rhee"}],"name":"Andrea"}],"name":"Michonne"},{"friend":[{"friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"}],"name":"Rick Grimes"},{"name":"Daryl Dixon"}]}}`, js)
}

func TestReflexive2(t *testing.T) {

	query := `
	{
		me(func:anyofterms(name, "Michonne Rick Daryl")) @IGNOREREFLEX {
			name
			friend {
				name
				friend {
					name
				}
			}
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"friend":[{"name":"Glenn Rhee"}],"name":"Andrea"}],"name":"Michonne"},{"friend":[{"friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"name":"Michonne"}],"name":"Rick Grimes"},{"name":"Daryl Dixon"}]}}`, js)
}

func TestReflexive3(t *testing.T) {

	query := `
	{
		me(func:anyofterms(name, "Michonne Rick Daryl")) @IGNOREREFLEX @normalize {
			Me: name
			friend {
				Friend: name
				friend {
					Cofriend: name
				}
			}
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"Friend":"Rick Grimes","Me":"Michonne"},{"Friend":"Glenn Rhee","Me":"Michonne"},{"Friend":"Daryl Dixon","Me":"Michonne"},{"Cofriend":"Glenn Rhee","Friend":"Andrea","Me":"Michonne"},{"Cofriend":"Glenn Rhee","Friend":"Michonne","Me":"Rick Grimes"},{"Cofriend":"Daryl Dixon","Friend":"Michonne","Me":"Rick Grimes"},{"Cofriend":"Andrea","Friend":"Michonne","Me":"Rick Grimes"},{"Me":"Daryl Dixon"}]}}`, js)
}

func TestCascadeUid(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) @cascade {
				name
				gender
				friend {
					uid
					name
					friend{
						name
						dob
						age
					}
				}
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"uid":"0x17","friend":[{"age":38,"dob":"1910-01-01T00:00:00Z","name":"Michonne"}],"name":"Rick Grimes"},{"uid":"0x1f","friend":[{"age":15,"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"}],"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`, js)
}

func TestUseVariableBeforeDefinitionError(t *testing.T) {

	query := `
{
	me(func: anyofterms(name, "Michonne Daryl Andrea"), orderasc: val(avgAge)) {
		name
		friend {
			x as age
		}
		avgAge as avg(val(x))
	}
}`

	_, err := processToFastJson(t, query)
	require.Contains(t, err.Error(), "Variable: [avgAge] used before definition.")
}

func TestAggregateRoot1(t *testing.T) {

	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				sum(val(a))
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"sum(val(a))":72}]}}`, js)
}

func TestAggregateRoot2(t *testing.T) {

	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				avg(val(a))
				min(val(a))
				max(val(a))
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"avg(val(a))":24.000000},{"min(val(a))":15},{"max(val(a))":38}]}}`, js)
}

func TestAggregateRoot3(t *testing.T) {

	query := `
		{
			me1(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				sum(val(a))
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me1":[{"age":38},{"age":15},{"age":19}],"me":[{"sum(val(a))":72}]}}`, js)
}

func TestAggregateRoot4(t *testing.T) {

	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			me() {
				minVal as min(val(a))
				maxVal as max(val(a))
				Sum: math(minVal + maxVal)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"min(val(a))":15},{"max(val(a))":38},{"Sum":53.000000}]}}`, js)
}

func TestAggregateRoot5(t *testing.T) {

	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				# money edge doesn't exist
				m as money
			}

			me() {
				sum(val(m))
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"sum(val(m))":0.000000}]}}`, js)
}

func TestAggregateRoot6(t *testing.T) {
	query := `
		{
			uids as var(func: anyofterms(name, "Rick Michonne Andrea"))

			var(func: uid(uids)) @cascade {
				reason {
					killed_zombies as math(1)
				}
				zombie_count as sum(val(killed_zombies))
			}

			me(func: uid(uids)) {
				money: val(zombie_count)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
}

func TestAggregateRootError(t *testing.T) {

	query := `
		{
			var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as age
			}

			var(func: anyofterms(name, "Rick Michonne")) {
				a2 as age
			}

			me() {
				Sum: math(a + a2)
			}
		}
	`
	ctx := defaultContext()
	_, err := processToFastJsonCtxVars(t, query, ctx, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only aggregated variables allowed within empty block.")
}

func TestAggregateEmpty1(t *testing.T) {
	query := `
	{
		var(func: has(number)) {
			number as number
		}
		var() {
			highest as max(val(number))
		}

		all(func: eq(number, val(highest))) {
			uid
			number
		}
	}`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"all":[]}}`, js)
}

func TestAggregateEmpty2(t *testing.T) {
	query := `
		{
			var(func: has(number))
			{
				highest_number as number
			}
			all(func: eq(number, val(highest_number)))
			{
				uid
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"all":[]}}`, js)
}

func TestAggregateEmpty3(t *testing.T) {
	query := `
		{
			var(func: has(number))
			{
				highest_number as number
			}
			all(func: ge(number, val(highest_number)))
			{
				uid
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"all":[]}}`, js)
}

func TestFilterLang(t *testing.T) {
	// This tests the fix for #1334. While getting uids for filter, we fetch data keys when number
	// of uids is less than number of tokens. Lang tag was not passed correctly while fetching these
	// data keys.

	query := `
		{
			me(func: uid(0x1001, 0x1002, 0x1003)) @filter(ge(name@en, "D"))  {
				name@en
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@en":"European badger"},{"name@en":"Honey badger"},{"name@en":"Honey bee"}]}}`, js)
}

func TestMathCeil1(t *testing.T) {

	query := `
	{
		me as var(func: eq(name, "XxXUnknownXxX"))
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
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestMathCeil2(t *testing.T) {

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
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"ceilAge":14.000000}]}}`, js)
}

func TestAppendDummyValuesPanic(t *testing.T) {
	// This is a fix for #1359. We should check that SrcUIDs is not nil before accessing Uids.

	query := `
	{
		n(func:ge(uid, 0)) {
			count(uid)
		}
	}`
	_, err := processToFastJson(t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(), `Argument cannot be "uid"`)
}

func TestMultipleValueFilter(t *testing.T) {

	query := `
	{
		me(func: ge(graduation, "1930")) {
			name
			graduation
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","graduation":["1932-01-01T00:00:00Z"]},{"name":"Andrea","graduation":["1935-01-01T00:00:00Z","1933-01-01T00:00:00Z"]}]}}`, js)
}

func TestMultipleValueFilter2(t *testing.T) {

	query := `
	{
		me(func: le(graduation, "1933")) {
			name
			graduation
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","graduation":["1932-01-01T00:00:00Z"]},{"name":"Andrea","graduation":["1935-01-01T00:00:00Z","1933-01-01T00:00:00Z"]}]}}`, js)
}

func TestMultipleValueArray(t *testing.T) {

	query := `
	{
		me(func: uid(1)) {
			name
			graduation
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","graduation":["1932-01-01T00:00:00Z"]}]}}`, js)
}

func TestMultipleValueArray2(t *testing.T) {

	query := `
	{
		me(func: uid(1)) {
			graduation
			name
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","graduation":["1932-01-01T00:00:00Z"]}]}}`, js)
}

func TestMultipleValueHasAndCount(t *testing.T) {

	query := `
	{
		me(func: has(graduation)) {
			name
			count(graduation)
			graduation
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","count(graduation)":1,"graduation":["1932-01-01T00:00:00Z"]},{"name":"Andrea","count(graduation)":2,"graduation":["1935-01-01T00:00:00Z","1933-01-01T00:00:00Z"]}]}}`, js)
}

func TestMultipleValueSortError(t *testing.T) {

	query := `
	{
		me(func: anyofterms(name, "Michonne Rick"), orderdesc: graduation) {
			name
			graduation
		}
	}
	`
	ctx := defaultContext()
	_, err := processToFastJsonCtxVars(t, query, ctx, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Sorting not supported on attr: graduation of type: [scalar]")
}

func TestMultipleValueGroupByError(t *testing.T) {
	t.Skip()

	query := `
	{
		me(func: uid(1)) {
			friend @groupby(name, graduation) {
				count(uid)
			}
		}
	}
	`
	ctx := defaultContext()
	_, err := processToFastJsonCtxVars(t, query, ctx, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Groupby not allowed for attr: graduation of type list")
}

func TestMultiPolygonIntersects(t *testing.T) {

	usc, err := ioutil.ReadFile("testdata/us-coordinates.txt")
	require.NoError(t, err)
	query := `{
		me(func: intersects(geometry, "` + strings.TrimSpace(string(usc)) + `" )) {
			name
		}
	}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"},{"name":"San Carlos Airport"},{"name":"SF Bay area"},{"name":"Mountain View"},{"name":"San Carlos"}, {"name": "New York"}]}}`, js)
}

func TestMultiPolygonWithin(t *testing.T) {

	usc, err := ioutil.ReadFile("testdata/us-coordinates.txt")
	require.NoError(t, err)
	query := `{
		me(func: within(geometry, "` + strings.TrimSpace(string(usc)) + `" )) {
			name
		}
	}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Googleplex"},{"name":"Shoreline Amphitheater"},{"name":"San Carlos Airport"},{"name":"Mountain View"},{"name":"San Carlos"}]}}`, js)
}

// func TestMultiPolygonContains(t *testing.T) {
// 	// We should get this back as a result as it should contain our Denver polygon.
// 	multipoly, err := loadPolygon("testdata/us-coordinates.txt")
// 	require.NoError(t, err)
// 	addGeoData(t, 5108, multipoly, "USA")

// 	query := `{
// 		me(func: contains(geometry, "[[[ -1185.8203125, 41.27780646738183 ], [ -1189.1162109375, 37.64903402157866 ], [ -1182.1728515625, 36.84446074079564 ], [ -1185.8203125, 41.27780646738183 ]]]")) {
// 			name
// 		}
// 	}`

// 	js := processToFastJsonNoErr(t, query)
// 	require.JSONEq(t, `{"data": {"me":[{"name":"USA"}]}}`, js)
// }

func TestNearPointMultiPolygon(t *testing.T) {

	query := `{
		me(func: near(loc, [1.0, 1.0], 1)) {
			name
		}
	}`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"}]}}`, js)
}

func TestMultiSort1(t *testing.T) {

	time.Sleep(10 * time.Millisecond)

	query := `{
		me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderasc: age) {
			name
			age
		}
	}`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Alice","age":25},{"name":"Alice","age":75},{"name":"Alice","age":75},{"name":"Bob","age":25},{"name":"Bob","age":75},{"name":"Colin","age":25},{"name":"Elizabeth","age":25},{"name":"Elizabeth","age":75}]}}`, js)
}

func TestMultiSort2(t *testing.T) {

	query := `{
		me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderdesc: age) {
			name
			age
		}
	}`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Alice","age":75},{"name":"Alice","age":75},{"name":"Alice","age":25},{"name":"Bob","age":75},{"name":"Bob","age":25},{"name":"Colin","age":25},{"name":"Elizabeth","age":75},{"name":"Elizabeth","age":25}]}}`, js)
}

func TestMultiSort3(t *testing.T) {

	query := `{
		me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: age, orderdesc: name) {
			name
			age
		}
	}`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Elizabeth","age":25},{"name":"Colin","age":25},{"name":"Bob","age":25},{"name":"Alice","age":25},{"name":"Elizabeth","age":75},{"name":"Bob","age":75},{"name":"Alice","age":75},{"name":"Alice","age":75}]}}`, js)
}

func TestMultiSort4(t *testing.T) {

	query := `{
		me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderasc: salary) {
			name
			age
			salary
		}
	}`
	js := processToFastJsonNoErr(t, query)
	// Null value for third Alice comes at last.
	require.JSONEq(t, `{"data": {"me":[{"name":"Alice","age":25,"salary":10000.000000},{"name":"Alice","age":75,"salary":10002.000000},{"name":"Alice","age":75},{"name":"Bob","age":75},{"name":"Bob","age":25},{"name":"Colin","age":25},{"name":"Elizabeth","age":75},{"name":"Elizabeth","age":25}]}}`, js)
}

func TestMultiSort5(t *testing.T) {

	query := `{
		me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderdesc: salary) {
			name
			age
			salary
		}
	}`
	js := processToFastJsonNoErr(t, query)
	// Null value for third Alice comes at first.
	require.JSONEq(t, `{"data": {"me":[{"name":"Alice","age":75},{"name":"Alice","age":75,"salary":10002.000000},{"name":"Alice","age":25,"salary":10000.000000},{"name":"Bob","age":25},{"name":"Bob","age":75},{"name":"Colin","age":25},{"name":"Elizabeth","age":25},{"name":"Elizabeth","age":75}]}}`, js)
}

func TestMultiSort6Paginate(t *testing.T) {

	query := `{
		me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderdesc: age, first: 7) {
			name
			age
		}
	}`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Alice","age":75},{"name":"Alice","age":75},{"name":"Alice","age":25},{"name":"Bob","age":75},{"name":"Bob","age":25},{"name":"Colin","age":25},{"name":"Elizabeth","age":75}]}}`, js)
}

func TestMultiSort7Paginate(t *testing.T) {

	query := `{
		me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderasc: age, first: 7) {
			name
			age
		}
	}`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Alice","age":25},{"name":"Alice","age":75},{"name":"Alice","age":75},{"name":"Bob","age":25},{"name":"Bob","age":75},{"name":"Colin","age":25},{"name":"Elizabeth","age":25}]}}`, js)
}

func TestMultiSortPaginateWithOffset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		query  string
		result string
	}{
		{
			"Offset in middle of bucket",
			`{
			me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderasc: age, first: 6, offset: 1) {
				name
				age
			}
		}`,
			`{"data": {"me":[{"name":"Alice","age":75},{"name":"Alice","age":75},{"name":"Bob","age":25},{"name":"Bob","age":75},{"name":"Colin","age":25},{"name":"Elizabeth","age":25}]}}`,
		},
		{
			"Offset at boundary of bucket",
			`{
			me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderasc: age, first: 4, offset: 3) {
				name
				age
			}
		}`,
			`{"data": {"me":[{"name":"Bob","age":25},{"name":"Bob","age":75},{"name":"Colin","age":25},{"name":"Elizabeth","age":25}]}}`,
		},
		{
			"Offset in middle of second bucket",
			`{
			me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderasc: age, first: 3, offset: 4) {
				name
				age
			}
		}`,
			`{"data": {"me":[{"name":"Bob","age":75},{"name":"Colin","age":25},{"name":"Elizabeth","age":25}]}}`,
		},
		{
			"Offset equal to number of uids",
			`{
			me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderasc: age, first: 3, offset: 8) {
				name
				age
			}
		}`,
			`{"data": {"me":[]}}`,
		},
		{
			"Offset larger than records",
			`{
			me(func: uid(10005, 10006, 10001, 10002, 10003, 10004, 10007, 10000), orderasc: name, orderasc: age, first: 10, offset: 10000) {
				name
				age
			}
		}`,
			`{"data": {"me":[]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := processToFastJsonNoErr(t, tt.query)
			require.JSONEq(t, tt.result, js)
		})
	}
}

func TestFilterRootOverride(t *testing.T) {

	query := `{
		a as var(func: eq(name, "Michonne")) @filter(eq(name, "Rick Grimes"))

		me(func: uid(a)) {
			uid
			name
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestFilterRoot(t *testing.T) {

	query := `{
		me(func: eq(name, "Michonne")) @filter(eq(name, "Rick Grimes")) {
			uid
			name
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestMathAlias(t *testing.T) {

	query := `{
		me(func:allofterms(name, "Michonne")) {
			p as count(friend)
			score: math(p + 1)
			name
		}
	}`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"count(friend)":5,"score":6.000000,"name":"Michonne"}]}}`, js)
}

func TestUidVariable(t *testing.T) {

	query := `{
		var(func:allofterms(name, "Michonne")) {
			friend {
				f as uid
			}
		}

		me(func: uid(f)) {
			name
		}
	}`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}}`, js)
}

func TestMultipleValueVarError(t *testing.T) {

	query := `{
		var(func:ge(graduation, "1930")) {
			o as graduation
		}

		me(func: uid(o)) {
			graduation
		}
	}`

	ctx := defaultContext()
	_, err := processToFastJsonCtxVars(t, query, ctx, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Value variables not supported for predicate with list type.")
}

func TestReturnEmptyBlock(t *testing.T) {

	query := `{
		me(func:allofterms(name, "Michonne")) @filter(eq(name, "Rick Grimes")) {
		}

		me2(func: eq(name, "XYZ"))

		me3(func: eq(name, "Michonne")) {
			name
		}
	}`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[],"me2":[],"me3":[{"name":"Michonne"}]}}`, js)
}

func TestExpandVal(t *testing.T) {
	// We ignore password in expand(_all_)
	query := `
	{
		var(func: uid(1)) {
			pred as _predicate_
		}

		me(func: uid(1)) {
			expand(val(pred))
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"age":38,"full_name":"Michonne's large name for hashing","dob_day":"1910-01-01T00:00:00Z","power":13.250000,"noindex_name":"Michonne's name not indexed","survival_rate":98.990000,"name":"Michonne","sword_present":"true","alive":true,"dob":"1910-01-01T00:00:00Z","path":[{"path|weight":0.200000},{"path|weight":0.100000,"path|weight1":0.200000}],"bin_data":"YmluLWRhdGE=","loc":{"type":"Point","coordinates":[1.1,2]},"address":"31, 32 street, Jupiter","graduation":["1932-01-01T00:00:00Z"],"gender":"female","_xid_":"mich"}]}}`, js)
}

func TestGroupByGeoCrash(t *testing.T) {

	query := `
	{
	  q(func: uid(1, 23, 24, 25, 31)) @groupby(loc) {
	    count(uid)
	  }
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.Contains(t, js, `{"loc":{"type":"Point","coordinates":[1.1,2]},"count":2}`)
}

func TestPasswordError(t *testing.T) {

	query := `
	{
		q(func: uid(1)) {
			checkpwd(name, "Michonne")
		}
	}
	`
	ctx := defaultContext()
	_, err := processToFastJsonCtxVars(t, query, ctx, nil)
	require.Error(t, err)
	require.Contains(t,
		err.Error(), "checkpwd fn can only be used on attr: [name] with schema type password. Got type: string")
}

func TestCountPanic(t *testing.T) {

	query := `
	{
		q(func: uid(1, 300)) {
			uid
			name
			count(name)
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"q":[{"uid":"0x1","name":"Michonne","count(name)":1},{"uid":"0x12c","count(name)":0}]}}`, js)
}

func TestExpandAll(t *testing.T) {

	query := `
	{
		q(func: uid(1)) {
			expand(_all_) {
				name
			}
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"q":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"power":13.250000,"_xid_":"mich","noindex_name":"Michonne's name not indexed","son":[{"name":"Andre"},{"name":"Helmut"}],"address":"31, 32 street, Jupiter","dob_day":"1910-01-01T00:00:00Z","follow":[{"name":"Glenn Rhee"},{"name":"Andrea"}],"name":"Michonne","path":[{"name":"Glenn Rhee","path|weight":0.200000},{"name":"Andrea","path|weight":0.100000,"path|weight1":0.200000}],"school":[{"name":"School A"}],"full_name":"Michonne's large name for hashing","alive":true,"bin_data":"YmluLWRhdGE=","gender":"female","loc":{"type":"Point","coordinates":[1.1,2]},"graduation":["1932-01-01T00:00:00Z"],"age":38,"sword_present":"true","dob":"1910-01-01T00:00:00Z","survival_rate":98.990000,"~friend":[{"name":"Rick Grimes"}]}]}}`, js)
}

func TestExpandForward(t *testing.T) {
	query := `
	{
		q(func: uid(1)) {
			expand(_forward_) {
				name
			}
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"q":[{"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"power":13.250000,"_xid_":"mich","noindex_name":"Michonne's name not indexed","son":[{"name":"Andre"},{"name":"Helmut"}],"address":"31, 32 street, Jupiter","dob_day":"1910-01-01T00:00:00Z","follow":[{"name":"Glenn Rhee"},{"name":"Andrea"}],"name":"Michonne","path":[{"name":"Glenn Rhee","path|weight":0.200000},{"name":"Andrea","path|weight":0.100000,"path|weight1":0.200000}],"school":[{"name":"School A"}],"full_name":"Michonne's large name for hashing","alive":true,"bin_data":"YmluLWRhdGE=","gender":"female","loc":{"type":"Point","coordinates":[1.1,2]},"graduation":["1932-01-01T00:00:00Z"],"age":38,"sword_present":"true","dob":"1910-01-01T00:00:00Z","survival_rate":98.990000}]}}`, js)
}

func TestExpandReverse(t *testing.T) {
	query := `
	{
		q(func: uid(1)) {
			expand(_reverse_) {
				name
			}
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"q":[{"~friend":[{"name":"Rick Grimes"}]}]}}`, js)
}

func TestUidWithoutDebug(t *testing.T) {

	query := `
	{
		q(func: uid(1, 24)) {
			uid
			friend
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"q":[{"uid":"0x1"},{"uid":"0x18"}]}}`, js)
}

func TestUidWithoutDebug2(t *testing.T) {

	query := `
	{
		q(func: uid(1)) {
			uid
			friend {
				uid
			}
		}
	}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"q":[{"uid":"0x1","friend":[{"uid":"0x17"},{"uid":"0x18"},{"uid":"0x19"},{"uid":"0x1f"},{"uid":"0x65"}]}]}}`, js)
}

func TestExpandAll_empty_panic(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) @filter(eq(name,"foobar")){
				expand(_all_)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[]}}`, js)
}
