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

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
)

const (
	// Dgraph schema should look like this if the GraphQL layer has started and
	// successfully connected
	initSchema = `{
    "schema": [
        {
            "predicate": "dgraph.cors",
            "type": "string",
            "list": true,
            "index": true,
            "tokenizer": [
             "exact"
            ],
            "upsert": true
        },
        {
            "predicate": "dgraph.drop.op",
            "type": "string"
        },
        {
            "predicate":"dgraph.graphql.p_query",
            "type":"string"
        },
        {
            "predicate":"dgraph.graphql.p_sha256hash",
            "type":"string",
            "index":true,
            "tokenizer":["exact"]
        },
        {
            "predicate": "dgraph.graphql.schema",
            "type": "string"
        },
        {
            "predicate": "dgraph.graphql.schema_created_at",
            "type": "datetime"
		},
        {
            "predicate": "dgraph.graphql.schema_history",
            "type": "string"
		},
        {
            "predicate": "dgraph.graphql.xid",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "upsert": true
        },
        {
            "predicate": "dgraph.type",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "list": true
        }
    ],
    "types": [
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema"
                },{
                    "name": "dgraph.graphql.xid"
                }
            ],
            "name": "dgraph.graphql"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema_history"
                },{
                    "name": "dgraph.graphql.schema_created_at"
                }
            ],
            "name": "dgraph.graphql.history"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.p_query"
                },
                {
                    "name": "dgraph.graphql.p_sha256hash"
                }
            ],
            "name": "dgraph.graphql.persisted_query"
        }
    ]
}`

	firstTypes = `
	type A {
		b: String
	}`
	firstSchema = `{
    "schema": [
        {
            "predicate": "A.b",
            "type": "string"
        },
        {
            "predicate": "dgraph.cors",
            "type": "string",
            "list": true,
            "index": true,
            "tokenizer": [
             "exact"
            ],
            "upsert": true
        },
        {
            "predicate": "dgraph.drop.op",
            "type": "string"
        },
        {
            "predicate":"dgraph.graphql.p_query",
            "type":"string"
        },
        {
            "predicate":"dgraph.graphql.p_sha256hash",
            "type":"string",
            "index":true,
            "tokenizer":["exact"]
        },
        {
            "predicate": "dgraph.graphql.schema",
            "type": "string"
        },
        {
            "predicate": "dgraph.graphql.schema_created_at",
            "type": "datetime"
		},
        {
            "predicate": "dgraph.graphql.schema_history",
            "type": "string"
		},
        {
            "predicate": "dgraph.graphql.xid",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "upsert": true
        },
        {
            "predicate": "dgraph.type",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "list": true
        }
    ],
    "types": [
        {
            "fields": [
                {
                    "name": "A.b"
                }
            ],
            "name": "A"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema"
                },{
                    "name": "dgraph.graphql.xid"
                }
            ],
            "name": "dgraph.graphql"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema_history"
                },{
                    "name": "dgraph.graphql.schema_created_at"
                }
            ],
            "name": "dgraph.graphql.history"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.p_query"
                },
                {
                    "name": "dgraph.graphql.p_sha256hash"
                }
            ],
            "name": "dgraph.graphql.persisted_query"
        }
    ]
}`
	firstGQLSchema = `{
    "__type": {
        "name": "A",
        "fields": [
            {
                "name": "b"
            }
        ]
    }
}`

	updatedTypes = `
	type A {
		b: String
		c: Int
	}`
	updatedSchema = `{
    "schema": [
        {
            "predicate": "A.b",
            "type": "string"
        },
        {
            "predicate": "A.c",
            "type": "int"
        },
        {
            "predicate": "dgraph.cors",
            "type": "string",
            "list": true,
            "index": true,
            "tokenizer": [
             "exact"
            ],
            "upsert": true
        },
        {
            "predicate": "dgraph.drop.op",
            "type": "string"
        },
        {
            "predicate":"dgraph.graphql.p_query",
            "type":"string"
        },
        {
            "predicate":"dgraph.graphql.p_sha256hash",
            "type":"string",
            "index":true,
            "tokenizer":["exact"]
        },
        {
            "predicate": "dgraph.graphql.schema",
            "type": "string"
        },
        {
            "predicate": "dgraph.graphql.schema_created_at",
            "type": "datetime"
		},
        {
            "predicate": "dgraph.graphql.schema_history",
            "type": "string"
		},
        {
            "predicate": "dgraph.graphql.xid",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "upsert": true
        },
        {
            "predicate": "dgraph.type",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "list": true
        }
    ],
    "types": [
        {
            "fields": [
                {
                    "name": "A.b"
                },
                {
                    "name": "A.c"
                }
            ],
            "name": "A"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema"
                },{
                    "name": "dgraph.graphql.xid"
                }
            ],
            "name": "dgraph.graphql"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema_history"
                },{
                    "name": "dgraph.graphql.schema_created_at"
                }
            ],
            "name": "dgraph.graphql.history"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.p_query"
                },
                {
                    "name": "dgraph.graphql.p_sha256hash"
                }
            ],
            "name": "dgraph.graphql.persisted_query"
        }
    ]
}`
	updatedGQLSchema = `{
    "__type": {
        "name": "A",
        "fields": [
            {
                "name": "b"
            },
            {
                "name": "c"
            }
        ]
    }
}`

	adminSchemaEndptTypes = `
	type A {
		b: String
		c: Int
		d: Float
	}`
	adminSchemaEndptSchema = `{
    "schema": [
        {
            "predicate": "A.b",
            "type": "string"
        },
        {
            "predicate": "A.c",
            "type": "int"
        },
        {
            "predicate": "A.d",
            "type": "float"
        },
        {
            "predicate": "dgraph.cors",
            "type": "string",
            "list": true,
            "index": true,
            "tokenizer": [
             "exact"
            ],
            "upsert": true
        },
        {
            "predicate": "dgraph.drop.op",
            "type": "string"
        },
        {
            "predicate":"dgraph.graphql.p_query",
            "type":"string"
        },
        {
            "predicate":"dgraph.graphql.p_sha256hash",
            "type":"string",
            "index":true,
            "tokenizer":["exact"]
        },
        {
            "predicate": "dgraph.graphql.schema",
            "type": "string"
        },
        {
            "predicate": "dgraph.graphql.schema_created_at",
            "type": "datetime"
		},
        {
            "predicate": "dgraph.graphql.schema_history",
            "type": "string"
		},
        {
            "predicate": "dgraph.graphql.xid",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "upsert": true
        },
        {
            "predicate": "dgraph.type",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ],
            "list": true
        }
    ],
    "types": [
        {
            "fields": [
                {
                    "name": "A.b"
                },
                {
                    "name": "A.c"
                },
                {
                    "name": "A.d"
                }
            ],
            "name": "A"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema"
                },{
                    "name": "dgraph.graphql.xid"
                }
            ],
            "name": "dgraph.graphql"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.schema_history"
                },{
                    "name": "dgraph.graphql.schema_created_at"
                }
            ],
            "name": "dgraph.graphql.history"
        },
        {
            "fields": [
                {
                    "name": "dgraph.graphql.p_query"
                },
                {
                    "name": "dgraph.graphql.p_sha256hash"
                }
            ],
            "name": "dgraph.graphql.persisted_query"
        }
    ]
}`
	adminSchemaEndptGQLSchema = `{
    "__type": {
        "name": "A",
        "fields": [
            {
                "name": "b"
            },
            {
                "name": "c"
            },
            {
                "name": "d"
            }
        ]
    }
}`
)

