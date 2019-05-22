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
	"os"
	"testing"

	context "golang.org/x/net/context"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/z"
)

func TestGetUID(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				name
				uid
				gender
				alive
				friend {
					uid
					name
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1","alive":true,"friend":[{"uid":"0x17","name":"Rick Grimes"},{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"},{"uid":"0x1f","name":"Andrea"},{"uid":"0x65"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestQueryEmptyDefaultNames(t *testing.T) {
	query := `{
	  people(func: eq(name, "")) {
		uid
		name
	  }
	}`
	js := processQueryNoErr(t, query)
	// only two empty names should be retrieved as the other one is empty in a particular lang.
	require.JSONEq(t,
		`{"data":{"people": [{"uid":"0xdac","name":""}, {"uid":"0xdae","name":""}]}}`,
		js)
}

func TestQueryEmptyDefaultNameWithLanguage(t *testing.T) {
	query := `{
	  people(func: eq(name, "")) {
		name@ko:en:hi
	  }
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"people": [{"name@ko:en:hi":"상현"},{"name@ko:en:hi":"Amit"}]}}`,
		js)
}

func TestQueryNamesThatAreEmptyInLanguage(t *testing.T) {
	query := `{
	  people(func: eq(name@hi, "")) {
		name@en
	  }
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"people": [{"name@en":"Andrew"}]}}`,
		js)
}

func TestQueryNamesInLanguage(t *testing.T) {
	query := `{
	  people(func: eq(name@hi, "अमित")) {
		name@en
	  }
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"people": [{"name@en":"Amit"}]}}`,
		js)
}

func TestQueryAllLanguages(t *testing.T) {
	query := `{
	  people(func: eq(name@hi, "अमित")) {
		name@*
	  }
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"people": [{"name@en":"Amit", "name@hi":"अमित", "name":""}]}}`,
		js)
}

func TestQueryNamesBeforeA(t *testing.T) {
	query := `{
	  people(func: lt(name, "A")) {
		uid
		name
	  }
	}`
	js := processQueryNoErr(t, query)
	// only two empty names should be retrieved as the other one is empty in a particular lang.
	require.JSONEq(t,
		`{"data":{"people": [{"uid":"0xdac", "name":""}, {"uid":"0xdae", "name":""}]}}`,
		js)
}

func TestQueryNamesCompareEmpty(t *testing.T) {
	tests := []struct {
		in, out string
	}{
		{in: `{q(func: lt(name, "")) { name }}`,
			out: `{"data":{"q": []}}`},
		{in: `{q(func: le(name, "")) { uid name }}`,
			out: `{"data":{"q": [{"uid":"0xdac", "name":""}, {"uid":"0xdae", "name":""}]}}`},
		{in: `{q(func: gt(name, ""), first:3) { name }}`,
			out: `{"data":{"q": [{"name":"Michonne"}, {"name":"King Lear"}, {"name":"Margaret"}]}}`},
		{in: `{q(func: ge(name, ""), first:3, after:0x91d) { name }}`,
			out: `{"data":{"q": [{"name":""}, {"name":"Alex"}, {"name":""}]}}`},
	}
	for _, tc := range tests {
		js := processQueryNoErr(t, tc.in)
		require.JSONEq(t, tc.out, js)
	}
}

func TestQueryCountEmptyNames(t *testing.T) {
	tests := []struct {
		in, out, failure string
	}{
		{in: `{q(func: has(name)) @filter(eq(name, "")) {count(uid)}}`,
			out: `{"data":{"q": [{"count":2}]}}`},
		{in: `{q(func: has(name)) @filter(gt(name, "")) {count(uid)}}`,
			out: `{"data":{"q": [{"count":46}]}}`},
		{in: `{q(func: has(name)) @filter(ge(name, "")) {count(uid)}}`,
			out: `{"data":{"q": [{"count":48}]}}`},
		{in: `{q(func: has(name)) @filter(lt(name, "")) {count(uid)}}`,
			out: `{"data":{"q": [{"count":0}]}}`},
		{in: `{q(func: has(name)) @filter(le(name, "")) {count(uid)}}`,
			out: `{"data":{"q": [{"count":2}]}}`},
		{in: `{q(func: has(name)) @filter(anyofterms(name, "")) {count(uid)}}`,
			out: `{"data":{"q": [{"count":2}]}}`},
		{in: `{q(func: has(name)) @filter(allofterms(name, "")) {count(uid)}}`,
			out: `{"data":{"q": [{"count":2}]}}`},
		// NOTE: match with empty string filters values greater than the max distance.
		{in: `{q(func: has(name)) @filter(match(name, "", 8)) {count(uid)}}`,
			out: `{"data":{"q": [{"count":28}]}}`},
		{in: `{q(func: has(name)) @filter(uid_in(name, "")) {count(uid)}}`,
			failure: `Value "" in uid_in is not a number`},
	}
	for _, tc := range tests {
		js, err := processQuery(context.Background(), t, tc.in)
		if tc.failure != "" {
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.failure)
		} else {
			require.NoError(t, err)
			require.JSONEq(t, tc.out, js)
		}
	}
}

func TestQueryEmptyRoomsWithTermIndex(t *testing.T) {
	query := `{
		  offices(func: has(office)) {
			count(office.room @filter(eq(room, "")))
		  }
		}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"offices": [{"count(office.room)":1}]}}`,
		js)
}

func TestQueryCountEmptyNamesWithLang(t *testing.T) {
	query := `{
	  people_empty_name(func: has(name@hi)) @filter(eq(name@hi, "")) {
		count(uid)
	  }
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"people_empty_name": [{"count":1}]}}`,
		js)
}

func TestStocksStartsWithAInPortfolio(t *testing.T) {
	query := `{
	  portfolio(func: lt(symbol, "B")) {
		symbol
	  }
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"portfolio": [{"symbol":"AAPL"},{"symbol":"AMZN"},{"symbol":"AMD"}]}}`,
		js)
}

func TestFindFriendsWhoAreBetween15And19(t *testing.T) {
	query := `{
	  friends_15_and_19(func: uid(1)) {
		name
		friend @filter(ge(age, 15) AND lt(age, 19)) {
			name
			age
	    }
      }
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friends_15_and_19":[{"name":"Michonne","friend":[{"name":"Rick Grimes","age":15},{"name":"Glenn Rhee","age":15},{"name":"Daryl Dixon","age":17}]}]}}`,
		js)
}

