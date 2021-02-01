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
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

var (
	groupOneHTTP   = testutil.ContainerAddr("alpha1", 8080)
	groupTwoHTTP   = testutil.ContainerAddr("alpha2", 8080)
	groupThreeHTTP = testutil.ContainerAddr("alpha3", 8080)
	groupOnegRPC   = testutil.SockAddr

	groupOneGraphQLServer   = "http://" + groupOneHTTP + "/graphql"
	groupTwoGraphQLServer   = "http://" + groupTwoHTTP + "/graphql"
	groupThreeGraphQLServer = "http://" + groupThreeHTTP + "/graphql"

	groupOneAdminServer = "http://" + groupOneHTTP + "/admin"
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
	groupOnePreUpdateCounter := common.RetryProbeGraphQL(t, groupOneHTTP).SchemaUpdateCounter
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, schema, nil)

	// since the schema has been updated on group one, the schemaUpdateCounter on all the servers
	// should have got incremented and must be the same, indicating that the schema update has
	// reached all the servers.
	common.AssertSchemaUpdateCounterIncrement(t, groupOneHTTP, groupOnePreUpdateCounter)
	common.AssertSchemaUpdateCounterIncrement(t, groupTwoHTTP, groupOnePreUpdateCounter)
	common.AssertSchemaUpdateCounterIncrement(t, groupThreeHTTP, groupOnePreUpdateCounter)

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

	// Also, the introspection query on all the servers should
	// give the same result as they have the same schema.
	introspectionResult := introspect.ExecuteAsPost(t, groupOneGraphQLServer)
	common.RequireNoGQLErrors(t, introspectionResult)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))

	introspectionResult = introspect.ExecuteAsPost(t, groupTwoGraphQLServer)
	common.RequireNoGQLErrors(t, introspectionResult)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))

	introspectionResult = introspect.ExecuteAsPost(t, groupThreeGraphQLServer)
	common.RequireNoGQLErrors(t, introspectionResult)
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
	groupThreePreUpdateCounter := groupOnePreUpdateCounter + 1
	common.SafelyUpdateGQLSchema(t, groupThreeHTTP, schema, nil)

	common.AssertSchemaUpdateCounterIncrement(t, groupOneHTTP, groupThreePreUpdateCounter)
	common.AssertSchemaUpdateCounterIncrement(t, groupTwoHTTP, groupThreePreUpdateCounter)
	common.AssertSchemaUpdateCounterIncrement(t, groupThreeHTTP, groupThreePreUpdateCounter)

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
	introspectionResult = introspect.ExecuteAsPost(t, groupOneGraphQLServer)
	common.RequireNoGQLErrors(t, introspectionResult)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))

	introspectionResult = introspect.ExecuteAsPost(t, groupTwoGraphQLServer)
	common.RequireNoGQLErrors(t, introspectionResult)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))

	introspectionResult = introspect.ExecuteAsPost(t, groupThreeGraphQLServer)
	common.RequireNoGQLErrors(t, introspectionResult)
	testutil.CompareJSON(t, expectedResult, string(introspectionResult.Data))
}

