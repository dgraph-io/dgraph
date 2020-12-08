/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package common

import (
	"context"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
)

var rootDir = "./data"

type suite struct {
	t    *testing.T
	opts suiteOpts
}

type suiteOpts struct {
	schema    string
	gqlSchema string
	rdfs      string
	bulkSuite bool
}

func newSuiteInternal(t *testing.T, opts suiteOpts) *suite {
	if testing.Short() {
		t.Skip("Skipping system test with long runtime.")
	}

	s := &suite{
		t:    t,
		opts: opts,
	}

	require.NoError(s.t, makeDirEmpty(rootDir))
	rdfFile := filepath.Join(rootDir, "rdfs.rdf")
	require.NoError(s.t, ioutil.WriteFile(rdfFile, []byte(opts.rdfs), 0644))
	schemaFile := filepath.Join(rootDir, "schema.txt")
	require.NoError(s.t, ioutil.WriteFile(schemaFile, []byte(opts.schema), 0644))
	gqlSchemaFile := filepath.Join(rootDir, "gql_schema.txt")
	require.NoError(s.t, ioutil.WriteFile(gqlSchemaFile, []byte(opts.gqlSchema), 0644))
	s.setup(t, "schema.txt", "rdfs.rdf", "gql_schema.txt")
	return s
}

func newLiveOnlySuite(t *testing.T, schema, rdfs, gqlSchema string) *suite {
	opts := suiteOpts{
		schema:    schema,
		gqlSchema: gqlSchema,
		rdfs:      rdfs,
		bulkSuite: false,
	}
	return newSuiteInternal(t, opts)
}

func newBulkOnlySuite(t *testing.T, schema, rdfs, gqlSchema string) *suite {
	opts := suiteOpts{
		schema:    schema,
		gqlSchema: gqlSchema,
		rdfs:      rdfs,
		bulkSuite: true,
	}
	return newSuiteInternal(t, opts)
}

func newSuiteFromFile(t *testing.T, schemaFile, rdfFile, gqlSchemaFile string) *suite {
	if testing.Short() {
		t.Skip("Skipping system test with long runtime.")
	}
	s := &suite{t: t}

	s.setup(t, schemaFile, rdfFile, gqlSchemaFile)
	return s
}

func (s *suite) setup(t *testing.T, schemaFile, rdfFile, gqlSchemaFile string) {
	require.NoError(s.t, makeDirEmpty(filepath.Join(rootDir, "out", "0")))
	if s.opts.bulkSuite {
		err := testutil.BulkLoad(testutil.BulkOpts{
			Zero:          testutil.SockAddrZero,
			Shards:        1,
			RdfFile:       rdfFile,
			SchemaFile:    schemaFile,
			GQLSchemaFile: gqlSchemaFile,
			Dir:           rootDir,
		})

		require.NoError(t, err)
		err = testutil.StartAlphas("../bulk/alpha.yml")
		require.NoError(t, err)
		return
	}

	err := testutil.LiveLoad(testutil.LiveOpts{
		Zero:       testutil.SockAddrZero,
		Alpha:      testutil.ContainerAddr("alpha1", 9080),
		RdfFile:    rdfFile,
		SchemaFile: schemaFile,
		Dir:        rootDir,
	})

	require.NoError(t, err)
	return
}

func makeDirEmpty(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

func (s *suite) cleanup() {
	// NOTE: Shouldn't raise any errors here or fail a test, since this is
	// called when we detect an error (don't want to mask the original problem).
	if s.opts.bulkSuite {
		testutil.StopAlphas("../bulk/alpha.yml")
		_ = os.RemoveAll(rootDir)
		return
	}
	dg, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
	if err == nil {
		_ = dg.Alter(context.Background(), &api.Operation{
			DropAll: true,
		})
	}
	_ = os.RemoveAll(rootDir)
}

func testCase(query, wantResult string) func(*testing.T) {
	return func(t *testing.T) {
		// Check results of the bulk loader.
		dg, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
		require.NoError(t, err)
		ctx2, cancel2 := context.WithTimeout(context.Background(), time.Minute)
		defer cancel2()

		txn := dg.NewTxn()
		resp, err := txn.Query(ctx2, query)
		require.NoError(t, err)
		testutil.CompareJSON(t, wantResult, string(resp.GetJson()))
	}
}