func TestGetNonListUidPredicate(t *testing.T) {
	query := `
		{
			me(func: uid(0x02)) {
				uid
				best_friend {
					uid
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x2", "best_friend": {"uid": "0x40"}}]}}`,
		js)
}

func TestNonListUidPredicateReverse1(t *testing.T) {
	query := `
		{
			me(func: uid(0x40)) {
				uid
				~best_friend {
					uid
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x40", "~best_friend": [{"uid":"0x2"},{"uid":"0x3"},{"uid":"0x4"}]}]}}`,
		js)
}

func TestNonListUidPredicateReverse2(t *testing.T) {
	query := `
		{
			me(func: uid(0x40)) {
				uid
				~best_friend {
					pet {
						name
					}
					uid
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x40", "~best_friend": [
			{"uid":"0x2","pet":[{"name":"Garfield"}]},
			{"uid":"0x3","pet":[{"name":"Bear"}]},
			{"uid":"0x4","pet":[{"name":"Nemo"}]}]}]}}`,
		js)
}

func TestGeAge(t *testing.T) {
	query := `{
		  senior_citizens(func: ge(age, 75)) {
			name
			age
		  }
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"senior_citizens": [{"name":"Elizabeth", "age":75}, {"name":"Alice", "age":75}, {"age":75, "name":"Bob"}, {"name":"Alice", "age":75}]}}`,
		js)
}

func TestGtAge(t *testing.T) {
	query := `
    {
			senior_citizens(func: gt(age, 75)) {
				name
				age
			}
    }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"senior_citizens":[]}}`, js)
}

func TestLeAge(t *testing.T) {
	query := `{
		  minors(func: le(age, 15)) {
			name
			age
		  }
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"minors": [{"name":"Rick Grimes", "age":15}, {"name":"Glenn Rhee", "age":15}]}}`,
		js)
}

func TestLtAge(t *testing.T) {
	query := `
    {
			minors(func: Lt(age, 15)) {
				name
				age
			}
    }`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"minors":[]}}`, js)
}

func TestGetUIDInDebugMode(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				name
				uid
				gender
				alive
				friend {
					uid
					name
				}
			}
		}
	`

	ctx := context.Background()
	ctx = context.WithValue(ctx, DebugKey, "true")
	js, err := processQuery(ctx, t, query)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1","alive":true,"friend":[{"uid":"0x17","name":"Rick Grimes"},{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"},{"uid":"0x1f","name":"Andrea"},{"uid":"0x65"}],"gender":"female","name":"Michonne"}]}}`,
		js)

}

func TestReturnUids(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				name
				uid
				gender
				alive
				friend {
					uid
					name
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1","alive":true,"friend":[{"uid":"0x17","name":"Rick Grimes"},{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"},{"uid":"0x1f","name":"Andrea"},{"uid":"0x65"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestGetUIDNotInChild(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) {
				name
				uid
				gender
				alive
				friend {
					name
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x1","alive":true,"gender":"female","name":"Michonne", "friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}]}}`,
		js)
}

func TestCascadeDirective(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) @cascade {
				name
				gender
				friend {
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"friend":[{"friend":[{"age":38,"dob":"1910-01-01T00:00:00Z","name":"Michonne"}],"name":"Rick Grimes"},{"friend":[{"age":15,"dob":"1909-05-05T00:00:00Z","name":"Glenn Rhee"}],"name":"Andrea"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func TestLevelBasedFacetVarAggSum(t *testing.T) {
	query := `
		{
			friend(func: uid(1000)) {
				path @facets(L1 as weight)
				sumw: sum(val(L1))
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"path":[{"path|weight":0.100000},{"path|weight":0.700000}],"sumw":0.800000}]}}`,
		js)
}

func TestLevelBasedFacetVarSum(t *testing.T) {
	query := `
		{
			friend(func: uid(1000)) {
				path @facets(L1 as weight) {
						path @facets(L2 as weight) {
							c as count(follow)
							L4 as math(c+L2+L1)
						}
				}
			}

			sum(func: uid(L4), orderdesc: val(L4)) {
				name
				val(L4)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"friend":[{"path":[{"path":[{"count(follow)":1,"val(L4)":1.200000,"path|weight":0.100000},{"count(follow)":1,"val(L4)":3.900000,"path|weight":1.500000}],"path|weight":0.100000},{"path":[{"count(follow)":1,"val(L4)":3.900000,"path|weight":0.600000}],"path|weight":0.700000}]}],"sum":[{"name":"John","val(L4)":3.900000},{"name":"Matt","val(L4)":1.200000}]}}`,
		js)
}

func TestLevelBasedSumMix1(t *testing.T) {
	query := `
		{
			friend(func: uid( 1)) {
				a as age
				path @facets(L1 as weight) {
					L2 as math(a+L1)
			 	}
			}
			sum(func: uid(L2), orderdesc: val(L2)) {
				name
				val(L2)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"age":38,"path":[{"val(L2)":38.200000,"path|weight":0.200000},{"val(L2)":38.100000,"path|weight":0.100000}]}],"sum":[{"name":"Glenn Rhee","val(L2)":38.200000},{"name":"Andrea","val(L2)":38.100000}]}}`,
		js)
}

func TestLevelBasedFacetVarSum1(t *testing.T) {
	query := `
		{
			friend(func: uid( 1000)) {
				path @facets(L1 as weight) {
					name
					path @facets(L2 as weight) {
						L3 as math(L1+L2)
					}
			 }
			}
			sum(func: uid(L3), orderdesc: val(L3)) {
				name
				val(L3)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"path":[{"name":"Bob","path":[{"val(L3)":0.200000,"path|weight":0.100000},{"val(L3)":2.900000,"path|weight":1.500000}],"path|weight":0.100000},{"name":"Matt","path":[{"val(L3)":2.900000,"path|weight":0.600000}],"path|weight":0.700000}]}],"sum":[{"name":"John","val(L3)":2.900000},{"name":"Matt","val(L3)":0.200000}]}}`,
		js)
}

