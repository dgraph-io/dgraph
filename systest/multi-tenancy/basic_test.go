//go:build (!oss && integration) || upgrade

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
	"strings"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/dgraphtest"
	"github.com/dgraph-io/dgraph/v24/ee/acl"
	"github.com/dgraph-io/dgraph/v24/x"
)

const (
	aclQueryTimeout = 5 * time.Second
)

func (msuite *MultitenancyTestSuite) TestAclBasic() {
	t := msuite.T()

	// Galaxy Login
	hcli, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli.AccessJwt, "galaxy token is nil")
	require.NoError(t, err, "login with namespace failed")

	// Create a new namespace
	ns, err := hcli.AddNamespace()
	require.NoError(t, err)
	require.Greater(t, int(ns), 0)

	// Add some data to namespace 1
	gcli, cleanup, err := msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))
	msuite.AddData(gcli)

	// Upgrade
	msuite.Upgrade()

	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))

	query := `{
		me(func: has(name)) {
			nickname
			name
		}
	}`
	resp, err := gcli.Query(query)
	require.NoError(t, err)
	require.NoError(t, dgraphapi.CompareJSON(
		`{"me": [{"name":"guy1","nickname":"RG"},{"name": "guy2", "nickname":"RG2"}]}`, string(resp.Json)))

	// groot of namespace 0 should not see the data of namespace-1
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))
	resp, err = gcli.Query(query)
	require.NoError(t, err)
	require.NoError(t, dgraphapi.CompareJSON(`{"me": []}`, string(resp.Json)))

	// Login to namespace 1 via groot and create new user alice.
	hcli, err = msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns)
	require.NotNil(t, hcli.AccessJwt, "token for the namespace is nil")
	require.NoErrorf(t, err, "login with namespace %d failed", ns)
	_, err = hcli.CreateUser("alice", "newpassword")
	require.NoError(t, err)

	// Alice should not be able to see data added by groot in namespace 1
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), "alice", "newpassword", ns))
	resp, err = gcli.Query(query)
	require.NoError(t, err)
	require.NoError(t, dgraphapi.CompareJSON(`{}`, string(resp.Json)))

	// Create a new group, add alice to that group and give read access of <name> to dev group.
	_, err = hcli.CreateGroup("dev")
	require.NoError(t, err)
	require.NoError(t, hcli.AddUserToGroup("alice", "dev"))
	require.NoError(t, hcli.AddRulesToGroup("dev",
		[]dgraphapi.AclRule{{Predicate: "name", Permission: acl.Read.Code}}, true))

	// Now alice should see the name predicate but not nickname.
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), "alice", "newpassword", ns))
	dgraphapi.PollTillPassOrTimeout(gcli, query, `{"me": [{"name":"guy1"},{"name": "guy2"}]}`, aclQueryTimeout)
}

func (msuite *MultitenancyTestSuite) TestNameSpaceLimitFlag() {
	t := msuite.T()

	// Galaxy login
	hcli, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli.AccessJwt, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Create a new namespace
	ns, err := hcli.AddNamespace()
	require.NoError(t, err)

	// Upgrade
	msuite.Upgrade()

	// Log into namespace
	gcli, cleanup, e := msuite.dc.Client()
	defer cleanup()
	require.NoError(t, e)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))
	require.NoError(t, gcli.SetupSchema(`name: string .`))

	// trying to load more triplets than allowed,It should return error.
	_, err = AddNumberOfTriples(gcli, 1, 53)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Requested UID lease(53) is greater than allowed(50).")

	_, err = AddNumberOfTriples(gcli, 60, 100)
	require.NoError(t, err)

	// we have set uid-lease=50 so we are trying lease more uids,it should return error.
	_, err = AddNumberOfTriples(gcli, 142, 153)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot lease UID because UID lease for the namespace")
}

