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
	"testing"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/stretchr/testify/require"
)

func TestRecurseError(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) @recurse(loop: true) {
				nonexistent_pred
				friend
				name
			}
		}`

	ctx := defaultContext()
	_, err := processToFastJsonCtxVars(t, query, ctx, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Depth must be > 0 when loop is true for recurse query.")
}

func TestRecurseQuery(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) @recurse {
				nonexistent_pred
				friend
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne", "friend":[{"name":"Rick Grimes", "friend":[{"name":"Michonne"}]},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea", "friend":[{"name":"Glenn Rhee"}]}]}]}}`, js)
}

func TestRecurseExpand(t *testing.T) {

	query := `
		{
			me(func: uid(32)) @recurse {
				expand(_all_)
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"school":[{"name":"San Mateo High School","district":[{"name":"San Mateo School District","county":[{"state":[{"name":"California","abbr":"CA"}],"name":"San Mateo County"}]}]}]}]}}`, js)
}

func TestRecurseExpandRepeatedPredError(t *testing.T) {

	query := `
		{
			me(func: uid(32)) @recurse {
				name
				expand(_all_)
			}
		}`

	ctx := defaultContext()
	_, err := processToFastJsonCtxVars(t, query, ctx, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Repeated subgraph: [name] while using expand()")
}

func TestRecurseQueryOrder(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) @recurse {
				friend(orderdesc: dob)
				dob
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"dob":"1910-01-01T00:00:00Z","friend":[{"dob":"1910-01-02T00:00:00Z","friend":[{"dob":"1910-01-01T00:00:00Z","name":"Michonne"}],"name":"Rick Grimes"},{"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"},{"dob":"1909-01-10T00:00:00Z","name":"Daryl Dixon"},{"dob":"1901-01-15T00:00:00Z","friend":[{"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"}],"name":"Andrea"}],"name":"Michonne"}]}}`,
		js)
}

func TestRecurseQueryAllowLoop(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) @recurse {
				friend
				dob
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"friend":[{"friend":[{"dob":"1910-01-01T00:00:00Z","name":"Michonne"}],"dob":"1910-01-02T00:00:00Z","name":"Rick Grimes"},{"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"},{"dob":"1909-01-10T00:00:00Z","name":"Daryl Dixon"},{"friend":[{"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"}],"dob":"1901-01-15T00:00:00Z","name":"Andrea"}],"dob":"1910-01-01T00:00:00Z","name":"Michonne"}]}}`, js)
}

func TestRecurseQueryAllowLoop2(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) @recurse(depth: 4,loop: true) {
				friend
				dob
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"friend":[{"friend":[{"friend":[{"dob":"1910-01-02T00:00:00Z","name":"Rick Grimes"},{"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"},{"dob":"1909-01-10T00:00:00Z","name":"Daryl Dixon"},{"dob":"1901-01-15T00:00:00Z","name":"Andrea"}],"dob":"1910-01-01T00:00:00Z","name":"Michonne"}],"dob":"1910-01-02T00:00:00Z","name":"Rick Grimes"},{"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"},{"dob":"1909-01-10T00:00:00Z","name":"Daryl Dixon"},{"friend":[{"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"}],"dob":"1901-01-15T00:00:00Z","name":"Andrea"}],"dob":"1910-01-01T00:00:00Z","name":"Michonne"}]}}`, js)
}

func TestRecurseQueryLimitDepth1(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) @recurse(depth: 2) {
				friend
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne", "friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}]}}`, js)
}

func TestRecurseQueryLimitDepth2(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) @recurse(depth: 2) {
				uid
				non_existent
				friend
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1","friend":[{"uid":"0x17","name":"Rick Grimes"},{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"},{"uid":"0x1f","name":"Andrea"},{"uid":"0x65"}],"name":"Michonne"}]}}`, js)
}

func TestRecurseVariable(t *testing.T) {

	query := `
			{
				var(func: uid(0x01)) @recurse {
					a as friend
				}

				me(func: uid(a)) {
					name
				}
			}
		`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}}`, js)
}

func TestRecurseVariableUid(t *testing.T) {

	query := `
			{
				var(func: uid(0x01)) @recurse {
					friend
					a as uid
				}

				me(func: uid(a)) {
					name
				}
			}
		`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}}`, js)
}

func TestRecurseVariableVar(t *testing.T) {

	query := `
			{
				var(func: uid(0x01)) @recurse {
					friend
					school
					a as name
				}

				me(func: uid(a)) {
					name
				}
			}
		`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"},{"name":"School A"},{"name":"School B"}]}}`, js)
}

