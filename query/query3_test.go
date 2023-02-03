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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/testutil"
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

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Depth must be > 0 when loop is true for recurse query")
}

func TestRecurseNestedError1(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) @recurse {
				friend {
					name
				}
				name
			}
		}`

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"recurse queries require that all predicates are specified in one level")
}

func TestRecurseNestedError2(t *testing.T) {
	query := `
		{
			me(func: uid(0x01)) @recurse {
				friend {
					pet {
						name
					}
				}
			}
		}`

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(),
		"recurse queries require that all predicates are specified in one level")
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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

	_, err := processQuery(context.Background(), t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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

	_, err := processQuery(context.Background(), t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
		  "data": {
		    "_path_": [
		      {
		        "path": {
		          "path": {
		            "path": {
		              "uid": "0x3e9",
		              "path|weight": 0.1
		            },
		            "uid": "0x3e8",
		            "path|weight": 0.1
		          },
		          "uid": "0x1f",
		          "path|weight": 0.1
		        },
		        "uid": "0x1",
		        "_weight_": 0.3
		      }
		    ]
		  }
		}
	`, js)
}

func TestKShortestPathWeightedMinMaxNoEffect(t *testing.T) {

	query := `
		{
			shortest(from: 1, to:1001, numpaths: 4, minweight:0, maxweight: 1000) {
				path @facets(weight)
			}
		}`
	// We only get one path in this case as the facet is present only in one path.
	// The path meets the weight requirements so it does not get filtered.
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
		  "data": {
		    "_path_": [
		      {
		        "path": {
		          "path": {
		            "path": {
		              "uid": "0x3e9",
		              "path|weight": 0.1
		            },
		            "uid": "0x3e8",
		            "path|weight": 0.1
		          },
		          "uid": "0x1f",
		          "path|weight": 0.1
		        },
		        "uid": "0x1",
		        "_weight_": 0.3
		      }
		    ]
		  }
		}
	`, js)
}

func TestKShortestPathWeightedMinWeight(t *testing.T) {

	query := `
		{
			shortest(from: 1, to:1001, numpaths: 4, minweight: 3) {
				path @facets(weight)
			}
		}`
	// We get no paths as the only path does not match the weight requirements.
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{}}`, js)
}

func TestKShortestPathWeightedMaxWeight(t *testing.T) {

	query := `
		{
			shortest(from: 1, to:1001, numpaths: 4, maxweight: 0.1) {
				path @facets(weight)
			}
		}`
	// We get no paths as the only path does not match the weight requirements.
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{}}`, js)
}

func TestKShortestPathWeighted_LimitDepth(t *testing.T) {

	query := `
		{
			shortest(from: 1, to:1001, depth:1, numpaths: 4) {
				path @facets(weight)
			}
		}`
	// We only get one path in this case as the facet is present only in one path.
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
		  "data": {
		    "_path_": [
		      {
		        "path": {
		          "path": {
		            "path": {
		              "path": {
		                "path": {
		                  "uid": "0x3eb",
		                  "path|weight": 0.6
		                },
		                "uid": "0x3ea",
		                "path|weight": 0.1
		              },
		              "uid": "0x3e9",
		              "path|weight": 0.1
		            },
		            "uid": "0x3e8",
		            "path|weight": 0.1
		          },
		          "uid": "0x1f",
		          "path|weight": 0.1
		        },
		        "uid": "0x1",
		        "_weight_": 1
		      },
		      {
		        "path": {
		          "path": {
		            "path": {
		              "path": {
		                "uid": "0x3eb",
		                "path|weight": 0.6
		              },
		              "uid": "0x3ea",
		              "path|weight": 0.7
		            },
		            "uid": "0x3e8",
		            "path|weight": 0.1
		          },
		          "uid": "0x1f",
		          "path|weight": 0.1
		        },
		        "uid": "0x1",
		        "_weight_": 1.5
		      },
		      {
		        "path": {
		          "path": {
		            "path": {
		              "path": {
		                "uid": "0x3eb",
		                "path|weight": 1.5
		              },
		              "uid": "0x3e9",
		              "path|weight": 0.1
		            },
		            "uid": "0x3e8",
		            "path|weight": 0.1
		          },
		          "uid": "0x1f",
		          "path|weight": 0.1
		        },
		        "uid": "0x1",
		        "_weight_": 1.8
		      }
		    ]
		  }
		}
	`, js)
}

