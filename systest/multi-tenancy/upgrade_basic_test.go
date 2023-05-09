//go:build upgrade

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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
	"net/http"
	"os"
	//"path/filepath"
	"testing"
	"time"
	"strings"
	"encoding/json"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

type Rule struct {
	Predicate  string `json:"predicate"`
	Permission int32  `json:"permission"`
}

// Upgrade table
var uTable = []struct{
		srcVer string
		dstVer string
}{
	{"v22.0.2", "v23.0.0-rc1"},
	//{"0c9f60156", "v22.0.2"},
	//{"0c9f60156", "v23.0.0-rc1"},
}

type MultitenancyTestSuite struct {
	suite.Suite
	lc *dgraphtest.LocalCluster
	client *dgraphtest.GrpcClient
	httpclient *dgraphtest.HTTPClient
	cleanup_client_conns func()
}

func (suite *MultitenancyTestSuite) SetupSuite() {
	fmt.Println("*** SetupSuite Start ***")
}

func (suite *MultitenancyTestSuite) TearDownSuite() {
	fmt.Println("*** TearDownSuite End ***")
}

func (suite *MultitenancyTestSuite) SetupTest() {
	fmt.Println("### SetupTest Start ###")
}

func (suite *MultitenancyTestSuite) TearDownTest() {
	fmt.Println("### TearDownTest End ###")
}

type inputTripletsCount struct {
	lowerLimit int
	upperLimit int
}

func (suite *MultitenancyTestSuite) prepare() {
	t := suite.T()

	err := suite.client.Dgraph.LoginIntoNamespace(context.Background(), "groot", "password", x.GalaxyNamespace) 
	require.NoError(t, err, "login with galaxy failed")
	require.NoError(t, suite.client.Dgraph.Alter(context.Background(), &api.Operation{DropAll: true}))
}

var timeout = 5 * time.Second

func (suite *MultitenancyTestSuite) setupSourceDB(srcDB string) {
	os.Setenv("GOOS", "linux")
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithACL(20 * time.Second).WithEncryption().WithVersion(srcDB)

	var err error
	suite.lc, err = dgraphtest.NewLocalCluster(conf)
	x.Panic(err)

	err = suite.lc.Start()
	x.Panic(err)

	var cleanup_client_conns func()
	suite.client, cleanup_client_conns, err = suite.lc.Client()
	suite.cleanup_client_conns = cleanup_client_conns
	x.Panic(err)

	suite.httpclient, err = suite.lc.HTTPClient()
	x.Panic(err)
}

func (suite *MultitenancyTestSuite) UpgradeAndSetupClient(dstDB string) {
	var err error
	t := suite.T()

	if err := suite.lc.Upgrade(dstDB, dgraphtest.StopStart); err != nil {
		t.Fatal(err)
	}

	var cleanup_client_conns func()
	suite.client, cleanup_client_conns, err = suite.lc.Client()
	suite.cleanup_client_conns = cleanup_client_conns
	x.Panic(err)

	suite.httpclient, err = suite.lc.HTTPClient()
	x.Panic(err)
}