func TestRecurseVariable2(t *testing.T) {

	query := `
			{

				var(func: uid(0x1)) @recurse {
					f2 as friend
					f as follow
				}

				me(func: uid(f)) {
					name
				}

				me2(func: uid(f2)) {
					name
				}
			}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Glenn Rhee"},{"name":"Andrea"},{"name":"Alice"},{"name":"Bob"},{"name":"Matt"},{"name":"John"}],"me2":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}}`, js)
}

func TestShortestPath_ExpandError(t *testing.T) {

	query := `
		{
			A as shortest(from:0x01, to:101) {
				expand(_all_)
			}

			me(func: uid( A)) {
				name
			}
		}`

	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestShortestPath_NoPath(t *testing.T) {

	query := `
		{
			A as shortest(from:0x01, to:101) {
				path
				follow
			}

			me(func: uid(A)) {
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestKShortestPath_NoPath(t *testing.T) {

	query := `
		{
			A as shortest(from:0x01, to:101, numpaths: 2) {
				path
				nonexistent_pred
				follow
			}

			me(func: uid(A)) {
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestKShortestPathWeighted(t *testing.T) {

	query := `
		{
			shortest(from: 1, to:1001, numpaths: 4) {
				path @facets(weight)
			}
		}`
	// We only get one path in this case as the facet is present only in one path.
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"_path_":[{"uid":"0x1","path":[{"uid":"0x1f","path":[{"uid":"0x3e8","path":[{"uid":"0x3e9","path|weight":0.100000}],"path|weight":0.100000}],"path|weight":0.100000}]}]}}`,
		js)
}

func TestKShortestPathWeighted_LimitDepth(t *testing.T) {

	query := `
		{
			shortest(from: 1, to:1001, depth:1, numpaths: 4) {
				path @facets(weight)
			}
		}`
	// We only get one path in this case as the facet is present only in one path.
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {}}`,
		js)
}

func TestKShortestPathWeighted1(t *testing.T) {

	query := `
		{
			shortest(from: 1, to:1003, numpaths: 3) {
				path @facets(weight)
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"_path_":[{"uid":"0x1","path":[{"uid":"0x1f","path":[{"uid":"0x3e8","path":[{"uid":"0x3e9","path":[{"uid":"0x3ea","path":[{"uid":"0x3eb","path|weight":0.600000}],"path|weight":0.100000}],"path|weight":0.100000}],"path|weight":0.100000}],"path|weight":0.100000}]},{"uid":"0x1","path":[{"uid":"0x1f","path":[{"uid":"0x3e8","path":[{"uid":"0x3ea","path":[{"uid":"0x3eb","path|weight":0.600000}],"path|weight":0.700000}],"path|weight":0.100000}],"path|weight":0.100000}]},{"uid":"0x1","path":[{"uid":"0x1f","path":[{"uid":"0x3e8","path":[{"uid":"0x3e9","path":[{"uid":"0x3eb","path|weight":1.500000}],"path|weight":0.100000}],"path|weight":0.100000}],"path|weight":0.100000}]}]}}`,
		js)
}

func TestTwoShortestPath(t *testing.T) {

	query := `
		{
			A as shortest(from: 1, to:1002, numpaths: 2) {
				path
			}

			me(func: uid( A)) {
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1","path":[{"uid":"0x1f","path":[{"uid":"0x3e8","path":[{"uid":"0x3ea"}]}]}]},{"uid":"0x1","path":[{"uid":"0x1f","path":[{"uid":"0x3e8","path":[{"uid":"0x3e9","path":[{"uid":"0x3ea"}]}]}]}]}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Alice"},{"name":"Matt"}]}}`,
		js)
}

func TestShortestPath(t *testing.T) {

	query := `
		{
			A as shortest(from:0x01, to:31) {
				friend
			}

			me(func: uid( A)) {
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1","friend":[{"uid":"0x1f"}]}],"me":[{"name":"Michonne"},{"name":"Andrea"}]}}`,
		js)
}

func TestShortestPathRev(t *testing.T) {

	query := `
		{
			A as shortest(from:23, to:1) {
				friend
			}

			me(func: uid( A)) {
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x17","friend":[{"uid":"0x1"}]}],"me":[{"name":"Rick Grimes"},{"name":"Michonne"}]}}`,
		js)
}

func TestFacetVarRetrieval(t *testing.T) {

	query := `
		{
			var(func: uid(1)) {
				path @facets(f as weight)
			}

			me(func: uid( 24)) {
				val(f)
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"val(f)":0.200000}]}}`,
		js)
}