func TestKShortestPathWeighted1MinMaxWeight(t *testing.T) {

	query := `
		{
			shortest(from: 1, to:1003, numpaths: 3, minweight: 1.3, maxweight: 1.5) {
				path @facets(weight)
			}
		}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
		  "data": {
		    "_path_": [
		      {
		        "path": {
		          "path": {
		            "path": {
		              "path": {
		                "uid": "0x3eb",
		                "path|weight": 0.6
		              },
		              "uid": "0x3ea",
		              "path|weight": 0.7
		            },
		            "uid": "0x3e8",
		            "path|weight": 0.1
		          },
		          "uid": "0x1f",
		          "path|weight": 0.1
		        },
		        "uid": "0x1",
		        "_weight_": 1.5
		      }
		    ]
		  }
		}
	`, js)
}

func TestKShortestPathDepth(t *testing.T) {
	// Shortest path between 1 and 1000 is the path 1 => 31 => 1001 => 1000
	// but if the depth is less than 3 then there is no direct path between
	// 1 and 1000. Also if depth >=5 there is another path
	// 1 => 31 => 1001 => 1003 => 1002 => 1000
	query := `
	query test ($depth: int, $numpaths: int) {
		path as shortest(from: 1, to: 1000, depth: $depth, numpaths: $numpaths) {
			follow
		}
		me(func: uid(path)) {
			name
		}
	}`

	emptyPath := `{"data": {"me":[]}}`

	onePath := `{
	"data": {
	  "me": [
		{"name": "Michonne"},
		{"name": "Andrea"},
		{"name": "Bob"},
		{"name": "Alice"}
	  ],
	  "_path_": [
		{
		  "follow": {
			"follow": {
			  "follow": {
				"uid": "0x3e8"
			  },
			  "uid": "0x3e9"
			},
			"uid": "0x1f"
		  },
		  "uid": "0x1",
		  "_weight_": 3
		}
	  ]
	}
  }`
	twoPaths := `{
	"data": {
	 "me": [
	{"name": "Michonne"},
	{"name": "Andrea"},
	{"name": "Bob"},
	{"name": "Alice"}
	 ],
	 "_path_": [
	  {
	   "follow": {
		"follow": {
		 "follow": {
		  "uid": "0x3e8"
		 },
		 "uid": "0x3e9"
		},
		"uid": "0x1f"
	   },
	   "uid": "0x1",
	   "_weight_": 3
	  },
	  {
	   "follow": {
		"follow": {
		 "follow": {
		  "follow": {
		   "follow": {
			"uid": "0x3e8"
		   },
		   "uid": "0x3ea"
		  },
		  "uid": "0x3eb"
		 },
		 "uid": "0x3e9"
		},
		"uid": "0x1f"
	   },
	   "uid": "0x1",
	   "_weight_": 5
	  }
	 ]
	}
   }`
	tests := []struct {
		depth, numpaths, output string
	}{
		{
			"2",
			"4",
			emptyPath,
		},
		{
			"3",
			"4",
			onePath,
		},
		{
			"4",
			"4",
			onePath,
		},
		{
			"5",
			"4",
			twoPaths,
		},
		{
			"6",
			"4",
			twoPaths,
		},
	}

	t.Parallel()
	for _, tc := range tests {
		t.Run(fmt.Sprintf("depth_%s_numpaths_%s", tc.depth, tc.numpaths), func(t *testing.T) {
			js, err := processQueryWithVars(t, query, map[string]string{"$depth": tc.depth,
				"$numpaths": tc.numpaths})
			require.NoError(t, err)
			require.JSONEq(t, tc.output, js)
		})
	}
}

func TestKShortestPathTwoPaths(t *testing.T) {
	query := `
	{
		A as shortest(from: 51, to:55, numpaths: 2, depth:2) {
			connects @facets(weight)
		}
		me(func: uid(A)) {
			name
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{
		"data": {
		 "me": [
		  {"name": "A"},
		  {"name": "C"},
		  {"name": "D"},
		  {"name": "E"}
		 ],
		 "_path_": [
		  {
		   "connects": {
			"connects": {
			 "connects": {
			  "uid": "0x37",
			  "connects|weight": 1
			 },
			 "uid": "0x36",
			 "connects|weight": 1
			},
			"uid": "0x35",
			"connects|weight": 1
		   },
		   "uid": "0x33",
		   "_weight_": 3
		  },
		  {
		   "connects": {
			"connects": {
			 "uid": "0x37",
			 "connects|weight": 1
			},
			"uid": "0x36",
			"connects|weight": 10
		   },
		   "uid": "0x33",
		   "_weight_": 11
		  }
		 ]
		}
	   }`, js)
}

