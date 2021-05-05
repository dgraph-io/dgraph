/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func prepare(t *testing.T) {
	dc := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	require.NoError(t, dc.Alter(context.Background(), &api.Operation{DropAll: true}))
}

func readFile(t *testing.T, path string) []byte {
	data, err := ioutil.ReadFile(path)
	require.NoError(t, err)
	return data
}

func getHttpToken(t *testing.T, user, password string, ns uint64) *testutil.HttpToken {
	jwt := testutil.GetAccessJwt(t, testutil.JwtParams{
		User:   user,
		Groups: []string{"guardians"},
		Ns:     ns,
		Exp:    time.Hour,
		Secret: readFile(t, "../../ee/acl/hmac-secret"),
	})

	return &testutil.HttpToken{
		UserId:    user,
		Password:  password,
		AccessJwt: jwt,
	}
}

func graphqlHelper(t *testing.T, query string, headers http.Header,
	expectedResult string) {
	params := &common.GraphQLParams{
		Query:   query,
		Headers: headers,
	}
	queryResult := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, queryResult)
	testutil.CompareJSON(t, expectedResult, string(queryResult.Data))
}

func TestDisallowNonGalaxy(t *testing.T) {
	prepare(t)

	galaxyToken := getHttpToken(t, "groot", "password", x.GalaxyNamespace)
	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)
	require.Greater(t, int(ns), 0)

	nsToken := getHttpToken(t, "groot", "password", ns)
	header := http.Header{}
	header.Set("X-Dgraph-AccessToken", nsToken.AccessJwt)

	// User from namespace ns should be able to query/mutate.
	schema := `
	type Author {
		id: ID!
		name: String
	}`
	common.SafelyUpdateGQLSchema(t, common.Alpha1HTTP, schema, header)

	graphqlHelper(t, `
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

	query := `
	query {
		queryAuthor {
			name
		}
	}`
	graphqlHelper(t, query, header,
		`{
			"queryAuthor": [
				{
					"name":"Alice"
				}
			]
		}`)

	// Login to namespace 1 via groot and create new user alice. Non-galaxy namespace user should
	// not be able to do so in cloud mode.
	_, err = testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  testutil.AdminUrl(),
		UserID:    "groot",
		Passwd:    "password",
		Namespace: ns,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "operation is not allowed in cloud mode")

	// Ns guardian should not be able to create user.
	resp := testutil.CreateUser(t, nsToken, "alice", "newpassword")
	require.Greater(t, len(resp.Errors), 0)
	require.Contains(t, resp.Errors.Error(), "unauthorized to mutate acl predicates")

	// Galaxy guardian should be able to create user.
	resp = testutil.CreateUser(t, galaxyToken, "alice", "newpassword")
	require.Equal(t, 0, len(resp.Errors))
}

func TestEnvironmentAccess(t *testing.T) {
	prepare(t)

	galaxyToken := getHttpToken(t, "groot", "password", x.GalaxyNamespace)
	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)
	require.Greater(t, int(ns), 0)

	nsToken := getHttpToken(t, "groot", "password", ns)
	header := http.Header{}
	header.Set("X-Dgraph-AccessToken", nsToken.AccessJwt)

	// Create a minio bucket.
	bucketname := "dgraph-export"
	mc, err := testutil.NewMinioClient()
	require.NoError(t, err)
	require.NoError(t, mc.MakeBucket(bucketname, ""))
	minioDest := "minio://minio:9001/dgraph-export?secure=false"

	// Export without the minio creds should fail for non-galaxy.
	resp := testutil.Export(t, nsToken, minioDest, "", "")
	require.Greater(t, len(resp.Errors), 0)
	require.Contains(t, resp.Errors.Error(), "task failed")

	// Export without the minio creds should work for non-galaxy.
	resp = testutil.Export(t, nsToken, minioDest, "accesskey", "secretkey")
	require.Zero(t, len(resp.Errors))

	// Galaxy guardian should provide the crednetials as well.
	resp = testutil.Export(t, galaxyToken, minioDest, "accesskey", "secretkey")
	require.Zero(t, len(resp.Errors))

}

func TestMain(m *testing.M) {
	fmt.Printf("Using adminEndpoint : %s for cloud test.\n", testutil.AdminUrl())
	os.Exit(m.Run())
}