func TestLevelBasedFacetVarSum2(t *testing.T) {
	query := `
		{
			friend(func: uid( 1000)) {
				path @facets(L1 as weight) {
					path @facets(L2 as weight) {
						path @facets(L3 as weight) {
							L4 as math(L1+L2+L3)
						}
					}
				}
			}
			sum(func: uid(L4), orderdesc: val(L4)) {
				name
				val(L4)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"path":[{"path":[{"path":[{"val(L4)":0.800000,"path|weight":0.600000}],"path|weight":0.100000},{"path":[{"val(L4)":2.900000}],"path|weight":1.500000}],"path|weight":0.100000},{"path":[{"path":[{"val(L4)":2.900000}],"path|weight":0.600000}],"path|weight":0.700000}]}],"sum":[{"name":"Bob","val(L4)":2.900000},{"name":"John","val(L4)":0.800000}]}}`,
		js)
}

func TestQueryConstMathVal(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Rick Michonne Andrea")) {
				a as math(24/8 * 3)
			}

			AgeOrder(func: uid(f)) {
				name
				val(a)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"AgeOrder":[{"name":"Michonne","val(a)":9.000000},{"name":"Rick Grimes","val(a)":9.000000},{"name":"Andrea","val(a)":9.000000},{"name":"Andrea With no friends","val(a)":9.000000}]}}`,
		js)
}

func TestQueryVarValAggSince(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as dob
				b as math(since(a)/(60*60*24*365))
			}

			AgeOrder(func: uid(f), orderasc: val(b)) {
				name
				val(a)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"AgeOrder":[{"name":"Rick Grimes","val(a)":"1910-01-02T00:00:00Z"},{"name":"Michonne","val(a)":"1910-01-01T00:00:00Z"},{"name":"Andrea","val(a)":"1901-01-15T00:00:00Z"}]}}`,
		js)
}

func TestQueryVarValAggNestedFuncConst(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				p as math(a + s % n + 10)
				q as math(a * s * n * -1)
			}

			MaxMe(func: uid(f), orderasc: val(p)) {
				name
				val(p)
				val(a)
				val(n)
				val(s)
			}

			MinMe(func: uid(f), orderasc: val(q)) {
				name
				val(q)
				val(a)
				val(n)
				val(s)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"MaxMe":[{"name":"Rick Grimes","val(a)":15,"val(n)":38,"val(p)":25.000000,"val(s)":38},{"name":"Andrea","val(a)":19,"val(n)":15,"val(p)":29.000000,"val(s)":15},{"name":"Michonne","val(a)":38,"val(n)":15,"val(p)":52.000000,"val(s)":19}],"MinMe":[{"name":"Rick Grimes","val(a)":15,"val(n)":38,"val(q)":-21660.000000,"val(s)":38},{"name":"Michonne","val(a)":38,"val(n)":15,"val(q)":-10830.000000,"val(s)":19},{"name":"Andrea","val(a)":19,"val(n)":15,"val(q)":-4275.000000,"val(s)":15}]}}`,
		js)
}

func TestQueryVarValAggNestedFuncMinMaxVars(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				p as math(max(max(a, s), n))
				q as math(min(min(a, s), n))
			}

			MaxMe(func: uid(f), orderasc: val(p)) {
				name
				val(p)
				val(a)
				val(n)
				val(s)
			}

			MinMe(func: uid(f), orderasc: val(q)) {
				name
				val(q)
				val(a)
				val(n)
				val(s)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"MinMe":[{"name":"Michonne","val(a)":38,"val(n)":15,"val(q)":15,"val(s)":19},{"name":"Rick Grimes","val(a)":15,"val(n)":38,"val(q)":15,"val(s)":38},{"name":"Andrea","val(a)":19,"val(n)":15,"val(q)":15,"val(s)":15}],"MaxMe":[{"name":"Andrea","val(a)":19,"val(n)":15,"val(p)":19,"val(s)":15},{"name":"Michonne","val(a)":38,"val(n)":15,"val(p)":38,"val(s)":19},{"name":"Rick Grimes","val(a)":15,"val(n)":38,"val(p)":38,"val(s)":38}]}}`,
		js)
}

func TestQueryVarValAggNestedFuncConditional(t *testing.T) {
	query := `
	{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				condLog as math(cond(a > 10, logbase(n, 5), 1))
				condExp as math(cond(a < 40, 1, pow(2, n)))
			}

			LogMe(func: uid(f), orderasc: val(condLog)) {
				name
				val(condLog)
				val(n)
				val(a)
			}

			ExpMe(func: uid(f), orderasc: val(condExp)) {
				name
				val(condExp)
				val(n)
				val(a)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"ExpMe":[{"name":"Michonne","val(a)":38,"val(condExp)":1.000000,"val(n)":15},{"name":"Rick Grimes","val(a)":15,"val(condExp)":1.000000,"val(n)":38},{"name":"Andrea","val(a)":19,"val(condExp)":1.000000,"val(n)":15}],"LogMe":[{"name":"Michonne","val(a)":38,"val(condLog)":1.682606,"val(n)":15},{"name":"Andrea","val(a)":19,"val(condLog)":1.682606,"val(n)":15},{"name":"Rick Grimes","val(a)":15,"val(condLog)":2.260159,"val(n)":38}]}}`,
		js)
}

