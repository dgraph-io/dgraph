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

package schema

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200/protos/api"

	"github.com/dgraph-io/dgraph/worker"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

const (
	groupOneServer        = "http://localhost:8180/graphql"
	groupOneAdminServer   = "http://localhost:8180/admin"
	groupOnegRPC          = "localhost:9180"
	groupTwoServer        = "http://localhost:8182/graphql"
	groupTwoAdminServer   = "http://localhost:8182/admin"
	groupThreeServer      = "http://localhost:8183/graphql"
	groupThreeAdminServer = "http://localhost:8183/admin"
)

// This test is supposed to test the graphql schema subscribe feature. Whenever schema is updated
// in a dgraph alpha for one group, that update should also be propagated to alpha nodes in other
// groups.
func TestSchemaSubscribe(t *testing.T) {
	schema := `
	type Author {
		id: ID!
		name: String!
	}`
	updateGQLSchemaRequireNoErrors(t, schema, groupOneAdminServer)

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

	// Now update schema on an alpha node for group 3 and see if nodes in group 1 and 2 also get it.
	schema = `
	type Author {
		id: ID!
		name: String!
		posts: [Post]
	}

	interface Post {
		id: ID!
	}`
	updateGQLSchemaRequireNoErrors(t, schema, groupThreeAdminServer)

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

// TestConcurrentSchemaUpdates checks that if there are too many concurrent requests to update the
// GraphQL schema, then the system works as expected by either:
// 	1. failing the schema update because there is another one in progress, OR
// 	2. if the schema update succeeds, then the last successful schema update is reflected by both
//	Dgraph and GraphQL schema
//
// It also makes sure that only one node exists for GraphQL schema in Dgraph after all the
// concurrent requests have executed.
func TestConcurrentSchemaUpdates(t *testing.T) {
	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	testutil.DropAll(t, dg)

	tcases := []struct {
		graphQLSchema string
		dgraphSchema  string
		url           string
	}{
		{
			graphQLSchema: `
			type A {
				b: String!
			}`,
			dgraphSchema: `{
            "predicate": "A.b",
            "type": "string"
        }`,
			url: groupOneAdminServer,
		},
		{
			graphQLSchema: `
			type A {
				b: String! @search(by: [term])
			}`,
			dgraphSchema: `{
            "predicate": "A.b",
            "type": "string",
            "index": true,
            "tokenizer": [
                "term"
            ]
        }`,
			url: groupTwoAdminServer,
		},
		{
			graphQLSchema: `
			type A {
				b: String! @search(by: [exact])
			}`,
			dgraphSchema: `{
            "predicate": "A.b",
            "type": "string",
            "index": true,
            "tokenizer": [
                "exact"
            ]
        }`,
			url: groupThreeAdminServer,
		},
	}

	numTcases := len(tcases)
	numRequests := 100
	var lastSuccessReqTimestamp int64 = -1
	lastSuccessTcaseIdx := -1

	mux := sync.Mutex{}
	wg := sync.WaitGroup{}

	// send too many concurrent schema update requests to different servers
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(reqIdx int) {
			tcaseIdx := reqIdx % numTcases
			// if the update succeeded, save the success request timestamp and tcase index
			if updateGQLSchemaConcurrent(t, tcases[tcaseIdx].graphQLSchema, tcases[tcaseIdx].url) {
				now := time.Now().UnixNano()
				mux.Lock()
				if now > lastSuccessReqTimestamp {
					lastSuccessReqTimestamp = now
					lastSuccessTcaseIdx = tcaseIdx
				}
				mux.Unlock()
			}
			wg.Done()
		}(i)
	}

	// wait for all of them to finish
	wg.Wait()

	// make sure at least one update request succeeded
	require.GreaterOrEqual(t, lastSuccessReqTimestamp, int64(0))
	require.GreaterOrEqual(t, lastSuccessTcaseIdx, 0)

	// build the final Dgraph schema
	finalDgraphSchema := fmt.Sprintf(`{
    "schema": [
        %s,
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
}`, tcases[lastSuccessTcaseIdx].dgraphSchema)
	finalGraphQLSchema := tcases[lastSuccessTcaseIdx].graphQLSchema

	// now check that both the final GraphQL schema and Dgraph schema are the ones we expect
	require.Equal(t, finalGraphQLSchema, getGQLSchemaRequireId(t, groupOneAdminServer))
	require.JSONEq(t, finalDgraphSchema, getDgraphSchema(t, dg))

	// now check that there is exactly one node for GraphQL schema in Dgraph,
	// and that contains the same schema as the one we expect
	res, err := dg.NewReadOnlyTxn().Query(context.Background(), `
	query {
		gqlSchema(func: has(dgraph.graphql.schema)) {
			uid
			dgraph.graphql.schema
		}
	}`)
	require.NoError(t, err)

	var resp struct {
		GqlSchema []struct {
			Uid    string
			Schema string `json:"dgraph.graphql.schema"`
		}
	}
	require.NoError(t, json.Unmarshal(res.GetJson(), &resp))
	require.Len(t, resp.GqlSchema, 1)
	require.Equal(t, finalGraphQLSchema, resp.GqlSchema[0].Schema)
}