// There are 5 paths between 51 to 55 under "connects" predicate.
// This tests checks if the algorithm finds only 5 paths and doesn't add
// cyclical paths when forced to search for 6 or more paths.
func TestKShortestPathAllPaths(t *testing.T) {
	for _, q := range []string{
		`{A as shortest(from: 51, to:55, numpaths: 5) {connects @facets(weight)}
		me(func: uid(A)) {name}}`,
		`{A as shortest(from: 51, to:55, numpaths: 6) {connects @facets(weight)}
		me(func: uid(A)) {name}}`,
		`{A as shortest(from: 51, to:55, numpaths: 10) {connects @facets(weight)}
		me(func: uid(A)) {name}}`,
	} {
		js := processQueryNoErr(t, q)
		expected := `
		{
			"data":{
				"me":[
					{
						"name":"A"
					},
					{
						"name":"C"
					},
					{
						"name":"D"
					},
					{
						"name":"E"
					}
				],
				"_path_":[
					{
						"connects":{
							"connects":{
								"connects":{
									"uid":"0x37",
									"connects|weight":1
								},
								"uid":"0x36",
								"connects|weight":1
							},
							"uid":"0x35",
							"connects|weight":1
						},
						"uid":"0x33",
						"_weight_":3
					},
					{
						"connects":{
							"connects":{
								"uid":"0x37",
								"connects|weight":1
							},
							"uid":"0x36",
							"connects|weight":10
						},
						"uid":"0x33",
						"_weight_":11
					},
					{
						"connects":{
							"connects":{
								"connects":{
									"connects":{
										"uid":"0x37",
										"connects|weight":1
									},
									"uid":"0x36",
									"connects|weight":10
								},
								"uid":"0x34",
								"connects|weight":10
							},
							"uid":"0x35",
							"connects|weight":1
						},
						"uid":"0x33",
						"_weight_":22
					},
					{
						"connects":{
							"connects":{
								"connects":{
									"uid":"0x37",
									"connects|weight":1
								},
								"uid":"0x36",
								"connects|weight":10
							},
							"uid":"0x34",
							"connects|weight":11
						},
						"uid":"0x33",
						"_weight_":22
					},
					{
						"connects":{
							"connects":{
								"connects":{
									"connects":{
										"uid":"0x37",
										"connects|weight":1
									},
									"uid":"0x36",
									"connects|weight":1
								},
								"uid":"0x35",
								"connects|weight":10
							},
							"uid":"0x34",
							"connects|weight":11
						},
						"uid":"0x33",
						"_weight_":23
					}
				]
			}
		}
		`
		testutil.CompareJSON(t, expected, js)
	}
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[
			{"uid":"0x1","_weight_":3,"path":{"uid":"0x1f","path":{"uid":"0x3e8","path":{"uid":"0x3ea"}}}},
			{"uid":"0x1","_weight_":4,"path":{"uid":"0x1f","path":{"uid":"0x3e8","path":{"uid":"0x3e9","path":{"uid":"0x3ea"}}}}}],
		"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Alice"},{"name":"Matt"}]}}`,
		js)
}

func TestTwoShortestPathMaxWeight(t *testing.T) {

	query := `
		{
			A as shortest(from: 1, to:1002, numpaths: 2, maxweight:1) {
				path
			}

			me(func: uid( A)) {
				name
			}
		}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
}

func TestTwoShortestPathMinWeight(t *testing.T) {

	query := `
		{
			A as shortest(from: 1, to:1002, numpaths: 2, minweight:10) {
				path
			}

			me(func: uid( A)) {
				name
			}
		}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1", "_weight_": 1, "friend":{"uid":"0x1f"}}],"me":[{"name":"Michonne"},{"name":"Andrea"}]}}`,
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x17","_weight_":1, "friend":{"uid":"0x1"}}],"me":[{"name":"Rick Grimes"},{"name":"Michonne"}]}}`,
		js)
}