func TestFacetVarRetrieveOrder(t *testing.T) {

	query := `
		{
			var(func: uid(1)) {
				path @facets(f as weight)
			}

			me(func: uid(f), orderasc: val(f)) {
				name
				nonexistent_pred
				val(f)
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea","val(f)":0.100000},{"name":"Glenn Rhee","val(f)":0.200000}]}}`,
		js)
}

func TestShortestPathWeightsMultiFacet_Error(t *testing.T) {

	query := `
		{
			A as shortest(from:1, to:1002) {
				path @facets(weight, weight1)
			}

			me(func: uid( A)) {
				name
			}
		}`

	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestShortestPathWeights(t *testing.T) {

	query := `
		{
			A as shortest(from:1, to:1002) {
				path @facets(weight)
			}

			me(func: uid( A)) {
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Alice"},{"name":"Bob"},{"name":"Matt"}],"_path_":[{"uid":"0x1","path":[{"uid":"0x1f","path":[{"uid":"0x3e8","path":[{"uid":"0x3e9","path":[{"uid":"0x3ea","path|weight":0.100000}],"path|weight":0.100000}],"path|weight":0.100000}],"path|weight":0.100000}]}]}}`,
		js)
}

func TestShortestPath2(t *testing.T) {

	query := `
		{
			A as shortest(from:0x01, to:1000) {
				path
			}

			me(func: uid( A)) {
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1","path":[{"uid":"0x1f","path":[{"uid":"0x3e8"}]}]}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Alice"}]}}
`,
		js)
}

func TestShortestPath4(t *testing.T) {

	query := `
		{
			A as shortest(from:1, to:1003) {
				path
				follow
			}

			me(func: uid( A)) {
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1","follow":[{"uid":"0x1f","follow":[{"uid":"0x3e9","follow":[{"uid":"0x3eb"}]}]}]}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Bob"},{"name":"John"}]}}`,
		js)
}

func TestShortestPath_filter(t *testing.T) {

	query := `
		{
			A as shortest(from:1, to:1002) {
				path @filter(not anyofterms(name, "alice"))
				follow
			}

			me(func: uid(A)) {
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1","follow":[{"uid":"0x1f","follow":[{"uid":"0x3e9","path":[{"uid":"0x3ea"}]}]}]}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Bob"},{"name":"Matt"}]}}`,
		js)
}

func TestShortestPath_filter2(t *testing.T) {

	query := `
		{
			A as shortest(from:1, to:1002) {
				path @filter(not anyofterms(name, "alice"))
				follow @filter(not anyofterms(name, "bob"))
			}

			me(func: uid(A)) {
				name
			}
		}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": { "me": []}}`, js)
}

func TestUseVarsFilterMultiId(t *testing.T) {

	query := `
		{
			var(func: uid(0x01)) {
				L as friend {
					friend
				}
			}

			var(func: uid(31)) {
				G as friend
			}

			friend(func:anyofterms(name, "Michonne Andrea Glenn")) @filter(uid(G, L)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"friend":[{"name":"Glenn Rhee"},{"name":"Andrea"}]}}`,
		js)
}

func TestUseVarsMultiFilterId(t *testing.T) {

	query := `
		{
			var(func: uid(0x01)) {
				L as friend
			}

			var(func: uid(31)) {
				G as friend
			}

			friend(func: uid(L)) @filter(uid(G)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"friend":[{"name":"Glenn Rhee"}]}}`,
		js)
}

func TestUseVarsCascade(t *testing.T) {

	query := `
		{
			var(func: uid(0x01)) @cascade {
				L as friend {
				  friend
				}
			}

			me(func: uid(L)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Rick Grimes"}, {"name":"Andrea"} ]}}`,
		js)
}

func TestUseVars(t *testing.T) {

	query := `
		{
			var(func: uid(0x01)) {
				L as friend
			}

			me(func: uid(L)) {
				name
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}}`,
		js)
}

func TestGetUIDCount(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				uid
				gender
				alive
				count(friend)
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1","alive":true,"count(friend)":5,"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestDebug1(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				alive
				count(friend)
			}
		}
	`

	ctx := context.WithValue(defaultContext(), DebugKey, "true")
	buf, _ := processToFastJsonCtxVars(t, query, ctx, nil)

	var mp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(buf), &mp))

	data := mp["data"].(map[string]interface{})
	resp := data["me"]
	uid := resp.([]interface{})[0].(map[string]interface{})["uid"].(string)
	require.EqualValues(t, "0x1", uid)
}

func TestDebug2(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				alive
				count(friend)
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	var mp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(js), &mp))

	resp := mp["data"].(map[string]interface{})["me"]
	uid, ok := resp.([]interface{})[0].(map[string]interface{})["uid"].(string)
	require.False(t, ok, "No uid expected but got one %s", uid)
}

func TestDebug3(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(func: uid(1, 24)) @filter(ge(dob, "1910-01-01")) {
				name
			}
		}
	`
	ctx := context.WithValue(defaultContext(), DebugKey, "true")
	buf, err := processToFastJsonCtxVars(t, query, ctx, nil)

	require.NoError(t, err)

	var mp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(buf), &mp))

	resp := mp["data"].(map[string]interface{})["me"]
	require.NotNil(t, resp)
	require.EqualValues(t, 1, len(resp.([]interface{})))
	uid := resp.([]interface{})[0].(map[string]interface{})["uid"].(string)
	require.EqualValues(t, "0x1", uid)
}

