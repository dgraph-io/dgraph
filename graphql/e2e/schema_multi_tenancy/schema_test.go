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
	"net/http"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
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

// This test is supposed to test the graphql schema subscribe feature for multiple namespaces.
// Whenever schema is updated in a dgraph alpha for one group for any namespace,
// that update should also be propagated to alpha nodes in other groups.
func TestSchemaSubscribe(t *testing.T) {
	t.Skip()
	dg, err := testutil.DgraphClientWithGroot(groupOnegRPC)
	require.NoError(t, err)
	testutil.DropAll(t, dg)

	header := http.Header{}
	header.Set(accessJwtHeader, testutil.GrootHttpLogin(groupOneAdminServer).AccessJwt)
	schema := `
	type Author {
		id: ID!
		name: String!
	}`
	grp1NS0PreUpdateCounter := common.RetryProbeGraphQL(t, groupOneHTTP, header).SchemaUpdateCounter
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, schema, header)
	// since the schema has been updated on group one, the schemaUpdateCounter on all the servers
	// should have got incremented and must be the same, indicating that the schema update has
	// reached all the servers.
	common.AssertSchemaUpdateCounterIncrement(t, groupOneHTTP, grp1NS0PreUpdateCounter, header)
	common.AssertSchemaUpdateCounterIncrement(t, groupTwoHTTP, grp1NS0PreUpdateCounter, header)
	common.AssertSchemaUpdateCounterIncrement(t, groupThreeHTTP, grp1NS0PreUpdateCounter, header)

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
		Query:   introspectionQuery,
		Headers: header,
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

	// Now update schema on an alpha node for group 3 for new namespace and see if nodes in group 1
	// and 2 also get it.
	ns := common.CreateNamespace(t, header)
	header.Set(accessJwtHeader, testutil.GrootHttpLoginNamespace(groupOneAdminServer, ns).AccessJwt)
	schema = `
	type Author {
		id: ID!
		name: String!
		posts: [Post]
	}

	interface Post {
		id: ID!
	}`
	grp3NS1PreUpdateCounter := uint64(0) // this has to be 0 as namespace was just created
	common.SafelyUpdateGQLSchema(t, groupThreeHTTP, schema, header)

	common.AssertSchemaUpdateCounterIncrement(t, groupOneHTTP, grp3NS1PreUpdateCounter, header)
	common.AssertSchemaUpdateCounterIncrement(t, groupTwoHTTP, grp3NS1PreUpdateCounter, header)
	common.AssertSchemaUpdateCounterIncrement(t, groupThreeHTTP, grp3NS1PreUpdateCounter, header)

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

	header.Set(accessJwtHeader, testutil.GrootHttpLogin(groupOneAdminServer).AccessJwt)
	common.DeleteNamespace(t, ns, header)
}

// This test ensures that even though different namespaces have the same GraphQL schema, if their
// data is different the same should be reflected in the GraphQL responses.
// In a way, it also tests lazy-loading of GraphQL schema.
func TestSchemaNamespaceWithData(t *testing.T) {
	dg, err := testutil.DgraphClientWithGroot(groupOnegRPC)
	require.NoError(t, err)
	testutil.DropAll(t, dg)

	header := http.Header{}
	header.Set(accessJwtHeader, testutil.GrootHttpLogin(groupOneAdminServer).AccessJwt)
	schema := `
	type Author {
		id: ID!
		name: String!
	}`
	common.SafelyUpdateGQLSchema(t, common.Alpha1HTTP, schema, header)

	ns := common.CreateNamespace(t, header)
	header1 := http.Header{}
	header1.Set(accessJwtHeader, testutil.GrootHttpLoginNamespace(groupOneAdminServer,
		ns).AccessJwt)
	common.SafelyUpdateGQLSchema(t, common.Alpha1HTTP, schema, header1)

	require.Equal(t, schema, common.AssertGetGQLSchema(t, common.Alpha1HTTP, header).Schema)
	require.Equal(t, schema, common.AssertGetGQLSchema(t, common.Alpha1HTTP, header1).Schema)

	query := `
	mutation {
		addAuthor(input:{name: "Alice"}) {
			author{
				name
			}
		}
	}`
	expectedResult :=
		`{
			"addAuthor": {
				"author":[{
					"name":"Alice"
				}]
			}
		}`
	queryAuthor := &common.GraphQLParams{
		Query:   query,
		Headers: header,
	}
	queryResult := queryAuthor.ExecuteAsPost(t, groupOneGraphQLServer)
	common.RequireNoGQLErrors(t, queryResult)
	testutil.CompareJSON(t, expectedResult, string(queryResult.Data))

	Query1 := `
	query {
		queryAuthor {
			name
		}
	}`
	expectedResult =
		`{
			"queryAuthor": [
				{
					"name":"Alice"
				}
			]
		}`
	queryAuthor.Query = Query1
	queryAuthor.Headers = header
	queryResult = queryAuthor.ExecuteAsPost(t, groupOneGraphQLServer)
	common.RequireNoGQLErrors(t, queryResult)
	testutil.CompareJSON(t, expectedResult, string(queryResult.Data))

	query2 := `
	query {
		queryAuthor {
			name
		}
	}`
	expectedResult =
		`{
			"queryAuthor": []
		}`
	queryAuthor.Query = query2
	queryAuthor.Headers = header1
	queryResult = queryAuthor.ExecuteAsPost(t, groupOneGraphQLServer)
	common.RequireNoGQLErrors(t, queryResult)
	testutil.CompareJSON(t, expectedResult, string(queryResult.Data))

	common.DeleteNamespace(t, ns, header)
}
