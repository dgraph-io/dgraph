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
	"testing"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

// This test is supposed to test the graphql schema subscribe feature. Whenever schema is updated
// in a dgraph alpha for one group, that update should also be propogated to alpha nodes in other
// groups.
func TestSchemaSubscribe(t *testing.T) {
	groupOneServer := "http://localhost:8180/graphql"
	groupOneAdminServer := "http://localhost:8180/admin"
	groupTwoServer := "http://localhost:8182/graphql"
	// groupTwoAdminServer := "http://localhost:8182/admin"
	groupThreeServer := "http://localhost:8183/graphql"
	groupThreeAdminServer := "http://localhost:8183/admin"

	schema := `
	type Author {
		id: ID!
		name: String!
	}`
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
	addResult := add.ExecuteAsPost(t, groupOneAdminServer)
	require.Nil(t, addResult.Errors)

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
	add.Variables["sch"] = schema
	addResult = add.ExecuteAsPost(t, groupThreeAdminServer)
	require.Nil(t, addResult.Errors)

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
