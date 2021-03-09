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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func prepare(t *testing.T) {
	dc := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	require.NoError(t, dc.Alter(context.Background(), &api.Operation{DropAll: true}))
}

// TODO(Ahsan): This is just a basic test, for the purpose of development. The functions used in
// this file can me made common to the other acl tests as well. Needs some refactoring as well.
func TestAclBasic(t *testing.T) {
	prepare(t)
	galaxyToken := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})

	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)
	require.Greater(t, int(ns), 0)

	// Add some data to namespace 1
	dc := testutil.DgClientWithLogin(t, "groot", "password", ns)
	testutil.AddData(t, dc)

	query := `
		{
			me(func: has(name)) {
				nickname
				name
			}
		}
	`
	resp := testutil.QueryData(t, dc, query)
	testutil.CompareJSON(t,
		`{"me": [{"name":"guy1","nickname":"RG"},
		{"name": "guy2", "nickname":"RG2"}]}`,
		string(resp))

	// groot of namespace 0 should not see the data of namespace-1
	dc = testutil.DgClientWithLogin(t, "groot", "password", 0)
	resp = testutil.QueryData(t, dc, query)
	testutil.CompareJSON(t, `{"me": []}`, string(resp))

	// Login to namespace 1 via groot and create new user alice.
	token := testutil.Login(t, &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns})
	testutil.CreateUser(t, token, "alice", "newpassword")

	// Alice should not be able to see data added by groot in namespace 1
	dc = testutil.DgClientWithLogin(t, "alice", "newpassword", ns)
	resp = testutil.QueryData(t, dc, query)
	testutil.CompareJSON(t, `{}`, string(resp))

	// Create a new group, add alice to that group and give read access of <name> to dev group.
	testutil.CreateGroup(t, token, "dev")
	testutil.AddToGroup(t, token, "alice", "dev")
	testutil.AddRulesToGroup(t, token, "dev",
		[]testutil.Rule{{Predicate: "name", Permission: acl.Read.Code}})

	// Wait for acl cache to get updated
	time.Sleep(5 * time.Second)

	// Now alice should see the name predicate but not nickname.
	dc = testutil.DgClientWithLogin(t, "alice", "newpassword", ns)
	resp = testutil.QueryData(t, dc, query)
	testutil.CompareJSON(t, `{"me": [{"name":"guy1"},{"name": "guy2"}]}`, string(resp))

}

func TestCreateNamespace(t *testing.T) {
	prepare(t)
	galaxyToken := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})

	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)

	token := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns})

	// Create a new namespace using guardian of other namespace.
	_, err = testutil.CreateNamespaceWithRetry(t, token)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")
}

func TestDeleteNamespace(t *testing.T) {
	prepare(t)
	galaxyToken := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})

	dg := make(map[uint64]*dgo.Dgraph)
	dg[x.GalaxyNamespace] = testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)
	dg[ns] = testutil.DgClientWithLogin(t, "groot", "password", ns)

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
		resp := testutil.QueryData(t, dg[ns], query)
		testutil.CompareJSON(t, expected, string(resp))
	}

	err = addData(x.GalaxyNamespace)
	require.NoError(t, err)
	check(x.GalaxyNamespace, `{"me": [{"name":"0"}]}`)
	err = addData(ns)
	require.NoError(t, err)
	check(ns, fmt.Sprintf(`{"me": [{"name":"%d"}]}`, ns))

	require.NoError(t, testutil.DeleteNamespace(t, galaxyToken, ns))

	err = addData(x.GalaxyNamespace)
	require.NoError(t, err)
	check(x.GalaxyNamespace, `{"me": [{"name":"0"}, {"name":"0"}]}`)
	err = addData(ns)
	require.Contains(t, err.Error(), "Key is using the banned prefix")
	check(ns, `{"me": []}`)

	// No one should be able to delete the default namespace. Not even guardian of galaxy.
	err = testutil.DeleteNamespace(t, galaxyToken, x.GalaxyNamespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot delete default namespace")
}

type liveOpts struct {
	rdfs      string
	schema    string
	gqlSchema string
	creds     *testutil.LoginParams
	forceNs   int64
}

func liveLoadData(t *testing.T, opts *liveOpts) error {
	// Prepare directories.
	dir, err := ioutil.TempDir("", "multi")
	require.NoError(t, err)
	defer func() {
		os.RemoveAll(dir)
	}()
	rdfFile := filepath.Join(dir, "rdfs.rdf")
	require.NoError(t, ioutil.WriteFile(rdfFile, []byte(opts.rdfs), 0644))
	schemaFile := filepath.Join(dir, "schema.txt")
	require.NoError(t, ioutil.WriteFile(schemaFile, []byte(opts.schema), 0644))
	gqlSchemaFile := filepath.Join(dir, "gql_schema.txt")
	require.NoError(t, ioutil.WriteFile(gqlSchemaFile, []byte(opts.gqlSchema), 0644))
	// Load the data.
	return testutil.LiveLoad(testutil.LiveOpts{
		Zero:       testutil.ContainerAddr("zero1", 5080),
		Alpha:      testutil.ContainerAddr("alpha1", 9080),
		RdfFile:    rdfFile,
		SchemaFile: schemaFile,
		Creds:      opts.creds,
		ForceNs:    opts.forceNs,
	})
}

