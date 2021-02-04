package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

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
	newtoken, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:   adminEndpoint,
		UserID:     token.UserId,
		Passwd:     token.Password,
		RefreshJwt: token.RefreshToken,
	})
	require.NoError(t, err)
	token.AccessJwt = newtoken.AccessJwt
	token.RefreshToken = newtoken.RefreshToken
	return testutil.MakeGQLRequestWithAccessJwt(t, &params, token.AccessJwt)
}

func createNamespace(t *testing.T, token *testutil.HttpToken) uint64 {
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
	require.NoError(t, json.Unmarshal(resp.Data, &result))
	require.Contains(t, result.CreateNamespace.Message, "Created namespace successfully")
	return uint64(result.CreateNamespace.NamespaceId)
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
	require.Equal(t, int(nsID), result.DeleteNamespace.NamespaceId)
	require.Contains(t, result.DeleteNamespace.Message, "Deleted namespace successfully")
}

func TestCreateDelete(t *testing.T) {
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint: adminEndpoint,
		UserID:   x.GrootId,
		Passwd:   "password",
	})
	require.NoError(t, err, "login failed")

	ns := createNamespace(t, token)
	require.Equal(t, 1, int(ns))

	deleteNamespace(t, token, 1)
}

func TestMain(m *testing.M) {
	adminEndpoint = "http://" + testutil.SockAddrHttp + "/admin"
	fmt.Printf("Using adminEndpoint : %s for multy-tenancy test.\n", adminEndpoint)
	os.Exit(m.Run())
}
