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

package multi_tenancy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

const (
	accessJwtHeader = "X-Dgraph-AccessToken"
)

var (
	groupOneHTTP   = testutil.ContainerAddr("alpha1", 8080)
	groupTwoHTTP   = testutil.ContainerAddr("alpha2", 8080)
	groupThreeHTTP = testutil.ContainerAddr("alpha3", 8080)

	groupOneGraphQLServer   = "http://" + groupOneHTTP + "/graphql"
	groupTwoGraphQLServer   = "http://" + groupTwoHTTP + "/graphql"
	groupThreeGraphQLServer = "http://" + groupThreeHTTP + "/graphql"

	groupOneAdminServer = "http://" + groupOneHTTP + "/admin"
)

// This test is supposed to test the graphql schema subscribe feature for multiple namespaces.
// Whenever schema is updated in a dgraph alpha for one group for any namespace,
// that update should also be propagated to alpha nodes in other groups.
func TestSchemaSubscribe(t *testing.T) {
	// TODO: need to fix the race condition for license propagation, the sleep helps propagate the EE license correctly
	time.Sleep(time.Second * 10)
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
func TestGraphQLResponse(t *testing.T) {
	common.SafelyDropAllWithGroot(t)

	header := http.Header{}
	header.Set(accessJwtHeader, testutil.GrootHttpLogin(groupOneAdminServer).AccessJwt)

	ns := common.CreateNamespace(t, header)
	header1 := http.Header{}
	header1.Set(accessJwtHeader, testutil.GrootHttpLoginNamespace(groupOneAdminServer,
		ns).AccessJwt)

	// initially, when no schema is set, we should get error: `there is no GraphQL schema in Dgraph`
	// for both the namespaces
	query := `
	query {
		queryAuthor {
			name
		}
	}`
	resp0 := (&common.GraphQLParams{Query: query, Headers: header}).ExecuteAsPost(t,
		groupOneGraphQLServer)
	resp1 := (&common.GraphQLParams{Query: query, Headers: header1}).ExecuteAsPost(t,
		groupOneGraphQLServer)
	expectedErrs := x.GqlErrorList{{Message: "Not resolving queryAuthor. " +
		"There's no GraphQL schema in Dgraph. Use the /admin API to add a GraphQL schema"}}
	require.Equal(t, expectedErrs, resp0.Errors)
	require.Equal(t, expectedErrs, resp1.Errors)
	require.Nil(t, resp0.Data)
	require.Nil(t, resp1.Data)

	schema := `
	type Author {
		id: ID!
		name: String!
	}`
	common.SafelyUpdateGQLSchema(t, common.Alpha1HTTP, schema, header)
	common.SafelyUpdateGQLSchema(t, common.Alpha1HTTP, schema, header1)

	require.Equal(t, schema, common.AssertGetGQLSchema(t, common.Alpha1HTTP, header).Schema)
	require.Equal(t, schema, common.AssertGetGQLSchema(t, common.Alpha1HTTP, header1).Schema)

	queryHelper(t, groupOneGraphQLServer, `
	mutation {
		addAuthor(input:{name: "Alice"}) {
			author{
				name
			}
		}
	}`, header,
		`{
			"addAuthor": {
				"author":[{
					"name":"Alice"
				}]
			}
		}`)

	queryHelper(t, groupOneGraphQLServer, query, header,
		`{
			"queryAuthor": [
				{
					"name":"Alice"
				}
			]
		}`)

	queryHelper(t, groupOneGraphQLServer, query, header1,
		`{
			"queryAuthor": []
		}`)

	common.DeleteNamespace(t, ns, header)
}

func TestAuth(t *testing.T) {
	common.SafelyDropAllWithGroot(t)

	header := http.Header{}
	header.Set(accessJwtHeader, testutil.GrootHttpLogin(groupOneAdminServer).AccessJwt)
	schema := `
	type User @auth(
		query: { rule: """
			query($USER: String!) {
				queryUser(filter: { username: { eq: $USER } }) {
				__typename
				}
			}
		"""}
	) {
		id: ID!
		username: String! @id
		isPublic: Boolean @search
	}
	# Dgraph.Authorization  {"VerificationKey":"secret","Header":"Authorization","Namespace":"https://dgraph.io/jwt/claims","Algo":"HS256"}`
	common.SafelyUpdateGQLSchema(t, common.Alpha1HTTP, schema, header)

	ns := common.CreateNamespace(t, header)
	header1 := http.Header{}
	header1.Set(accessJwtHeader, testutil.GrootHttpLoginNamespace(groupOneAdminServer,
		ns).AccessJwt)
	schema1 := `
	type User @auth(
		query: { rule: """
			query {
				queryUser(filter: { isPublic: true }) {
					__typename
				}
			}
		"""}
	) {
		id: ID!
		username: String! @id
		isPublic: Boolean @search
	}
	# Dgraph.Authorization  {"VerificationKey":"secret1","Header":"Authorization1","Namespace":"https://dgraph.io/jwt/claims1","Algo":"HS256"}`
	common.SafelyUpdateGQLSchema(t, common.Alpha1HTTP, schema1, header1)

	require.Equal(t, schema, common.AssertGetGQLSchema(t, common.Alpha1HTTP, header).Schema)
	require.Equal(t, schema1, common.AssertGetGQLSchema(t, common.Alpha1HTTP, header1).Schema)

	addUserMutation := `mutation {
		addUser(input:[
			{username: "Alice", isPublic: false},
			{username: "Bob", isPublic: true}
		]) {
			user {
				username
			}
		}
	}`

	// for namespace 0, after adding multiple users, we should only get back the user "Alice"
	header = common.GetJWT(t, "Alice", nil, &testutil.AuthMeta{
		PublicKey: "secret",
		Namespace: "https://dgraph.io/jwt/claims",
		Algo:      "HS256",
		Header:    "Authorization",
	})
	header.Set(accessJwtHeader, testutil.GrootHttpLogin(groupOneAdminServer).AccessJwt)
	queryHelper(t, groupOneGraphQLServer, addUserMutation, header, `{
		"addUser": {
			"user":[{
				"username":"Alice"
			}]
		}
	}`)

	// for namespace 1, after adding multiple users, we should only get back the public users
	header1 = common.GetJWT(t, "Alice", nil, &testutil.AuthMeta{
		PublicKey: "secret1",
		Namespace: "https://dgraph.io/jwt/claims1",
		Algo:      "HS256",
		Header:    "Authorization1",
	})
	header1.Set(accessJwtHeader, testutil.GrootHttpLoginNamespace(groupOneAdminServer,
		ns).AccessJwt)
	queryHelper(t, groupOneGraphQLServer, addUserMutation, header1, `{
		"addUser": {
			"user":[{
				"username":"Bob"
			}]
		}
	}`)

	common.DeleteNamespace(t, ns, header)
}

// TestCORS checks that all the CORS headers are correctly set in the response for each namespace.
func TestCORS(t *testing.T) {
	header := http.Header{}
	header.Set(accessJwtHeader, testutil.GrootHttpLogin(groupOneAdminServer).AccessJwt)
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, `
	type TestCORS {
		id: ID!
		name: String
		cf: String @custom(http:{
			url: "https://play.dgraph.io",
			method: GET,
			forwardHeaders: ["Test-CORS"]
		})
	}
	# Dgraph.Allow-Origin "https://play.dgraph.io"
	# Dgraph.Authorization  {"VerificationKey":"secret","Header":"X-Test-Dgraph","Namespace":"https://dgraph.io/jwt/claims","Algo":"HS256"}
	`, header)

	ns := common.CreateNamespace(t, header)
	header1 := http.Header{}
	header1.Set(accessJwtHeader, testutil.GrootHttpLoginNamespace(groupOneAdminServer,
		ns).AccessJwt)
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, `
	type TestCORS {
		id: ID!
		name: String
		cf: String @custom(http:{
			url: "https://play.dgraph.io",
			method: GET,
			forwardHeaders: ["Test-CORS1"]
		})
	}
	# Dgraph.Allow-Origin "https://play1.dgraph.io"
	# Dgraph.Authorization  {"VerificationKey":"secret","Header":"X-Test-Dgraph1","Namespace":"https://dgraph.io/jwt/claims","Algo":"HS256"}
	`, header1)

	// testCORS for namespace 0
	testCORS(t, 0, "https://play.dgraph.io", "https://play.dgraph.io",
		strings.Join([]string{x.AccessControlAllowedHeaders, "Test-CORS", "X-Test-Dgraph"}, ","))

	// testCORS for the new namespace
	testCORS(t, ns, "https://play1.dgraph.io", "https://play1.dgraph.io",
		strings.Join([]string{x.AccessControlAllowedHeaders, "Test-CORS1", "X-Test-Dgraph1"}, ","))

	// cleanup
	common.DeleteNamespace(t, ns, header)
}

