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

package admin_auth

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/x"
)

const (
	authTokenHeader = "X-Dgraph-AuthToken"
	authToken       = "itIsSecret"
	wrongAuthToken  = "wrongToken"

	accessJwtHeader = "X-Dgraph-AccessToken"
)

func TestLoginWithPoorManAuth(t *testing.T) {
	// without X-Dgraph-AuthToken should give error
	params := getGrootLoginParams()
	assertAuthTokenError(t, params.ExecuteAsPost(t, common.GraphqlAdminURL))

	// setting a wrong value for the token should still give error
	params.Headers.Set(authTokenHeader, wrongAuthToken)
	assertAuthTokenError(t, params.ExecuteAsPost(t, common.GraphqlAdminURL))

	// setting correct value for the token should not give any GraphQL error
	params.Headers.Set(authTokenHeader, authToken)
	var resp *common.GraphQLResponse
	for i := 0; i < 10; i++ {
		resp = params.ExecuteAsPost(t, common.GraphqlAdminURL)
		if len(resp.Errors) == 0 {
			break
		}
		time.Sleep(time.Second)
	}
	common.RequireNoGQLErrors(t, resp)
}

func TestAdminPoorManWithAcl(t *testing.T) {
	schema := `type Person {
		id: ID!
		name: String!
	}`
	// without auth token and access JWT headers, should give auth token related error
	headers := http.Header{}
	assertAuthTokenError(t, common.RetryUpdateGQLSchema(t, common.Alpha1HTTP, schema, headers))

	// setting a wrong value for the auth token should still give auth token related error
	headers.Set(authTokenHeader, wrongAuthToken)
	assertAuthTokenError(t, common.RetryUpdateGQLSchema(t, common.Alpha1HTTP, schema, headers))

	// setting correct value for the auth token should now give ACL related GraphQL error
	headers.Set(authTokenHeader, authToken)
	assertMissingAclError(t, common.RetryUpdateGQLSchema(t, common.Alpha1HTTP, schema, headers))

	// setting wrong value for the access JWT should still give ACL related GraphQL error
	headers.Set(accessJwtHeader, wrongAuthToken)
	assertBadAclError(t, common.RetryUpdateGQLSchema(t, common.Alpha1HTTP, schema, headers))

	// setting correct value for both tokens should not give errors
	accessJwt, _ := grootLogin(t)
	headers.Set(accessJwtHeader, accessJwt)
	common.AssertUpdateGQLSchemaSuccess(t, common.Alpha1HTTP, schema, headers)
}

func assertAuthTokenError(t *testing.T, resp *common.GraphQLResponse) {
	require.Equal(t, x.GqlErrorList{{
		Message:    "Invalid X-Dgraph-AuthToken",
		Extensions: map[string]interface{}{"code": "ErrorUnauthorized"},
	}}, resp.Errors)
	require.Nil(t, resp.Data)
}

func assertMissingAclError(t *testing.T, resp *common.GraphQLResponse) {
	require.Equal(t, x.GqlErrorList{{
		Message: "resolving updateGQLSchema failed because rpc error: code = PermissionDenied desc = no accessJwt available",
		Locations: []x.Location{{
			Line:   2,
			Column: 4,
		}},
	}}, resp.Errors)
}

func assertBadAclError(t *testing.T, resp *common.GraphQLResponse) {
	require.Equal(t, x.GqlErrorList{{
		Message: "resolving updateGQLSchema failed because rpc error: code = Unauthenticated desc = unable to parse jwt token: token contains an invalid number of segments",
		Locations: []x.Location{{
			Line:   2,
			Column: 4,
		}},
	}}, resp.Errors)
}

func grootLogin(t *testing.T) (string, string) {
	loginParams := getGrootLoginParams()
	loginParams.Headers.Set(authTokenHeader, authToken)
	resp := loginParams.ExecuteAsPost(t, common.GraphqlAdminURL)
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