// Regression test for https://github.com/dgraph-io/dgraph/issues/3657.
func TestShortestPathPassword(t *testing.T) {
	query := `
		{
			A as shortest(from:0x01, to:31) {
				password
				friend
			}

			me(func: uid( A)) {
				name
			}
		}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1", "_weight_": 1, "friend":{"uid":"0x1f"}}],
			"me":[{"name":"Michonne"},{"name":"Andrea"}]}}`, js)
}

func TestShortestPathWithUidVariable(t *testing.T) {
	query := `
	{
		a as var(func: uid(0x01))
		b as var(func: uid(31))

		shortest(from: uid(a), to: uid(b)) {
			password
			friend
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1", "_weight_": 1, "friend":{"uid":"0x1f"}}]}}`, js)
}

func TestShortestPathWithUidVariableAndFunc(t *testing.T) {
	query := `
	{
		a as var(func: eq(name, "Michonne"))
		b as var(func: eq(name, "Andrea"))

		shortest(from: uid(a), to: uid(b)) {
			password
			friend
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1", "_weight_": 1, "friend":{"uid":"0x1f"}}]}}`, js)
}

func TestShortestPathWithUidVariableError(t *testing.T) {
	query := `
	{
		a as var(func: eq(name, "Alice"))
		b as var(func: eq(name, "Andrea"))

		shortest(from: uid(a), to: uid(b)) {
			password
			friend
		}
	}`

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestShortestPathWithUidVariableNoMatch(t *testing.T) {
	query := `
	{
		a as var(func: eq(name, "blah blah"))
		b as var(func: eq(name, "foo bar"))

		shortest(from: uid(a), to: uid(b)) {
			password
			friend
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{}}`, js)
}

func TestShortestPathWithUidVariableNoMatchForFrom(t *testing.T) {
	query := `
	{
		a as var(func: eq(name, "blah blah"))
		b as var(func: eq(name, "Michonne"))

		shortest(from: uid(a), to: uid(b)) {
			password
			friend
		}
	}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{}}`, js)
}

func TestShortestPathWithDepth(t *testing.T) {
	// Shortest path between A and B is the path A => C => D => B but if the depth is less than 3
	// then the direct path between A and B should be returned.
	query := `
	query test ($depth: int, $numpaths: int) {
		a as var(func: eq(name, "A"))
		b as var(func: eq(name, "B"))

		path as shortest(from: uid(a), to: uid(b), depth: $depth, numpaths: $numpaths) {
			connects @facets(weight)
		}

		path(func: uid(path)) {
			uid
			name
		}
	}`

	directPath := `
	{
		"data": {
			"path": [
				{
					"uid": "0x33",
					"name": "A"
				},
				{
					"uid": "0x34",
					"name": "B"
				}
			],
			"_path_": [
				{
					"connects": {
						"uid": "0x34",
						"connects|weight": 11
					},
					"uid": "0x33",
					"_weight_": 11
				}
			]
		}
	}`

	shortestPath := `
	{
		"data": {
			"path": [
				{
					"uid": "0x33",
					"name": "A"
				},
				{
					"uid": "0x35",
					"name": "C"
				},
				{
					"uid": "0x36",
					"name": "D"
				},
				{
					"uid": "0x34",
					"name": "B"
				}
			],
			"_path_": [
				{
					"connects": {
						"connects": {
							"connects": {
								"uid": "0x34",
								"connects|weight": 2
							},
							"connects|weight": 1,
							"uid": "0x36"
						},
						"uid": "0x35",
						"connects|weight": 1
					},
					"uid": "0x33",
					"_weight_": 4
				}
			]
		}
	}`

	emptyPath := `{"data":{"path":[]}}`

	allPaths := `{
		"data": {
		 "path": [
		  {"uid": "0x33","name": "A"},
		  {"uid": "0x35","name": "C"},
		  {"uid": "0x36","name": "D"},
		  {"uid": "0x34","name": "B"}
		 ],
		 "_path_": [
		  {
		   "connects": {
			"connects": {
			 "connects": {
			  "uid": "0x34",
			  "connects|weight": 2
			 },
			 "uid": "0x36",
			 "connects|weight": 1
			},
			"uid": "0x35",
			"connects|weight": 1
		   },
		   "uid": "0x33",
		   "_weight_": 4
		  },
		  {
		   "connects": {
			"connects": {
			 "uid": "0x34",
			 "connects|weight": 10
			},
			"uid": "0x35",
			"connects|weight": 1
		   },
		   "uid": "0x33",
		   "_weight_": 11
		  },
		  {
		   "connects": {
			"uid": "0x34",
			"connects|weight": 11
		   },
		   "uid": "0x33",
		   "_weight_": 11
		  },
		  {
		   "connects": {
			"connects": {
			 "uid": "0x34",
			 "connects|weight": 2
			},
			"uid": "0x36",
			"connects|weight": 10
		   },
		   "uid": "0x33",
		   "_weight_": 12
		  },
		  {
		   "connects": {
			"connects": {
			 "connects": {
			  "uid": "0x34",
			  "connects|weight": 10
			 },
			 "uid": "0x35",
			 "connects|weight": 10
			},
			"uid": "0x36",
			"connects|weight": 10
		   },
		   "uid": "0x33",
		   "_weight_": 30
		  }
		 ]
		}
	}
	`

	tests := []struct {
		depth, numpaths, output string
	}{
		{
			"0",
			"1",
			emptyPath,
		},
		{
			"1",
			"1",
			directPath,
		},
		{
			"2",
			"1",
			shortestPath,
		},
		{
			"3",
			"1",
			shortestPath,
		},
		{
			"10",
			"1",
			shortestPath,
		},
		//The test cases below are for k-shortest path queries with varying depths.
		{
			"0",
			"10",
			emptyPath,
		},
		{
			"1",
			"10",
			directPath,
		},
		{
			"2",
			"10",
			allPaths,
		},
		{
			"10",
			"10",
			allPaths,
		},
	}

	t.Parallel()
	for _, tc := range tests {
		t.Run(fmt.Sprintf("depth_%s_numpaths_%s", tc.depth, tc.numpaths), func(t *testing.T) {
			js, err := processQueryWithVars(t, query, map[string]string{"$depth": tc.depth,
				"$numpaths": tc.numpaths})
			require.NoError(t, err)
			require.JSONEq(t, tc.output, js)
		})
	}

}