func TestQueryVarValAggNestedFuncConditional2(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				condLog as math(cond(a==38, n/2, 1))
				condExp as math(cond(a!=38, 1, sqrt(2*n)))
			}

			LogMe(func: uid(f), orderasc: val(condLog)) {
				name
				val(condLog)
				val(n)
				val(a)
			}

			ExpMe(func: uid(f), orderasc: val(condExp)) {
				name
				val(condExp)
				val(n)
				val(a)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"ExpMe":[{"name":"Rick Grimes","val(a)":15,"val(condExp)":1.000000,"val(n)":38},{"name":"Andrea","val(a)":19,"val(condExp)":1.000000,"val(n)":15},{"name":"Michonne","val(a)":38,"val(condExp)":5.477226,"val(n)":15}],"LogMe":[{"name":"Rick Grimes","val(a)":15,"val(condLog)":1.000000,"val(n)":38},{"name":"Andrea","val(a)":19,"val(condLog)":1.000000,"val(n)":15},{"name":"Michonne","val(a)":38,"val(condLog)":7.500000,"val(n)":15}]}}`,
		js)
}

func TestQueryVarValAggNestedFuncUnary(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				combiLog as math(a + ln(s - n))
				combiExp as math(a + exp(s - n))
			}

			LogMe(func: uid(f), orderasc: val(combiLog)) {
				name
				val(combiLog)
				val(a)
				val(n)
				val(s)
			}

			ExpMe(func: uid(f), orderasc: val(combiExp)) {
				name
				val(combiExp)
				val(a)
				val(n)
				val(s)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"ExpMe":[{"name":"Rick Grimes","val(a)":15,"val(combiExp)":16.000000,"val(n)":38,"val(s)":38},{"name":"Andrea","val(a)":19,"val(combiExp)":20.000000,"val(n)":15,"val(s)":15},{"name":"Michonne","val(a)":38,"val(combiExp)":92.598150,"val(n)":15,"val(s)":19}],"LogMe":[{"name":"Rick Grimes","val(a)":15,"val(combiLog)":-179769313486231570814527423731704356798070567525844996598917476803157260780028538760589558632766878171540458953514382464234321326889464182768467546703537516986049910576551282076245490090389328944075868508455133942304583236903222948165808559332123348274797826204144723168738177180919299881250404026184124858368.000000,"val(n)":38,"val(s)":38},{"name":"Andrea","val(a)":19,"val(combiLog)":-179769313486231570814527423731704356798070567525844996598917476803157260780028538760589558632766878171540458953514382464234321326889464182768467546703537516986049910576551282076245490090389328944075868508455133942304583236903222948165808559332123348274797826204144723168738177180919299881250404026184124858368.000000,"val(n)":15,"val(s)":15},{"name":"Michonne","val(a)":38,"val(combiLog)":39.386294,"val(n)":15,"val(s)":19}]}}`,
		js)
}

func TestQueryVarValAggNestedFunc(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				combi as math(a + n * s)
			}

			me(func: uid(f), orderasc: val(combi)) {
				name
				val(combi)
				val(a)
				val(n)
				val(s)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea","val(a)":19,"val(combi)":244,"val(n)":15,"val(s)":15},{"name":"Michonne","val(a)":38,"val(combi)":323,"val(n)":15,"val(s)":19},{"name":"Rick Grimes","val(a)":15,"val(combi)":1459,"val(n)":38,"val(s)":38}]}}`,
		js)
}

func TestQueryVarValAggMinMaxSelf(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				a as age
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				sum as math(n +  a + s)
			}

			me(func: uid(f), orderasc: val(sum)) {
				name
				val(sum)
				val(s)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea","val(s)":15,"val(sum)":49},{"name":"Michonne","val(s)":19,"val(sum)":72},{"name":"Rick Grimes","val(s)":38,"val(sum)":91}]}}`,
		js)
}

func TestQueryVarValAggMinMax(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				sum as math(n + s)
			}

			me(func: uid(f), orderdesc: val(sum)) {
				name
				val(n)
				val(s)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Rick Grimes","val(n)":38,"val(s)":38},{"name":"Michonne","val(n)":15,"val(s)":19},{"name":"Andrea","val(n)":15,"val(s)":15}]}}`,
		js)
}

func TestQueryVarValAggMinMaxAlias(t *testing.T) {
	query := `
		{
			f as var(func: anyofterms(name, "Michonne Andrea Rick")) {
				friend {
					x as age
				}
				n as min(val(x))
				s as max(val(x))
				sum as math(n + s)
			}

			me(func: uid(f), orderdesc: val(sum)) {
				name
				MinAge: val(n)
				MaxAge: val(s)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Rick Grimes","MinAge":38,"MaxAge":38},{"name":"Michonne","MinAge":15,"MaxAge":19},{"name":"Andrea","MinAge":15,"MaxAge":15}]}}`,
		js)
}

func TestQueryVarValAggMul(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				f as friend {
					n as age
					s as count(friend)
					mul as math(n * s)
				}
			}

			me(func: uid(f), orderdesc: val(mul)) {
				name
				val(s)
				val(n)
				val(mul)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea","val(mul)":19.000000,"val(n)":19,"val(s)":1},{"name":"Rick Grimes","val(mul)":15.000000,"val(n)":15,"val(s)":1},{"name":"Glenn Rhee","val(mul)":0.000000,"val(n)":15,"val(s)":0},{"name":"Daryl Dixon","val(mul)":0.000000,"val(n)":17,"val(s)":0},{"val(mul)":0.000000,"val(s)":0}]}}`,
		js)
}

func TestQueryVarValAggOrderDesc(t *testing.T) {
	query := `
		{
			info(func: uid( 1)) {
				f as friend {
					n as age
					s as count(friend)
					sum as math(n + s)
				}
			}

			me(func: uid(f), orderdesc: val(sum)) {
				name
				age
				count(friend)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"info":[{"friend":[{"age":15,"count(friend)":1,"val(sum)":16.000000},{"age":15,"count(friend)":0,"val(sum)":15.000000},{"age":17,"count(friend)":0,"val(sum)":17.000000},{"age":19,"count(friend)":1,"val(sum)":20.000000},{"count(friend)":0,"val(sum)":0.000000}]}],"me":[{"age":19,"count(friend)":1,"name":"Andrea"},{"age":17,"count(friend)":0,"name":"Daryl Dixon"},{"age":15,"count(friend)":1,"name":"Rick Grimes"},{"age":15,"count(friend)":0,"name":"Glenn Rhee"},{"count(friend)":0}]}}`,
		js)
}

func TestQueryVarValAggOrderAsc(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				f as friend {
					n as age
					s as survival_rate
					sum as math(n + s)
				}
			}

			me(func: uid(f), orderasc: val(sum)) {
				name
				age
				survival_rate
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"age":15,"name":"Rick Grimes","survival_rate":1.600000},{"age":15,"name":"Glenn Rhee","survival_rate":1.600000},{"age":17,"name":"Daryl Dixon","survival_rate":1.600000},{"age":19,"name":"Andrea","survival_rate":1.600000}]}}`,
		js)
}

func TestQueryVarValOrderAsc(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				f as friend {
					n as name
				}
			}

			me(func: uid(f), orderasc: val(n)) {
				name
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea"},{"name":"Daryl Dixon"},{"name":"Glenn Rhee"},{"name":"Rick Grimes"}]}}`,
		js)
}

