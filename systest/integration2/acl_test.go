//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/buildvars"
	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/x"

	"github.com/stretchr/testify/require"
)

// skipIfFIPS skips the current test when buildvars.FIPSEnabled is true. Upgrade-path
// tests pin a pre-FIPS upstream version for the "old" binary; that
// version predates any FIPS-enforcing toolchain, so attempting to build
// it under a FIPS configuration either fails outright or produces a
// binary that refuses to start. Semantically valid in non-FIPS builds.
func skipIfFIPS(t *testing.T) {
	if buildvars.FIPSEnabled {
		t.Skip("upgrade-path test pins a pre-FIPS upstream version; skipping under FIPS build")
	}
}

type S struct {
	Predicate string   `json:"predicate"`
	Type      string   `json:"type"`
	Index     bool     `json:"index"`
	Tokenizer []string `json:"tokenizer"`
	Unique    bool     `json:"unique"`
}

type Received struct {
	Schema []S `json:"schema"`
}

func testDuplicateUserUpgradeStrat(t *testing.T, strat dgraphtest.UpgradeStrategy) {
	skipIfFIPS(t)
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).
		WithReplicas(1).WithACL(time.Hour).WithVersion("v23.0.1")
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))
	require.NoError(t, gc.SetupSchema(`name: string  .`))

	rdfs := `
	_:a <name> "alice" .
	_:b <name> "bob" .
	_:c <name> "sagar" .
	_:d <name> "ajay" .`
	_, err = gc.Mutate(&api.Mutation{SetNquads: []byte(rdfs), CommitNow: true})
	require.NoError(t, c.Upgrade("local", strat))
	gc, cleanup, err = c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err = c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	query := "schema {}"
	resp, err := gc.Query(query)
	require.NoError(t, err)

	var received Received
	require.NoError(t, json.Unmarshal([]byte(resp.Json), &received))
	for _, s := range received.Schema {
		if s.Predicate == "dgraph.xid" {
			require.True(t, s.Unique)
		}
	}

	query = `{
		q(func: has(name)) {
		    count(uid)
		}
	}`
	resp, err = gc.Query(query)
	require.NoError(t, err)
	require.Contains(t, string(resp.Json), `"count":4`)
}

func TestDuplicateUserWithLiveLoader(t *testing.T) {
	testDuplicateUserUpgradeStrat(t, dgraphtest.ExportImport)
}

func TestDuplicateUserWithBackupRestore(t *testing.T) {
	testDuplicateUserUpgradeStrat(t, dgraphtest.BackupRestore)
}

func TestDuplicateUserWithInPlace(t *testing.T) {
	testDuplicateUserUpgradeStrat(t, dgraphtest.InPlace)
}