func queryHelper(t *testing.T, server, query string, headers http.Header,
	expectedResult string) {
	params := &common.GraphQLParams{
		Query:   query,
		Headers: headers,
	}
	queryResult := params.ExecuteAsPost(t, server)
	common.RequireNoGQLErrors(t, queryResult)
	testutil.CompareJSON(t, expectedResult, string(queryResult.Data))
}

func testCORS(t *testing.T, namespace uint64, reqOrigin, expectedAllowedOrigin,
	expectedAllowedHeaders string) {
	params := &common.GraphQLParams{
		Query: `query {	queryTestCORS { name } }`,
	}
	req, err := params.CreateGQLPost(groupOneGraphQLServer)
	require.NoError(t, err)

	if reqOrigin != "" {
		req.Header.Set("Origin", reqOrigin)
	}
	req.Header.Set(accessJwtHeader, testutil.GrootHttpLoginNamespace(groupOneAdminServer, namespace).AccessJwt)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)

	// GraphQL server should always return OK and JSON content, even when there are errors
	require.Equal(t, resp.StatusCode, http.StatusOK)
	require.Equal(t, strings.ToLower(resp.Header.Get("Content-Type")), "application/json")
	// assert that the CORS headers are there as expected
	require.Equal(t, resp.Header.Get("Access-Control-Allow-Origin"), expectedAllowedOrigin)
	require.Equal(t, resp.Header.Get("Access-Control-Allow-Methods"), "POST, OPTIONS")
	require.Equal(t, resp.Header.Get("Access-Control-Allow-Headers"), expectedAllowedHeaders)
	require.Equal(t, resp.Header.Get("Access-Control-Allow-Credentials"), "true")

	gqlRes := &common.GraphQLResponse{}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, gqlRes))
	common.RequireNoGQLErrors(t, gqlRes)
	testutil.CompareJSON(t, `{"queryTestCORS":[]}`, string(gqlRes.Data))
}

