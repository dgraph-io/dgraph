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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

var (
	groupOneServer        = "http://"+ testutil.ContainerAddr("alpha1", 8080) +"/graphql"
	groupOneAdminServer   = "http://"+ testutil.ContainerAddr("alpha1", 8080) +"/admin"
	groupOnegRPC          = testutil.SockAddr
	groupTwoServer        = "http://"+ testutil.ContainerAddr("alpha2", 8080) +"/graphql"
	groupTwoAdminServer   = "http://"+ testutil.ContainerAddr("alpha2", 8080) +"/admin"
	groupThreeServer      = "http://"+ testutil.ContainerAddr("alpha3", 8080) +"/graphql"
	groupThreeAdminServer = "http://"+ testutil.ContainerAddr("alpha3", 8080) +"/admin"
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

	expectedResult :=
		`{
			"__type": {
				"name":"Author",
				"fields": [
					{
						"name": "id"
					},
					{
						"name": "name"
					}
				]
			}
		}`
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

	expectedResult =
		`{
			"__type": {
				"name": "Author",
				"fields": [
					{
						"name": "id"
					},
					{
						"name": "name"
					},
					{
						"name": "posts"
					},
					{
						"name": "postsAggregate"
					}
				]
			}
		}`
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
			"predicate":"dgraph.drop.op",
			"type":"string"
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

// TestIntrospectionQueryAfterDropAll make sure that Introspection query after drop_all doesn't give any internal error
func TestIntrospectionQueryAfterDropAll(t *testing.T) {
	// First Do the drop_all operation
	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	testutil.DropAll(t, dg)
	// wait for a bit
	time.Sleep(time.Second)

	introspectionQuery := `
	query{
		__schema{
		   types{
			 name
		   }
		}
	}`
	introspect := &common.GraphQLParams{
		Query: introspectionQuery,
	}

	// On doing Introspection Query Now, We should get the Expected Error Message, not the Internal Error.
	introspectionResult := introspect.ExecuteAsPost(t, groupOneServer)
	require.Len(t, introspectionResult.Errors, 1)
	gotErrorMessage := introspectionResult.Errors[0].Message
	expectedErrorMessage := "Not resolving __schema. There's no GraphQL schema in Dgraph.  Use the /admin API to add a GraphQL schema"
	require.Equal(t, expectedErrorMessage, gotErrorMessage)
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

// TestSchemaHistory checks the admin schema history API working properly or not.
func TestSchemaHistory(t *testing.T) {
	// Drop all to remove all the previous schema history.
	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropOp: api.Operation_DATA, RunInBackground: false}))

	// Let's get the schema. It should return empty results.
	get := &common.GraphQLParams{
		Query: `query{
			querySchemaHistory(first:10){
			  schema
			  created_at
			}
		  }`,
	}
	getResult := get.ExecuteAsPost(t, groupOneAdminServer)
	require.Nil(t, getResult.Errors)

	require.JSONEq(t, `{
		"querySchemaHistory": []
	  }`, string(getResult.Data))

	// Let's add an schema and expect the history in the history api.
	updateGQLSchemaRequireNoErrors(t, `
	type A {
		b: String!
	}`, groupOneAdminServer)
	time.Sleep(time.Second)

	getResult = get.ExecuteAsPost(t, groupOneAdminServer)
	require.Nil(t, getResult.Errors)
	type History struct {
		Schema    string `json:"schema"`
		CreatedAt string `json:"created_at"`
	}
	type schemaHistory struct {
		QuerySchemaHistory []History `json:"querySchemaHistory"`
	}
	history := schemaHistory{}
	require.NoError(t, json.Unmarshal(getResult.Data, &history))
	require.Equal(t, int(1), len(history.QuerySchemaHistory))
	require.Equal(t, history.QuerySchemaHistory[0].Schema, "\n\ttype A {\n\t\tb: String!\n\t}")

	// Let's update the same schema. But we should not get the 2 history because, we
	// are updating the same schema.
	updateGQLSchemaRequireNoErrors(t, `
	type A {
		b: String!
	}`, groupOneAdminServer)
	time.Sleep(time.Second)

	getResult = get.ExecuteAsPost(t, groupOneAdminServer)
	require.Nil(t, getResult.Errors)
	history = schemaHistory{}
	require.NoError(t, json.Unmarshal(getResult.Data, &history))
	require.Equal(t, int(1), len(history.QuerySchemaHistory))
	require.Equal(t, history.QuerySchemaHistory[0].Schema, "\n\ttype A {\n\t\tb: String!\n\t}")

	// Let's update a new schema and check the history.
	updateGQLSchemaRequireNoErrors(t, `
	type B {
		b: String!
	}`, groupOneAdminServer)
	time.Sleep(time.Second)

	getResult = get.ExecuteAsPost(t, groupOneAdminServer)
	require.Nil(t, getResult.Errors)
	history = schemaHistory{}
	require.NoError(t, json.Unmarshal(getResult.Data, &history))
	require.Equal(t, int(2), len(history.QuerySchemaHistory))
	require.Equal(t, history.QuerySchemaHistory[0].Schema, "\n\ttype B {\n\t\tb: String!\n\t}")
	require.Equal(t, history.QuerySchemaHistory[1].Schema, "\n\ttype A {\n\t\tb: String!\n\t}")

	// Check offset working properly or not.
	get = &common.GraphQLParams{
		Query: `query{
			querySchemaHistory(first:10, offset:1){
			  schema
			  created_at
			}
		  }`,
	}
	getResult = get.ExecuteAsPost(t, groupOneAdminServer)
	require.Nil(t, getResult.Errors)
	history = schemaHistory{}
	require.NoError(t, json.Unmarshal(getResult.Data, &history))
	require.Equal(t, int(1), len(history.QuerySchemaHistory))
	require.Equal(t, history.QuerySchemaHistory[0].Schema, "\n\ttype A {\n\t\tb: String!\n\t}")

	// Let's drop eveything and see whether we getting empty results are not.
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropOp: api.Operation_DATA, RunInBackground: false}))
	getResult = get.ExecuteAsPost(t, groupOneAdminServer)
	require.Nil(t, getResult.Errors)
	require.JSONEq(t, `{
		"querySchemaHistory": []
	  }`, string(getResult.Data))
}