func TestCount(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				alive
				count(friend)
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":true,"count(friend)":5,"gender":"female","name":"Michonne"}]}}`,
		js)
}
func TestCountAlias(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				alive
				friendCount: count(friend)
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":true,"friendCount":5,"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestCountError1(t *testing.T) {
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(func: uid( 0x01)) {
				count(friend {
					name
				})
				name
				gender
				alive
			}
		}
	`
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
}

func TestCountError2(t *testing.T) {
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(func: uid( 0x01)) {
				count(friend {
					c {
						friend
					}
				})
				name
				gender
				alive
			}
		}
	`
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
}

func TestCountError3(t *testing.T) {
	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(func: uid( 0x01)) {
				count(friend
				name
				gender
				alive
			}
		}
	`
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
}

func TestMultiCountSort(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		f as var(func: anyofterms(name, "michonne rick andrea")) {
		 	n as count(friend)
		}

		countorder(func: uid(f), orderasc: val(n)) {
			name
			count(friend)
		}
	}
`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"countorder":[{"count(friend)":0,"name":"Andrea With no friends"},{"count(friend)":1,"name":"Rick Grimes"},{"count(friend)":1,"name":"Andrea"},{"count(friend)":5,"name":"Michonne"}]}}`,
		js)
}

func TestMultiLevelAgg(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		sumorder(func: anyofterms(name, "michonne rick andrea")) {
			name
			friend {
				s as count(friend)
			}
			sum(val(s))
		}
	}
`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"sumorder":[{"friend":[{"count(friend)":1},{"count(friend)":0},{"count(friend)":0},{"count(friend)":1},{"count(friend)":0}],"name":"Michonne","sum(val(s))":2},{"friend":[{"count(friend)":5}],"name":"Rick Grimes","sum(val(s))":5},{"friend":[{"count(friend)":0}],"name":"Andrea","sum(val(s))":0},{"name":"Andrea With no friends"}]}}`,
		js)
}

func TestMultiLevelAgg1(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		var(func: anyofterms(name, "michonne rick andrea")) @filter(gt(count(friend), 0)){
			friend {
				s as count(friend)
			}
			ss as sum(val(s))
		}

		sumorder(func: uid(ss), orderasc: val(ss)) {
			name
			val(ss)
		}
	}
`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"sumorder":[{"name":"Andrea","val(ss)":0},{"name":"Michonne","val(ss)":2},{"name":"Rick Grimes","val(ss)":5}]}}`,
		js)
}

func TestMultiLevelAgg1Error(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		var(func: anyofterms(name, "michonne rick andrea")) @filter(gt(count(friend), 0)){
			friend {
				s as count(friend)
				ss as sum(val(s))
			}
		}

		sumorder(func: uid(ss), orderasc: val(ss)) {
			name
			val(ss)
		}
	}
`
	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestMultiAggSort(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		f as var(func: anyofterms(name, "michonne rick andrea")) {
			name
			friend {
				x as dob
			}
			mindob as min(val(x))
			maxdob as max(val(x))
		}

		maxorder(func: uid(f), orderasc: val(maxdob)) {
			name
			val(maxdob)
		}

		minorder(func: uid(f), orderasc: val(mindob)) {
			name
			val(mindob)
		}
	}
`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"maxorder":[{"name":"Andrea","val(maxdob)":"1909-05-05T00:00:00Z"},{"name":"Rick Grimes","val(maxdob)":"1910-01-01T00:00:00Z"},{"name":"Michonne","val(maxdob)":"1910-01-02T00:00:00Z"}],"minorder":[{"name":"Michonne","val(mindob)":"1901-01-15T00:00:00Z"},{"name":"Andrea","val(mindob)":"1909-05-05T00:00:00Z"},{"name":"Rick Grimes","val(mindob)":"1910-01-01T00:00:00Z"}]}}`,
		js)
}

func TestMinMulti(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		me(func: anyofterms(name, "michonne rick andrea")) {
			name
			friend {
				x as dob
			}
			min(val(x))
			max(val(x))
		}
	}
