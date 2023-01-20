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

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/testutil"
)

const (
	firstGqlSchema = `
	type A {
		b: String
	}`
	firstPreds = `
	{
		"predicate": "A.b",
		"type": "string"
	}`
	firstTypes = `
	{
		"fields": [
			{
				"name": "A.b"
			}
		],
		"name": "A"
	}`
	firstIntrospectionResponse = `{
    "__type": {
        "name": "A",
        "fields": [
            {
                "name": "b"
            }
        ]
    }
}`

	updatedGqlSchema = `
	type A {
		b: String
		c: Int
	}`
	updatedPreds = `
	{
		"predicate": "A.b",
		"type": "string"
	},
	{
		"predicate": "A.c",
		"type": "int"
	}`
	updatedTypes = `
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
	}`
	updatedIntrospectionResponse = `{
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

	adminSchemaEndptGqlSchema = `
	type A {
		b: String
		c: Int
		d: Float
	}`
	adminSchemaEndptPreds = `
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
        }`
	adminSchemaEndptTypes = `
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
	}`
	adminSchemaEndptIntrospectionResponse = `{
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
	d, err := grpc.Dial(Alpha1gRPC, grpc.WithInsecure())
	require.NoError(t, err)

	oldCounter := RetryProbeGraphQL(t, Alpha1HTTP, nil).SchemaUpdateCounter
	client := dgo.NewDgraphClient(api.NewDgraphClient(d))
	testutil.DropAll(t, client)
	AssertSchemaUpdateCounterIncrement(t, Alpha1HTTP, oldCounter, nil)

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

	addSchemaAndData(schema, data, client, nil)
}

func schemaIsInInitialState(t *testing.T, client *dgo.Dgraph) {
	testutil.VerifySchema(t, client, testutil.SchemaOptions{ExcludeAclSchema: true})
}

func addGQLSchema(t *testing.T, client *dgo.Dgraph) {
	SafelyUpdateGQLSchemaOnAlpha1(t, firstGqlSchema)

	testutil.VerifySchema(t, client, testutil.SchemaOptions{
		UserPreds:        firstPreds,
		UserTypes:        firstTypes,
		ExcludeAclSchema: true,
	})

	introspect(t, firstIntrospectionResponse)
}

func updateSchema(t *testing.T, client *dgo.Dgraph) {
	SafelyUpdateGQLSchemaOnAlpha1(t, updatedGqlSchema)

	testutil.VerifySchema(t, client, testutil.SchemaOptions{
		UserPreds:        updatedPreds,
		UserTypes:        updatedTypes,
		ExcludeAclSchema: true,
	})

	introspect(t, updatedIntrospectionResponse)
}

func updateSchemaThroughAdminSchemaEndpt(t *testing.T, client *dgo.Dgraph) {
	assertUpdateGqlSchemaUsingAdminSchemaEndpt(t, Alpha1HTTP, adminSchemaEndptGqlSchema, nil)

	testutil.VerifySchema(t, client, testutil.SchemaOptions{
		UserPreds:        adminSchemaEndptPreds,
		UserTypes:        adminSchemaEndptTypes,
		ExcludeAclSchema: true,
	})

	introspect(t, adminSchemaEndptIntrospectionResponse)
}

func gqlSchemaNodeHasXid(t *testing.T, client *dgo.Dgraph) {
	resp, err := client.NewReadOnlyTxn().Query(context.Background(), `query {
		gqlSchema(func: has(dgraph.graphql.schema)) {
			dgraph.graphql.xid
			dgraph.type
		}
	}`)
	require.NoError(t, err)
	// confirm that there is only one node having GraphQL schema, it has xid,
	// and its type is dgraph.graphql
	require.JSONEq(t, `{
		"gqlSchema": [{
			"dgraph.graphql.xid": "dgraph.graphql.schema",
			"dgraph.type": ["dgraph.graphql"]
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
	resp, err := http.Get(dgraphHealthURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	healthRes, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(healthRes, &health))

	// These fields might have changed between the GraphQL and /health calls.
	// If we don't remove them, the test would be flaky.
	opts := []cmp.Option{
		cmpopts.IgnoreFields(pb.HealthInfo{}, "Uptime"),
		cmpopts.IgnoreFields(pb.HealthInfo{}, "LastEcho"),
		cmpopts.IgnoreFields(pb.HealthInfo{}, "Ongoing"),
		cmpopts.IgnoreFields(pb.HealthInfo{}, "MaxAssigned"),
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
				maxUID
				maxTxnTs
				maxNsID
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
					maxNodes
				}
			}
		}`,
	}
	gqlResponse := queryParams.ExecuteAsPost(t, GraphqlAdminURL)
	RequireNoGQLErrors(t, gqlResponse)

	var result struct {
		State struct {
			Groups []struct {
				Id         uint32
				Members    []*pb.Member
				Tablets    []*pb.Tablet
				SnapshotTs uint64
			}
			Zeros     []*pb.Member
			MaxUID    uint64
			MaxTxnTs  uint64
			MaxNsID   uint64
			MaxRaftId uint64
			Removed   []*pb.Member
			Cid       string
			License   struct {
				User     string
				ExpiryTs int64
				Enabled  bool
				MaxNodes uint64
			}
		}
	}

	err := json.Unmarshal(gqlResponse.Data, &result)
	require.NoError(t, err)

	var state pb.MembershipState
	resp, err := http.Get(dgraphStateURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	stateRes, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(stateRes), &state))

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
	require.Equal(t, state.MaxUID, result.State.MaxUID)
	require.Equal(t, state.MaxTxnTs, result.State.MaxTxnTs)
	require.Equal(t, state.MaxNsID, result.State.MaxNsID)
	require.Equal(t, state.MaxRaftId, result.State.MaxRaftId)
	require.True(t, len(state.Removed) == len(result.State.Removed))
	if len(state.Removed) != 0 {
		require.Equal(t, state.Removed, result.State.Removed)
	}
	require.Equal(t, state.Cid, result.State.Cid)
	require.Equal(t, state.License.User, result.State.License.User)
	require.Equal(t, state.License.ExpiryTs, result.State.License.ExpiryTs)
	require.Equal(t, state.License.MaxNodes, result.State.License.MaxNodes)
	require.Equal(t, state.License.Enabled, result.State.License.Enabled)
}