// TestUpdateGQLSchemaAfterDropAll makes sure that updating the GraphQL schema after drop_all works
func TestUpdateGQLSchemaAfterDropAll(t *testing.T) {
	updateGQLSchemaRequireNoErrors(t, `
			type A {
				b: String!
			}`, groupOneAdminServer)

	// now do drop_all
	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	testutil.DropAll(t, dg)

	// need to wait a bit, because the update notification takes time to reach the alpha
	time.Sleep(time.Second)
	// now retrieving the GraphQL schema should report no schema
	gqlSchema := getGQLSchemaRequireId(t, groupOneAdminServer)
	require.Empty(t, gqlSchema)

	// updating the schema now should work
	schema := `
			type A {
				b: String! @id
			}`
	updateGQLSchemaRequireNoErrors(t, schema, groupOneAdminServer)
	// we should get the schema we expect
	require.Equal(t, schema, getGQLSchemaRequireId(t, groupOneAdminServer))
}

// TestGQLSchemaAfterDropData checks whether if the schema still exists after drop_data
func TestGQLSchemaAfterDropData(t *testing.T) {
	schema := `
			type A {
				b: String!
			}`
	updateGQLSchemaRequireNoErrors(t, schema, groupOneAdminServer)

	// now do drop_data
	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropOp: api.Operation_DATA}))

	// lets wait a bit to be sure that the update notification has reached the alpha,
	// otherwise we are anyways gonna get the previous schema from the in-memory schema
	time.Sleep(time.Second)
	// we should still get the schema we inserted earlier
	require.Equal(t, schema, getGQLSchemaRequireId(t, groupOneAdminServer))

}

func updateGQLSchema(t *testing.T, schema, url string) *common.GraphQLResponse {
	req := &common.GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					schema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": schema},
	}
	resp := req.ExecuteAsPost(t, url)
	require.NotNil(t, resp)
	return resp
}

func updateGQLSchemaRequireNoErrors(t *testing.T, schema, url string) {
	require.Nil(t, updateGQLSchema(t, schema, url).Errors)
}

func updateGQLSchemaConcurrent(t *testing.T, schema, url string) bool {
	res := updateGQLSchema(t, schema, url)
	err := res.Errors.Error()
	require.NotContains(t, err, worker.ErrMultipleGraphQLSchemaNodes)
	require.NotContains(t, err, worker.ErrGraphQLSchemaAlterFailed)

	return res.Errors == nil
}

type gqlSchema struct {
	Id     string
	Schema string
}

func getGQLSchema(t *testing.T, url string) gqlSchema {
	get := &common.GraphQLParams{
		Query: `query {
			getGQLSchema {
				id
				schema
			}
		}`,
	}
	getResult := get.ExecuteAsPost(t, url)
	require.Nil(t, getResult.Errors)

	var resp struct {
		GetGQLSchema gqlSchema
	}
	require.NoError(t, json.Unmarshal(getResult.Data, &resp))

	return resp.GetGQLSchema
}

func getGQLSchemaRequireId(t *testing.T, url string) string {
	schema := getGQLSchema(t, url)
	require.NotEmpty(t, schema.Id, "Got empty ID in getGQLSchema")
	return schema.Schema
}

func getDgraphSchema(t *testing.T, dg *dgo.Dgraph) string {
	resp, err := dg.NewReadOnlyTxn().Query(context.Background(), "schema {}")
	require.NoError(t, err)

	return string(resp.GetJson())
}