`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"dob":"1910-01-02T00:00:00Z"},{"dob":"1909-05-05T00:00:00Z"},{"dob":"1909-01-10T00:00:00Z"},{"dob":"1901-01-15T00:00:00Z"}],"max(val(x))":"1910-01-02T00:00:00Z","min(val(x))":"1901-01-15T00:00:00Z","name":"Michonne"},{"friend":[{"dob":"1910-01-01T00:00:00Z"}],"max(val(x))":"1910-01-01T00:00:00Z","min(val(x))":"1910-01-01T00:00:00Z","name":"Rick Grimes"},{"friend":[{"dob":"1909-05-05T00:00:00Z"}],"max(val(x))":"1909-05-05T00:00:00Z","min(val(x))":"1909-05-05T00:00:00Z","name":"Andrea"},{"name":"Andrea With no friends"}]}}`,
		js)
}

func TestMinMultiAlias(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
	{
		me(func: anyofterms(name, "michonne rick andrea")) {
			name
			friend {
				x as dob
			}
			mindob: min(val(x))
			maxdob: max(val(x))
		}
	}
`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"dob":"1910-01-02T00:00:00Z"},{"dob":"1909-05-05T00:00:00Z"},{"dob":"1909-01-10T00:00:00Z"},{"dob":"1901-01-15T00:00:00Z"}],"maxdob":"1910-01-02T00:00:00Z","mindob":"1901-01-15T00:00:00Z","name":"Michonne"},{"friend":[{"dob":"1910-01-01T00:00:00Z"}],"maxdob":"1910-01-01T00:00:00Z","mindob":"1910-01-01T00:00:00Z","name":"Rick Grimes"},{"friend":[{"dob":"1909-05-05T00:00:00Z"}],"maxdob":"1909-05-05T00:00:00Z","mindob":"1909-05-05T00:00:00Z","name":"Andrea"},{"name":"Andrea With no friends"}]}}`,
		js)
}

func TestMinSchema(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
                {
                        me(func: uid(0x01)) {
                                name
                                gender
                                alive
                                friend {
																	x as survival_rate
                                }
																min(val(x))
                        }
                }
        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female","alive":true,"friend":[{"survival_rate":1.600000},{"survival_rate":1.600000},{"survival_rate":1.600000},{"survival_rate":1.600000}],"min(val(x))":1.600000}]}}`,
		js)

	schema.State().Set("survival_rate", pb.SchemaUpdate{ValueType: pb.Posting_ValType(types.IntID)})
	js = processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female","alive":true,"friend":[{"survival_rate":1},{"survival_rate":1},{"survival_rate":1},{"survival_rate":1}],"min(val(x))":1}]}}`,
		js)
	schema.State().Set("survival_rate", pb.SchemaUpdate{ValueType: pb.Posting_ValType(types.FloatID)})
}

func TestAvg(t *testing.T) {

	query := `
	{
		me(func: uid(0x01)) {
			name
			gender
			alive
			friend {
				x as shadow_deep
			}
			avg(val(x))
		}
	}
`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":true,"avg(val(x))":9.000000,"friend":[{"shadow_deep":4},{"shadow_deep":14}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestSum(t *testing.T) {

	query := `
                {
                        me(func: uid(0x01)) {
                                name
                                gender
                                alive
                                friend {
                                    x as shadow_deep
                                }
																sum(val(x))
                        }
                }
        `
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":true,"friend":[{"shadow_deep":4},{"shadow_deep":14}],"gender":"female","name":"Michonne","sum(val(x))":18}]}}`,
		js)
}

func TestQueryPassword(t *testing.T) {

	// Password is not fetchable
	query := `
                {
                        me(func: uid(0x01)) {
                                name
                                password
                        }
                }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"}]}}`, js)
}

func TestPasswordExpandAll1(t *testing.T) {
	// We ignore password in expand(_all_)
	query := `
    {
        me(func: uid(0x01)) {
			expand(_all_)
		}
    }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"path":[{"path|weight":0.200000},{"path|weight":0.100000,"path|weight1":0.200000}],"age":38,"full_name":"Michonne's large name for hashing","dob_day":"1910-01-01T00:00:00Z","_xid_":"mich","loc":{"type":"Point","coordinates":[1.1,2]},"address":"31, 32 street, Jupiter","graduation":["1932-01-01T00:00:00Z"],"dob":"1910-01-01T00:00:00Z","bin_data":"YmluLWRhdGE=","power":13.250000,"survival_rate":98.990000,"name":"Michonne","sword_present":"true","alive":true,"gender":"female","noindex_name":"Michonne's name not indexed"}]}}`, js)
}

