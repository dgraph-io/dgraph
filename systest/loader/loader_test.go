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
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

// TestLoaderXidmap checks that live loader re-uses xidmap on loading data from two different files
func TestLoaderXidmap(t *testing.T) {
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	ctx := context.Background()
	testutil.DropAll(t, dg)
	tmpDir, err := ioutil.TempDir("", "loader_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	data, err := filepath.Abs("testdata/first.rdf.gz")
	require.NoError(t, err)
	liveCmd := exec.Command(testutil.DgraphBinaryPath(), "live",
		"--files", data,
		"--alpha", testutil.SockAddr,
		"--zero", testutil.SockAddrZero,
		"-x", "x",
	)
	liveCmd.Dir = tmpDir
	require.NoError(t, liveCmd.Run())

	// Load another file, live should reuse the xidmap.
	data, err = filepath.Abs("testdata/second.rdf.gz")
	require.NoError(t, err)
	liveCmd = exec.Command(testutil.DgraphBinaryPath(), "live",
		"--files", data,
		"--alpha", testutil.SockAddr,
		"--zero", testutil.SockAddrZero,
		"-x", "x",
	)
	liveCmd.Dir = tmpDir
	liveCmd.Stdout = os.Stdout
	liveCmd.Stderr = os.Stdout
	require.NoError(t, liveCmd.Run())

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
