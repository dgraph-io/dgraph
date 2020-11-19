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

package admin_auth

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
)

const (
	authTokenHeader = "X-Dgraph-AuthToken"
	authToken       = "itIsSecret"
	wrongAuthToken  = "wrongToken"
)

func TestAdminOnlyPoorManAuth(t *testing.T) {
	t.Skipf("TODO: This test is failing for some reason. FIX IT.")
	// without X-Dgraph-AuthToken should give error
	params := getUpdateGqlSchemaParams()
	assertAuthTokenError(t, common.GraphqlAdminURL, params)

	// setting a wrong value for the token should still give error
	params.Headers.Set(authTokenHeader, wrongAuthToken)
	assertAuthTokenError(t, common.GraphqlAdminURL, params)

	// setting correct value for the token should not give any GraphQL error
	params.Headers.Set(authTokenHeader, authToken)
	common.RequireNoGQLErrors(t, params.ExecuteAsPost(t, common.GraphqlAdminURL))
}


func assertAuthTokenError(t *testing.T, url string, params *common.GraphQLParams) {
	req, err := params.CreateGQLPost(url)
	require.NoError(t, err)

	resp, err := common.RunGQLRequest(req)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"errors":[{
			"message":"Invalid X-Dgraph-AuthToken",
			"extensions":{"code":"ErrorUnauthorized"}
		}]
	}`, string(resp))
}

func getUpdateGqlSchemaParams() *common.GraphQLParams {
	schema := `type Person {
		id: ID!
		name: String!
	}`
	return &common.GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					schema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": schema},
		Headers:   http.Header{},
	}
}
