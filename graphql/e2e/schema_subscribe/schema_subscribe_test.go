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

package schema_subscribe

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

const (
	groupOneServer        = "http://localhost:8180/graphql"
	groupOneAdminServer   = "http://localhost:8180/admin"
	groupOneAdmingRPC     = "localhost:9280"
	groupTwoServer        = "http://localhost:8182/graphql"
	groupTwoAdminServer   = "http://localhost:8182/admin"
	groupThreeServer      = "http://localhost:8183/graphql"
	groupThreeAdminServer = "http://localhost:8183/admin"

	finalDgraphSchema = `{
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
        }
    ]
}`
)

// This test is supposed to test the graphql schema subscribe feature. Whenever schema is updated
// in a dgraph alpha for one group, that update should also be propogated to alpha nodes in other
// groups.
func TestSchemaSubscribe(t *testing.T) {
	schema := `
	type Author {
		id: ID!
		name: String!
	}`
	updateGQLSchema(t, schema, groupOneAdminServer)

	introspectionQuery := `
	query {
		__type(name: "Author") {
			name
			fields {
				name
			}
		}
	}`
	introspect := &common.GraphQLParams{
		Query: introspectionQuery,
	}

	expectedResult := `{"__type":{"name":"Author","fields":[{"name":"id"},{"name":"name"}]}}`
	introspectionResult := introspect.ExecuteAsPost(t, groupOneServer)
	require.Nil(t, introspectionResult.Errors)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))

	introspectionResult = introspect.ExecuteAsPost(t, groupTwoServer)
	require.Nil(t, introspectionResult.Errors)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))

	introspectionResult = introspect.ExecuteAsPost(t, groupThreeServer)
	require.Nil(t, introspectionResult.Errors)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))

	// Now update schema on an alpha node for group 3 and see nodes in group 1 and 2 also get it.
	schema = `
	type Author {
		id: ID!
		name: String!
		posts: [Post]
	}

	interface Post {
		id: ID!
	}`
	updateGQLSchema(t, schema, groupThreeAdminServer)

	expectedResult = `{"__type":{"name":"Author","fields":[{"name":"id"},{"name":"name"},{"name":"posts"}]}}`
	introspectionResult = introspect.ExecuteAsPost(t, groupOneServer)
	require.Nil(t, introspectionResult.Errors)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))

	introspectionResult = introspect.ExecuteAsPost(t, groupTwoServer)
	require.Nil(t, introspectionResult.Errors)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))

	introspectionResult = introspect.ExecuteAsPost(t, groupThreeServer)
	require.Nil(t, introspectionResult.Errors)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))
}

func TestConcurrentSchemaUpdates(t *testing.T) {
	tcases := []struct {
		schema string
		url    string
	}{
		{
			schema: `
			type A {
				b: String!
			}`,
			url: groupOneAdminServer,
		},
		{
			schema: `
			type A {
				b: String! @search(by: [term])
			}`,
			url: groupTwoAdminServer,
		},
		{
			schema: `
			type A {
				b: String! @search(by: [exact])
			}`,
			url: groupThreeAdminServer,
		},
	}
	numTcases := len(tcases)
	numRequests := 18

	var wg sync.WaitGroup
	// send too many concurrent schema update requests to different servers
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			counter := idx + 1
			fmt.Printf("request: %d, updateGQLSchema: start\n", counter)
			updateGQLSchema(t, tcases[idx%numTcases].schema, tcases[i%numTcases].url)
			fmt.Printf("request: %d, updateGQLSchema: end\n", counter)
			wg.Done()
		}(i)
	}

	fmt.Printf("waiting for all the updateGQLSchema requests to finish...\n")
	wg.Wait()

	// now check that both the final GraphQL schema and Dgraph schema are the one we expect
	require.Equal(t, tcases[(numRequests-1)%numTcases].schema, getGQLSchema(t, groupOneAdminServer))
	require.JSONEq(t, finalDgraphSchema, getDgraphSchema(t, groupOneAdmingRPC))
}

func updateGQLSchema(t *testing.T, schema, url string) {
	add := &common.GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					schema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": schema},
	}
	addResult := add.ExecuteAsPost(t, url)
	require.Nilf(t, addResult.Errors, "Errors: %+v", addResult.Errors)
}

func getGQLSchema(t *testing.T, url string) string {
	get := &common.GraphQLParams{
		Query: `query {
			getGQLSchema {
				schema
			}
		}`,
	}
	getResult := get.ExecuteAsPost(t, url)
	require.Nil(t, getResult.Errors)

	var resp struct {
		GetGQLSchema struct {
			Id     string
			Schema string
		}
	}
	require.NoError(t, json.Unmarshal(getResult.Data, &resp))
	require.NotEmpty(t, resp.GetGQLSchema.Id, "Got empty ID in getGQLSchema")

	return resp.GetGQLSchema.Schema
}

func getDgraphSchema(t *testing.T, gRPCTarget string) string {
	d, err := grpc.Dial(gRPCTarget, grpc.WithInsecure())
	require.NoError(t, err)

	client := dgo.NewDgraphClient(api.NewDgraphClient(d))
	resp, err := client.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	return string(resp.GetJson())
}
