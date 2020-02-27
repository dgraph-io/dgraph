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
	"context"
	"encoding/json"
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

type rule struct {
	Predicate  string `json:"predicate"`
	Permission uint   `json:"permission"`
}

type group struct {
	Name  string `json:"name"`
	Rules []rule `json:"rules"`
}

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

func testAddUpdateGroupWithDuplicateRules(t *testing.T) {
	groupName := "dev"
	addedRules := []rule{
		{
			Predicate:  "test",
			Permission: 1,
		},
		{
			Predicate:  "test",
			Permission: 2,
		},
		{
			Predicate:  "test1",
			Permission: 3,
		},
	}

	addedGroup := addGroup(t, groupName, addedRules)

	require.Equal(t, groupName, addedGroup.Name)
	require.Len(t, addedGroup.Rules, 2)
	require.ElementsMatch(t, addedRules[1:], addedGroup.Rules)

	updatedRules := []rule{
		{
			Predicate:  "test",
			Permission: 3,
		},
		{
			Predicate:  "test2",
			Permission: 1,
		},
		{
			Predicate:  "test2",
			Permission: 2,
		},
	}
	updatedGroup := updateGroup(t, groupName, updatedRules, nil)

	require.Equal(t, groupName, updatedGroup.Name)
	require.Len(t, updatedGroup.Rules, 3)
	require.ElementsMatch(t, []rule{updatedRules[0], addedRules[2], updatedRules[2]},
		updatedGroup.Rules)

	updatedGroup1 := updateGroup(t, groupName, nil, []string{"test1"})

	require.Equal(t, groupName, updatedGroup1.Name)
	require.Len(t, updatedGroup1.Rules, 2)
	require.ElementsMatch(t, []rule{updatedRules[0], updatedRules[2]}, updatedGroup1.Rules)

	deleteGroup(t, groupName)
}

func addGroup(t *testing.T, name string, rules []rule) *group {
	queryParams := &GraphQLParams{
		Query: `mutation addGroup($name: String!, $rules: [RuleRef]){
			addGroup(input: [
				{
					name: $name
					rules: $rules
				}
			]) {
				group {
					name
					rules {
						predicate
						permission
					}
				}
			}
		}`,
		Variables: map[string]interface{}{
			"name":  name,
			"rules": rules,
		},
	}
	gqlResponse := queryParams.ExecuteAsPost(t, graphqlAdminTestAdminURL)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		AddGroup struct {
			Group []group
		}
	}
	err := json.Unmarshal(gqlResponse.Data, &result)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.AddGroup)
	require.NotNil(t, result.AddGroup.Group)
	require.Len(t, result.AddGroup.Group, 1)

	return &result.AddGroup.Group[0]
}

func updateGroup(t *testing.T, name string, setRules []rule, removeRules []string) *group {
	queryParams := &GraphQLParams{
		Query: `mutation updateGroup($name: String!, $setRules: [RuleRef], $removeRules: [String]){
			updateGroup(input: {
				filter: {
					name: {
						eq: $name
					}
				}
				set: {
					rules: $setRules
				}
				remove: {
					rules: $removeRules
				}
			}) {
				group {
					name
					rules {
						predicate
						permission
					}
				}
			}
		}`,
		Variables: map[string]interface{}{
			"name":        name,
			"setRules":    setRules,
			"removeRules": removeRules,
		},
	}
	gqlResponse := queryParams.ExecuteAsPost(t, graphqlAdminTestAdminURL)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		UpdateGroup struct {
			Group []group
		}
	}
	err := json.Unmarshal(gqlResponse.Data, &result)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.UpdateGroup)
	require.NotNil(t, result.UpdateGroup.Group)
	require.Len(t, result.UpdateGroup.Group, 1)

	return &result.UpdateGroup.Group[0]
}

func deleteGroup(t *testing.T, name string) {
	queryParams := &GraphQLParams{
		Query: `mutation deleteGroup($name: String!){
			deleteGroup(filter: {
				name : {
					eq: $name
				}
			}) {
				msg
			}
		}`,
		Variables: map[string]interface{}{
			"name": name,
		},
	}
	gqlResponse := queryParams.ExecuteAsPost(t, graphqlAdminTestAdminURL)
	requireNoGQLErrors(t, gqlResponse)

	var result struct {
		DeleteGroup struct {
			Msg string
		}
	}
	err := json.Unmarshal(gqlResponse.Data, &result)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.DeleteGroup)
	require.Equal(t, result.DeleteGroup.Msg, "Deleted")
}