func admin(t *testing.T) {
	d, err := grpc.Dial(AlphagRPC, grpc.WithInsecure())
	require.NoError(t, err)

	client := dgo.NewDgraphClient(api.NewDgraphClient(d))
	testutil.DropAll(t, client)

	hasSchema, err := hasCurrentGraphQLSchema(GraphqlAdminURL)
	require.NoError(t, err)
	require.False(t, hasSchema)

	schemaIsInInitialState(t, client)
	addGQLSchema(t, client)
	updateSchema(t, client)
	updateSchemaThroughAdminSchemaEndpt(t, client)
	gqlSchemaNodeHasXid(t, client)

	// restore the state to the initial schema and data.
	testutil.DropAll(t, client)

	schemaFile := "schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		panic(err)
	}

	jsonFile := "test_data.json"
	data, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		panic(errors.Wrapf(err, "Unable to read file %s.", jsonFile))
	}

	addSchemaAndData(schema, data, client)
}

func schemaIsInInitialState(t *testing.T, client *dgo.Dgraph) {
	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)
	require.JSONEq(t, initSchema, string(resp.GetJson()))
}

func addGQLSchema(t *testing.T, client *dgo.Dgraph) {
	err := addSchema(GraphqlAdminURL, firstTypes)
	require.NoError(t, err)

	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	require.JSONEq(t, firstSchema, string(resp.GetJson()))

	introspect(t, firstGQLSchema)
}

