/*
 * Copyright 2017-2021 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

var adminEndpoint string

func makeRequest(t *testing.T, token *testutil.HttpToken,
	params testutil.GraphQLParams) *testutil.GraphQLResponse {
	resp := testutil.MakeGQLRequestWithAccessJwt(t, &params, token.AccessJwt)
	if len(resp.Errors) == 0 || !strings.Contains(resp.Errors.Error(), "Token is expired") {
		return resp
	}
	var err error
	fmt.Println("Here3")
	newtoken, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:   adminEndpoint,
		UserID:     token.UserId,
		Passwd:     token.Password,
		RefreshJwt: token.RefreshToken,
	})
	require.NoError(t, err)
	token.AccessJwt = newtoken.AccessJwt
	token.RefreshToken = newtoken.RefreshToken
	fmt.Println("Here")
	return testutil.MakeGQLRequestWithAccessJwt(t, &params, token.AccessJwt)
}

func createNamespace(t *testing.T, token *testutil.HttpToken) (uint64, error) {
	fmt.Println("Here1")
	createNs := `
		mutation{
 			createNamespace(input: {namespaceId: 1}){
    			namespaceId
    			message
  			}
		}`

	params := testutil.GraphQLParams{
		Query: createNs,
	}
	resp := makeRequest(t, token, params)
	var result struct {
		CreateNamespace struct {
			NamespaceId int    `json:"namespaceId"`
			Message     string `json:"message"`
		}
	}
	fmt.Println("Here2")
	require.NoError(t, json.Unmarshal(resp.Data, &result))
	spew.Dump(result)
	if strings.Contains(result.CreateNamespace.Message, "Created namespace successfully") {
		return uint64(result.CreateNamespace.NamespaceId), nil
	}
	return 0, errors.New("Failed to create namespace")
}

func deleteNamespace(t *testing.T, token *testutil.HttpToken, nsID uint64) {
	deleteReq := `mutation deleteNamespace($namespaceId: Int!) {
			deleteNamespace(input: {namespaceId: $namespaceId}){
    		namespaceId
    		message
  		}
	}`

	params := testutil.GraphQLParams{
		Query: deleteReq,
		Variables: map[string]interface{}{
			"namespaceId": nsID,
		},
	}
	resp := makeRequest(t, token, params)
	var result struct {
		DeleteNamespace struct {
			NamespaceId int    `json:"namespaceId"`
			Message     string `json:"message"`
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &result))
	spew.Dump(result)
	require.Equal(t, int(nsID), result.DeleteNamespace.NamespaceId)
	require.Contains(t, result.DeleteNamespace.Message, "Deleted namespace successfully")
}

func TestAclLogin(t *testing.T) {
	galaxyToken, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    x.GrootId,
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")

	spew.Dump(galaxyToken)
	ns, err := createNamespace(t, galaxyToken)
	require.NoError(t, err)
	require.Equal(t, 1, int(ns))
	deleteNamespace(t, galaxyToken, 1)

	ns, err = createNamespace(t, galaxyToken)
	require.NoError(t, err)
	require.Equal(t, 2, int(ns))
}

func TestMain(m *testing.M) {
	adminEndpoint = "http://" + testutil.SockAddrHttp + "/admin"
	fmt.Printf("Using adminEndpoint : %s for multy-tenancy test.\n", adminEndpoint)
	os.Exit(m.Run())
}