func TestPasswordExpandAll2(t *testing.T) {
	query := `
    {
        me(func: uid(0x01)) {
			expand(_all_)
			checkpwd(password, "12345")
		}
    }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"_xid_":"mich","address":"31, 32 street, Jupiter","path":[{"path|weight":0.200000},{"path|weight":0.100000,"path|weight1":0.200000}],"sword_present":"true","dob_day":"1910-01-01T00:00:00Z","gender":"female","dob":"1910-01-01T00:00:00Z","survival_rate":98.990000,"noindex_name":"Michonne's name not indexed","name":"Michonne","graduation":["1932-01-01T00:00:00Z"],"bin_data":"YmluLWRhdGE=","loc":{"type":"Point","coordinates":[1.1,2]},"age":38,"full_name":"Michonne's large name for hashing","alive":true,"power":13.250000,"checkpwd(password)":false}]}}`, js)
}

func TestPasswordExpandError(t *testing.T) {
	query := `
    {
        me(func: uid(0x01)) {
			expand(_all_)
			password
		}
    }
	`

	_, err := processToFastJson(t, query)
	require.Contains(t, err.Error(), "Repeated subgraph: [password]")
}

func TestCheckPassword(t *testing.T) {
	query := `
                {
                        me(func: uid(0x01)) {
                                name
                                checkpwd(password, "123456")
                        }
                }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","checkpwd(password)":true}]}}`, js)
}

func TestCheckPasswordIncorrect(t *testing.T) {
	query := `
                {
                        me(func: uid(0x01)) {
                                name
                                checkpwd(password, "654123")
                        }
                }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","checkpwd(password)":false}]}}`, js)
}

// ensure, that old and deprecated form is not allowed
func TestCheckPasswordParseError(t *testing.T) {
	query := `
                {
                        me(func: uid(0x01)) {
                                name
                                checkpwd("654123")
                        }
                }
	`
	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestCheckPasswordDifferentAttr1(t *testing.T) {

	query := `
                {
                        me(func: uid(23)) {
                                name
                                checkpwd(pass, "654321")
                        }
                }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes","checkpwd(pass)":true}]}}`, js)
}

func TestCheckPasswordDifferentAttr2(t *testing.T) {

	query := `
                {
                        me(func: uid(23)) {
                                name
                                checkpwd(pass, "invalid")
                        }
                }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes","checkpwd(pass)":false}]}}`, js)
}

func TestCheckPasswordInvalidAttr(t *testing.T) {

	query := `
                {
                        me(func: uid(0x1)) {
                                name
                                checkpwd(pass, "123456")
                        }
                }
	`
	js := processToFastJsonNoErr(t, query)
	// for id:0x1 there is no pass attribute defined (there's only password attribute)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","checkpwd(pass)":false}]}}`, js)
}

// test for old version of checkpwd with hardcoded attribute name
func TestCheckPasswordQuery1(t *testing.T) {

	query := `
                {
                        me(func: uid(0x1)) {
                                name
                                password
                        }
                }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"}]}}`, js)
}

// test for improved version of checkpwd with custom attribute name
func TestCheckPasswordQuery2(t *testing.T) {

	query := `
                {
                        me(func: uid(23)) {
                                name
                                pass
                        }
                }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"}]}}`, js)
}

// test for improved version of checkpwd with alias for unknown attribute
func TestCheckPasswordQuery3(t *testing.T) {

	query := `
                {
                        me(func: uid(23)) {
                                name
																secret: checkpwd(pass, "123456")
                        }
                }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes","secret":false}]}}`, js)
}

// test for improved version of checkpwd with alias for known attribute
func TestCheckPasswordQuery4(t *testing.T) {

	query := `
                {
                        me(func: uid(0x01)) {
                                name
																secreto: checkpwd(password, "123456")
                        }
                }
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne","secreto":true}]}}`, js)
}

func TestToSubgraphInvalidFnName(t *testing.T) {
	query := `
                {
                        me(func:invalidfn1(name, "some cool name")) {
                                name
                                gender
                                alive
                        }
                }
        `
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Function name: invalidfn1 is not valid.")
}

