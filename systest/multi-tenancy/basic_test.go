//go:build (!oss && integration) || upgrade
// +build !oss,integration upgrade

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
	"time"
	"strings"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgo/v230"
	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

const timeout = 5 * time.Second

type inputTripletsCount struct {
	lowerLimit int
	upperLimit int
}

// TODO(Ahsan): This is just a basic test, for the purpose of development. The functions used in
// this file can me made common to the other acl tests as well. Needs some refactoring as well.
func (suite *MultitenancyTestSuite) TestAclBasic() {
	t := suite.T()

	// Galaxy Login
	hcli, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", x.GalaxyNamespace)
	require.NotNil(t, hcli, "galaxy token is nil")
	require.NoError(t, err, "login with namespace failed")

	// Create a new namespace
	var ns uint64
	ns, err = hcli.CreateNamespaceWithRetry()
	require.NoError(t, err)
	require.Greater(t, int(ns), 0)

	// Add some data to namespace 1
	gcli, cu, e := suite.dc.Client()
	suite.cleanup = cu
	require.NoError(t, e)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), "groot", "password", ns))
	suite.AddData(gcli)

	// Upgrade
	suite.Upgrade(dgraphtest.BackupRestore)

	query := `
		{
			me(func: has(name)) {
				nickname
				name
			}
		}
	`
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), "groot", "password", ns))
	resp := suite.QueryData(gcli, query)
	testutil.CompareJSON(t,
		`{"me": [{"name":"guy1","nickname":"RG"},
		{"name": "guy2", "nickname":"RG2"}]}`,
		string(resp))

	// groot of namespace 0 should not see the data of namespace-1
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), "groot", "password", 0))
	resp = suite.QueryData(gcli, query)
	testutil.CompareJSON(t, `{"me": []}`, string(resp))

	// Login to namespace 1 via groot and create new user alice.
	hcli, err = suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", ns)
	require.NotNil(t, hcli, "token for the namespace is nil")
	require.NoErrorf(t, err, "login with namespace %d failed", ns)
	_, err = hcli.CreateUser("alice", "newpassword")
	require.NoError(t, err)

	// Alice should not be able to see data added by groot in namespace 1
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), "alice", "newpassword", ns))
	resp = suite.QueryData(gcli, query)
	testutil.CompareJSON(t, `{}`, string(resp))

	// Create a new group, add alice to that group and give read access of <name> to dev group.
	require.NoError(t, hcli.CreateGroup("dev"))
	require.NoError(t, hcli.AddToGroup("alice", "dev"))
	var expectedOutput, actual string
	expectedOutput, actual, err = hcli.AddRulesToGroup("dev",
		[]dgraphtest.Rule{{Predicate: "name", Permission: acl.Read.Code}}, true)
	require.NoError(t, err)
	testutil.CompareJSON(t, expectedOutput, actual)

	// Now alice should see the name predicate but not nickname.
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), "alice", "newpassword", ns))
	testutil.PollTillPassOrTimeout(t, gcli.Dgraph, query, `{"me": [{"name":"guy1"},{"name": "guy2"}]}`, timeout)
}

func (suite *MultitenancyTestSuite) TestNameSpaceLimitFlag() {
	t := suite.T()

	testInputs := []inputTripletsCount{{1, 53}, {60, 100}, {141, 153}}

	// Galaxy login
	hcli, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", x.GalaxyNamespace)
	require.NotNil(t, hcli, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Create a new namespace
	ns, err := hcli.CreateNamespaceWithRetry()
	require.NoError(t, err)

	// Upgrade
	suite.Upgrade(dgraphtest.BackupRestore)

	// Log into namespace
	gcli, cu, e := suite.dc.Client()
	suite.cleanup = cu
	require.NoError(t, e)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), "groot", "password", ns))
	require.NoError(t, gcli.Alter(context.Background(),
		&api.Operation{Schema: `name: string .`}))

	// trying to load more triplets than allowed,It should return error.
	_, err = AddNumberOfTriples(gcli.Dgraph, testInputs[0].lowerLimit, testInputs[0].upperLimit)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Requested UID lease(53) is greater than allowed(50).")

	_, err = AddNumberOfTriples(gcli.Dgraph, testInputs[1].lowerLimit, testInputs[1].upperLimit)
	require.NoError(t, err)

	// we have set uid-lease=50 so we are trying lease more uids,it should return error.
	_, err = AddNumberOfTriples(gcli.Dgraph, testInputs[2].lowerLimit, testInputs[2].upperLimit)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot lease UID because UID lease for the namespace")
}