func TestQueryVarValOrderDob(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				f as friend {
					n as dob
				}
			}

			me(func: uid(f), orderasc: val(n)) {
				name
				dob
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Andrea", "dob":"1901-01-15T00:00:00Z"},{"name":"Daryl Dixon", "dob":"1909-01-10T00:00:00Z"},{"name":"Glenn Rhee", "dob":"1909-05-05T00:00:00Z"},{"name":"Rick Grimes", "dob":"1910-01-02T00:00:00Z"}]}}`,
		js)
}

func TestQueryVarValOrderError(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				friend {
					n as name
				}
			}

			me(func: uid(n), orderdesc: n) {
				name
			}
		}
	`
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot sort by unknown attribute n")
}

func TestQueryVarValOrderDesc(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				f as friend {
					n as name
				}
			}

			me(func: uid(f), orderdesc: val(n)) {
				name
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"},{"name":"Daryl Dixon"},{"name":"Andrea"}]}}`,
		js)
}

func TestQueryVarValOrderDescMissing(t *testing.T) {
	query := `
		{
			var(func: uid( 1034)) {
				f As friend {
					n As name
				}
			}

			me(func: uid(f), orderdesc: val(n)) {
				name
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me": []}}`, js)
}

func TestGroupByRoot(t *testing.T) {
	query := `
	{
		me(func: uid(1, 23, 24, 25, 31)) @groupby(age) {
				count(uid)
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":38,"count":1},{"age":15,"count":2}]}]}}`,
		js)
}

func TestGroupByRootEmpty(t *testing.T) {
	// Predicate agent doesn't exist.
	query := `
	{
		me(func: uid(1, 23, 24, 25, 31)) @groupby(agent) {
				count(uid)
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {}}`, js)
}

func TestGroupByRootAlias(t *testing.T) {
	query := `
	{
		me(func: uid(1, 23, 24, 25, 31)) @groupby(age) {
			Count: count(uid)
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"@groupby":[{"age":17,"Count":1},{"age":19,"Count":1},{"age":38,"Count":1},{"age":15,"Count":2}]}]}}`, js)
}

func TestGroupByRootAlias2(t *testing.T) {
	query := `
	{
		me(func: uid(1, 23, 24, 25, 31)) @groupby(Age: age) {
			Count: count(uid)
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"@groupby":[{"Age":17,"Count":1},{"Age":19,"Count":1},{"Age":38,"Count":1},{"Age":15,"Count":2}]}]}}`, js)
}

func TestGroupBy_RepeatAttr(t *testing.T) {
	query := `
	{
		me(func: uid(1)) {
			friend @groupby(age) {
				count(uid)
			}
			friend {
				name
				age
			}
			name
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":15,"count":2}]},{"age":15,"name":"Rick Grimes"},{"age":15,"name":"Glenn Rhee"},{"age":17,"name":"Daryl Dixon"},{"age":19,"name":"Andrea"}],"name":"Michonne"}]}}`,
		js)
}

func TestGroupBy(t *testing.T) {
	query := `
	{
		age(func: uid(1)) {
			friend {
				age
				name
			}
		}

		me(func: uid(1)) {
			friend @groupby(age) {
				count(uid)
			}
			name
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"age":[{"friend":[{"age":15,"name":"Rick Grimes"},{"age":15,"name":"Glenn Rhee"},{"age":17,"name":"Daryl Dixon"},{"age":19,"name":"Andrea"}]}],"me":[{"friend":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":15,"count":2}]}],"name":"Michonne"}]}}`,
		js)
}

func TestGroupByCountval(t *testing.T) {
	query := `
		{
			var(func: uid( 1)) {
				friend @groupby(school) {
					a as count(uid)
				}
			}

			order(func :uid(a), orderdesc: val(a)) {
				name
				val(a)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"order":[{"name":"School B","val(a)":3},{"name":"School A","val(a)":2}]}}`,
		js)
}

func TestGroupByAggval(t *testing.T) {
	query := `
		{
			var(func: uid(1)) {
				friend @groupby(school) {
					a as max(name)
					b as min(name)
				}
			}

			orderMax(func :uid(a), orderdesc: val(a)) {
				name
				val(a)
			}

			orderMin(func :uid(b), orderdesc: val(b)) {
				name
				val(b)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"orderMax":[{"name":"School B","val(a)":"Rick Grimes"},{"name":"School A","val(a)":"Glenn Rhee"}],"orderMin":[{"name":"School A","val(b)":"Daryl Dixon"},{"name":"School B","val(b)":"Andrea"}]}}`,
		js)
}

func TestGroupByAlias(t *testing.T) {
	query := `
		{
			me(func: uid(1)) {
				friend @groupby(school) {
					MaxName: max(name)
					MinName: min(name)
					UidCount: count(uid)
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"friend":[{"@groupby":[{"school":"0x1388","MaxName":"Glenn Rhee","MinName":"Daryl Dixon","UidCount":2},{"school":"0x1389","MaxName":"Rick Grimes","MinName":"Andrea","UidCount":3}]}]}]}}`, js)
}

func TestGroupByAgg(t *testing.T) {
	query := `
		{
			me(func: uid( 1)) {
				friend @groupby(age) {
					max(name)
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"@groupby":[{"age":17,"max(name)":"Daryl Dixon"},{"age":19,"max(name)":"Andrea"},{"age":15,"max(name)":"Rick Grimes"}]}]}]}}`,
		js)
}

func TestGroupByMulti(t *testing.T) {
	query := `
		{
			me(func: uid(1)) {
				friend @groupby(FRIEND: friend,name) {
					count(uid)
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"friend":[{"@groupby":[{"count":1,"FRIEND":"0x1","name":"Rick Grimes"},{"count":1,"FRIEND":"0x18","name":"Andrea"}]}]}]}}`,
		js)
}

func TestGroupByMulti2(t *testing.T) {
	query := `
		{
			me(func: uid(1)) {
				Friend: friend @groupby(Friend: friend,Name: name) {
					Count: count(uid)
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"me":[{"Friend":[{"@groupby":[{"Friend":"0x1","Name":"Rick Grimes","Count":1},{"Friend":"0x18","Name":"Andrea","Count":1}]}]}]}}`,
		js)
}