func TestToSubgraphInvalidFnName2(t *testing.T) {
	query := `
                {
                        me(func:anyofterms(name, "some cool name")) {
                                name
                                friend @filter(invalidfn2(name, "some name")) {
                                       name
                                }
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ToSubGraph(ctx, res.Query[0])
	require.Error(t, err)
}

func TestToSubgraphInvalidFnName3(t *testing.T) {
	query := `
                {
                        me(func:anyofterms(name, "some cool name")) {
                                name
                                friend @filter(anyofterms(name, "Andrea") or
                                               invalidfn3(name, "Andrea Rhee")){
                                        name
                                }
                        }
                }
        `
	res, err := gql.Parse(gql.Request{Str: query})
	require.NoError(t, err)

	ctx := context.Background()
	_, err = ToSubGraph(ctx, res.Query[0])
	require.Error(t, err)
}

func TestToSubgraphInvalidFnName4(t *testing.T) {
	query := `
                {
                        f as var(func:invalidfn4(name, "Michonne Rick Glenn")) {
                                name
                        }
                        you(func:anyofterms(name, "Michonne")) {
                                friend @filter(uid(f)) {
                                        name
                                }
                        }
                }
        `
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Function name: invalidfn4 is not valid.")
}

func TestToSubgraphInvalidArgs1(t *testing.T) {
	query := `
                {
                        me(func: uid(0x01)) {
                                name
                                gender
                                friend(disorderasc: dob) @filter(le(dob, "1909-03-20")) {
                                        name
                                }
                        }
                }
        `
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got invalid keyword: disorderasc")
}

func TestToSubgraphInvalidArgs2(t *testing.T) {
	query := `
                {
                        me(func: uid(0x01)) {
                                name
                                gender
                                friend(offset:1, invalidorderasc:1) @filter(anyofterms(name, "Andrea")) {
                                        name
                                }
                        }
                }
        `
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Got invalid keyword: invalidorderasc")
}

func TestToFastJSON(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				alive
				friend {
					name
				}
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":true,"friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestFieldAlias(t *testing.T) {

	// Alright. Now we have everything set up. Let's create the query.
	query := `
		{
			me(func: uid(0x01)) {
				MyName:name
				gender
				alive
				Buddies:friend {
					BudName:name
				}
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"alive":true,"Buddies":[{"BudName":"Rick Grimes"},{"BudName":"Glenn Rhee"},{"BudName":"Daryl Dixon"},{"BudName":"Andrea"}],"gender":"female","MyName":"Michonne"}]}}`,
		js)
}

func TestToFastJSONFilter(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea SomethingElse")) {
					name
				}
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female","friend":[{"name":"Andrea"}]}]}}`,
		js)
}

func TestToFastJSONFilterMissBrac(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(anyofterms(name, "Andrea SomethingElse") {
					name
				}
			}
		}
	`
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
}

func TestToFastJSONFilterallofterms(t *testing.T) {

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(allofterms(name, "Andrea SomethingElse")) {
					name
				}
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female"}]}}`, js)
}

func TestInvalidStringIndex(t *testing.T) {
	// no FTS index defined for name

	query := `
		{
			me(func: uid(0x01)) {
				name
				gender
				friend @filter(alloftext(name, "Andrea SomethingElse")) {
					name
				}
			}
		}
	`

	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestValidFullTextIndex(t *testing.T) {
	// no FTS index defined for name

	query := `
		{
			me(func: uid(0x01)) {
				name
				friend @filter(alloftext(alias, "BOB")) {
					alias
				}
			}
		}
	`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne", "friend":[{"alias":"Bob Joe"}]}]}}`, js)
}

// dob (date of birth) is not a string
func TestFilterRegexError(t *testing.T) {

	query := `
    {
      me(func: uid(0x01)) {
        name
        friend @filter(regexp(dob, /^[a-z A-Z]+$/)) {
          name
        }
      }
    }
`

	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestFilterRegex1(t *testing.T) {

	query := `
    {
      me(func: uid(0x01)) {
        name
        friend @filter(regexp(name, /^[a-z A-Z]+$/)) {
          name
        }
      }
    }
`

	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestFilterRegex2(t *testing.T) {

	query := `
    {
      me(func: uid(0x01)) {
        name
        friend @filter(regexp(name, /^[^ao]+$/)) {
          name
        }
      }
    }
`

	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

func TestFilterRegex3(t *testing.T) {

	query := `
    {
      me(func: uid(0x01)) {
        name
        friend @filter(regexp(name, /^Rick/)) {
          name
        }
      }
    }
`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne", "friend":[{"name":"Rick Grimes"}]}]}}`, js)
}

func TestFilterRegex4(t *testing.T) {

	query := `
    {
      me(func: uid(0x01)) {
        name
        friend @filter(regexp(name, /((en)|(xo))n/)) {
          name
        }
      }
    }
`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne", "friend":[{"name":"Glenn Rhee"},{"name":"Daryl Dixon"} ]}]}}`, js)
}

func TestFilterRegex5(t *testing.T) {

	query := `
    {
      me(func: uid(0x01)) {
        name
        friend @filter(regexp(name, /^[a-zA-z]*[^Kk ]?[Nn]ight/)) {
          name
        }
      }
    }
`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne"}]}}`, js)
}

func TestFilterRegex6(t *testing.T) {
	query := `
    {
	  me(func: uid(0x1234)) {
		pattern @filter(regexp(value, /miss((issippi)|(ouri))/)) {
			value
		}
      }
    }
`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"pattern":[{"value":"mississippi"}, {"value":"missouri"}]}]}}`, js)
}

func TestFilterRegex7(t *testing.T) {

	query := `
    {
	  me(func: uid(0x1234)) {
		pattern @filter(regexp(value, /[aeiou]mission/)) {
			value
		}
      }
    }
`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"pattern":[{"value":"omission"}, {"value":"dimission"}]}]}}`, js)
}