func (msuite *MultitenancyTestSuite) TestPersistentQuery() {
	t := msuite.T()

	// Galaxy Login
	hcli1, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli1.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli1.AccessJwt, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Create a new namespace
	ns, err := hcli1.AddNamespace()
	require.NoError(t, err)

	// Upgrade
	msuite.Upgrade()

	// Galaxy Login
	hcli1, err = msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli1.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli1.AccessJwt, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Log into ns
	hcli2, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli2.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns)
	require.NotNil(t, hcli2.AccessJwt, "token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

	sch := `type Product {
		productID: ID!
		name: String @search(by: ["term"])
	}`
	require.NoError(t, hcli1.UpdateGQLSchema(sch))
	require.NoError(t, hcli2.UpdateGQLSchema(sch))

	p1 := "query {queryProduct{productID}}"
	sha1 := "7a8ff7a69169371c1eb52a8921387079ca281bb2d55feb4b535cbf0ab3896be5"
	_, err = hcli1.PostPersistentQuery(p1, sha1)
	require.NoError(t, err)

	p2 := "query {queryProduct{name}}"
	sha2 := "0efcdde144167b1046360b73c7f6bec325d9f555099a2ae9b820a13328d270e4"
	_, err = hcli2.PostPersistentQuery(p2, sha2)
	require.NoError(t, err)

	// User cannnot see persistent query from other namespace.
	_, err = hcli1.PostPersistentQuery("", sha2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PersistedQueryNotFound")

	_, err = hcli2.PostPersistentQuery("", sha1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PersistedQueryNotFound")

	hcli3 := &dgraphapi.HTTPClient{HttpToken: &dgraphapi.HttpToken{AccessJwt: ""}}
	_, err = hcli3.PostPersistentQuery("", sha1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported protocol scheme")
}

func (msuite *MultitenancyTestSuite) TestTokenExpired() {
	t := msuite.T()

	// Galaxy Login
	hcli, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli.HttpToken, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	// Create a new namespace
	ns, err := hcli.AddNamespace()
	require.NoError(t, err)

	// Upgrade
	msuite.Upgrade()

	// ns Login
	hcli, err = msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns)
	require.NotNil(t, hcli.HttpToken, "token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

	// Relogin using refresh JWT.
	token := hcli.HttpToken
	err = hcli.LoginUsingToken(ns)
	require.NotNil(t, token, "token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns)

	// Create another namespace
	_, err = hcli.AddNamespace()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")
}

func (msuite *MultitenancyTestSuite) TestTwoPermissionSetsInNameSpacesWithAcl() {
	t := msuite.T()

	// Galaxy Login
	ghcli, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = ghcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, ghcli, "galaxy token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", x.GalaxyNamespace)

	query := `{
		me(func: has(name)) {
			nickname
			name
		}
	}`
	ns1, err := ghcli.AddNamespace()
	require.NoError(t, err)

	// Add data to namespace 1
	gcli, cleanup, e := msuite.dc.Client()
	defer cleanup()
	require.NoError(t, e)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns1))
	msuite.AddData(gcli)

	user1, user2 := "alice", "bob"
	user1passwd, user2passwd := "newpassword", "newpassword"

	// Create user alice in ns1
	hcli, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns1)
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns1)
	_, err = hcli.CreateUser(user1, user1passwd)
	require.NoError(t, err)

	// Create a new group, add alice to that group and give read access to <name> in the dev group.
	msuite.createGroupAndSetPermissions(ns1, "dev", user1, "name")

	// Create second namespace
	ns2, err := ghcli.AddNamespace()
	require.NoError(t, err)

	// Add data to namespace 2
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns2))
	msuite.AddData(gcli)

	// Create user bob
	hcli, err = msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns2)
	require.NoErrorf(t, err, "login with namespace %d failed", ns2)
	_, err = hcli.CreateUser(user2, user2passwd)
	require.NoError(t, err)

	// Create a new group, add bob to that group and give read access of <nickname> to dev group.
	msuite.createGroupAndSetPermissions(ns2, "dev", user2, "nickname")

	// Upgrade
	msuite.Upgrade()

	// Alice should not be able to see <nickname> in namespace 1
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), user1, user1passwd, ns1))
	dgraphapi.PollTillPassOrTimeout(gcli, query, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, aclQueryTimeout)

	// Query via bob and check result
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), user2, user2passwd, ns2))
	require.NoError(t, dgraphapi.PollTillPassOrTimeout(gcli, query, `{}`, aclQueryTimeout))

	// Query namespace-1 via alice and check result to ensure it still works
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), user1, user1passwd, ns1))
	resp, err := gcli.Query(query)
	require.NoError(t, err)
	require.NoError(t, dgraphapi.CompareJSON(`{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, string(resp.Json)))

	// Change permissions in namespace-2
	hcli, err = msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns2)
	require.NoErrorf(t, err, "login as groot into namespace %d failed", ns2)
	require.NoError(t, hcli.AddRulesToGroup("dev",
		[]dgraphapi.AclRule{{Predicate: "name", Permission: acl.Read.Code}}, false))

	// Query namespace-2
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), user2, user2passwd, ns2))
	require.NoError(t, dgraphapi.PollTillPassOrTimeout(gcli, query,
		`{"me": [{"name":"guy2", "nickname": "RG2"}, {"name":"guy1", "nickname": "RG"}]}`, aclQueryTimeout))

	// Query namespace-1
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(), user1, user1passwd, ns1))
	resp, err = gcli.Query(query)
	require.NoError(t, err)
	require.NoError(t, dgraphapi.CompareJSON(`{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, string(resp.Json)))
}

func (msuite *MultitenancyTestSuite) TestCreateNamespace() {
	t := msuite.T()

	// Galaxy Login
	hcli, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli.AccessJwt, "Galaxy token is nil")
	require.NoErrorf(t, err, "login failed")

	// Create a new namespace
	ns, err := hcli.AddNamespace()
	require.NoError(t, err)

	// Upgrade
	msuite.Upgrade()

	// Log into the namespace as groot
	hcli, err = msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns)
	require.NotNil(t, hcli.AccessJwt, "namespace token is nil")
	require.NoErrorf(t, err, "login with namespace %d failed", ns)

	// Create a new namespace using guardian of other namespace.
	_, err = hcli.AddNamespace()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")
}