func updateSchema(t *testing.T, client *dgo.Dgraph) {
	err := addSchema(GraphqlAdminURL, updatedTypes)
	require.NoError(t, err)

	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	require.JSONEq(t, updatedSchema, string(resp.GetJson()))

	introspect(t, updatedGQLSchema)
}

func updateSchemaThroughAdminSchemaEndpt(t *testing.T, client *dgo.Dgraph) {
	err := addSchemaThroughAdminSchemaEndpt(graphqlAdminTestAdminSchemaURL, adminSchemaEndptTypes)
	require.NoError(t, err)

	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	require.JSONEq(t, adminSchemaEndptSchema, string(resp.GetJson()))

	introspect(t, adminSchemaEndptGQLSchema)
}

func gqlSchemaNodeHasXid(t *testing.T, client *dgo.Dgraph) {
	resp, err := client.NewReadOnlyTxn().Query(context.Background(), `query {
		gqlSchema(func: type(dgraph.graphql)) {
			dgraph.graphql.xid
		}
	}`)
	require.NoError(t, err)
	// confirm that there is only one node of type dgraph.graphql and it has xid.
	require.JSONEq(t, `{
		"gqlSchema": [{
			"dgraph.graphql.xid": "dgraph.graphql.schema"
		}]
	}`, string(resp.GetJson()))
}

func introspect(t *testing.T, expected string) {
	queryParams := &GraphQLParams{
		Query: `query {
			__type(name: "A") {
				name
				fields {
					name
				}
			}
		}`,
	}

	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	require.JSONEq(t, expected, string(gqlResponse.Data))
}

// The GraphQL /admin health result should be the same as /health
func health(t *testing.T) {
	queryParams := &GraphQLParams{
		Query: `query {
        health {
          instance
          address
          status
          group
          version
          uptime
          lastEcho
          ee_features
        }
      }`,
	}
	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlAdminURL)
	RequireNoGQLErrors(t, gqlResponse)

	var result struct {
		Health []pb.HealthInfo
	}

	err := json.Unmarshal([]byte(gqlResponse.Data), &result)
	require.NoError(t, err)

	var health []pb.HealthInfo
	resp, err := http.Get(adminDgraphHealthURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	healthRes, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(healthRes, &health))

	// Uptime and LastEcho might have changed between the GraphQL and /health calls.
	// If we don't remove them, the test would be flakey.
	opts := []cmp.Option{
		cmpopts.IgnoreFields(pb.HealthInfo{}, "Uptime"),
		cmpopts.IgnoreFields(pb.HealthInfo{}, "LastEcho"),
		cmpopts.IgnoreFields(pb.HealthInfo{}, "Ongoing"),
		cmpopts.EquateEmpty(),
	}
	if diff := cmp.Diff(health, result.Health, opts...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}