// verifyEmptySchema verifies that the schema is not set in the GraphQL server.
func verifyEmptySchema(t *testing.T) {
	schema := getGQLSchema(t, groupOneAdminServer)
	require.Empty(t, schema.Schema)
}

func TestGQLSchemaValidate(t *testing.T) {
	testCases := []struct {
		schema string
		errors x.GqlErrorList
		valid  bool
	}{
		{
			schema: `
				type Task @auth(
					query: { rule: "{$USERROLE: { eq: \"USER\"}}" }
				) {
					id: ID!
					name: String!
					occurrences: [TaskOccurrence] @hasInverse(field: task)
				}
				
				type TaskOccurrence @auth(
					query: { rule: "query { queryTaskOccurrence { task { id } } }" }
				) {
					id: ID!
					due: DateTime
					comp: DateTime
					task: Task @hasInverse(field: occurrences)
				}
			`,
			valid: true,
		},
		{
			schema: `
				type X {
					id: ID @dgraph(pred: "X.id")
					name: String
				}
				type Y {
					f1: String! @dgraph(pred:"~movie")
				}
			`,
			errors: x.GqlErrorList{{Message: "input:3: Type X; Field id: has the @dgraph directive but fields of type ID can't have the @dgraph directive."}, {Message: "input:7: Type Y; Field f1 is of type String, but reverse predicate in @dgraph directive only applies to fields with object types."}},
			valid:  false,
		},
	}

	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	testutil.DropAll(t, dg)

	validateUrl := groupOneAdminServer + "/schema/validate"
	var response x.QueryResWithData
	for _, tcase := range testCases {
		resp, err := http.Post(validateUrl, "text/plain", bytes.NewBuffer([]byte(tcase.schema)))
		require.NoError(t, err)

		decoder := json.NewDecoder(resp.Body)
		err = decoder.Decode(&response)
		require.NoError(t, err)

		// Verify that we only validate the schema and not set it.
		verifyEmptySchema(t)

		if tcase.valid {
			require.Equal(t, resp.StatusCode, http.StatusOK)
			continue
		}
		require.Equal(t, resp.StatusCode, http.StatusBadRequest)
		require.NotNil(t, response.Errors)
		require.Equal(t, len(response.Errors), len(tcase.errors))
		for idx, err := range response.Errors {
			require.Equal(t, err.Message, tcase.errors[idx].Message)
		}
	}
}