func TestGroupByMultiParents(t *testing.T) {
	query := `
		{
			me(func: uid(1,23,31)) {
				name
				friend @groupby(name, age) {
					count(uid)
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name":"Michonne","friend":[{"@groupby":[{"name":"Andrea","age":19,"count":1},{"name":"Daryl Dixon","age":17,"count":1},{"name":"Glenn Rhee","age":15,"count":1},{"name":"Rick Grimes","age":15,"count":1}]}]},{"name":"Rick Grimes","friend":[{"@groupby":[{"name":"Michonne","age":38,"count":1}]}]},{"name":"Andrea","friend":[{"@groupby":[{"name":"Glenn Rhee","age":15,"count":1}]}]}]}}`, js)
}

func TestGroupByMultiParents_2(t *testing.T) {
	// We dont have any data for uid 99999
	query := `
		{
			me(func: uid(1,23,99999,31)) {
				name
				friend @groupby(name, age) {
					count(uid)
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name":"Michonne","friend":[{"@groupby":[{"name":"Andrea","age":19,"count":1},{"name":"Daryl Dixon","age":17,"count":1},{"name":"Glenn Rhee","age":15,"count":1},{"name":"Rick Grimes","age":15,"count":1}]}]},{"name":"Rick Grimes","friend":[{"@groupby":[{"name":"Michonne","age":38,"count":1}]}]},{"name":"Andrea","friend":[{"@groupby":[{"name":"Glenn Rhee","age":15,"count":1}]}]}]}}`, js)

}

func TestGroupByAgeMultiParents(t *testing.T) {
	// We dont have any data for uid 99999, 99998.
	query := `
		{
			me(func: uid(23,99999,31, 99998,1)) {
				name
				friend @groupby(age) {
					count(uid)
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name":"Michonne","friend":[{"@groupby":[{"age":17,"count":1},{"age":19,"count":1},{"age":15,"count":2}]}]},{"name":"Rick Grimes","friend":[{"@groupby":[{"age":38,"count":1}]}]},{"name":"Andrea","friend":[{"@groupby":[{"age":15,"count":1}]}]}]}}`, js)
}

func TestGroupByFriendsMultipleParents(t *testing.T) {

	// We dont have any data for uid 99999, 99998.
	query := `
		{
			me(func: uid(23,99999,31, 99998,1)) {
				name
				friend @groupby(friend) {
					count(uid)
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"name":"Michonne","friend":[{"@groupby":[{"friend":"0x1","count":1},{"friend":"0x18","count":1}]}]},{"name":"Rick Grimes","friend":[{"@groupby":[{"friend":"0x17","count":1},{"friend":"0x18","count":1},{"friend":"0x19","count":1},{"friend":"0x1f","count":1},{"friend":"0x65","count":1}]}]},{"name":"Andrea"}]}}`, js)
}

func TestGroupByFriendsMultipleParentsVar(t *testing.T) {

	// We dont have any data for uid 99999, 99998.
	query := `
		{
			var(func: uid(23,99999,31, 99998,1)) {
				name
				friend @groupby(friend) {
					f as count(uid)
				}
			}

			me(func: uid(f), orderdesc: val(f)) {
				uid
				name
				val(f)
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"uid":"0x18","name":"Glenn Rhee","val(f)":2},{"uid":"0x1","name":"Michonne","val(f)":1},{"uid":"0x17","name":"Rick Grimes","val(f)":1},{"uid":"0x19","name":"Daryl Dixon","val(f)":1},{"uid":"0x1f","name":"Andrea","val(f)":1},{"uid":"0x65","val(f)":1}]}}`, js)
}

func TestMultiEmptyBlocks(t *testing.T) {

	query := `
		{
			you(func: uid(0x01)) {
			}

			me(func: uid(0x02)) {
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"you": [], "me": []}}`, js)
}

func TestUseVarsMultiCascade1(t *testing.T) {

	query := `
		{
			him(func: uid(0x01)) @cascade {
				L as friend {
					B as friend
					name
			 	}
			}

			me(func: uid(L, B)) {
				name
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"him": [{"friend":[{"name":"Rick Grimes"}, {"name":"Andrea"}]}], "me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}, {"name":"Andrea"}]}}`,
		js)
}

func TestUseVarsMultiCascade(t *testing.T) {

	query := `
		{
			var(func: uid(0x01)) @cascade {
				L as friend {
				 	B as friend
				}
			}

			me(func: uid(L, B)) {
				name
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Glenn Rhee"}, {"name":"Andrea"}]}}`,
		js)
}

func TestUseVarsMultiOrder(t *testing.T) {

	query := `
		{
			var(func: uid(0x01)) {
				L as friend(first:2, orderasc: dob)
			}

			var(func: uid(0x01)) {
				G as friend(first:2, offset:2, orderasc: dob)
			}

			friend1(func: uid(L)) {
				name
			}

			friend2(func: uid(G)) {
				name
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"friend1":[{"name":"Daryl Dixon"}, {"name":"Andrea"}],"friend2":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}}`,
		js)
}

func TestFilterFacetval(t *testing.T) {

	query := `
		{
			friend(func: uid(0x01)) {
				path @facets(L as weight) {
					name
				 	friend @filter(uid(L)) {
						name
						val(L)
					}
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"path":[{"name":"Glenn Rhee","path|weight":0.200000},{"name":"Andrea","friend":[{"name":"Glenn Rhee","val(L)":0.200000}],"path|weight":0.100000}]}]}}`,
		js)
}

func TestFilterFacetVar1(t *testing.T) {

	query := `
		{
			friend(func: uid(0x01)) {
				path @facets(L as weight1) {
					name
				 	friend @filter(uid(L)){
						name
					}
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data":{"friend":[{"path":[{"name":"Glenn Rhee"},{"name":"Andrea","path|weight1":0.200000}]}]}}`,
		js)
}

func TestUseVarsFilterVarReuse1(t *testing.T) {

	query := `
		{
			friend(func: uid(0x01)) {
				friend {
					L as friend {
						name
						friend @filter(uid(L)) {
							name
						}
					}
				}
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"friend":[{"friend":[{"friend":[{"name":"Michonne", "friend":[{"name":"Glenn Rhee"}]}]}, {"friend":[{"name":"Glenn Rhee"}]}]}]}}`,
		js)
}

