//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
)

// disableDraining disables draining mode before each test for increased reliability.
func disableDraining(t *testing.T) {
	drainRequest := `mutation draining {
 		draining(enable: false) {
    		response {
        		code
        		message
      		}
  		}
	}`

	params := testutil.GraphQLParams{
		Query: drainRequest,
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	token, err := testutil.Login(t, &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: 0})
	require.NoError(t, err, "login failed")

	client := &http.Client{}
	req, err := http.NewRequest("POST", testutil.AdminUrl(), bytes.NewBuffer(b))
	require.Nil(t, err)
	req.Header.Add("content-type", "application/json")
	req.Header.Add("X-Dgraph-AccessToken", token.AccessJwt)

	resp, err := client.Do(req)
	require.NoError(t, err)
	buf, err := io.ReadAll(resp.Body)
	fmt.Println(string(buf))
	require.NoError(t, err)
	require.Contains(t, string(buf), "draining mode has been set to false")
}

func sendRestoreRequest(t *testing.T, location, backupId string, backupNum int) {
	if location == "" {
		location = "/data/backup2"
	}
	params := &testutil.GraphQLParams{
		Query: `mutation restore($location: String!, $backupId: String, $backupNum: Int) {
			restore(input: {location: $location, backupId: $backupId, backupNum: $backupNum}) {
				code
				message
			}
		}`,
		Variables: map[string]interface{}{
			"location":  location,
			"backupId":  backupId,
			"backupNum": backupNum,
		},
	}

	token, err := testutil.Login(t, &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: 0})
	require.NoError(t, err, "login failed")

	resp := testutil.MakeGQLRequestWithAccessJwt(t, params, token.AccessJwt)
	resp.RequireNoGraphQLErrors(t)

	var restoreResp struct {
		Restore struct {
			Code      string
			Message   string
			RestoreId int
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &restoreResp))
	require.Equal(t, restoreResp.Restore.Code, "Success")
}

func TestAclCacheRestore(t *testing.T) {
	// TODO: need to fix the race condition for license propagation, the sleep helps propagate the EE license correctly
	time.Sleep(time.Second * 10)
	disableDraining(t)
	conn, err := grpc.Dial(testutil.SockAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	require.NoError(t, dg.Login(context.Background(), "groot", "password"))

	sendRestoreRequest(t, "/backups", "vibrant_euclid5", 1)
	testutil.WaitForRestore(t, dg, testutil.SockAddrHttp)

	token, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "alice1", Passwd: "password", Namespace: 0})
	require.NoError(t, err, "login failed")
	params := &common.GraphQLParams{
		Query: `query{
					queryPerson{
						name
						age
					}
				}`,

		Headers: make(http.Header),
	}
	params.Headers.Set("X-Dgraph-AccessToken", token.AccessJwt)

	resp := params.ExecuteAsPost(t, common.GraphqlURL)
	require.Nil(t, resp.Errors)

	expected := `
	{
		"queryPerson": [
		  {
			"name": "MinhajSh",
			"age": 20
		  }
		]
	}
	`
	require.JSONEq(t, expected, string(resp.Data))
}