func TestLiveLoadMulti(t *testing.T) {
	prepare(t)
	dc0 := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	galaxyCreds := &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace}
	galaxyToken := testutil.Login(t, galaxyCreds)

	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)
	dc1 := testutil.DgClientWithLogin(t, "groot", "password", ns)

	// Load data.
	require.NoError(t, liveLoadData(t, &liveOpts{
		rdfs: fmt.Sprintf(`
		_:a <name> "galaxy alice" .
		_:b <name> "galaxy bob" .
		_:a <name> "ns alice" <%#x> .
		_:b <name> "ns bob" <%#x> .
`, ns, ns),
		schema: fmt.Sprintf(`
		name: string @index(term) .
		[%#x] name: string .
`, ns),
		creds:   galaxyCreds,
		forceNs: -1,
	}))

	query1 := `
		{
			me(func: has(name)) {
				name
			}
		}
	`
	query2 := `
		{
			me(func: anyofterms(name, "galaxy")) {
				name
			}
		}
	`
	query3 := `
		{
			me(func: anyofterms(name, "ns")) {
				name
			}
		}
	`

	resp := testutil.QueryData(t, dc0, query1)
	testutil.CompareJSON(t,
		`{"me": [{"name":"galaxy alice"}, {"name": "galaxy bob"}]}`, string(resp))
	resp = testutil.QueryData(t, dc1, query1)
	testutil.CompareJSON(t,
		`{"me": [{"name":"ns alice"}, {"name": "ns bob"}]}`, string(resp))

	resp = testutil.QueryData(t, dc0, query2)
	testutil.CompareJSON(t,
		`{"me": [{"name":"galaxy alice"}, {"name": "galaxy bob"}]}`, string(resp))

	_, err = dc1.NewReadOnlyTxn().Query(context.Background(), query3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Attribute name is not indexed")

	// live load data into namespace ns using the guardian of galaxy.
	require.NoError(t, liveLoadData(t, &liveOpts{
		rdfs: fmt.Sprintf(`
		_:a <name> "ns chew" .
		_:b <name> "ns dan" <%#x> .
		_:c <name> "ns eon" <%#x> .
`, ns, 0x100),
		schema: `
		name: string @index(term) .
`,
		creds:   galaxyCreds,
		forceNs: int64(ns),
	}))

	resp = testutil.QueryData(t, dc1, query3)
	testutil.CompareJSON(t,
		`{"me": [{"name":"ns alice"}, {"name": "ns bob"},{"name":"ns chew"},
		{"name": "ns dan"},{"name":"ns eon"}]}`, string(resp))

	// Try loading data into a namespace that does not exist. Expect a failure.
	err = liveLoadData(t, &liveOpts{
		rdfs:   fmt.Sprintf(`_:c <name> "ns eon" <%#x> .`, ns),
		schema: `name: string @index(term) .`,
		creds: &testutil.LoginParams{UserID: "groot", Passwd: "password",
			Namespace: x.GalaxyNamespace},
		forceNs: int64(0x123456), // Assuming this namespace does not exist.
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot load into namespace 0x123456")

	// Try loading into a multiple namespaces.
	err = liveLoadData(t, &liveOpts{
		rdfs:    fmt.Sprintf(`_:c <name> "ns eon" <%#x> .`, ns),
		schema:  `[0x123456] name: string @index(term) .`,
		creds:   galaxyCreds,
		forceNs: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Namespace 0x123456 doesn't exist for pred")

	err = liveLoadData(t, &liveOpts{
		rdfs:    fmt.Sprintf(`_:c <name> "ns eon" <0x123456> .`),
		schema:  `name: string @index(term) .`,
		creds:   galaxyCreds,
		forceNs: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot load nquad")

	// Load data by non-galaxy user.
	err = liveLoadData(t, &liveOpts{
		rdfs: `_:c <name> "ns hola" .`,
		schema: `
		name: string @index(term) .
`,
		creds:   &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns},
		forceNs: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot load into namespace")

	err = liveLoadData(t, &liveOpts{
		rdfs: `_:c <name> "ns hola" .`,
		schema: `
		name: string @index(term) .
`,
		creds:   &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns},
		forceNs: 10,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot load into namespace")

	require.NoError(t, liveLoadData(t, &liveOpts{
		rdfs: fmt.Sprintf(`
		_:a <name> "ns free" .
		_:b <name> "ns gary" <%#x> .
		_:c <name> "ns hola" <%#x> .
`, ns, 0x100),
		schema: `
		name: string @index(term) .
`,
		creds: &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns},
	}))
	resp = testutil.QueryData(t, dc1, query3)
	testutil.CompareJSON(t,
		`{"me": [{"name":"ns alice"}, {"name": "ns bob"},{"name":"ns chew"},
		{"name": "ns dan"},{"name":"ns eon"}, {"name": "ns free"},{"name":"ns gary"},
		{"name": "ns hola"}]}`, string(resp))

}

func TestMain(m *testing.M) {
	fmt.Printf("Using adminEndpoint : %s for multy-tenancy test.\n", testutil.AdminUrl())
	os.Exit(m.Run())
}