func TestShortestPathWithDepth_direct_path_is_shortest(t *testing.T) {
	// Direct path from D to B is the shortest path between D and B. As we increase the depth, it
	// should still be the shortest path returned between the two nodes.
	query := `
	query test ($depth: int) {
		a as var(func: eq(name, "D"))
		b as var(func: eq(name, "B"))

		path as shortest(from: uid(a), to: uid(b), depth: $depth) {
			connects @facets(weight)
		}

		path(func: uid(path)) {
			uid
			name
		}
	}`

	directPath := `{
		"data": {
			"path": [
				{
					"uid": "0x36",
					"name": "D"
				},
				{
					"uid": "0x34",
					"name": "B"
				}
			],
			"_path_": [
				{
					"connects": {
						"uid": "0x34",
						"connects|weight": 2
					},
					"uid": "0x36",
					"_weight_": 2
				}
			]
		}
	}`

	tests := []struct {
		name, depth, output string
	}{
		{
			"depth 0",
			"0",
			`{"data":{"path":[]}}`,
		},
		{
			"depth 1",
			"1",
			directPath,
		},
		{
			"depth 2",
			"2",
			directPath,
		},
		{
			"depth 3",
			"3",
			directPath,
		},
		{
			"depth 10",
			"10",
			directPath,
		},
	}

	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			js, err := processQueryWithVars(t, query, map[string]string{"$depth": tc.depth})
			require.NoError(t, err)
			require.JSONEq(t, tc.output, js)
		})
	}

}

func TestShortestPathWithDepth_no_direct_path(t *testing.T) {
	// There is no direct path between A and E and the shortest path is for depth 3.
	query := `
	query test ($depth: int) {
		a as var(func: eq(name, "A"))
		b as var(func: eq(name, "E"))

		path as shortest(from: uid(a), to: uid(b), depth: $depth) {
			connects @facets(weight)
		}

		path(func: uid(path)) {
			uid
			name
		}
	}`

	shortestPath := `{
		"data": {
			"path": [
				{
					"uid": "0x33",
					"name": "A"
				},
				{
					"uid": "0x35",
					"name": "C"
				},
				{
					"uid": "0x36",
					"name": "D"
				},
				{
					"uid": "0x37",
					"name": "E"
				}
			],
			"_path_": [
				{
					"connects": {
						"connects": {
							"connects": {
								"uid": "0x37",
								"connects|weight": 1
							},
							"uid": "0x36",
							"connects|weight": 1
						},
						"uid": "0x35",
						"connects|weight": 1
					},
					"uid": "0x33",
					"_weight_": 3
				}
			]
		}
	}`

	emptyPath := `{"data":{"path":[]}}`

	tests := []struct {
		name, depth, output string
	}{
		{
			"depth 0",
			"0",
			emptyPath,
		},
		{
			"depth 1",
			"1",
			emptyPath,
		},
		{
			"depth 2",
			"2",
			shortestPath,
		},
		{
			"depth 3",
			"3",
			shortestPath,
		},
		{
			"depth 10",
			"10",
			shortestPath,
		},
	}

	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			js, err := processQueryWithVars(t, query, map[string]string{"$depth": tc.depth})
			require.NoError(t, err)
			require.JSONEq(t, tc.output, js)
		})
	}

}

