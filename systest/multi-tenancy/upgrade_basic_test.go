//go:build integration || upgrade

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
	//"net/http"
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
	//"github.com/dgraph-io/dgraph/graphql/e2e/common"
	//"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

type Rule struct {
	Predicate  string `json:"predicate"`
	Permission int32  `json:"permission"`
}

var uTable = []struct{
		srcVer string
		dstVer string
}{
	{"0c9f60156", "v22.0.2"},
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
	require.NoError(t, x.RetryUntilSuccess(50, 100*time.Millisecond, func() error {
		return suite.client.Dgraph.LoginIntoNamespace(context.Background(), "groot", "password", x.GalaxyNamespace) 
	})) 
	require.NoError(t, suite.client.Dgraph.Alter(context.Background(), &api.Operation{DropAll: true}))
}

var timeout = 5 * time.Second

func (suite *MultitenancyTestSuite) setupSourceDB(srcDB string) {
	os.Setenv("GOOS", "linux")
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(3).WithReplicas(3).
		WithACL(300 * time.Second).WithEncryption().WithVersion(srcDB)

	var err error
	suite.lc, err = dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	suite.lc.Start()

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
		suite.setupSourceDB(uE.srcVer)

		suite.prepare()

		var err error
		t := suite.T()
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
		//fmt.Printf("\nresp1 = %+v\n\n", string(resp))
		testutil.CompareJSON(t,
			`{"me": [{"name":"guy1","nickname":"RG"},
			{"name": "guy2", "nickname":"RG2"}]}`,
			string(resp))

		// groot of namespace 0 should not see the data of namespace-1
		suite.client = suite.DgClientWithLogin("groot", "password", 0)
		resp = suite.QueryData(query)
		//fmt.Printf("\nresp2 = %+v\n\n", string(resp))
		testutil.CompareJSON(t, `{"me": []}`, string(resp))

		// Login to namespace 1 via groot and create new user alice.
		err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", ns)
		require.NotNil(t, suite.httpclient, "token for the namespace is nil")
		require.NoErrorf(t, err, "login with namespace %d failed", ns)
		suite.CreateUser("alice", "newpassword")

		// Alice should not be able to see data added by groot in namespace 1
		suite.client = suite.DgClientWithLogin("alice", "newpassword", ns)
		resp = suite.QueryData(query)
		//fmt.Printf("\nresp3 = %+v\n\n", string(resp))
		testutil.CompareJSON(t, `{}`, string(resp))

		// Create a new group, add alice to that group and give read access of <name> to dev group.
		suite.CreateGroup("dev")
		suite.AddToGroup("alice", "dev")
		suite.AddRulesToGroup("dev",
			[]Rule{{Predicate: "name", Permission: acl.Read.Code}}, true)

		// Now alice should see the name predicate but not nickname.
		suite.client = suite.DgClientWithLogin("alice", "newpassword", ns)
		testutil.PollTillPassOrTimeout(t, suite.client.Dgraph, query, `{"me": [{"name":"guy1"},{"name": "guy2"}]}`, timeout)

		suite.UpgradeAndDoAclBasic(ns, uE.dstVer)

		suite.cleanup_client_conns()
		suite.lc.Cleanup()
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndSetupClient(dstDB string) {
	var err error
	t := suite.T()

	if err := suite.lc.Upgrade(suite.httpclient, dstDB, dgraphtest.StopStart); err != nil {
		t.Fatal(err)
	}

	var cleanup_client_conns func()
	suite.client, cleanup_client_conns, err = suite.lc.Client()
	suite.cleanup_client_conns = cleanup_client_conns
	x.Panic(err)

	suite.httpclient, err = suite.lc.HTTPClient()
	x.Panic(err)
}

func (suite *MultitenancyTestSuite) UpgradeAndDoAclBasic(ns uint64, dstDB string) {
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
	//fmt.Printf("\nresp1 = %+v\n\n", string(resp))
	testutil.CompareJSON(t,
		`{"me": [{"name":"guy1","nickname":"RG"},
		{"name": "guy2", "nickname":"RG2"}]}`,
		string(resp))

	// groot of namespace 0 should not see the data of namespace-1
	suite.client = suite.DgClientWithLogin("groot", "password", 0)
	resp = suite.QueryData(query)
	//fmt.Printf("\nresp2 = %+v\n\n", string(resp))
	testutil.CompareJSON(t, `{"me": []}`, string(resp))

	// alice already a member of 'dev' should see the name predicate but not nickname,
	// same as before the upgrade.
	suite.client = suite.DgClientWithLogin("alice", "newpassword", ns)
	testutil.PollTillPassOrTimeout(t, suite.client.Dgraph, query, `{"me": [{"name":"guy1"},{"name": "guy2"}]}`, timeout)
}

func (suite *MultitenancyTestSuite) TestCreateNamespace() {
	for _, uE := range uTable {
		suite.setupSourceDB(uE.srcVer)

		suite.prepare()
		t := suite.T()

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

		suite.UpgradeAndCheckCreateNamespace(ns, uE.dstVer)

		suite.cleanup_client_conns()
		suite.lc.Cleanup()
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndCheckCreateNamespace(ns uint64, dstDB string) {
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
	
func (suite *MultitenancyTestSuite) TestDeleteNamespace() {
	for _, uE := range uTable {
		suite.setupSourceDB(uE.srcVer)

		t := suite.T()
		suite.prepare()

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

		//suite.UpgradeAndCheckDeleteNamespace(uE.dstVer)

		suite.cleanup_client_conns()
		suite.lc.Cleanup()
	}
}

func (suite *MultitenancyTestSuite) UpgradeAndCheckDeleteNamespace(dstDB string) {
	suite.UpgradeAndSetupClient(dstDB)

	t := suite.T()
	suite.prepare()

	var err error
	err, suite.httpclient = suite.HTTPClientWithLogin("groot", "password", x.GalaxyNamespace)
	require.NoErrorf(t, err, "login failed")

	// Check the Delete Namespace in the upgraded version identical to as done in the source version.

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
	t.Logf("DgClientWithLogin(): id = %+v\tpassword = %+v\tns = %+v\tuserClient.Dgraph = %+v", id, password, ns, userClient.Dgraph)

	return userClient
}

func (suite *MultitenancyTestSuite) HTTPClientWithLogin(id, password string, ns uint64) (error, *dgraphtest.HTTPClient) {
	t := suite.T()
	userClient := suite.httpclient

	err := x.RetryUntilSuccess(20, 100*time.Millisecond, func() error {
		return userClient.LoginIntoNamespace(id, password, ns) 
	})
	t.Logf("HTTPClientWithLogin(): id = %+v\tpassword = %+v\tns = %+v\tHttpToken = %+v", id, password, ns, userClient.HttpToken)

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
	//t.Logf("RESPONSE = %+v", r)
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

func TestMultitenancyTestSuite(t *testing.T) {
	suite.Run(t, new(MultitenancyTestSuite))
}