func TestFilterRegex8(t *testing.T) {

	query := `
    {
	  me(func: uid(0x1234)) {
		pattern @filter(regexp(value, /^(trans)?mission/)) {
			value
		}
      }
    }
`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"pattern":[{"value":"mission"}, {"value":"missionary"}, {"value":"transmission"}]}]}}`, js)
}

func TestFilterRegex9(t *testing.T) {

	query := `
    {
	  me(func: uid(0x1234)) {
		pattern @filter(regexp(value, /s.{2,5}mission/)) {
			value
		}
      }
    }
`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"pattern":[{"value":"submission"}, {"value":"subcommission"}, {"value":"discommission"}]}]}}`, js)
}

func TestFilterRegex10(t *testing.T) {

	query := `
    {
	  me(func: uid(0x1234)) {
		pattern @filter(regexp(value, /[^m]iss/)) {
			value
		}
      }
    }
`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"pattern":[{"value":"mississippi"}, {"value":"whissle"}]}]}}`, js)
}

func TestFilterRegex11(t *testing.T) {

	query := `
    {
	  me(func: uid(0x1234)) {
		pattern @filter(regexp(value, /SUB[cm]/i)) {
			value
		}
      }
    }
`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"pattern":[{"value":"submission"}, {"value":"subcommission"}]}]}}`, js)
}

// case insensitive mode may be turned on with modifier:
// http://www.regular-expressions.info/modifiers.html - this is completely legal
func TestFilterRegex12(t *testing.T) {

	query := `
    {
	  me(func: uid(0x1234)) {
		pattern @filter(regexp(value, /(?i)SUB[cm]/)) {
			value
		}
      }
    }
`

	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"pattern":[{"value":"submission"}, {"value":"subcommission"}]}]}}`, js)
}

// case insensitive mode may be turned on and off with modifier:
// http://www.regular-expressions.info/modifiers.html - this is completely legal
func TestFilterRegex13(t *testing.T) {

	query := `
    {
	  me(func: uid(0x1234)) {
		pattern @filter(regexp(value, /(?i)SUB[cm](?-i)ISSION/)) {
			value
		}
      }
    }
`

	// no results are returned, becaues case insensive mode is turned off before 'ISSION'
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

// invalid regexp modifier
func TestFilterRegex14(t *testing.T) {

	query := `
    {
	  me(func: uid(0x1234)) {
		pattern @filter(regexp(value, /pattern/x)) {
			value
		}
      }
    }
`

	_, err := processToFastJson(t, query)
	require.Error(t, err)
}

// multi-lang - simple
func TestFilterRegex15(t *testing.T) {

	query := `
		{
			me(func:regexp(name@ru, /Барсук/)) {
				name@ru
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@ru":"Барсук"}]}}`,
		js)
}

// multi-lang - test for bug (#945) - multi-byte runes
func TestFilterRegex16(t *testing.T) {

	query := `
		{
			me(func:regexp(name@ru, /^артём/i)) {
				name@ru
			}
		}
	`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@ru":"Артём Ткаченко"}]}}`,
		js)
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
	js := processToFastJsonNoErr(t, query)
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
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": []}}`, js)
}

func TestNoResultsPagination(t *testing.T) {
	query := `{
		q(func: has(nonexistent_pred), first: 50) {
			uid
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": []}}`, js)
}

func TestNoResultsGroupBy(t *testing.T) {
	query := `{
		q(func: has(nonexistent_pred)) @groupby(name) {
			count(uid)
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {}}`, js)
}

func TestNoResultsOrder(t *testing.T) {
	query := `{
		q(func: has(nonexistent_pred), orderasc: name) {
			uid
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": []}}`, js)
}

func TestNoResultsCount(t *testing.T) {
	query := `{
		q(func: has(nonexistent_pred)) {
			uid
			count(friend)
		}
	}`
	js := processToFastJsonNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": []}}`, js)
}