func (msuite *MultitenancyTestSuite) TestResetPassword() {
	t := msuite.T()

	// Galaxy Login
	hcli1, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli1.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NotNil(t, hcli1.HttpToken, "Galaxy token is nil")
	require.NoErrorf(t, err, "login failed")

	// Create a new namespace
	ns, err := hcli1.AddNamespace()
	require.NoError(t, err)

	// Reset Password
	_, err = hcli1.ResetPassword(dgraphapi.DefaultUser, "newpassword", ns)
	require.NoError(t, err)

	// Upgrade
	msuite.Upgrade()

	// Try and Fail with old password for groot
	hcli2, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli2.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns)
	require.Error(t, err, "expected error because incorrect login")
	require.Empty(t, hcli2.AccessJwt, "nil token because incorrect login")

	// Try and succeed with new password for groot
	hcli3, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli3.LoginIntoNamespace(dgraphapi.DefaultUser, "newpassword", ns)
	require.NoError(t, err, "login failed")
	require.Equal(t, hcli3.Password, "newpassword", "new password matches the reset password")
}

func (msuite *MultitenancyTestSuite) TestDeleteNamespace() {
	t := msuite.T()

	// Galaxy Login
	hcli, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NoErrorf(t, err, "login failed")

	dg := make(map[uint64]*dgraphapi.GrpcClient)
	gcli, cleanup, e := msuite.dc.Client()
	defer cleanup()
	require.NoError(t, e)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))
	dg[x.GalaxyNamespace] = gcli

	// Create a new namespace
	ns, err := hcli.AddNamespace()
	require.NoError(t, err)

	// Log into namespace as groot.
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))
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
		resp, err := dg[ns].Query(query)
		require.NoError(t, err)
		require.NoError(t, dgraphapi.CompareJSON(expected, string(resp.Json)))
	}

	require.NoError(t, addData(x.GalaxyNamespace))
	check(x.GalaxyNamespace, `{"me": [{"name":"0"}]}`)

	require.NoError(t, addData(ns))
	check(ns, fmt.Sprintf(`{"me": [{"name":"%d"}]}`, ns))

	// Upgrade
	msuite.Upgrade()
	dg = make(map[uint64]*dgraphapi.GrpcClient)

	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))
	dg[x.GalaxyNamespace] = gcli

	// Log into namespace as groot
	gcli, cleanup, err = msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))
	dg[ns] = gcli

	// Galaxy Login
	hcli, err = msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NoError(t, err, "login failed")

	// Delete namespace
	nid, err := hcli.DeleteNamespace(ns)
	require.NoError(t, err)
	require.Equal(t, ns, nid)
	require.NoError(t, addData(x.GalaxyNamespace))
	check(x.GalaxyNamespace, `{"me": [{"name":"0"}, {"name":"0"}]}`)
	err = addData(ns)
	require.Contains(t, err.Error(), "Key is using the banned prefix")
	check(ns, `{"me": []}`)

	// No one should be able to delete the default namespace. Not even guardian of galaxy.
	_, err = hcli.DeleteNamespace(x.GalaxyNamespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot delete default namespace")

	// Deleting a non-existent namespace should error out
	// This test is valid after a bug was fixed in the commit 90139243fef645d36f4e571657c4ecbf4548aed5
	supported, err := dgraphtest.IsHigherVersion(msuite.dc.GetVersion(), "90139243fef645d36f4e571657c4ecbf4548aed5")
	require.NoError(t, err)
	if supported {
		_, err = hcli.DeleteNamespace(ns + 5)
		require.Error(t, err)
		require.Contains(t, err.Error(), "error deleting non-existing namespace")
		for i := 0; i < 5; i++ {
			ns, err = hcli.AddNamespace()
			require.NoError(t, err)
		}
	}
}

func (msuite *MultitenancyTestSuite) AddData(gcli *dgraphapi.GrpcClient) {
	rdfs := `
		_:a <name> "guy1" .
		_:a <nickname> "RG" .
		_:b <name> "guy2" .
		_:b <nickname> "RG2" .
	`
	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err := gcli.Mutate(mu)
	require.NoError(msuite.T(), err)
}

func AddNumberOfTriples(gcli *dgraphapi.GrpcClient, start, end int) (*api.Response, error) {
	triples := strings.Builder{}
	for i := start; i <= end; i++ {
		triples.WriteString(fmt.Sprintf("_:person%[1]v <name> \"person%[1]v\" .\n", i))
	}
	mu := &api.Mutation{SetNquads: []byte(triples.String()), CommitNow: true}
	return gcli.Mutate(mu)
}

func (msuite *MultitenancyTestSuite) createGroupAndSetPermissions(namespace uint64, group, user, predicate string) {
	t := msuite.T()
	hcli, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, namespace))
	require.NotNil(t, hcli.AccessJwt, "namespace token is nil")
	require.NoErrorf(t, err, "login as groot into namespace %d failed", namespace)
	_, err = hcli.CreateGroup(group)
	require.NoError(t, err)
	require.NoError(t, hcli.AddUserToGroup(user, group))
	require.NoError(t, hcli.AddRulesToGroup(group,
		[]dgraphapi.AclRule{{Predicate: predicate, Permission: acl.Read.Code}}, true))
}