// TestNamespacesQueryField checks that namespaces field in state query of /admin endpoint is
// properly working.
func TestNamespacesQueryField(t *testing.T) {
	header := http.Header{}
	header.Set(accessJwtHeader, testutil.GrootHttpLogin(groupOneAdminServer).AccessJwt)

	namespaceQuery :=
		`query {
			state {
				namespaces
			}
		}`

	// Test namespaces query shows 0 as the only namespace.
	queryHelper(t, groupOneAdminServer, namespaceQuery, header,
		`{
			"state": {
				"namespaces":[0]
			}
		}`)

	ns1 := common.CreateNamespace(t, header)
	ns2 := common.CreateNamespace(t, header)
	header1 := http.Header{}
	header1.Set(accessJwtHeader, testutil.GrootHttpLoginNamespace(groupOneAdminServer,
		ns1).AccessJwt)

	// Test namespaces query shows no namespace in case user is not guardian of galaxy.
	queryHelper(t, groupOneAdminServer, namespaceQuery, header1,
		`{
			"state": {
				"namespaces":[]
			}
		}`)

	// Test namespaces query shows all 3 namespaces, 0,ns1,ns2 in case user is guardian of galaxy.
	queryHelper(t, groupOneAdminServer, namespaceQuery, header,
		`{
			"state": {
				"namespaces":[0,`+
			strconv.FormatUint(ns1, 10)+`,`+
			strconv.FormatUint(ns2, 10)+`]
			}
		}`)

	// cleanup
	common.DeleteNamespace(t, ns1, header)
	common.DeleteNamespace(t, ns2, header)
}
