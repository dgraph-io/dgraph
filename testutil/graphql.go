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

package testutil

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgrijalva/jwt-go"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/x"

	"github.com/stretchr/testify/require"
)

const ExportRequest = `mutation {
	export(input: {format: "json"}) {
		response {
			code
			message
		}
	}
}`

type GraphQLParams struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type GraphQLError struct {
	Message string
}

type GraphQLResponse struct {
	Data       json.RawMessage        `json:"data,omitempty"`
	Errors     x.GqlErrorList         `json:"errors,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

func (resp *GraphQLResponse) RequireNoGraphQLErrors(t *testing.T) {
	if resp == nil {
		return
	}
	require.Nil(t, resp.Errors, "required no GraphQL errors, but received :\n%s",
		resp.Errors.Error())
}

func RequireNoGraphQLErrors(t *testing.T, resp *http.Response) {
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	var result *GraphQLResponse
	err = json.Unmarshal(b, &result)
	require.NoError(t, err)
	require.Nil(t, result.Errors)
}

type clientCustomClaims struct {
	AuthVariables map[string]interface{} `json:"https://xyz.io/jwt/claims"`
	jwt.StandardClaims
}

func GetSignedToken(t *testing.T,
	authVars map[string]interface{},
	metaInfo *authorization.AuthMeta) string {
	claims := clientCustomClaims{
		authVars,
		jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
			Issuer:    "test",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(metaInfo.PublicKey))
	require.NoError(t, err)
	return ss
}

func AddClaimsToContext(
	ctx context.Context,
	t *testing.T,
	authVars map[string]interface{},
	metaInfo *authorization.AuthMeta) context.Context {

	md := metadata.New(nil)
	md.Append("authorizationJwt", GetSignedToken(t, authVars, metaInfo))
	return metadata.NewIncomingContext(ctx, md)
}
