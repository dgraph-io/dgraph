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
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dgraph-io/dgraph/x"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
)

const (
	poorManAdminURL        = "http://localhost:8180/admin"
	poorManWithAclAdminURL = "http://localhost:8280/admin"

	authTokenHeader = "X-Dgraph-AuthToken"
	authToken       = "itIsSecret"
	wrongAuthToken  = "wrongToken"

	accessJwtHeader = "X-Dgraph-AccessToken"
)

func TestLoginWithPoorManAuth(t *testing.T) {
	// without X-Dgraph-AuthToken should give error
	params := getGrootLoginParams()
	assertAuthTokenError(t, poorManWithAclAdminURL, params)

	// setting a wrong value for the token should still give error
	params.Headers.Set(authTokenHeader, wrongAuthToken)
	assertAuthTokenError(t, poorManWithAclAdminURL, params)

	// setting correct value for the token should not give any GraphQL error
	params.Headers.Set(authTokenHeader, authToken)
	common.RequireNoGQLErrors(t, params.ExecuteAsPost(t, poorManWithAclAdminURL))
}

func TestAdminOnlyPoorManAuth(t *testing.T) {
	// without X-Dgraph-AuthToken should give error
	params := getUpdateGqlSchemaParams()
	assertAuthTokenError(t, poorManAdminURL, params)

	// setting a wrong value for the token should still give error
	params.Headers.Set(authTokenHeader, wrongAuthToken)
	assertAuthTokenError(t, poorManAdminURL, params)

	// setting correct value for the token should not give any GraphQL error
	params.Headers.Set(authTokenHeader, authToken)
	common.RequireNoGQLErrors(t, params.ExecuteAsPost(t, poorManAdminURL))
}

func TestAdminPoorManWithAcl(t *testing.T) {
	// without auth token and access JWT headers, should give auth token related error
	params := getUpdateGqlSchemaParams()
	assertAuthTokenError(t, poorManWithAclAdminURL, params)

	// setting a wrong value for the auth token should still give auth token related error
	params.Headers.Set(authTokenHeader, wrongAuthToken)
	assertAuthTokenError(t, poorManWithAclAdminURL, params)

	// setting correct value for the auth token should now give ACL related GraphQL error
	params.Headers.Set(authTokenHeader, authToken)
	assertMissingAclError(t, params)

	// setting wrong value for the access JWT should still give ACL related GraphQL error
	params.Headers.Set(accessJwtHeader, wrongAuthToken)
	assertBadAclError(t, params)

	// setting correct value for both tokens should not give errors
	accessJwt, _ := grootLogin(t)
	params.Headers.Set(accessJwtHeader, accessJwt)
	common.RequireNoGQLErrors(t, params.ExecuteAsPost(t, poorManWithAclAdminURL))
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

func assertMissingAclError(t *testing.T, params *common.GraphQLParams) {
	resp := params.ExecuteAsPost(t, poorManWithAclAdminURL)
	require.Equal(t, x.GqlErrorList{{
		Message: "resolving updateGQLSchema failed because rpc error: code = PermissionDenied desc = no accessJwt available",
		Locations: []x.Location{{
			Line:   2,
			Column: 4,
		}},
	}}, resp.Errors)
}

func assertBadAclError(t *testing.T, params *common.GraphQLParams) {
	resp := params.ExecuteAsPost(t, poorManWithAclAdminURL)
	require.Equal(t, x.GqlErrorList{{
		Message: "resolving updateGQLSchema failed because rpc error: code = Unauthenticated desc = unable to parse jwt token:token contains an invalid number of segments",
		Locations: []x.Location{{
			Line:   2,
			Column: 4,
		}},
	}}, resp.Errors)
}

func grootLogin(t *testing.T) (string, string) {
	loginParams := getGrootLoginParams()
	loginParams.Headers.Set(authTokenHeader, authToken)
	resp := loginParams.ExecuteAsPost(t, poorManWithAclAdminURL)
	common.RequireNoGQLErrors(t, resp)

	var loginResp struct {
		Login struct {
			Response struct {
				AccessJWT  string
				RefreshJWT string
			}
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &loginResp))

	return loginResp.Login.Response.AccessJWT, loginResp.Login.Response.RefreshJWT
}

func getGrootLoginParams() *common.GraphQLParams {
	return &common.GraphQLParams{
		Query: `mutation login($userId: String, $password: String, $refreshToken: String) {
			login(userId: $userId, password: $password, refreshToken: $refreshToken) {
				response {
					accessJWT
					refreshJWT
				}
			}
		}`,
		Variables: map[string]interface{}{
			"userId":       x.GrootId,
			"password":     "password",
			"refreshToken": "",
		},
		Headers: http.Header{},
	}
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