func AddNumberOfTriples(dg *dgo.Dgraph, start, end int) (*api.Response, error) {
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

func postPersistentQuery(hcli *dgraphtest.HTTPClient, query, sha string) ([]byte, error) {
	params := dgraphtest.GraphQLParams{
		Query: query,
		Extensions: &schema.RequestExtensions{PersistedQuery: schema.PersistedQuery{
			Sha256Hash: sha,
		}},
	}
	return hcli.RunGraphqlQuery(params, false)
}

func (suite *MultitenancyTestSuite) TestPersistentQuery() {
	t := suite.T()

	// Galaxy Login
	hcli1, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli1.LoginIntoNamespace("groot", "password", x.GalaxyNamespace)
	require.NotNil(t, hcli1, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Make a copy of the galaxy token
	galaxyToken := hcli1.HttpToken
	require.NotNil(t, galaxyToken, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Create a new namespace
	ns, err := hcli1.CreateNamespaceWithRetry()
	require.NoError(t, err)

	// Upgrade
	suite.Upgrade(dgraphtest.BackupRestore)

	// Log into ns
	hcli2, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli2.LoginIntoNamespace("groot", "password", ns)
	// Make a token copy
	nsToken := hcli2.HttpToken
	require.NotNil(t, nsToken, "Token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

	sch := `type Product {
			productID: ID!
			name: String @search(by: [term])
		}`
	suite.postGqlSchema(sch, galaxyToken.AccessJwt)
	suite.postGqlSchema(sch, nsToken.AccessJwt)

	p1 := "query {queryProduct{productID}}"
	sha1 := "7a8ff7a69169371c1eb52a8921387079ca281bb2d55feb4b535cbf0ab3896be5"
	_, err = postPersistentQuery(hcli1, p1, sha1)
	require.NoError(t, err)

	p2 := "query {queryProduct{name}}"
	sha2 := "0efcdde144167b1046360b73c7f6bec325d9f555099a2ae9b820a13328d270e4"
	_, err = postPersistentQuery(hcli2, p2, sha2)
	require.NoError(t, err)

	// User cannnot see persistent query from other namespace.
	_, err = postPersistentQuery(hcli1, "", sha2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PersistedQueryNotFound")

	_, err = postPersistentQuery(hcli2, "", sha1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PersistedQueryNotFound")

	hcli3 := &dgraphtest.HTTPClient {
			HttpToken: &dgraphtest.HttpToken{AccessJwt: ""},
	}
	_, err = postPersistentQuery(hcli3, "", sha1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported protocol scheme")
}

func (suite *MultitenancyTestSuite) TestTokenExpired() {
	t := suite.T()

	// Galaxy Login
	hcli, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", x.GalaxyNamespace)
	require.NotNil(t, hcli.HttpToken, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Create a new namespace
	ns, err := hcli.CreateNamespaceWithRetry()
	require.NoError(t, err)

	// Upgrade
	suite.Upgrade(dgraphtest.BackupRestore)

	// ns Login
	hcli, err = suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", ns)
	require.NotNil(t, hcli.HttpToken, "token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

	// Relogin using refresh JWT.
	token := hcli.HttpToken
	err = hcli.LoginUsingToken(ns)
	require.NotNil(t, token, "token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

	// Create another namespace
	_, err = hcli.CreateNamespaceWithRetry()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")
}

func (suite *MultitenancyTestSuite) createGroupAndSetPermissions(namespace uint64, group, user, predicate string) {
	t := suite.T()
	hcli, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", namespace)
	require.NotNil(t, hcli, "namespace token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", namespace)
	require.NoError(t, hcli.CreateGroup(group))
	require.NoError(t, hcli.AddToGroup(user, group))
	var expectedOutput, actual string
	expectedOutput, actual, err = hcli.AddRulesToGroup(group,
		[]dgraphtest.Rule{{Predicate: predicate, Permission: acl.Read.Code}}, true)
	require.NoError(t, err)
	testutil.CompareJSON(t, expectedOutput, actual)
}

func (suite *MultitenancyTestSuite) TestTwoPermissionSetsInNameSpacesWithAcl() {
	t := suite.T()

	// Galaxy Login
	ghcli, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = ghcli.LoginIntoNamespace("groot", "password", x.GalaxyNamespace)
	require.NotNil(t, ghcli, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	query := `
		{
			me(func: has(name)) {
				nickname
				name
			}
		}
	`
	// Create first namespace
	ns1, err := ghcli.CreateNamespaceWithRetry()
	require.NoError(t, err)

	// Add data to namespace 1
	gcli, cu, e := suite.dc.Client()
	suite.cleanup = cu
	require.NoError(t, e)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), "groot", "password", ns1))
	suite.AddData(gcli)

	user1, user2 := "alice", "bob"
	user1passwd, user2passwd := "newpassword", "newpassword"

	// Create user alice in ns1
	hcli, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", ns1)
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns1)
	_, err = hcli.CreateUser(user1, user1passwd)
	require.NoError(t, err)

	// Create a new group, add alice to that group and give read access to <name> in the dev group.
	suite.createGroupAndSetPermissions(ns1, "dev", user1, "name")

	// Create second namespace
	ns2, err := ghcli.CreateNamespaceWithRetry()
	require.NoError(t, err)

	// Add data to namespace 2
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	_ = gcli.LoginIntoNamespace(context.Background(), "groot", "password", ns2)
	suite.AddData(gcli)

	// Create user bob
	hcli, err = suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", ns2)
	require.NoErrorf(t, err, "login with namespace %d failed", ns2)
	_, err = hcli.CreateUser(user2, user2passwd)
	require.NoError(t, err)

	// Create a new group, add bob to that group and give read access of <nickname> to dev group.
	suite.createGroupAndSetPermissions(ns2, "dev", user2, "nickname")

	// Upgrade
	suite.Upgrade(dgraphtest.BackupRestore)

	// Alice should not be able to see <nickname> in namespace 1
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	_ = gcli.LoginIntoNamespace(context.Background(), user1, user1passwd, ns1)
	testutil.PollTillPassOrTimeout(t, gcli.Dgraph, query, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, timeout)

	// Query via bob and check result
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	_ = gcli.LoginIntoNamespace(context.Background(), user2, user2passwd, ns2)
	testutil.PollTillPassOrTimeout(t, gcli.Dgraph, query, `{}`, timeout)

	// Query namespace-1 via alice and check result to ensure it still works
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	_ = gcli.LoginIntoNamespace(context.Background(), user1, user1passwd, ns1)
	resp := suite.QueryData(gcli, query)
	testutil.CompareJSON(t, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, string(resp))

	// Change permissions in namespace-2
	hcli, err = suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", ns2)
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns2)
	_, _, err = hcli.AddRulesToGroup("dev",
		[]dgraphtest.Rule{{Predicate: "name", Permission: acl.Read.Code}}, false)
	require.NoError(t, err)

	// Query namespace-2
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	_ = gcli.LoginIntoNamespace(context.Background(), user2, user2passwd, ns2)
	testutil.PollTillPassOrTimeout(t, gcli.Dgraph, query,
		`{"me": [{"name":"guy2", "nickname": "RG2"}, {"name":"guy1", "nickname": "RG"}]}`, timeout)

	// Query namespace-1
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	_ = gcli.LoginIntoNamespace(context.Background(), user1, user1passwd, ns1)
	resp = suite.QueryData(gcli, query)
	testutil.CompareJSON(t, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, string(resp))
}

func (suite *MultitenancyTestSuite) TestCreateNamespace() {
	t := suite.T()

	// Galaxy Login
	hcli, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", x.GalaxyNamespace)
	require.NotNil(t, hcli, "Galaxy token is nil")
	require.NoErrorf(t, err, "login failed")

	// Create a new namespace
	ns, err := hcli.CreateNamespaceWithRetry()
	require.NoError(t, err)

	// Upgrade
	suite.Upgrade(dgraphtest.BackupRestore)

	// Log into the namespace as groot
	hcli, err = suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", ns)
	require.NotNil(t, hcli, "namespace token is nil")
	require.NoErrorf(t, err, "login with namespace %d failed", ns)

	// Create a new namespace using guardian of other namespace.
	_, err = hcli.CreateNamespaceWithRetry()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")
}

func (suite *MultitenancyTestSuite) TestResetPassword() {
	t := suite.T()

	// Galaxy Login
	hcli1, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli1.LoginIntoNamespace("groot", "password", x.GalaxyNamespace)
	require.NotNil(t, hcli1.HttpToken, "Galaxy token is nil")
	require.NoErrorf(t, err, "login failed")

	// Create a new namespace
	ns, err := hcli1.CreateNamespaceWithRetry()
	require.NoError(t, err)

	// Reset Password
	_, err = hcli1.ResetPassword("groot", "newpassword", ns)
	require.NoError(t, err)

	// Upgrade
	suite.Upgrade(dgraphtest.BackupRestore)

	// Try and Fail with old password for groot
	hcli2, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli2.LoginIntoNamespace("groot", "password", ns)
	require.Error(t, err, "expected error because incorrect login")
	require.Nil(t, hcli2, "nil token because incorrect login")

	// Try and succeed with new password for groot
	hcli3, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli3.LoginIntoNamespace("groot", "newpassword", ns)
	require.NoError(t, err, "login failed")
	require.Equal(t, hcli3.HttpToken.Password, "newpassword", "new password matches the reset password")
}

func (suite *MultitenancyTestSuite) TestDeleteNamespace() {
	t := suite.T()

	// Galaxy Login
	hcli, err := suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", x.GalaxyNamespace)
	require.NoErrorf(t, err, "login failed")

	dg := make(map[uint64]*dgraphtest.GrpcClient)
	gcli, cu, e := suite.dc.Client()
	suite.cleanup = cu
	require.NoError(t, e)
	_ = gcli.LoginIntoNamespace(context.Background(), "groot", "password", x.GalaxyNamespace)
	dg[x.GalaxyNamespace] = gcli

	// Create a new namespace
	ns, err := hcli.CreateNamespaceWithRetry()
	require.NoError(t, err)

	// Log into namespace as groot.
	gcli, suite.cleanup, err = suite.dc.Client()
	require.NoError(t, err)
	_ = gcli.LoginIntoNamespace(context.Background(), "groot", "password", ns)
	dg[ns] = gcli

	addData := func(ns uint64) error {
		mutation := &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`
			_:a <name> "%d" .
		`, ns)),
			CommitNow: true,
		}
		_, err := dg[ns].NewTxn().Mutate(context.Background(), mutation)
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
		resp := suite.QueryData(dg[ns], query)
		testutil.CompareJSON(t, expected, string(resp))
	}

	require.NoError(t, addData(x.GalaxyNamespace))
	check(x.GalaxyNamespace, `{"me": [{"name":"0"}]}`)

	require.NoError(t, addData(ns))
	check(ns, fmt.Sprintf(`{"me": [{"name":"%d"}]}`, ns))

	// Upgrade
	suite.Upgrade(dgraphtest.BackupRestore)

	// Galaxy Login
	hcli, err = suite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace("groot", "password", x.GalaxyNamespace)
	require.NoError(t, err, "login failed")

	// Delete namespace
	r, err := hcli.DeleteNamespace(ns)
	require.NoError(t, err)
	require.Equal(t, int(ns), r.DeleteNamespace.NamespaceId)
	require.Contains(t, r.DeleteNamespace.Message, "Deleted namespace successfully")
	require.NoError(t, addData(x.GalaxyNamespace))
	check(x.GalaxyNamespace, `{"me": [{"name":"0"}, {"name":"0"}]}`)
	err = addData(ns)
	require.Contains(t, err.Error(), "Key is using the banned prefix")
	check(ns, `{"me": []}`)

	// No one should be able to delete the default namespace. Not even guardian of galaxy.
	_, err = hcli.DeleteNamespace(x.GalaxyNamespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot delete default namespace")
}

func (suite *MultitenancyTestSuite) AddData(gcli *dgraphtest.GrpcClient) {
		mutation := &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "guy1" .
			_:a <nickname> "RG" .
			_:b <name> "guy2" .
			_:b <nickname> "RG2" .
		`),  
		CommitNow: true,
	}
	_, err := gcli.NewTxn().Mutate(context.Background(), mutation)
	t := suite.T()
	require.NoError(t, err)
}

func (suite *MultitenancyTestSuite) QueryData(gcli *dgraphtest.GrpcClient, query string) []byte {
	resp, err := gcli.NewReadOnlyTxn().Query(context.Background(), query)
	t := suite.T()
	require.NoError(t, err)
	return resp.GetJson()
}
