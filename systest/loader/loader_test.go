/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

	"github.com/spf13/viper"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

// TestLoaderXidmap checks that live loader re-uses xidmap on loading data from two different files
func TestLoaderXidmap(t *testing.T) {
	conf := viper.GetViper()
	conf.Set("tls", fmt.Sprintf("ca-cert=%s; server-name=%s; internal-port=%v;",
		// ca-cert
		"../../tlstest/mtls_internal/tls/live/ca.crt",
		// server-name
		"alpha1",
		// internal-port
		true))

	dg, err := testutil.DgraphClientWithCerts(testutil.SockAddr, conf)
	require.NoError(t, err)
	ctx := context.Background()
	testutil.DropAll(t, dg)
	tmpDir, err := ioutil.TempDir("", "loader_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	data, err := filepath.Abs("testdata/first.rdf.gz")
	require.NoError(t, err)

	tlsDir, err := filepath.Abs("../../tlstest/mtls_internal/tls/live")
	require.NoError(t, err)

	tlsFlag := fmt.Sprintf(
		`ca-cert=%s; internal-port=%v; client-cert=%s; client-key=%s; server-name=%s;`,
		// ca-cert
		tlsDir+"/ca.crt",
		// internal-port
		true,
		// client-cert
		tlsDir+"/client.liveclient.crt",
		// client-key
		tlsDir+"/client.liveclient.key",
		// server-name
		"alpha1")

	err = testutil.ExecWithOpts([]string{testutil.DgraphBinaryPath(), "live",
		"--tls", tlsFlag,
		"--files", data,
		"--alpha", testutil.SockAddr,
		"--zero", testutil.SockAddrZero,
		"--force-namespace", "0",
		"-x", "x"}, testutil.CmdOpts{Dir: tmpDir})
	require.NoError(t, err)

	// Load another file, live should reuse the xidmap.
	data, err = filepath.Abs("testdata/second.rdf.gz")
	require.NoError(t, err)
	err = testutil.ExecWithOpts([]string{testutil.DgraphBinaryPath(), "live",
		"--tls", tlsFlag,
		"--files", data,
		"--alpha", testutil.SockAddr,
		"--zero", testutil.SockAddrZero,
		"--force-namespace", "0",
		"-x", "x"}, testutil.CmdOpts{Dir: tmpDir})
	require.NoError(t, err)

	op := api.Operation{Schema: "name: string @index(exact) ."}
	x.Check(dg.Alter(ctx, &op))

	query := `
	{
		q(func: eq(name, "Alice")) {
			age
			location
			friend{
				name
			}
		}
	}`
	expected := `{"q":[{"age":"13","location":"Wonderland","friend":[{"name":"Bob"}]}]}`

	resp, err := dg.NewReadOnlyTxn().Query(ctx, query)
	require.NoError(t, err)
	testutil.CompareJSON(t, expected, string(resp.GetJson()))

}