func TestUpdateGQLSchemaFields(t *testing.T) {
	schema := `
	type Author {
		id: ID!
		name: String!
	}`

	generatedSchema, err := ioutil.ReadFile("generatedSchema.graphql")
	require.NoError(t, err)

	req := &common.GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					schema
					generatedSchema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": schema},
	}
	resp := req.ExecuteAsPost(t, groupOneAdminServer)
	require.NotNil(t, resp)
	require.Nilf(t, resp.Errors, "%s", resp.Errors)

	var updateResp struct {
		UpdateGQLSchema struct {
			GQLSchema struct {
				Schema          string
				GeneratedSchema string
			}
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &updateResp))

	require.Equal(t, schema, updateResp.UpdateGQLSchema.GQLSchema.Schema)
	require.Equal(t, string(generatedSchema), updateResp.UpdateGQLSchema.GQLSchema.GeneratedSchema)
}

func TestIntrospection(t *testing.T) {
	// note that both the types implement the same interface and have a field called `name`, which
	// has exact same name as a field in full introspection query.
	schema := `
	interface Node {
		id: ID!
	}
	
	type Human implements Node {
		name: String
	}
	
	type Dog implements Node {
		name: String
	}`
	updateGQLSchemaRequireNoErrors(t, schema, groupOneAdminServer)
	query, err := ioutil.ReadFile("../../schema/testdata/introspection/input/full_query.graphql")
	require.NoError(t, err)

	introspectionParams := &common.GraphQLParams{Query: string(query)}
	resp := introspectionParams.ExecuteAsPost(t, groupOneServer)

	// checking that there are no errors in the response, i.e., we always get some data in the
	// introspection response.
	require.Nilf(t, resp.Errors, "%s", resp.Errors)
	require.NotEmpty(t, resp.Data)
	// TODO: we should actually compare data here, but there seems to be some issue with either the
	// introspection response or the JSON comparison. Needs deeper looking.
}

func TestDeleteSchemaAndExport(t *testing.T) {
	// first apply a schema
	schema := `
	type Person {
		name: String
	}`
	schemaResp := updateGQLSchemaReturnSchema(t, schema, groupOneAdminServer)

	// now delete it with S * * delete mutation
	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	txn := dg.NewTxn()
	_, err = txn.Mutate(context.Background(), &api.Mutation{
		DelNquads: []byte(fmt.Sprintf("<%s> * * .", schemaResp.Id)),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(context.Background()))

	// running an export shouldn't give any errors
	exportReq := &common.GraphQLParams{
		Query: `mutation {
		  export(input: {format: "rdf"}) {
			exportedFiles
		  }
		}`,
	}
	exportGqlResp := exportReq.ExecuteAsPost(t, groupOneAdminServer)
	common.RequireNoGQLErrors(t, exportGqlResp)

	// applying a new schema should still work
	newSchemaResp := updateGQLSchemaReturnSchema(t, schema, groupOneAdminServer)
	// we can assert that the uid allocated to new schema isn't same as the uid for old schema
	require.NotEqual(t, schemaResp.Id, newSchemaResp.Id)
}

func updateGQLSchema(t *testing.T, schema, url string) *common.GraphQLResponse {
	req := &common.GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					id
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

func updateGQLSchemaReturnSchema(t *testing.T, schema, url string) gqlSchema {
	resp := updateGQLSchema(t, schema, url)
	require.Nil(t, resp.Errors)

	var updateResp struct {
		UpdateGQLSchema struct {
			GqlSchema gqlSchema
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &updateResp))
	return updateResp.UpdateGQLSchema.GqlSchema
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
