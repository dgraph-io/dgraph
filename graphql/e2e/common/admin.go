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
	"github.com/gogo/protobuf/jsonpb"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const (
	// Dgraph schema should look like this if the GraphQL layer has started and
	// successfully connected
	initSchema = `{
    "schema": [
        {
            "predicate": "dgraph.graphql.schema",
            "type": "string"
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
                }
            ],
            "name": "dgraph.graphql"
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
            "predicate": "dgraph.graphql.schema",
            "type": "string"
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
                }
            ],
            "name": "dgraph.graphql"
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
            "predicate": "dgraph.graphql.schema",
            "type": "string"
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
                }
            ],
            "name": "dgraph.graphql"
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
            "predicate": "dgraph.graphql.schema",
            "type": "string"
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
                }
            ],
            "name": "dgraph.graphql"
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
	d, err := grpc.Dial(alphaAdminTestgRPC, grpc.WithInsecure())
	require.NoError(t, err)

	client := dgo.NewDgraphClient(api.NewDgraphClient(d))

	hasSchema, err := hasCurrentGraphQLSchema(graphqlAdminTestAdminURL)
	require.NoError(t, err)
	require.False(t, hasSchema)

	schemaIsInInitialState(t, client)
	addGQLSchema(t, client)
	updateSchema(t, client)
	updateSchemaThroughAdminSchemaEndpt(t, client)
}

func schemaIsInInitialState(t *testing.T, client *dgo.Dgraph) {
	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	require.JSONEq(t, initSchema, string(resp.GetJson()))
}

func addGQLSchema(t *testing.T, client *dgo.Dgraph) {
	err := addSchema(graphqlAdminTestAdminURL, firstTypes)
	require.NoError(t, err)

	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	require.JSONEq(t, firstSchema, string(resp.GetJson()))

	introspect(t, firstGQLSchema)
}

func updateSchema(t *testing.T, client *dgo.Dgraph) {
	err := addSchema(graphqlAdminTestAdminURL, updatedTypes)
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

	gqlResponse := queryParams.ExecuteAsPost(t, graphqlAdminTestURL)
	requireNoGQLErrors(t, gqlResponse)

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
        }
      }`,
	}
	gqlResponse := queryParams.ExecuteAsPost(t, graphqlAdminTestAdminURL)
	requireNoGQLErrors(t, gqlResponse)

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
	}
	if diff := cmp.Diff(health, result.Health, opts...); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
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
	gqlResponse := queryParams.ExecuteAsPost(t, graphqlAdminTestAdminURL)
	requireNoGQLErrors(t, gqlResponse)

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
