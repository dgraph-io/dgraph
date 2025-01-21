//go:build integration2

/*
 * Copyright 2025 Hypermode Inc. and Contributors *
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
	"encoding/json"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/hypermodeinc/dgraph/v24/dgraphapi"
	"github.com/hypermodeinc/dgraph/v24/dgraphtest"
	"github.com/hypermodeinc/dgraph/v24/x"

	"github.com/stretchr/testify/require"
)

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
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.GalaxyNamespace))
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
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))

	hc, err = c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.GalaxyNamespace))

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