func TestUseVarsFilterVarReuse2(t *testing.T) {

	query := `
		{
			friend(func:anyofterms(name, "Michonne Andrea Glenn")) {
				friend {
				 L as friend {
					nonexistent_pred
					name
					friend @filter(uid(L)) {
						name
					}
				}
			}
		}
	}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"friend":[{"friend":[{"friend":[{"name":"Michonne", "friend":[{"name":"Glenn Rhee"}]}]}, {"friend":[{"name":"Glenn Rhee"}]}]}]}}`,
		js)
}

func TestDoubleOrder(t *testing.T) {

	query := `
    {
		me(func: uid(1)) {
			friend(orderdesc: dob) @facets(orderasc: weight)
		}
	}
  `
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestVarInAggError(t *testing.T) {

	query := `
    {
			var(func: uid( 1)) {
				friend {
					a as age
				}
			}

			# var not allowed in min filter
			me(func: min(val(a))) {
				name
			}
		}
  `
	_, err := gql.Parse(gql.Request{Str: query})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Function name: min is not valid.")
}

func TestVarInIneqError(t *testing.T) {

	query := `
    {
			var(func: uid( 1)) {
				f as friend {
					a as age
				}
			}

			me(func: uid(f)) @filter(gt(val(a), "alice")) {
				name
			}
		}
  `
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestVarInIneqScore(t *testing.T) {

	query := `
    {
			var(func: uid( 1)) {
				friend {
					a as age
					s as count(friend)
					score as math(2*a + 3 * s + 1)
				}
			}

			me(func: ge(val(score), 35)) {
				name
				val(score)
				val(a)
				val(s)
			}
		}
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Daryl Dixon","val(a)":17,"val(s)":0,"val(score)":35.000000},{"name":"Andrea","val(a)":19,"val(s)":1,"val(score)":42.000000}]}}`,
		js)
}

func TestVarInIneq(t *testing.T) {

	query := `
    {
			var(func: uid( 1)) {
				f as friend {
					a as age
				}
			}

			me(func: uid(f)) @filter(gt(val(a), 18)) {
				name
			}
		}
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Andrea"}]}}`, js)
}

func TestVarInIneq2(t *testing.T) {

	query := `
    {
			var(func: uid(1)) {
				friend {
					a as age
				}
			}

			me(func: gt(val(a), 18)) {
				name
			}
		}
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Andrea"}]}}`, js)
}

func TestVarInIneq3(t *testing.T) {

	query := `
    {
			var(func: uid(0x1f)) {
				a as name
			}

			me(func: eq(name, val(a))) {
				name
			}
		}
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Andrea"}]}}`, js)
}

func TestVarInIneq4(t *testing.T) {

	query := `
    {
			var(func: uid(0x1f)) {
				a as name
			}

			me(func: uid(0x1f)) @filter(eq(name, val(a))) {
				name
			}
		}
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Andrea"}]}}`, js)
}

func TestVarInIneq5(t *testing.T) {

	query1 := `
    {
			var(func: uid(1)) {
				friend {
				  a as name
			  }
			}

			me(func: eq(name, val(a))) {
				name
			}
		}
  `
	query2 := `
    {
			var(func: uid(1)) {
				friend {
				  a as name
			  }
			}

			me(func: uid(a)) {
				name: val(a)
			}
		}
  `
	js1 := processQueryNoErr(t, query1)
	js2 := processQueryNoErr(t, query2)
	require.JSONEq(t, js2, js1)
}

func TestNestedFuncRoot(t *testing.T) {
	query := `
    {
			me(func: gt(count(friend), 2)) {
				name
			}
		}
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"}]}}`, js)
}

func TestNestedFuncRoot2(t *testing.T) {
	query := `
		{
			me(func: ge(count(friend), 1)) {
				name
			}
		}
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"},{"name":"Rick Grimes"},{"name":"Andrea"}]}}`, js)
}

func TestNestedFuncRoot4(t *testing.T) {

	query := `
		{
			me(func: le(count(friend), 1)) {
				name
			}
		}
  `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Rick Grimes"},{"name":"Andrea"}]}}`, js)
}

func TestCountUidToVar(t *testing.T) {
	query := `
	{
		var(func: has(school), first: 3) {
			f as count(uid)
		}

		me(func: uid(1)) {
			score: math(f)
		}
	}
    `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"score": 3}]}}`, js)
}

func TestCountUidToVarMultiple(t *testing.T) {
	query := `
	{
		var(func: has(school), first: 3) {
			f as count(uid)
		}

		var(func: has(follow), first: 4) {
			g as count(uid)
		}

		me(func: uid(1)) {
			score: math(f + g)
		}
	}
    `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"score": 7}]}}`, js)
}