// TestConcurrentSchemaUpdates checks that if there are too many concurrent requests to update the
// GraphQL schema, then the system works as expected by either:
// 	1. failing the schema update because there is another one in progress, OR
// 	2. if the schema update succeeds, then the last successful schema update is reflected by both
//	Dgraph and GraphQL schema
//
// It also tests that only one node exists for GraphQL schema in Dgraph after all the
// concurrent requests have executed.
func TestConcurrentSchemaUpdates(t *testing.T) {
	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	testutil.DropAll(t, dg)

	tcases := []struct {
		graphQLSchema string
		dgraphSchema  string
		authority     string
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
			authority: groupOneHTTP,
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
			authority: groupTwoHTTP,
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
			authority: groupThreeHTTP,
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
			if updateGQLSchemaConcurrent(t, tcases[tcaseIdx].graphQLSchema, tcases[tcaseIdx].authority) {
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

	// find final GraphQL & Dgraph schemas
	finalGraphQLSchema := tcases[lastSuccessTcaseIdx].graphQLSchema
	finalDgraphPreds := tcases[lastSuccessTcaseIdx].dgraphSchema
	finalDgraphTypes := `
	{
		"fields": [
			{
				"name": "A.b"
			}
		],
		"name": "A"
	}`

	// now check that both the final GraphQL schema and Dgraph schema are the ones we expect
	require.Equal(t, finalGraphQLSchema, common.AssertGetGQLSchemaRequireId(t, groupOneHTTP).Schema)
	testutil.VerifySchema(t, dg, testutil.SchemaOptions{
		UserPreds:        finalDgraphPreds,
		UserTypes:        finalDgraphTypes,
		ExcludeAclSchema: true,
	})

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
	oldCounter := common.RetryProbeGraphQL(t, groupOneHTTP).SchemaUpdateCounter
	// Then, Do the drop_all operation
	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	testutil.DropAll(t, dg)
	// wait for the schema update to reach the GraphQL layer
	common.AssertSchemaUpdateCounterIncrement(t, groupOneHTTP, oldCounter)

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
	introspectionResult := introspect.ExecuteAsPost(t, groupOneGraphQLServer)
	require.Len(t, introspectionResult.Errors, 1)
	gotErrorMessage := introspectionResult.Errors[0].Message
	expectedErrorMessage := "Not resolving __schema. There's no GraphQL schema in Dgraph.  Use the /admin API to add a GraphQL schema"
	require.Equal(t, expectedErrorMessage, gotErrorMessage)
}

// TestUpdateGQLSchemaAfterDropAll makes sure that updating the GraphQL schema after drop_all works
func TestUpdateGQLSchemaAfterDropAll(t *testing.T) {
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, `
	type A {
		b: String!
	}`, nil)
	oldCounter := common.RetryProbeGraphQL(t, groupOneHTTP).SchemaUpdateCounter

	// now do drop_all
	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	testutil.DropAll(t, dg)

	// need to wait a bit, because the update notification takes time to reach the alpha
	common.AssertSchemaUpdateCounterIncrement(t, groupOneHTTP, oldCounter)
	// now retrieving the GraphQL schema should report no schema
	require.Empty(t, common.AssertGetGQLSchemaRequireId(t, groupOneHTTP).Schema)

	// updating the schema now should work
	schema := `
			type A {
				b: String! @id
			}`
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, schema, nil)
	// we should get the schema we expect
	require.Equal(t, schema, common.AssertGetGQLSchemaRequireId(t, groupOneHTTP).Schema)
}

// TestGQLSchemaAfterDropData checks if the schema still exists after drop_data
func TestGQLSchemaAfterDropData(t *testing.T) {
	schema := `
			type A {
				b: String!
			}`
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, schema, nil)
	oldCounter := common.RetryProbeGraphQL(t, groupOneHTTP).SchemaUpdateCounter

	// now do drop_data
	dg, err := testutil.DgraphClient(groupOnegRPC)
	require.NoError(t, err)
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropOp: api.Operation_DATA}))

	// lets wait a bit to be sure that the update notification has reached the alpha,
	// otherwise we are anyways gonna get the previous schema from the in-memory schema
	time.Sleep(5 * time.Second)
	// drop_data should not increment the schema update counter
	newCounter := common.RetryProbeGraphQL(t, groupOneHTTP).SchemaUpdateCounter
	require.Equal(t, oldCounter, newCounter)
	// we should still get the schema we inserted earlier
	require.Equal(t, schema, common.AssertGetGQLSchemaRequireId(t, groupOneHTTP).Schema)

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
	common.RequireNoGQLErrors(t, getResult)

	require.JSONEq(t, `{
		"querySchemaHistory": []
	  }`, string(getResult.Data))

	// Let's add an schema and expect the history in the history api.
	schema := `
	type A {
		b: String!
	}`
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, schema, nil)

	getResult = get.ExecuteAsPost(t, groupOneAdminServer)
	common.RequireNoGQLErrors(t, getResult)
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
	require.Equal(t, history.QuerySchemaHistory[0].Schema, schema)

	// Let's update with the same schema. But we should not get the 2 history because, we
	// are updating with the same schema.
	common.AssertUpdateGQLSchemaSuccess(t, groupOneHTTP, schema, nil)

	getResult = get.ExecuteAsPost(t, groupOneAdminServer)
	common.RequireNoGQLErrors(t, getResult)
	history = schemaHistory{}
	require.NoError(t, json.Unmarshal(getResult.Data, &history))
	require.Equal(t, int(1), len(history.QuerySchemaHistory))
	require.Equal(t, history.QuerySchemaHistory[0].Schema, schema)

	// this wait is necessary to make sure that the new schema is created atleast 1s after the old
	// schema, ensuring that the new schema is reported first in the query.
	time.Sleep(time.Second)
	// Let's update a new schema and check the history.
	newSchema := `
	type B {
		b: String!
	}`
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, newSchema, nil)

	getResult = get.ExecuteAsPost(t, groupOneAdminServer)
	common.RequireNoGQLErrors(t, getResult)
	history = schemaHistory{}
	require.NoError(t, json.Unmarshal(getResult.Data, &history))
	require.Equal(t, int(2), len(history.QuerySchemaHistory))
	require.Equal(t, newSchema, history.QuerySchemaHistory[0].Schema)
	require.Equal(t, schema, history.QuerySchemaHistory[1].Schema)

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
	common.RequireNoGQLErrors(t, getResult)
	history = schemaHistory{}
	require.NoError(t, json.Unmarshal(getResult.Data, &history))
	require.Equal(t, int(1), len(history.QuerySchemaHistory))
	require.Equal(t, history.QuerySchemaHistory[0].Schema, schema)

	// Let's drop everything and see whether we get empty results or not.
	require.NoError(t, dg.Alter(context.Background(), &api.Operation{DropOp: api.Operation_DATA, RunInBackground: false}))
	getResult = get.ExecuteAsPost(t, groupOneAdminServer)
	common.RequireNoGQLErrors(t, getResult)
	require.JSONEq(t, `{
		"querySchemaHistory": []
	  }`, string(getResult.Data))
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
		require.Empty(t, common.AssertGetGQLSchema(t, groupOneHTTP).Schema)

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
	require.Equal(t, string(generatedSchema), common.SafelyUpdateGQLSchema(t, groupOneHTTP,
		schema, nil).GeneratedSchema)
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
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, schema, nil)
	query, err := ioutil.ReadFile("../../schema/testdata/introspection/input/full_query.graphql")
	require.NoError(t, err)

	introspectionParams := &common.GraphQLParams{Query: string(query)}
	resp := introspectionParams.ExecuteAsPost(t, groupOneGraphQLServer)

	// checking that there are no errors in the response, i.e., we always get some data in the
	// introspection response.
	common.RequireNoGQLErrors(t, resp)
	require.NotEmpty(t, resp.Data)
	// TODO: we should actually compare data here, but there seems to be some issue with either the
	// introspection response or the JSON comparison. Needs deeper looking.
}

