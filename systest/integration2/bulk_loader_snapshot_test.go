//go:build integration2

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors *
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
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

const (
	personSchema = `
		type Person {
    	    name
    	}
	
		name: string @index(fulltext) .
		age : int @index(int) .
		friend: [uid] .
	`

	rdfData = `
	_:a <name> "Alice" .
	_:a <age> "10" .
	_:a <dgraph.type> "Person" .
		
	_:b <name> "Bob" .
	_:b <age> "10" .
	_:b <dgraph.type> "Person" .

	_:c <name> "Charlie" .
	_:c <age> "10" .
	_:c <dgraph.type> "Person" .

	_:d <name> "Dave" .
	_:d <age> "10" .
	_:d <dgraph.type> "Person" .

	_:a <friend> _:c .
	_:a <friend> _:b .
	_:c <friend> _:d .
	_:c <friend> _:d .
	`

	requestTimeout = 120 * time.Second
)

func queryAlphaWith(t *testing.T, query string, client *dgraphtest.GrpcClient) *api.Response {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	require.NoError(t, client.LoginIntoNamespace(ctx, dgraphtest.DefaultUser,
		dgraphtest.DefaultPassword, x.GalaxyNamespace))
	resp, err := client.Query(query)
	require.NoError(t, err, "Error while querying data")
	return resp
}

func TestBulkLoaderSnapshot(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(1).WithReplicas(3).
		WithBulkLoadOutDir(t.TempDir()).WithACL(time.Hour).WithVerbosity(2)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()

	// start zero
	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(true))

	baseDir := t.TempDir()
	dqlSchemaFile := filepath.Join(baseDir, "person.schema")
	require.NoError(t, ioutil.WriteFile(dqlSchemaFile, []byte(personSchema), os.ModePerm))
	dataFile := filepath.Join(baseDir, "person.rdf")
	require.NoError(t, ioutil.WriteFile(dataFile, []byte(rdfData), os.ModePerm))

	opts := dgraphtest.BulkOpts{
		DataFiles:   []string{dataFile},
		SchemaFiles: []string{dqlSchemaFile},
	}
	require.NoError(t, c.BulkLoad(opts))

	// start Alpha 0
	require.NoError(t, c.StartAlpha(0))
	require.NoError(t, c.HealthCheckAlpha(0))

	// get gRPC client
	gc, cleanup, err := c.ClientForAlpha(0)
	require.NoError(t, err)
	defer cleanup()

	// run some queries and ensure everything looks good
	query := `{
		q1(func: type(Person)){
			name
		}
	}`
	resp := queryAlphaWith(t, query, gc)
	testutil.CompareJSON(
		t,
		`{"q1": [{"name": "Dave"},{"name": "Alice"},{"name": "Charlie"},{"name": "Bob"}]}`,
		string(resp.GetJson()),
	)

	// start Alpha 1
	require.NoError(t, c.StartAlpha(1))
	require.NoError(t, c.HealthCheckAlpha(1))

	// get gRPC client
	gc, cleanup, err = c.ClientForAlpha(1)
	require.NoError(t, err)
	defer cleanup()

	resp = queryAlphaWith(t, query, gc)

	testutil.CompareJSON(
		t,
		`{"q1": [{"name": "Dave"},{"name": "Alice"},{"name": "Charlie"},{"name": "Bob"}]}`,
		string(resp.GetJson()),
	)

}