func partialHealth(t *testing.T) {
	queryParams := &GraphQLParams{
		Query: `query {
            health {
              instance
              status
              group
            }
        }`,
	}
	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlAdminURL)
	RequireNoGQLErrors(t, gqlResponse)
	testutil.CompareJSON(t, `{
        "health": [
          {
            "instance": "zero",
            "status": "healthy",
            "group": "0"
          },
          {
            "instance": "alpha",
            "status": "healthy",
            "group": "1"
          }
        ]
      }`, string(gqlResponse.Data))
}

// The /admin endpoints should respond to alias
func adminAlias(t *testing.T) {
	queryParams := &GraphQLParams{
		Query: `query {
            dgraphHealth: health {
              type: instance
              status
              inGroup: group
            }
        }`,
	}
	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlAdminURL)
	RequireNoGQLErrors(t, gqlResponse)
	testutil.CompareJSON(t, `{
        "dgraphHealth": [
          {
            "type": "zero",
            "status": "healthy",
            "inGroup": "0"
          },
          {
            "type": "alpha",
            "status": "healthy",
            "inGroup": "1"
          }
        ]
      }`, string(gqlResponse.Data))
}

// The GraphQL /admin state result should be the same as /state
func adminState(t *testing.T) {
	queryParams := &GraphQLParams{
		Query: `query {
			state {
				counter
				groups {
					id
					members {
						id
						groupId
						addr
						leader
						amDead
						lastUpdate
						clusterInfoOnly
						forceGroupId
					}
					tablets {
						groupId
						predicate
						force
						space
						remove
						readOnly
						moveTs
					}
					snapshotTs
				}
				zeros {
					id
					groupId
					addr
					leader
					amDead
					lastUpdate
					clusterInfoOnly
					forceGroupId
				}
				maxLeaseId
				maxTxnTs
				maxRaftId
				removed {
					id
					groupId
					addr
					leader
					amDead
					lastUpdate
					clusterInfoOnly
					forceGroupId
				}
				cid
				license {
					user
					expiryTs
					enabled
				}
			}
		}`,
	}
	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlAdminURL)
	RequireNoGQLErrors(t, gqlResponse)

	var result struct {
		State struct {
			Counter uint64
			Groups  []struct {
				Id         uint32
				Members    []*pb.Member
				Tablets    []*pb.Tablet
				SnapshotTs uint64
			}
			Zeros      []*pb.Member
			MaxLeaseId uint64
			MaxTxnTs   uint64
			MaxRaftId  uint64
			Removed    []*pb.Member
			Cid        string
			License    struct {
				User     string
				ExpiryTs int64
				Enabled  bool
			}
		}
	}

	err := json.Unmarshal(gqlResponse.Data, &result)
	require.NoError(t, err)

	var state pb.MembershipState
	resp, err := http.Get(adminDgraphStateURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	stateRes, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(stateRes), &state))

	require.Equal(t, state.Counter, result.State.Counter)
	for _, group := range result.State.Groups {
		require.Contains(t, state.Groups, group.Id)
		expectedGroup := state.Groups[group.Id]

		for _, member := range group.Members {
			require.Contains(t, expectedGroup.Members, member.Id)
			expectedMember := expectedGroup.Members[member.Id]

			require.Equal(t, expectedMember, member)
		}

		for _, tablet := range group.Tablets {
			require.Contains(t, expectedGroup.Tablets, tablet.Predicate)
			expectedTablet := expectedGroup.Tablets[tablet.Predicate]

			require.Equal(t, expectedTablet, tablet)
		}

		require.Equal(t, expectedGroup.SnapshotTs, group.SnapshotTs)
	}
	for _, zero := range result.State.Zeros {
		require.Contains(t, state.Zeros, zero.Id)
		expectedZero := state.Zeros[zero.Id]

		require.Equal(t, expectedZero, zero)
	}
	require.Equal(t, state.MaxLeaseId, result.State.MaxLeaseId)
	require.Equal(t, state.MaxTxnTs, result.State.MaxTxnTs)
	require.Equal(t, state.MaxRaftId, result.State.MaxRaftId)
	require.True(t, len(state.Removed) == len(result.State.Removed))
	if len(state.Removed) != 0 {
		require.Equal(t, state.Removed, result.State.Removed)
	}
	require.Equal(t, state.Cid, result.State.Cid)
	require.Equal(t, state.License.User, result.State.License.User)
	require.Equal(t, state.License.ExpiryTs, result.State.License.ExpiryTs)
	require.Equal(t, state.License.Enabled, result.State.License.Enabled)
}