func TestApolloServiceResolver(t *testing.T) {
	schema := `
	type Mission {
		id: ID!
		crew: [Astronaut]
		designation: String!
		startDate: String
		endDate: String
	}
	
	type Astronaut @key(fields: "id") @extends {
		id: ID! @external
		missions: [Mission]
	}
	
	type User @remote {
		id: ID!
		name: String!
	}
	
	type Car @auth(
		password: { rule: "{$ROLE: { eq: \"Admin\" } }"}
	){
		id: ID!
		name: String!
	}
	
	type Query {
		getMyFavoriteUsers(id: ID!): [User] @custom(http: {
			url: "http://my-api.com",
			method: "GET"
		})
	}
	`
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, schema, nil)
	serviceQueryParams := &common.GraphQLParams{Query: `
	query {
		_service {
			s: sdl
		}
	}`}
	resp := serviceQueryParams.ExecuteAsPost(t, groupOneGraphQLServer)
	common.RequireNoGQLErrors(t, resp)
	var gqlRes struct {
		Service struct {
			S string
		} `json:"_service"`
	}
	require.NoError(t, json.Unmarshal(resp.Data, &gqlRes))

	sdl, err := ioutil.ReadFile("apollo_service_response.graphql")
	require.NoError(t, err)

	require.Equal(t, string(sdl), gqlRes.Service.S)
}

func TestDeleteSchemaAndExport(t *testing.T) {
	// first apply a schema
	schema := `
	type Person {
		name: String
	}`
	schemaResp := common.SafelyUpdateGQLSchema(t, groupOneHTTP, schema, nil)

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
	newSchemaResp := common.AssertUpdateGQLSchemaSuccess(t, groupOneHTTP, schema, nil)
	// we can assert that the uid allocated to new schema isn't same as the uid for old schema
	require.NotEqual(t, schemaResp.Id, newSchemaResp.Id)
}

func updateGQLSchemaConcurrent(t *testing.T, schema, authority string) bool {
	res := common.RetryUpdateGQLSchema(t, authority, schema, nil)
	err := res.Errors.Error()
	require.NotContains(t, err, worker.ErrMultipleGraphQLSchemaNodes)
	require.NotContains(t, err, worker.ErrGraphQLSchemaAlterFailed)

	return res.Errors == nil
}

func TestMain(m *testing.M) {
	err := common.CheckGraphQLStarted(common.GraphqlAdminURL)
	if err != nil {
		x.Log(err, "Waited for GraphQL test server to become available, but it never did.")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