func TestShortestPathWithDepth_test_for_hoppy_behavior(t *testing.T) {
	// This test checks that we only increase numHops when item.hop > numHops -1

	query := `
	query test ($depth: int) {
		a as var(func: eq(name, "F"))
		b as var(func: eq(name, "J"))

		path as shortest(from: uid(a), to: uid(b), depth: $depth) {
			connects @facets(weight)
		}

		path(func: uid(path)) {
			uid
			name
		}
	}`

	shortestPath := `
		{
		    "data": {
		        "path": [
		            {
		                "uid": "0x38",
		                "name": "F"
		            },
		            {
		                "uid": "0x3a",
		                "name": "H"
		            },
		            {
		                "uid": "0x3b",
		                "name": "I"
		            },
		            {
		                "uid": "0x3c",
		                "name": "J"
		            }
		        ],
		        "_path_": [
		            {
		                "connects": {
		                    "connects": {
		                        "connects": {
		                            "uid": "0x3c",
		                            "connects|weight": 1
		                        },
		                        "uid": "0x3b",
		                        "connects|weight": 1
		                    },
		                    "uid": "0x3a",
		                    "connects|weight": 1
		                },
		                "uid": "0x38",
		                "_weight_": 3
		            }
		        ]
		    }
		}
	`

	tests := []struct {
		name, depth, output string
	}{
		{
			"depth 0",
			"0",
			`{"data":{"path":[]}}`,
		},
		{
			"depth 1",
			"1",
			`{"data":{"path":[]}}`,
		},
		{
			"depth 2",
			"2",
			`{"data":{"path":[]}}`,
		},
		{
			"depth 3",
			"3",
			shortestPath,
		},
		{
			"depth 10",
			"10",
			shortestPath,
		},
	}

	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			js, err := processQueryWithVars(t, query, map[string]string{"$depth": tc.depth})
			require.NoError(t, err)
			require.JSONEq(t, tc.output, js)
		})
	}
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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

	_, err := processQuery(context.Background(), t, query)
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
		{
		    "data": {
		        "me": [
		            {
		                "name": "Michonne"
		            },
		            {
		                "name": "Andrea"
		            },
		            {
		                "name": "Alice"
		            },
		            {
		                "name": "Bob"
		            },
		            {
		                "name": "Matt"
		            }
		        ],
		        "_path_": [
		            {
		                "path": {
		                    "path": {
		                        "path": {
		                            "path": {
		                                "uid": "0x3ea",
		                                "path|weight": 0.1
		                            },
		                            "uid": "0x3e9",
		                            "path|weight": 0.1
		                        },
		                        "uid": "0x3e8",
		                        "path|weight": 0.1
		                    },
		                    "uid": "0x1f",
		                    "path|weight": 0.1
		                },
		                "uid": "0x1",
		                "_weight_": 0.4
		            }
		        ]
		    }
		}
	`, js)
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1","_weight_":2,"path":{"uid":"0x1f","path":{"uid":"0x3e8"}}}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Alice"}]}}
	`, js)
}

