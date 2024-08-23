//go:build integration
// +build integration

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
	"os"
	"path/filepath"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/testutil"
	"github.com/dgraph-io/dgraph/v24/x"
)

type liveOpts struct {
	rdfs      string
	schema    string
	gqlSchema string
	creds     *testutil.LoginParams
	forceNs   int64
}

func (msuite *MultitenancyTestSuite) liveLoadData(opts *liveOpts) error {
	t := msuite.T()
	dir := t.TempDir()

	rdfFile := filepath.Join(dir, "rdfs.rdf")
	require.NoError(t, os.WriteFile(rdfFile, []byte(opts.rdfs), 0644))
	schemaFile := filepath.Join(dir, "schema.txt")
	require.NoError(t, os.WriteFile(schemaFile, []byte(opts.schema), 0644))
	gqlSchemaFile := filepath.Join(dir, "gql_schema.txt")
	require.NoError(t, os.WriteFile(gqlSchemaFile, []byte(opts.gqlSchema), 0644))
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

func (msuite *MultitenancyTestSuite) TestLiveLoadMulti() {
	t := msuite.T()
	gcli0, cleanup, err := msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli0.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))

	hcli, err := msuite.dc.HTTPClient()
	require.NoError(t, err)
	err = hcli.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace)
	require.NoError(t, err, "login failed")

	// Create a new namespace
	ns, err := hcli.AddNamespace()
	require.NoError(t, err)
	gcli1, cleanup, err := msuite.dc.Client()
	defer cleanup()
	require.NoError(t, err)
	require.NoError(t, gcli1.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, ns))

	// Load data.
	galaxyCreds := &testutil.LoginParams{UserID: dgraphapi.DefaultUser,
		Passwd: dgraphapi.DefaultPassword, Namespace: x.GalaxyNamespace}
	require.NoError(t, msuite.liveLoadData(&liveOpts{
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

	query1 := `{
		me(func: has(name)) {
			name
		}
	}`
	query2 := `{
		me(func: anyofterms(name, "galaxy")) {
			name
		}
	}`
	query3 := `{
		me(func: anyofterms(name, "ns")) {
			name
		}
	}`

	resp, err := gcli0.Query(query1)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"me": [{"name":"galaxy alice"}, {"name": "galaxy bob"}]}`, string(resp.Json))
	resp, err = gcli1.Query(query1)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"me": [{"name":"ns alice"}, {"name": "ns bob"}]}`, string(resp.Json))

	resp, err = gcli0.Query(query2)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"me": [{"name":"galaxy alice"}, {"name": "galaxy bob"}]}`, string(resp.Json))

	_, err = gcli1.Query(query3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Attribute name is not indexed")

	// live load data into namespace ns using the guardian of galaxy.
	require.NoError(t, msuite.liveLoadData(&liveOpts{
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

	resp, err = gcli1.Query(query3)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"me": [{"name":"ns alice"}, {"name": "ns bob"},{"name":"ns chew"},
		{"name": "ns dan"},{"name":"ns eon"}]}`, string(resp.Json))

	// Try loading data into a namespace that does not exist. Expect a failure.
	err = msuite.liveLoadData(&liveOpts{
		rdfs:    fmt.Sprintf(`_:c <name> "ns eon" <%#x> .`, ns),
		schema:  `name: string @index(term) .`,
		creds:   galaxyCreds,
		forceNs: int64(0x123456), // Assuming this namespace does not exist.
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot load into namespace 0x123456")

	// Try loading into a multiple namespaces.
	err = msuite.liveLoadData(&liveOpts{
		rdfs:    fmt.Sprintf(`_:c <name> "ns eon" <%#x> .`, ns),
		schema:  `[0x123456] name: string @index(term) .`,
		creds:   galaxyCreds,
		forceNs: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Namespace 0x123456 doesn't exist for pred")

	err = msuite.liveLoadData(&liveOpts{
		rdfs:    `_:c <name> "ns eon" <0x123456> .`,
		schema:  `name: string @index(term) .`,
		creds:   galaxyCreds,
		forceNs: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot load nquad")

	// Load data by non-galaxy user.
	err = msuite.liveLoadData(&liveOpts{
		rdfs:    `_:c <name> "ns hola" .`,
		schema:  `name: string @index(term) .`,
		creds:   &testutil.LoginParams{UserID: dgraphapi.DefaultUser, Passwd: dgraphapi.DefaultPassword, Namespace: ns},
		forceNs: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot force namespace")

	err = msuite.liveLoadData(&liveOpts{
		rdfs:    `_:c <name> "ns hola" .`,
		schema:  `name: string @index(term) .`,
		creds:   &testutil.LoginParams{UserID: dgraphapi.DefaultUser, Passwd: dgraphapi.DefaultPassword, Namespace: ns},
		forceNs: 10,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot force namespace")

	require.NoError(t, msuite.liveLoadData(&liveOpts{
		rdfs: fmt.Sprintf(`
			_:a <name> "ns free" .
			_:b <name> "ns gary" <%#x> .
			_:c <name> "ns hola" <%#x> .`, ns, 0x100),
		schema: `name: string @index(term) .`,
		creds:  &testutil.LoginParams{UserID: dgraphapi.DefaultUser, Passwd: dgraphapi.DefaultPassword, Namespace: ns},
	}))

	resp, err = gcli1.Query(query3)
	require.NoError(t, err)
	testutil.CompareJSON(t, `{"me": [{"name":"ns alice"}, {"name": "ns bob"},{"name":"ns chew"},
		{"name": "ns dan"},{"name":"ns eon"}, {"name": "ns free"},{"name":"ns gary"},
		{"name": "ns hola"}]}`, string(resp.Json))
}