// TODO(Ahsan): This is just a basic test, for the purpose of development. The functions used in
// this file can me made common to the other acl tests as well. Needs some refactoring as well.
func (suite *MultitenancyTestSuite) TestAclBasic() {
	for _, uE := range uTable {
		fmt.Printf("\n*** Starting upgrade test from source version %s to destination version %s ***\n", uE.srcVer, uE.dstVer)
		suite.setupSourceDB(uE.srcVer)

		suite.prepare()

		var err error
		t := suite.T()

	// Cleanup, should be registered in this order
		t.Cleanup(suite.lc.Cleanup)
		t.Cleanup(suite.cleanup_client_conns)

	// Galaxy Login
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
		require.NotNil(t, suite.httpclient, "galaxy token is nil")
		require.NoError(t, err, "login with namespace failed")

	// Create a new namespace
		var ns uint64
		ns, err = suite.CreateNamespaceWithRetry()
		require.NoError(t, err)
		require.Greater(t, int(ns), 0)

	// Add some data to namespace 1
		suite.client = suite.DgClientWithLogin("groot", "password", ns)
		suite.AddData()

		query := `
			{
				me(func: has(name)) {
					nickname
					name
				}
			}
		`
		resp := suite.QueryData(query)
		testutil.CompareJSON(t,
			`{"me": [{"name":"guy1","nickname":"RG"},
			{"name": "guy2", "nickname":"RG2"}]}`,
			string(resp))

	// groot of namespace 0 should not see the data of namespace-1
		suite.client = suite.DgClientWithLogin("groot", "password", 0)
		resp = suite.QueryData(query)
		testutil.CompareJSON(t, `{"me": []}`, string(resp))

	// Login to namespace 1 via groot and create new user alice.
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns)
		require.NotNil(t, suite.httpclient, "token for the namespace is nil")
		require.NoErrorf(t, err, "login with namespace %d failed", ns)
		suite.CreateUser("alice", "newpassword")

	// Alice should not be able to see data added by groot in namespace 1
		suite.client = suite.DgClientWithLogin("alice", "newpassword", ns)
		resp = suite.QueryData(query)
		testutil.CompareJSON(t, `{}`, string(resp))

	// Create a new group, add alice to that group and give read access of <name> to dev group.
		suite.CreateGroup("dev")
		suite.AddToGroup("alice", "dev")
		suite.AddRulesToGroup("dev",
			[]Rule{{Predicate: "name", Permission: acl.Read.Code}}, true)

	// Now alice should see the name predicate but not nickname.
		suite.client = suite.DgClientWithLogin("alice", "newpassword", ns)
		testutil.PollTillPassOrTimeout(t, suite.client.Dgraph, query, `{"me": [{"name":"guy1"},{"name": "guy2"}]}`, timeout)

	// Upgrade
		suite.UpgradeAndTestAclBasic(ns, uE.dstVer)
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndTestAclBasic(ns uint64, dstDB string) {
	suite.UpgradeAndSetupClient(dstDB)

	var err error
	t := suite.T()

// Login into Galaxy as groot
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
	require.NotNil(t, suite.httpclient, "galaxy token is nil")
	require.NoError(t, err, "login with namespace failed")

// Login to namespace 1
	suite.client = suite.DgClientWithLogin("groot", "password", ns)

	query := `
		{
			me(func: has(name)) {
				nickname
				name
			}
		}
	`
// Data must be present after the upgrade
	resp := suite.QueryData(query)
	testutil.CompareJSON(t,
		`{"me": [{"name":"guy1","nickname":"RG"},
		{"name": "guy2", "nickname":"RG2"}]}`,
		string(resp))

// groot of namespace 0 should not see the data of namespace-1
	suite.client = suite.DgClientWithLogin("groot", "password", 0)
	resp = suite.QueryData(query)
	testutil.CompareJSON(t, `{"me": []}`, string(resp))

// alice already a member of 'dev' should see the name predicate but not nickname,
// same as before the upgrade.
	suite.client = suite.DgClientWithLogin("alice", "newpassword", ns)
	testutil.PollTillPassOrTimeout(t, suite.client.Dgraph, query, `{"me": [{"name":"guy1"},{"name": "guy2"}]}`, timeout)
}

func (suite *MultitenancyTestSuite) TestNameSpaceLimitFlag() {
  	for _, uE := range uTable {
		fmt.Printf("\n*** Starting upgrade test from source version %s to destination version %s ***\n", uE.srcVer, uE.dstVer)
  		suite.setupSourceDB(uE.srcVer)
  
  		suite.prepare()
  		t := suite.T()

	// Cleanup, should be registered in this order
		t.Cleanup(suite.lc.Cleanup)
		t.Cleanup(suite.cleanup_client_conns)

		testInputs := []inputTripletsCount{{1, 53}, {60, 100}, {141, 153}}
  
  	// Galaxy login
  		var err error
  		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
  		require.NotNil(t, suite.httpclient, "galaxy token is nil")
  		require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Create a new namespace
  		ns, err := suite.CreateNamespaceWithRetry()
  		require.NoError(t, err)

  		suite.client = suite.DgClientWithLogin("groot", "password", ns)
		require.NoError(t, suite.client.Dgraph.Alter(context.Background(),
			&api.Operation{Schema: `name: string .`}))

	// trying to load more triplets than allowed,It should return error.
		_, err = suite.AddNumberOfTriples(suite.client.Dgraph, testInputs[0].lowerLimit, testInputs[0].upperLimit)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Requested UID lease(53) is greater than allowed(50).")

		_, err = suite.AddNumberOfTriples(suite.client.Dgraph, testInputs[1].lowerLimit, testInputs[1].upperLimit)
		require.NoError(t, err)

	// we have set uid-lease=50 so we are trying lease more uids,it should return error.
		_, err = suite.AddNumberOfTriples(suite.client.Dgraph, testInputs[2].lowerLimit, testInputs[2].upperLimit)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Cannot lease UID because UID lease for the namespace")

	// Upgrade and test name space limit flag
		suite.UpgradeAndTestNameSpaceLimitFlag(ns, uE.dstVer)
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndTestNameSpaceLimitFlag(ns uint64, dstDB string) {
	suite.UpgradeAndSetupClient(dstDB)

	var err error
	t := suite.T()

	testInputs := []inputTripletsCount{{1, 53}, {60, 100}, {141, 153}}
  
// Galaxy login
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
	require.NotNil(t, suite.httpclient, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

// Log into namespace
	suite.client = suite.DgClientWithLogin("groot", "password", ns)

// trying to load more triplets than allowed,It should return error.
	_, err = suite.AddNumberOfTriples(suite.client.Dgraph, testInputs[0].lowerLimit, testInputs[0].upperLimit)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Requested UID lease(53) is greater than allowed(50).")

	_, err = suite.AddNumberOfTriples(suite.client.Dgraph, testInputs[1].lowerLimit, testInputs[1].upperLimit)
	require.NoError(t, err)

// we have set uid-lease=50 so we are trying lease more uids,it should return error.
	_, err = suite.AddNumberOfTriples(suite.client.Dgraph, testInputs[2].lowerLimit, testInputs[2].upperLimit)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot lease UID because UID lease for the namespace")
}

func (suite *MultitenancyTestSuite) AddNumberOfTriples(dg *dgo.Dgraph, start, end int) (*api.Response, error) {
	triples := strings.Builder{}
	for i := start; i <= end; i++ {
		triples.WriteString(fmt.Sprintf("_:person%[1]v <name> \"person%[1]v\" .\n", i))
	}
	resp, err := dg.NewTxn().Mutate(context.Background(), &api.Mutation{
		SetNquads: []byte(triples.String()),
		CommitNow: true,
	})
	return resp, err
}

func (suite *MultitenancyTestSuite) postGqlSchema(schema string, accessJwt string) {
	t := suite.T()
	testutil.DockerPrefix = suite.lc.GetClusterPrefix()
	groupOneHTTP := testutil.ContainerAddr("alpha1", 8080)
	header := http.Header{}
	header.Set("X-Dgraph-AccessToken", accessJwt)
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, schema, header)
}

func (suite *MultitenancyTestSuite) postPersistentQuery(query, sha, accessJwt string) *common.GraphQLResponse {
	t := suite.T()
	header := http.Header{}
	header.Set("X-Dgraph-AccessToken", accessJwt)
	queryCountryParams := &common.GraphQLParams{
		Query: query,
		Extensions: &schema.RequestExtensions{PersistedQuery: schema.PersistedQuery{
			Sha256Hash: sha,
		}},
		Headers: header,
	}
	testutil.DockerPrefix = suite.lc.GetClusterPrefix()
	url := "http://" + testutil.ContainerAddr("alpha1", 8080) + "/graphql"
	return queryCountryParams.ExecuteAsPost(t, url)
}

func (suite *MultitenancyTestSuite) TestPersistentQuery() {
  	for _, uE := range uTable {
		fmt.Printf("\n*** Starting upgrade test from source version %s to destination version %s ***\n", uE.srcVer, uE.dstVer)
  		suite.setupSourceDB(uE.srcVer)
  
  		suite.prepare()
  		t := suite.T()

	// Cleanup, should be registered in this order
		t.Cleanup(suite.lc.Cleanup)
		t.Cleanup(suite.cleanup_client_conns)

	// Galaxy Login
		var err error
  		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
		require.NotNil(t, suite.httpclient, "galaxy token is nil")
		require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Make a copy of the galaxy token
		gt := *suite.httpclient.HttpToken
		galaxyToken := &gt
  		require.NotNil(t, galaxyToken, "galaxy token is nil")
  		require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Create a new namespace
  		ns, err := suite.CreateNamespaceWithRetry()
  		require.NoError(t, err)

	// Log into ns
  		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns)

	// Make a token copy
		tt := *suite.httpclient.HttpToken
		token := &tt
  		require.NotNil(t, token, "Token is nil")
  		require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

		sch := `type Product {
				productID: ID!
				name: String @search(by: [term])
			}`
		suite.postGqlSchema(sch, galaxyToken.AccessJwt)
		suite.postGqlSchema(sch, token.AccessJwt)

		p1 := "query {queryProduct{productID}}"
		sha1 := "7a8ff7a69169371c1eb52a8921387079ca281bb2d55feb4b535cbf0ab3896be5"
		resp := suite.postPersistentQuery(p1, sha1, galaxyToken.AccessJwt)
		common.RequireNoGQLErrors(t, resp)

		p2 := "query {queryProduct{name}}"
		sha2 := "0efcdde144167b1046360b73c7f6bec325d9f555099a2ae9b820a13328d270e4"
		resp = suite.postPersistentQuery(p2, sha2, token.AccessJwt)
		common.RequireNoGQLErrors(t, resp)

	// User cannnot see persistent query from other namespace.
		resp = suite.postPersistentQuery("", sha2, galaxyToken.AccessJwt)
		require.Equal(t, 1, len(resp.Errors))
		require.Contains(t, resp.Errors[0].Message, "PersistedQueryNotFound")

		resp = suite.postPersistentQuery("", sha1, token.AccessJwt)
		require.Equal(t, 1, len(resp.Errors))
		require.Contains(t, resp.Errors[0].Message, "PersistedQueryNotFound")

		resp = suite.postPersistentQuery("", sha1, "")
		require.Equal(t, 1, len(resp.Errors))
		require.Contains(t, resp.Errors[0].Message, "no accessJwt available")

	// Upgrade and test persistent query
		suite.UpgradeAndTestPersistentQuery(ns, sha1, sha2, uE.dstVer)
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndTestPersistentQuery(ns uint64, sha1 string, sha2 string, dstDB string) {
	suite.UpgradeAndSetupClient(dstDB)

	var err error
	t := suite.T()

// Galaxy Login
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)

// Make a copy of the galaxy token
	gt := *suite.httpclient.HttpToken
	galaxyToken := &gt
	require.NotNil(t, galaxyToken, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

// Log into ns
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns)

// Make a token copy
	tt := *suite.httpclient.HttpToken
	token := &tt
	require.NotNil(t, token, "token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

// User cannnot see persistent query from other namespace.
	resp := suite.postPersistentQuery("", sha2, galaxyToken.AccessJwt)
	require.Equal(t, 1, len(resp.Errors))
	require.Contains(t, resp.Errors[0].Message, "PersistedQueryNotFound")

	resp = suite.postPersistentQuery("", sha1, token.AccessJwt)
	require.Equal(t, 1, len(resp.Errors))
	require.Contains(t, resp.Errors[0].Message, "PersistedQueryNotFound")

	resp = suite.postPersistentQuery("", sha1, "")
	require.Equal(t, 1, len(resp.Errors))
	require.Contains(t, resp.Errors[0].Message, "no accessJwt available")
}
func (suite *MultitenancyTestSuite) TestTokenExpired() {
  	for _, uE := range uTable {
		fmt.Printf("\n*** Starting upgrade test from source version %s to destination version %s ***\n", uE.srcVer, uE.dstVer)
  		suite.setupSourceDB(uE.srcVer)
  
  		suite.prepare()
  		t := suite.T()

	// Cleanup, should be registered in this order
		t.Cleanup(suite.lc.Cleanup)
		t.Cleanup(suite.cleanup_client_conns)

	// Galaxy Login
		var err error
  		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
  		require.NotNil(t, suite.httpclient.HttpToken, "galaxy token is nil")
  		require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Create a new namespace
  		ns, err := suite.CreateNamespaceWithRetry()
  		require.NoError(t, err)

	// ns Login
  		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns)
  		require.NotNil(t, suite.httpclient.HttpToken, "token is nil")
		require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

	// Relogin using refresh JWT.
		token := suite.httpclient.HttpToken
  		err, suite.httpclient = suite.HTTPClientLoginWithToken("groot", "password", ns, token.RefreshToken)
  		require.NotNil(t, token, "token is nil")
		require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

	// Create another namespace
  		_, err = suite.CreateNamespaceWithRetry()
  		require.Error(t, err)
		require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")

	// Upgrade and stest token expired
		suite.UpgradeAndTestTokenExpired(ns, uE.dstVer)
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndTestTokenExpired(ns uint64, dstDB string) {
	suite.UpgradeAndSetupClient(dstDB)

	var err error
	t := suite.T()

// Galaxy Login
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
	require.NotNil(t, suite.httpclient.HttpToken, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

// ns Login
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns)
	require.NotNil(t, suite.httpclient.HttpToken, "token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

// Relogin using refresh JWT.
	token := suite.httpclient.HttpToken
	err, suite.httpclient = suite.HTTPClientLoginWithToken("groot", "password", ns, token.RefreshToken)
	require.NotNil(t, token, "token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

// Create another namespace
	ns, err = suite.CreateNamespaceWithRetry()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")
}

func (suite *MultitenancyTestSuite) createGroupAndSetPermissions(namespace uint64, group, user, predicate string) {
	var err error
	t := suite.T()
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", namespace)
	require.NotNil(t, suite.httpclient, "namespace token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", namespace)
	suite.CreateGroup(group)
	suite.AddToGroup(user, group)
	suite.AddRulesToGroup(group,
		[]Rule{{Predicate: predicate, Permission: acl.Read.Code}}, true)
}

func (suite *MultitenancyTestSuite) TestTwoPermissionSetsInNameSpacesWithAcl() {
	for _, uE := range uTable {
		fmt.Printf("\n*** Starting upgrade test from source version %s to destination version %s ***\n", uE.srcVer, uE.dstVer)
		suite.setupSourceDB(uE.srcVer)

		suite.prepare()
		t := suite.T()

	// Cleanup, should be registered in this order
		t.Cleanup(suite.lc.Cleanup)
		t.Cleanup(suite.cleanup_client_conns)

	// Galaxy Login
		var err error
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
		require.NotNil(t, suite.httpclient, "galaxy token is nil")
		require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)
		gt := *suite.httpclient.HttpToken
		galaxyToken := &gt

		query := `
			{
				me(func: has(name)) {
					nickname
					name
				}
			}
		`
	// Create first namespace
		ns1, err := suite.CreateNamespaceWithRetry()
		require.NoError(t, err)

	// Add data to namespace 1
		suite.client = suite.DgClientWithLogin("groot", "password", ns1)
		suite.AddData()

	user1, user2 := "alice", "bob"
	user1passwd, user2passwd := "newpassword", "newpassword"

	// Create user alice
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns1)
		require.NoErrorf(t, err, "login as groot into namespace %d failed", ns1)
		suite.CreateUser(user1, user1passwd)

	// Create a new group, add alice to that group and give read access to <name> in the dev group.
		suite.createGroupAndSetPermissions(ns1, "dev", user1, "name")

	// Alice should not be able to see <nickname> in namespace 1
		suite.client = suite.DgClientWithLogin(user1, user1passwd, ns1)
		testutil.PollTillPassOrTimeout(t, suite.client.Dgraph, query, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, timeout)

	// Create second namespace
		suite.httpclient.HttpToken = galaxyToken
		ns2, err := suite.CreateNamespaceWithRetry()
		require.NoError(t, err)

	// Add data to namespace 2
		suite.client = suite.DgClientWithLogin("groot", "password", ns2)
		suite.AddData()

	// Create user bob
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns2)
		require.NoErrorf(t, err, "login with namespace %d failed", ns2)
		suite.CreateUser(user2, user2passwd)

	// Create a new group, add bob to that group and give read access of <nickname> to dev group.
		suite.createGroupAndSetPermissions(ns2, "dev", user2, "nickname")

	// Query via bob and check result
		suite.client = suite.DgClientWithLogin(user2, user2passwd, ns2)
		testutil.PollTillPassOrTimeout(t, suite.client.Dgraph, query, `{}`, timeout)

	// Query namespace-1 via alice and check result to ensure it still works
		suite.client = suite.DgClientWithLogin(user1, user1passwd, ns1)
		resp := suite.QueryData(query)
		testutil.CompareJSON(t, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, string(resp))

	// Change permissions in namespace-2
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns2)
		require.NoErrorf(t, err, "login as groot into namespace %d failed", ns2)
		suite.AddRulesToGroup("dev",
			[]Rule{{Predicate: "name", Permission: acl.Read.Code}}, false)

	// Query namespace-2
		suite.client = suite.DgClientWithLogin(user2, user2passwd, ns2)
		testutil.PollTillPassOrTimeout(t, suite.client.Dgraph, query,
			`{"me": [{"name":"guy2", "nickname": "RG2"}, {"name":"guy1", "nickname": "RG"}]}`, timeout)

	// Query namespace-1
		suite.client = suite.DgClientWithLogin(user1, user1passwd, ns1)
		resp = suite.QueryData(query)
		testutil.CompareJSON(t, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, string(resp))

	// Upgrade and test
		suite.UpgradeAndTestTwoPermissionSetsInNameSpacesWithAcl(ns1, ns2, query, user1, user1passwd, user2, user2passwd, uE.dstVer)
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndTestTwoPermissionSetsInNameSpacesWithAcl(ns1, ns2 uint64, query, user1, user1passwd, user2, user2passwd, dstDB string) {
	suite.UpgradeAndSetupClient(dstDB)

	t := suite.T()

// Query namespace-2
	suite.client = suite.DgClientWithLogin(user2, user2passwd, ns2)
	testutil.PollTillPassOrTimeout(t, suite.client.Dgraph, query,
		`{"me": [{"name":"guy2", "nickname": "RG2"}, {"name":"guy1", "nickname": "RG"}]}`, timeout)

// Query namespace-1
	suite.client = suite.DgClientWithLogin(user1, user1passwd, ns1)
	resp := suite.QueryData(query)
	testutil.CompareJSON(t, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, string(resp))
}

func (suite *MultitenancyTestSuite) TestCreateNamespace() {
	for _, uE := range uTable {
		fmt.Printf("\n*** Starting upgrade test from source version %s to destination version %s ***\n", uE.srcVer, uE.dstVer)
		suite.setupSourceDB(uE.srcVer)

		suite.prepare()
		t := suite.T()

	// Cleanup, should be registered in this order
		t.Cleanup(suite.lc.Cleanup)
		t.Cleanup(suite.cleanup_client_conns)

	// Galaxy Login
		var err error
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
		require.NotNil(t, suite.httpclient, "Galaxy token is nil")
		require.NoErrorf(t, err, "login failed")

	// Create a new namespace
		ns, err := suite.CreateNamespaceWithRetry()
		require.NoError(t, err)

	// Log into the namespace as groot
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns)
		require.NotNil(t, suite.httpclient, "namespace token is nil")
		require.NoErrorf(t, err, "login with namespace %d failed", ns)

	// Create a new namespace using guardian of other namespace.
		_, err = suite.CreateNamespaceWithRetry()
		require.Error(t, err)
		require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")

	// Upgrade
		suite.UpgradeAndTestCreateNamespace(ns, uE.dstVer)
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndTestCreateNamespace(ns uint64, dstDB string) {
	suite.UpgradeAndSetupClient(dstDB)

	var err error
	t := suite.T()

// As groot log into the namespace created in the source version.
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns)
	require.NotNil(t, suite.httpclient, "namespace token is nil")
	require.NoErrorf(t, err, "login with namespace %d failed", ns)

// Create a new namespace using guardian of other namespace.
	_, err = suite.CreateNamespaceWithRetry()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")
}
	
func (suite *MultitenancyTestSuite) TestResetPassword() {
  	for _, uE := range uTable {
		fmt.Printf("\n*** Starting upgrade test from source version %s to destination version %s ***\n", uE.srcVer, uE.dstVer)
  		suite.setupSourceDB(uE.srcVer)
  
		suite.prepare()
		t := suite.T()

	// Cleanup, should be registered in this order
		t.Cleanup(suite.lc.Cleanup)
		t.Cleanup(suite.cleanup_client_conns)

	// Galaxy Login
		var err error
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
		require.NotNil(t, suite.httpclient.HttpToken, "Galaxy token is nil")
		require.NoErrorf(t, err, "login failed")

	// Create a new namespace
		ns, err := suite.CreateNamespaceWithRetry()
		require.NoError(t, err)

	// Reset Password
		_, err = suite.ResetPassword("groot", "newpassword", ns)
		require.NoError(t, err)
		c := *suite.httpclient
		tokenAfterReset := &c

	// Try and Fail with old password for groot
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns)
		require.Error(t, err, "expected error because incorrect login")
		require.Nil(t, suite.httpclient, "nil token because incorrect login")

	// Try and succeed with new password for groot
		suite.httpclient = tokenAfterReset
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "newpassword", ns)
		require.NoError(t, err, "login failed")
		require.Equal(t, suite.httpclient.HttpToken.Password, "newpassword", "new password matches the reset password")

	// Upgrade and test reset password
		suite.UpgradeAndTestResetPassword(ns, tokenAfterReset, uE.dstVer)
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndTestResetPassword(ns uint64, tokenAfterReset *dgraphtest.HTTPClient, dstDB string) {
	suite.UpgradeAndSetupClient(dstDB)

	var err error
	t := suite.T()

// Try and Fail with old password for groot
	suite.httpclient = tokenAfterReset
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns)
	require.Error(t, err, "expected error because incorrect login")
	require.Nil(t, suite.httpclient, "nil token because incorrect login")

// Try and succeed with new password for groot
	suite.httpclient = tokenAfterReset
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "newpassword", ns)
	require.NoError(t, err, "login failed")
	require.Equal(t, suite.httpclient.HttpToken.Password, "newpassword", "new password matches the reset password")
}

func (suite *MultitenancyTestSuite) TestDeleteNamespace() {
	for _, uE := range uTable {
		fmt.Printf("\n*** Starting upgrade test from source version %s to destination version %s ***\n", uE.srcVer, uE.dstVer)
		suite.setupSourceDB(uE.srcVer)

		suite.prepare()
		t := suite.T()

	// Cleanup, should be registered in this order
		t.Cleanup(suite.lc.Cleanup)
		t.Cleanup(suite.cleanup_client_conns)

	// Galaxy Login
		var err error
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
		require.NoErrorf(t, err, "login failed")

		dg := make(map[uint64]dgo.Dgraph)
		dc := suite.DgClientWithLogin("groot", "password", x.GalaxyNamespace)
		dg[x.GalaxyNamespace] = *dc.Dgraph

	// Create a new namespace
		ns, err := suite.CreateNamespaceWithRetry()
		require.NoError(t, err)

	// Log into namespace as groot.
		dc = suite.DgClientWithLogin("groot", "password", ns)
		dg[ns] = *dc.Dgraph

		addData := func(ns uint64) error {
			mutation := &api.Mutation{
				SetNquads: []byte(fmt.Sprintf(`
				_:a <name> "%d" .
			`, ns)),
				CommitNow: true,
			}
			c := dg[ns]
			cc := &c
			_, err := cc.NewTxn().Mutate(context.Background(), mutation)
			return err
		}
		check := func(ns uint64, expected string) {
			query := `
			{
				me(func: has(name)) {
					name
				}
			}
		`
			ccc := dg[ns]
			suite.client.Dgraph = &ccc
			resp := suite.QueryData(query)
			testutil.CompareJSON(t, expected, string(resp))
		}

		require.NoError(t, addData(x.GalaxyNamespace))
		check(x.GalaxyNamespace, `{"me": [{"name":"0"}]}`)
		require.NoError(t, addData(ns))
		check(ns, fmt.Sprintf(`{"me": [{"name":"%d"}]}`, ns))

		require.NoError(t, suite.DeleteNamespace(ns))
		require.NoError(t, addData(x.GalaxyNamespace))
		check(x.GalaxyNamespace, `{"me": [{"name":"0"}, {"name":"0"}]}`)
		err = addData(ns)
		require.Contains(t, err.Error(), "Key is using the banned prefix")
		check(ns, `{"me": []}`)

	// No one should be able to delete the default namespace. Not even guardian of galaxy.
		err = suite.DeleteNamespace(x.GalaxyNamespace)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Cannot delete default namespace")

	// Upgrade
		suite.UpgradeAndTestDeleteNamespace(uE.dstVer)
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndTestDeleteNamespace(dstDB string) {
	suite.UpgradeAndSetupClient(dstDB)

	var err error
	t := suite.T()

// Galaxy Login
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
	require.NoErrorf(t, err, "login failed")

// Check the Delete Namespace in the upgraded version is identical to as in the source version.
	dg := make(map[uint64]dgo.Dgraph)
	dc := suite.DgClientWithLogin("groot", "password", x.GalaxyNamespace)
	dg[x.GalaxyNamespace] = *dc.Dgraph

// Create a new namespace
	ns, err := suite.CreateNamespaceWithRetry()
	require.NoError(t, err)

// Log into namespace as groot.
	dc = suite.DgClientWithLogin("groot", "password", ns)
	dg[ns] = *dc.Dgraph

	addData := func(ns uint64) error {
		mutation := &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`
			_:a <name> "%d" .
		`, ns)),
			CommitNow: true,
		}
		c := dg[ns]
		cc := &c
		_, err := cc.NewTxn().Mutate(context.Background(), mutation)
		return err
	}
	check := func(ns uint64, expected string) {
		query := `
		{
			me(func: has(name)) {
				name
			}
		}
	`
		ccc := dg[ns]
		suite.client.Dgraph = &ccc
		resp := suite.QueryData(query)
		testutil.CompareJSON(t, expected, string(resp))
	}

	require.NoError(t, addData(x.GalaxyNamespace))
	check(x.GalaxyNamespace, `{"me": [{"name":"0"}]}`)
	require.NoError(t, addData(ns))
	check(ns, fmt.Sprintf(`{"me": [{"name":"%d"}]}`, ns))

	require.NoError(t, suite.DeleteNamespace(ns))
	require.NoError(t, addData(x.GalaxyNamespace))
	check(x.GalaxyNamespace, `{"me": [{"name":"0"}, {"name":"0"}]}`)
	err = addData(ns)
	require.Contains(t, err.Error(), "Key is using the banned prefix")
	check(ns, `{"me": []}`)

// No one should be able to delete the default namespace. Not even guardian of galaxy.
	err = suite.DeleteNamespace(x.GalaxyNamespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot delete default namespace")
}

func (suite *MultitenancyTestSuite) DgClientWithLogin(id, password string, ns uint64) *dgraphtest.GrpcClient {
	t := suite.T()
	userClient := suite.client

	require.NoError(t, x.RetryUntilSuccess(50, 100*time.Millisecond, func() error {
		return userClient.Dgraph.LoginIntoNamespace(context.Background(), id, password, ns) 
	})) 

	return userClient
}

func (suite *MultitenancyTestSuite) HTTPClientWithLogin(id, password string, ns uint64) (error, *dgraphtest.HTTPClient) {
	//t := suite.T()

	var err error
	if suite.httpclient == nil {
		suite.httpclient, err = suite.lc.HTTPClient()
	}
	userClient := suite.httpclient

	err = x.RetryUntilSuccess(20, 100*time.Millisecond, func() error {
		return userClient.LoginIntoNamespace(id, password, ns) 
	})

	if err != nil {
		return err, nil
	}

	return err, userClient
}

func (suite *MultitenancyTestSuite) HTTPClientLoginWithToken(id, password string, ns uint64, token string) (error, *dgraphtest.HTTPClient) {
	//t := suite.T()

	var err error
	if suite.httpclient == nil {
		suite.httpclient, err = suite.lc.HTTPClient()
	}
	userClient := suite.httpclient

	err = x.RetryUntilSuccess(20, 100*time.Millisecond, func() error {
		return userClient.LoginNSWithToken(id, password, ns, token)
	})

	if err != nil {
		return err, nil
	}

	return err, userClient
}

func (suite *MultitenancyTestSuite) CreateNamespaceWithRetry() (uint64, error) {
	createNs := `mutation {
					 addNamespace
					  {   
					    namespaceId
					    message
					  }   
					}`  

	//params := GraphQLParams{
	//params := dgraphtest.GraphQLParams{
	params := dgraphtest.GraphQLParams{
		Query: createNs,
	}
	//resp, err := dgraphtest.RunAdminQuery(suite.lc, params)
	resp, err := suite.httpclient.RunGraphqlQuery(params, true)
	if err != nil {
		return 0, err 
	}   

	var result struct {
		AddNamespace struct {
			NamespaceId int    `json:"namespaceId"`
			Message     string `json:"message"`
		}   
	}   
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, errors.Wrap(err, "error unmarshalling CreateNamespaceWithRetry() response")
	}   
	if strings.Contains(result.AddNamespace.Message, "Created namespace successfully") {
		return uint64(result.AddNamespace.NamespaceId), nil
	}
	return 0, errors.New(result.AddNamespace.Message)
}

func (suite *MultitenancyTestSuite) DeleteNamespace(nsID uint64) error {
	deleteReq := `mutation deleteNamespace($namespaceId: Int!) {
			deleteNamespace(input: {namespaceId: $namespaceId}){
    		namespaceId
    		message
  		}
	}`

	params := dgraphtest.GraphQLParams{
		Query: deleteReq,
		Variables: map[string]interface{}{
			"namespaceId": nsID,
		},
	}

	resp, err := suite.httpclient.RunGraphqlQuery(params, true)
	if err != nil {
		return err 
	}   

	var result struct {
		DeleteNamespace struct {
			NamespaceId int    `json:"namespaceId"`
			Message     string `json:"message"`
		}
	}
	t := suite.T()
	require.NoError(t, json.Unmarshal(resp, &result))
	require.Equal(t, int(nsID), result.DeleteNamespace.NamespaceId)
	require.Contains(t, result.DeleteNamespace.Message, "Deleted namespace successfully")
	return nil
}

func (suite *MultitenancyTestSuite) AddData() {
		mutation := &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "guy1" .
			_:a <nickname> "RG" .
			_:b <name> "guy2" .
			_:b <nickname> "RG2" .
		`),  
		CommitNow: true,
	}
	_, err := suite.client.NewTxn().Mutate(context.Background(), mutation)
	t := suite.T()
	require.NoError(t, err)
}

func (suite *MultitenancyTestSuite) QueryData(query string) []byte {
	resp, err := suite.client.NewReadOnlyTxn().Query(context.Background(), query)
	t := suite.T()
	require.NoError(t, err)
	return resp.GetJson()
}

func (suite *MultitenancyTestSuite) CreateUser(username, password string) []byte {
	t := suite.T()
	addUser := `
	mutation addUser($name: String!, $pass: String!) {
		addUser(input: [{name: $name, password: $pass}]) {
			user {
				name
			}
		}
	}`

	params := dgraphtest.GraphQLParams{
		Query: addUser,
		Variables: map[string]interface{}{
			"name": username,
			"pass": password,
		},
	}
	//resp, err := dgraphtest.RunAdminQuery(suite.lc, params)
	resp, err := suite.httpclient.RunGraphqlQuery(params, true)
	if err != nil {
		t.Log("Error not NIL")
		return nil
	}

	type Response struct {
		AddUser struct {
			User []struct {
				Name string
			}
		}
	}
	var r Response
	err = json.Unmarshal(resp, &r)
	require.NoError(t, err)
	return resp
}

func (suite *MultitenancyTestSuite) CreateGroup(name string) {
	addGroup := `
	mutation addGroup($name: String!) {
		addGroup(input: [{name: $name}]) {
			group {
				name
			}
		}
	}`

	params := dgraphtest.GraphQLParams{
		Query: addGroup,
		Variables: map[string]interface{}{
			"name": name,
		},
	}
	resp, err := suite.httpclient.RunGraphqlQuery(params, true)
	t := suite.T()
	require.NoError(t, err)

	//resp.RequireNoGraphQLErrors(t)
	type Response struct {
		AddGroup struct {
			Group []struct {
				Name string
			}
		}
	}
	var r Response
	err = json.Unmarshal(resp, &r)
	require.NoError(t, err)
}

func (suite *MultitenancyTestSuite) AddToGroup(userName, group string) {
	addUserToGroup := `mutation updateUser($name: String!, $group: String!) {
		updateUser(input: {
			filter: {
				name: {
					eq: $name
				}
			},
			set: {
				groups: [
					{ name: $group }
				]
			}
		}) {
			user {
				name
				groups {
					name
				}
			}
		}
	}`

	params := dgraphtest.GraphQLParams{
		Query: addUserToGroup,
		Variables: map[string]interface{}{
			"name":  userName,
			"group": group,
		},
	}
	resp, err := suite.httpclient.RunGraphqlQuery(params, true)
	t := suite.T()
	require.NoError(t, err)

	var result struct {
		UpdateUser struct {
			User []struct {
				Name   string
				Groups []struct {
					Name string
				}
			}
			Name string
		}
	}
	err = json.Unmarshal(resp, &result)
	require.NoError(t, err)

	// There should be a user in response.
	require.Len(t, result.UpdateUser.User, 1)
	// User's name must be <userName>
	require.Equal(t, userName, result.UpdateUser.User[0].Name)

	var foundGroup bool
	for _, usr := range result.UpdateUser.User {
		for _, grp := range usr.Groups {
			if grp.Name == group {
				foundGroup = true
				break
			}
		}
	}
	require.True(t, foundGroup)
}

func (suite *MultitenancyTestSuite) AddRulesToGroup(group string, rules []Rule, newGroup bool) {
	addRuleToGroup := `mutation updateGroup($name: String!, $rules: [RuleRef!]!) {
		updateGroup(input: {
			filter: {
				name: {
					eq: $name
				}
			},
			set: {
				rules: $rules
			}
		}) {
			group {
				name
				rules {
					predicate
					permission
				}
			}
		}
	}`

	params := dgraphtest.GraphQLParams{
		Query: addRuleToGroup,
		Variables: map[string]interface{}{
			"name":  group,
			"rules": rules,
		},
	}
	resp, err := suite.httpclient.RunGraphqlQuery(params, true)
	t := suite.T()
	require.NoError(t, err)

	rulesb, err := json.Marshal(rules)
	require.NoError(t, err)
	expectedOutput := fmt.Sprintf(`{
		  "updateGroup": {
			"group": [
			  {
				"name": "%s",
				"rules": %s
			  }
			]
		  }
	  }`, group, rulesb)
	if newGroup {
		testutil.CompareJSON(t, expectedOutput, string(resp))
	}
}

func (suite *MultitenancyTestSuite) ResetPassword(userID, newPass string, nsID uint64) (string, error) {
	resetpasswd := `mutation resetPassword($userID: String!, $newpass: String!, $namespaceId: Int!){
		resetPassword(input: {userId: $userID, password: $newpass, namespace: $namespaceId}) {
		  userId
		  message
		}
	  }`

	params := dgraphtest.GraphQLParams{
		Query: resetpasswd,
		Variables: map[string]interface{}{
			"namespaceId": nsID,
			"userID":      userID,
			"newpass":     newPass,
		},
	}

	resp, err := suite.httpclient.MakeGraphqlRequest(params, true)
	if err != nil {
		return "", err
	}

	var result struct {
		ResetPassword struct {
			UserId  string `json:"userId"`
			Message string `json:"message"`
		}
	}
	t := suite.T()
	require.NoError(t, json.Unmarshal(resp.Data, &result))
	require.Equal(t, userID, result.ResetPassword.UserId)
	require.Contains(t, result.ResetPassword.Message, "Reset password is successful")
	return result.ResetPassword.UserId, nil
}

func TestMultitenancyTestSuite(t *testing.T) {
	suite.Run(t, new(MultitenancyTestSuite))
}