func TestShortestPath4(t *testing.T) {
	query := `
		{
			A as shortest(from:1, to:1003) {
				path
				follow
			}

			me(func: uid(A)) {
				name
			}
		}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `
	{
		"data": {
			"_path_":[
				{
					"uid":"0x1",
					"_weight_":3,
					"follow":{
						"uid":"0x1f",
						"follow":{
							"uid":"0x3e9",
							"follow":{
								"uid":"0x3eb"
							}
						}
					}
				}
			],
			"me":[
				{
					"name":"Michonne"
				},
				{
					"name":"Andrea"
				},
				{
					"name":"Bob"
				},
				{
					"name":"John"
				}
			]
		}
	}`, js)
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[{"uid":"0x1","_weight_":3,"follow":{"uid":"0x1f","follow":{"uid":"0x3e9","path":{"uid":"0x3ea"}}}}],"me":[{"name":"Michonne"},{"name":"Andrea"},{"name":"Bob"},{"name":"Matt"}]}}`,
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": { "me": []}}`, js)
}

func TestTwoShortestPathVariable(t *testing.T) {

	query := `
		{
			a as var(func: uid(1))
			b as var(func: uid(1002))

			A as shortest(from: uid(a), to: uid(b), numpaths: 2) {
				path
			}

			me(func: uid(A)) {
				name
			}
		}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"_path_":[
			{"uid":"0x1","_weight_":3,"path":{"uid":"0x1f","path":{"uid":"0x3e8",
			"path":{"uid":"0x3ea"}}}}, {"uid":"0x1","_weight_":4,
			"path":{"uid":"0x1f","path":{"uid":"0x3e8","path":{"uid":"0x3e9",
			"path":{"uid":"0x3ea"}}}}}], "me":[{"name":"Michonne"},{"name":"Andrea"}
			,{"name":"Alice"},{"name":"Matt"}]}}`,
		js)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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

	md := metadata.Pairs("debug", "true")
	ctx := context.Background()
	ctx = metadata.NewOutgoingContext(ctx, md)

	buf, _ := processQuery(ctx, t, query)

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

	js := processQueryNoErr(t, query)
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
	md := metadata.Pairs("debug", "true")
	ctx := context.Background()
	ctx = metadata.NewOutgoingContext(ctx, md)

	buf, err := processQuery(ctx, t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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
	_, err := processQuery(context.Background(), t, query)
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
	_, err := processQuery(context.Background(), t, query)
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
	_, err := processQuery(context.Background(), t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	_, err := processQuery(context.Background(), t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female","alive":true,"friend":[{"survival_rate":1.600000},{"survival_rate":1.600000},{"survival_rate":1.600000},{"survival_rate":1.600000}],"min(val(x))":1.600000}]}}`,
		js)

	setSchema(`survival_rate: int .`)

	js = processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne","gender":"female","alive":true,"friend":[{"survival_rate":1},{"survival_rate":1},{"survival_rate":1},{"survival_rate":1}],"min(val(x))":1}]}}`,
		js)
	setSchema(`survival_rate: float .`)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[{"name":"Michonne"}]}}`, js)
}

func TestPasswordExpandAll1(t *testing.T) {
	query := `
    {
        me(func: uid(0x01)) {
			expand(_all_)
		}
    }
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"alive":true, "gender":"female","name":"Michonne"}]}}`, js)
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"me":[{"alive":true, "checkpwd(password)":false,
	"gender":"female", "name":"Michonne"}]}}`, js)
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

	_, err := processQuery(context.Background(), t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	_, err := processQuery(context.Background(), t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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
	_, err := processQuery(context.Background(), t, query)
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
	_, err := processQuery(context.Background(), t, query)
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
	_, err := processQuery(context.Background(), t, query)
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
	_, err := processQuery(context.Background(), t, query)
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
	_, err := processQuery(context.Background(), t, query)
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
	_, err := processQuery(context.Background(), t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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
	_, err := processQuery(context.Background(), t, query)
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

	js := processQueryNoErr(t, query)
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

	_, err := processQuery(context.Background(), t, query)
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

	js := processQueryNoErr(t, query)
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

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
}

func TestFilterRegex1(t *testing.T) {

	query := `
    {
      me(func: uid(0x01)) {
        name
        friend @filter(regexp(name, /^[Glen Rh]+$/)) {
          name
        }
      }
    }
`

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne", "friend":[{"name":"Glenn Rhee"}]}]}}`, js)
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

	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name":"Michonne", "friend":[{"name":"Rick Grimes"},{"name":"Glenn Rhee"}]}]}}`, js)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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

	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
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

	_, err := processQuery(context.Background(), t, query)
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
	js := processQueryNoErr(t, query)
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
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"name@ru":"Артём Ткаченко"}]}}`,
		js)
}

func TestFilterRegex17(t *testing.T) {
	query := `
		{
			me(func:regexp(name, "")) {
				name
			}
		}
	`
	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Function 'regexp' requires 2 arguments,")
}

func TestTypeFunction(t *testing.T) {
	query := `
		{
			me(func: type(Person)) {
				uid
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid":"0x2"}, {"uid":"0x3"}, {"uid":"0x4"},{"uid":"0x17"},
		{"uid":"0x18"},{"uid":"0x19"}, {"uid":"0x1f"}, {"uid":"0xcb"}]}}`,
		js)
}

func TestTypeFunctionUnknownType(t *testing.T) {
	query := `
		{
			me(func: type(UnknownType)) {
				uid
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
}

func TestTypeFilter(t *testing.T) {
	query := `
		{
			me(func: uid(0x2)) @filter(type(Person)) {
				uid
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t,
		`{"data": {"me":[{"uid" :"0x2"}]}}`,
		js)
}

func TestTypeFilterUnknownType(t *testing.T) {
	query := `
		{
			me(func: uid(0x2)) @filter(type(UnknownType)) {
				uid
			}
		}
	`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"me":[]}}`, js)
}