func TestCountUidToVarCombinedWithNormalVar(t *testing.T) {
	query := `
	{
		var(func: has(school), first: 3) {
			f as count(uid)
		}

		var(func: has(follow)) {
			g as count(path)
		}

		me(func: uid(1)) {
			score: math(f + g)
		}
	}
    `
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"score": 5}]}}`, js)
}

func TestDefaultValueVar1(t *testing.T) {
	query := `
	{
		var(func: has(pred)) {
			n as uid
			cnt as count(nonexistent_pred)
		}

		data(func: uid(n)) @filter(gt(val(cnt), 4)) {
			expand(_all_)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"data":[]}}`, js)
}

func TestDefaultValueVar2(t *testing.T) {
	query := `
	{
		var(func: uid(0x1)) {
			cnt as nonexistent_pred
		}

		data(func: uid(0x1)) {
			val(cnt)
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"data":[]}}`, js)
}

func TestNonFlattenedResponse(t *testing.T) {
	query := `
	{
		me(func: eq(name@en, "Baz Luhrmann")) {
			uid
			director.film {
				name@en	
			}
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[
		{"uid":"0x2af8", "director.film": [
			{"name@en": "Strictly Ballroom"},
			{"name@en": "Puccini: La boheme (Sydney Opera)"},
			{"name@en": "No. 5 the film"}
		]}
	]}}`, js)

}

func TestDateTimeQuery(t *testing.T) {
	var query string

	// Test 19
	query = `
{
  q(func: has(created_at), orderdesc: created_at) {
		uid
		created_at
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x133","created_at":"2019-05-28T14:41:57+30:00"},{"uid":"0x130","created_at":"2019-03-28T15:41:57+30:00"},{"uid":"0x12d","created_at":"2019-03-28T14:41:57+30:00"},{"uid":"0x12e","created_at":"2019-03-28T13:41:57+29:00"},{"uid":"0x12f","created_at":"2019-03-27T14:41:57+06:00"},{"uid":"0x131","created_at":"2019-03-28T13:41:57+30:00"},{"uid":"0x132","created_at":"2019-03-24T14:41:57+05:30"}]}}`,
		processQueryNoErr(t, query))

	// Test 18
	query = `
{
	q(func: has(best_friend)) @cascade {
		uid
		best_friend @facets(lt(since, "2019-03-24")) @facets(since) {
			uid
		}
	}
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x3","best_friend":{"uid":"0x40","best_friend|since":"2018-03-24T14:41:57+05:30"}}]}}`,
		processQueryNoErr(t, query))

	// Test 17
	query = `
{
	q(func: has(best_friend)) @cascade {
		uid
		best_friend @facets(gt(since, "2019-03-27")) @facets(since) {
			uid
		}
	}
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x2","best_friend":{"uid":"0x40","best_friend|since":"2019-03-28T14:41:57+30:00"}}]}}`,
		processQueryNoErr(t, query))

	// Test 16
	query = `
{
	q(func: gt(created_at, "2019-03-28")) {
		uid
		created_at @facets(modified_at)
		updated_at @facets(modified_at)
	}
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x133","created_at":"2019-05-28T14:41:57+30:00","updated_at|modified_at":"2019-03-24T14:41:57+05:30","updated_at":"2019-05-28T00:00:00Z"}]}}`,
		processQueryNoErr(t, query))

	// Test 15
	query = `
{
	q(func: gt(age, 15)) @filter(gt(graduation, "1932") AND lt(graduation, "1934")) {
		uid
		graduation
	}
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x1f","graduation":["1935-01-01T00:00:00Z","1933-01-01T00:00:00Z"]}]}}`,
		processQueryNoErr(t, query))

	// Test 14
	query = `
{
	q(func: gt(age, 15)) @filter(le(graduation, "1932") OR gt(graduation, "1936")) {
		uid
		graduation
	}
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x1","graduation":["1932-01-01T00:00:00Z"]}]}}`,
		processQueryNoErr(t, query))

	// Test 13
	query = `
	{
		q(func: gt(age, 15)) @filter(lt(graduation, "1932") AND gt(graduation, "1936")) {
			uid
			graduation
		}
	}
	`
	require.JSONEq(t,
		`{"data":{"q":[]}}`,
		processQueryNoErr(t, query))

	// Test 12
	query = `
{
  q(func: le(dob, "1909-05-05")) {
    uid
    dob
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x18","dob":"1909-05-05T00:00:00Z"},{"uid":"0x19","dob":"1909-01-10T00:00:00Z"},{"uid":"0x1f","dob":"1901-01-15T00:00:00Z"}]}}`,
		processQueryNoErr(t, query))

	// Test 11
	query = `
{
  q(func: le(dob, "1909-05-05T00:00:00+05:30")) {
    uid
    dob
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x19","dob":"1909-01-10T00:00:00Z"},{"uid":"0x1f","dob":"1901-01-15T00:00:00Z"}]}}`,
		processQueryNoErr(t, query))

	// Test 10
	query = `
{
  q(func: eq(graduation, "1932-01-01T00:00:00+05:30")) {
    uid
    graduation
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[]}}`,
		processQueryNoErr(t, query))

	// Test 9
	query = `
{
  q(func: eq(graduation, "1932")) {
    uid
    graduation
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x1","graduation":["1932-01-01T00:00:00Z"]}]}}`,
		processQueryNoErr(t, query))

	// Test 8
	query = `
{
  q(func: lt(graduation, "1933")) {
    uid
    graduation
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x1","graduation":["1932-01-01T00:00:00Z"]}]}}`,
		processQueryNoErr(t, query))

	// Test 7
	query = `
{
  q(func: gt(graduation, "1932")) {
    uid
    graduation
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x1f","graduation":["1935-01-01T00:00:00Z","1933-01-01T00:00:00Z"]}]}}`,
		processQueryNoErr(t, query))

	// Test 6
	query = `
{
  q(func: le(updated_at, "2019-03-27T14:41:56+06:00")) {
    uid
    updated_at
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x131","updated_at":"2019-03-28T13:41:57+30:00"},{"uid":"0x132","updated_at":"2019-03-24T14:41:57+05:30"}]}}`,
		processQueryNoErr(t, query))

	// Test 5
	query = `
{
  q(func: ge(updated_at, "2019-03-28T13:41:57+00:00")) {
    uid
    updated_at
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x133","updated_at":"2019-05-28T00:00:00Z"}]}}`,
		processQueryNoErr(t, query))

	// Test 4
	query = `
{
  q(func: ge(updated_at, "2019-03-28T13:41:57")) {
    uid
    updated_at
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x133","updated_at":"2019-05-28T00:00:00Z"}]}}`,
		processQueryNoErr(t, query))

	// Test 3
	query = `
{
  q(func: le(created_at, "2019-03-27T14:41:56+06:00")) {
    uid
    created_at
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x131","created_at":"2019-03-28T13:41:57+30:00"},{"uid":"0x132","created_at":"2019-03-24T14:41:57+05:30"}]}}`,
		processQueryNoErr(t, query))

	// Test 2
	query = `
{
  q(func: ge(created_at, "2019-03-28T13:41:57+00:00")) {
    uid
    created_at
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x133","created_at":"2019-05-28T14:41:57+30:00"}]}}`,
		processQueryNoErr(t, query))

	// Test 1
	query = `
{
  q(func: ge(created_at, "2019-03-28T13:41:57")) {
    uid
    created_at
  }
}
`
	require.JSONEq(t,
		`{"data":{"q":[{"uid":"0x133","created_at":"2019-05-28T14:41:57+30:00"}]}}`,
		processQueryNoErr(t, query))
}

var client *dgo.Dgraph

func TestMain(m *testing.M) {
	client = z.DgraphClientWithGroot(z.SockAddr)

	populateCluster()
	os.Exit(m.Run())
}