func testCors(t *testing.T) {
	t.Run("testing normal retrieval", func(t *testing.T) {
		queryParams := &GraphQLParams{
			Query: `query{
                getAllowedCORSOrigins{
                  acceptedOrigins
                }
              }`,
		}
		gqlResponse := queryParams.ExecuteAsPost(t, GraphqlAdminURL)
		RequireNoGQLErrors(t, gqlResponse)
		require.JSONEq(t, ` {
            "getAllowedCORSOrigins": {
              "acceptedOrigins": [
                "*"
              ]
            }
          }`, string(gqlResponse.Data))
	})

	t.Run("mutating cors", func(t *testing.T) {
		queryParams := &GraphQLParams{
			Query: `mutation{
                replaceAllowedCORSOrigins(origins:["google.com"]){
                  acceptedOrigins
                }
              }`,
		}
		gqlResponse := queryParams.ExecuteAsPost(t, GraphqlAdminURL)
		RequireNoGQLErrors(t, gqlResponse)
		require.JSONEq(t, ` {
            "replaceAllowedCORSOrigins": {
              "acceptedOrigins": [
                "google.com"
              ]
            }
          }`, string(gqlResponse.Data))
	})

	t.Run("retrieve mutated cors", func(t *testing.T) {
		queryParams := &GraphQLParams{
			Query: `query{
                getAllowedCORSOrigins{
                  acceptedOrigins
                }
              }`,
		}
		gqlResponse := queryParams.ExecuteAsPost(t, GraphqlAdminURL)
		RequireNoGQLErrors(t, gqlResponse)
		require.JSONEq(t, ` {
            "getAllowedCORSOrigins": {
              "acceptedOrigins": [
                "google.com"
              ]
            }
          }`, string(gqlResponse.Data))

		// Wait for the subscription to hit.
		time.Sleep(2 * time.Second)

		client := &http.Client{}
		req, err := http.NewRequest("GET", GraphqlURL, nil)
		require.NoError(t, err)
		req.Header.Add("Origin", "google.com")
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.Header.Get("Access-Control-Allow-Origin"), "google.com")
		require.Equal(t, resp.Header.Get("Access-Control-Allow-Methods"), "POST, OPTIONS")
		require.Equal(t, resp.Header.Get("Access-Control-Allow-Headers"), x.AccessControlAllowedHeaders)
		require.Equal(t, resp.Header.Get("Access-Control-Allow-Credentials"), "true")

		client = &http.Client{}
		req, err = http.NewRequest("GET", GraphqlURL, nil)
		require.NoError(t, err)
		req.Header.Add("Origin", "googl.com")
		resp, err = client.Do(req)
		require.NoError(t, err)
		require.Equal(t, resp.Header.Get("Access-Control-Allow-Origin"), "")
		require.Equal(t, resp.Header.Get("Access-Control-Allow-Methods"), "")
		require.Equal(t, resp.Header.Get("Access-Control-Allow-Credentials"), "")
	})

	t.Run("mutating empty cors", func(t *testing.T) {
		queryParams := &GraphQLParams{
			Query: `mutation{
                replaceAllowedCORSOrigins(origins:[]){
                  acceptedOrigins
                }
              }`,
		}
		gqlResponse := queryParams.ExecuteAsPost(t, GraphqlAdminURL)
		RequireNoGQLErrors(t, gqlResponse)
		require.JSONEq(t, ` {
            "replaceAllowedCORSOrigins": {
              "acceptedOrigins": [
                "*"
              ]
            }
          }`, string(gqlResponse.Data))
	})
}
