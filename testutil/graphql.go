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
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/pkg/errors"

	"github.com/dgrijalva/jwt-go/v4"
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
	require.Nil(t, result.Errors, "Recieved GQL Errors: %+v", result.Errors)
}

func MakeGQLRequest(t *testing.T, params *GraphQLParams) *GraphQLResponse {
	return MakeGQLRequestWithAccessJwt(t, params, "")
}

func MakeGQLRequestWithAccessJwt(t *testing.T, params *GraphQLParams,
	accessToken string) *GraphQLResponse {
	adminUrl := "http://" + SockAddrHttp + "/admin"

	b, err := json.Marshal(params)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, adminUrl, bytes.NewBuffer(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("X-Dgraph-AccessToken", accessToken)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()
	b, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	var gqlResp GraphQLResponse
	err = json.Unmarshal(b, &gqlResp)
	require.NoError(t, err)

	return &gqlResp
}

type clientCustomClaims struct {
	Namespace     string
	AuthVariables map[string]interface{}
	jwt.StandardClaims
}

func (c clientCustomClaims) MarshalJSON() ([]byte, error) {
	// Encode the original
	m, err := json.Marshal(c.StandardClaims)
	if err != nil {
		return nil, err
	}

	// Decode it back to get a map
	var a interface{}
	err = json.Unmarshal(m, &a)
	if err != nil {
		return nil, err
	}

	b, ok := a.(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("error while marshalling custom claim json.")
	}

	// Set the proper namespace and delete additional data.
	b[c.Namespace] = c.AuthVariables
	delete(b, "AuthVariables")
	delete(b, "Namespace")

	// Return encoding of the map
	return json.Marshal(b)
}

type AuthMeta struct {
	PublicKey string
	Namespace string
	Algo      string
	Header    string
	AuthVars  map[string]interface{}
}

func (a *AuthMeta) GetSignedToken(privateKeyFile string) (string, error) {
	claims := clientCustomClaims{
		a.Namespace,
		a.AuthVars,
		jwt.StandardClaims{
			ExpiresAt: jwt.At(time.Now().Add(time.Minute * 5)),
			Issuer:    "test",
		},
	}

	var signedString string
	var err error
	if a.Algo == "HS256" {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signedString, err = token.SignedString([]byte(a.PublicKey))
		return signedString, err
	}
	if a.Algo != "RS256" {
		return signedString, err

	}
	keyData, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return signedString, errors.Errorf("unable to read private key file: %v", err)
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyData)
	if err != nil {
		return signedString, errors.Errorf("unable to parse private key: %v", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedString, err = token.SignedString(privateKey)
	return signedString, err
}

func (a *AuthMeta) AddClaimsToContext(ctx context.Context) (context.Context, error) {
	token, err := a.GetSignedToken("../e2e/auth/sample_private_key.pem")
	if err != nil {
		return ctx, err
	}

	md := metadata.New(nil)
	md.Append("authorizationJwt", token)
	return metadata.NewIncomingContext(ctx, md), nil
}

func AppendAuthInfo(schema []byte, algo, publicKeyFile string) ([]byte, error) {
	if algo == "HS256" {
		authInfo := `# Dgraph.Authorization {"VerificationKey":"secretkey","Header":"X-Test-Auth","Namespace":"https://xyz.io/jwt/claims","Algo":"HS256","Audience":["aud1","63do0q16n6ebjgkumu05kkeian","aud5"]}`
		return append(schema, []byte(authInfo)...), nil
	}

	if algo != "RS256" {
		return schema, nil
	}

	keyData, err := ioutil.ReadFile(publicKeyFile)
	if err != nil {
		return nil, err
	}

	// Replacing ASCII newline with "\n" as the authorization information in the schema should be
	// present in a single line.
	keyData = bytes.ReplaceAll(keyData, []byte{10}, []byte{92, 110})
	authInfo := `# Dgraph.Authorization {"VerificationKey":"` + string(keyData) + `","Header":"X-Test-Auth","Namespace":"https://xyz.io/jwt/claims","Algo":"RS256","Audience":["aud1","63do0q16n6ebjgkumu05kkeian","aud5"]}`
	return append(schema, []byte(authInfo)...), nil
}