func TestMaxPredicateSize(t *testing.T) {
	// Create a string that has more than than 2^16 chars.
	var b strings.Builder
	for i := 0; i < 10000; i++ {
		b.WriteString("abcdefg")
	}
	largePred := b.String()

	query := fmt.Sprintf(`
		{
			me(func: uid(0x2)) {
				%s {
					name
				}
			}
		}
	`, largePred)

	_, err := processQuery(context.Background(), t, query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Predicate name length cannot be bigger than 2^16")
}

func TestQueryUnknownType(t *testing.T) {
	query := `schema(type: UnknownType) {}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {}}`, js)
}

func TestQuerySingleType(t *testing.T) {
	query := `schema(type: Person) {}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"types":[{"fields":[{"name":"name"},{"name":"pet"},
	{"name":"friend"},{"name":"gender"},{"name":"alive"}],"name":"Person"}]}}`,
		js)
}

func TestQueryMultipleTypes(t *testing.T) {
	query := `schema(type: [Person, Animal]) {}`
	js := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"types":[{"fields":[{"name":"name"}],"name":"Animal"},
	{"fields":[{"name":"name"},{"name":"pet"},{"name":"friend"},{"name":"gender"},
	{"name":"alive"}],"name":"Person"}]}}`, js)
}

func TestRegexInFilterNoDataOnRoot(t *testing.T) {
	query := `
		{
			q(func: has(nonExistent)) @filter(regexp(make, /.*han/i)) {
				uid
				firstName
				lastName
			}
		}
	`
	res := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"q":[]}}`, res)
}

func TestRegexInFilterIndexedPredOnRoot(t *testing.T) {
	query := `
		{
			q(func: regexp(name, /.*nonExistent/i)) {
				uid
				firstName
				lastName
			}
		}
	`
	res := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data":{"q":[]}}`, res)
}

func TestMultiRegexInFilter(t *testing.T) {
	query := `
		{
			q(func: has(full_name)) @filter(regexp(full_name, /.*michonne/i) OR regexp(name, /.*michonne/i)) {
				expand(_all_)
			}
		}
	`
	res := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": [{"alive":true, "gender":"female",
	"name":"Michonne"}]}}`, res)
}

func TestMultiRegexInFilter2(t *testing.T) {
	query := `
		{
			q(func: has(firstName)) @filter(regexp(firstName, /.*han/i) OR regexp(lastName, /.*han/i)) {
				firstName
				lastName
			}
		}
	`

	// run 20 times ensure that there is no data race
	// https://github.com/dgraph-io/dgraph/issues/4030
	for i := 0; i < 20; i++ {
		res := processQueryNoErr(t, query)
		require.JSONEq(t, `{"data": {"q": [{"firstName": "Han", "lastName":"Solo"}]}}`, res)
	}
}

func TestRegexFuncWithAfter(t *testing.T) {
	query := `
		{
			q(func: regexp(name, /^Ali/i), after: 0x2710) {
				uid
				name
			}
		}
	`

	res := processQueryNoErr(t, query)
	require.JSONEq(t, `{"data": {"q": [{"name": "Alice", "uid": "0x2712"}, {"name": "Alice", "uid": "0x2714"}]}}`, res)
}
