/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/hypermodeinc/dgraph/v24/testutil"
)

var rootDir = "./data"
var rootBucket = "data"

type bsuite struct {
	t    *testing.T
	opts suiteOpts
}

type suiteOpts struct {
	schema    string
	gqlSchema string
	rdfs      string
	bulkSuite bool
	bulkOpts  bulkOpts
}

type bulkOpts struct {
	alpha   string
	forceNs uint64
}

func newSuiteInternal(t *testing.T, opts suiteOpts) *bsuite {
	if testing.Short() {
		t.Skip("Skipping system test with long runtime.")
	}

	s := &bsuite{
		t:    t,
		opts: opts,
	}
	require.NoError(s.t, makeDirEmpty(rootDir))
	rdfFile := filepath.Join(rootDir, "rdfs.rdf")
	require.NoError(s.t, os.WriteFile(rdfFile, []byte(opts.rdfs), 0644))
	schemaFile := filepath.Join(rootDir, "schema.txt")
	require.NoError(s.t, os.WriteFile(schemaFile, []byte(opts.schema), 0644))
	gqlSchemaFile := filepath.Join(rootDir, "gql_schema.txt")
	require.NoError(s.t, os.WriteFile(gqlSchemaFile, []byte(opts.gqlSchema), 0644))

	s.setup(t, schemaFile, rdfFile, gqlSchemaFile)
	return s
}

func newLiveOnlySuite(t *testing.T, schema, rdfs, gqlSchema string) *bsuite {
	opts := suiteOpts{
		schema:    schema,
		gqlSchema: gqlSchema,
		rdfs:      rdfs,
		bulkSuite: false,
	}
	return newSuiteInternal(t, opts)
}

func newBulkOnlySuite(t *testing.T, schema, rdfs, gqlSchema string) *bsuite {
	opts := suiteOpts{
		schema:    schema,
		gqlSchema: gqlSchema,
		rdfs:      rdfs,
		bulkSuite: true,
		bulkOpts:  bulkOpts{alpha: "../bulk/alpha.yml", forceNs: math.MaxUint64}, // preserve ns
	}
	return newSuiteInternal(t, opts)
}

func newSuiteFromFile(t *testing.T, schemaFile, rdfFile, gqlSchemaFile string) *bsuite {
	if testing.Short() {
		t.Skip("Skipping system test with long runtime.")
	}
	s := &bsuite{t: t}

	s.setup(t, schemaFile, rdfFile, gqlSchemaFile)
	return s
}

func (s *bsuite) setup(t *testing.T, schemaFile, rdfFile, gqlSchemaFile string) {
	var env []string
	require.NoError(s.t, makeDirEmpty(filepath.Join(rootDir, "out", "0")))
	if s.opts.bulkSuite {
		err := testutil.BulkLoad(testutil.BulkOpts{
			Zero:          testutil.ContainerAddr("zero1", 5080),
			Shards:        1,
			RdfFile:       rdfFile,
			SchemaFile:    schemaFile,
			GQLSchemaFile: gqlSchemaFile,
			Dir:           rootDir,
			Env:           env,
			Namespace:     s.opts.bulkOpts.forceNs,
		})

		require.NoError(t, err)
		err = testutil.StartAlphas(s.opts.bulkOpts.alpha)
		require.NoError(t, err)
		return
	}

	err := testutil.LiveLoad(testutil.LiveOpts{
		Zero:       testutil.ContainerAddr("zero1", 5080),
		Alpha:      testutil.ContainerAddr("alpha1", 9080),
		RdfFile:    rdfFile,
		SchemaFile: schemaFile,
		Dir:        rootDir,
		Env:        env,
	})

	require.NoError(t, err)
}

func makeDirEmpty(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

func (s *bsuite) cleanup(t *testing.T) {
	// NOTE: Shouldn't raise any errors here or fail a test, since this is
	// called when we detect an error (don't want to mask the original problem).
	if s.opts.bulkSuite {
		isRace := testutil.StopAlphasAndDetectRace([]string{"alpha1"})
		_ = os.RemoveAll(rootDir)
		if isRace {
			t.Fatalf("Failing because race condition is detected. " +
				"Please check the logs for " + "more details.")
		}
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

func testCaseWithAcl(query, wantResult, user, password string, ns uint64) func(*testing.T) {
	return func(t *testing.T) {
		// Check results of the bulk loader.
		dg, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
		require.NoError(t, err)
		ctx2, cancel2 := context.WithTimeout(context.Background(), time.Minute)
		defer cancel2()
		require.NoError(t, dg.LoginIntoNamespace(ctx2, user, password, ns))

		txn := dg.NewTxn()
		resp, err := txn.Query(ctx2, query)
		require.NoError(t, err)
		testutil.CompareJSON(t, wantResult, string(resp.GetJson()))
	}
}
