//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/x"
)

// sharedCluster is a 1-alpha, 1-zero ACL cluster started once in TestMain and
// reused by every test that doesn't need a special topology or cluster
// lifecycle control. Starting a LocalCluster costs tens of seconds and the
// suite runs twice (monolithic + partitioned), so sharing one cluster instead
// of booting one per test is what keeps this suite's wall time down. Tests get
// a clean state through setupTest, which drops all data first.
//
// Tests that keep their own cluster: TestVectorSnapshot (3 alphas / 3 zeros),
// TestVectorBackupManifestMultiGroup (2 alphas), TestPartitionedPipelines
// (stops and restarts an alpha), and the bulk-load bulk/target clusters
// (fresh p directories by construction).
var sharedCluster *dgraphtest.LocalCluster

func TestMain(m *testing.M) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	if err := c.Start(); err != nil {
		c.Cleanup(true)
		x.Panic(err)
	}
	sharedCluster = c

	code := m.Run()
	sharedCluster.Cleanup(code != 0)
	os.Exit(code)
}

// skipUnlessNightlyLane skips long-running tests outside the nightly lane.
// CI sets VECTOR_TEST_LANE=nightly on the scheduled run (which executes the
// whole suite) and leaves the PR-triggered run on the fast lane. The gated
// tests keep their full assertions — they are moved to the nightly lane, not
// weakened. Locally, run them with VECTOR_TEST_LANE=nightly.
func skipUnlessNightlyLane(t *testing.T) {
	t.Helper()
	if strings.EqualFold(os.Getenv("VECTOR_TEST_LANE"), "nightly") {
		return
	}
	t.Skip("long-running test: runs in the nightly lane (set VECTOR_TEST_LANE=nightly)")
}

// setupTest returns logged-in grpc and http clients for the shared cluster
// after wiping the data, schema, and namespaces left behind by earlier tests.
func setupTest(t *testing.T) (*dgraphapi.GrpcClient, *dgraphapi.HTTPClient) {
	gc, cleanup, err := sharedCluster.Client()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err := sharedCluster.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	require.NoError(t, gc.DropAll())
	return gc, hc
}

// setup is setupTest for suite methods.
func (vsuite *VectorTestSuite) setup() (*dgraphapi.GrpcClient, *dgraphapi.HTTPClient) {
	return setupTest(vsuite.T())
}

// testDirSeq disambiguates per-test container directories: the suite runs
// twice (monolithic + partitioned), so t.Name() alone is not unique across
// the two passes.
var testDirSeq atomic.Uint64

func uniqueTestDir(t *testing.T, parent string) string {
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	return filepath.Join(parent, fmt.Sprintf("%s-%d", name, testDirSeq.Add(1)))
}

// testBackupDir returns a backup destination unique to this test invocation.
// Backups on the shared cluster land in one docker volume, so tests must not
// share a directory or manifests from earlier tests leak into later ones.
func testBackupDir(t *testing.T) string {
	return uniqueTestDir(t, dgraphtest.DefaultBackupDir)
}

// testExportDir returns an export destination unique to this test invocation.
// CopyExportToHost and LiveLoadFromExport copy the entire directory, so
// sharing DefaultExportDir across tests would mix exports together.
func testExportDir(t *testing.T) string {
	return uniqueTestDir(t, dgraphtest.DefaultExportDir)
}

// setupBulkTarget runs the bulk loader on the data exported from the shared
// cluster (at exportDir) and starts a fresh target cluster on the bulk-loaded
// p directories. The caller gets a logged-in client for the target cluster;
// both clusters are cleaned up with the test.
func setupBulkTarget(t *testing.T, exportDir string, numShards int) *dgraphapi.GrpcClient {
	bulkOutDir := t.TempDir()
	bulkConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(numShards).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour).
		WithBulkLoadOutDir(bulkOutDir)

	bulkCluster, err := dgraphtest.NewLocalCluster(bulkConf)
	require.NoError(t, err)
	t.Cleanup(func() { bulkCluster.Cleanup(t.Failed()) })

	// Start only Zero for bulk loading
	require.NoError(t, bulkCluster.StartZero(0))
	require.NoError(t, bulkCluster.HealthCheck(true))

	// Copy exported files from the shared cluster's container to host for bulk load
	exportHostDir := t.TempDir()
	dataFiles, schemaFiles, err := sharedCluster.CopyExportToHost(exportDir, exportHostDir)
	require.NoError(t, err)
	require.NotEmpty(t, dataFiles, "should have exported data files")
	require.NotEmpty(t, schemaFiles, "should have exported schema files")

	// Run bulk load with exported data
	opts := dgraphtest.BulkOpts{
		DataFiles:   dataFiles,
		SchemaFiles: schemaFiles,
		OutDir:      bulkOutDir,
	}
	if numShards > 1 {
		opts.MapShards = numShards
		opts.ReduceShards = numShards
	}
	require.NoError(t, bulkCluster.BulkLoad(opts))

	// Create a new cluster that uses the bulk loaded p directories
	targetConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(numShards). // must match the number of shards
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour).
		WithBulkLoadOutDir(bulkOutDir)

	targetCluster, err := dgraphtest.NewLocalCluster(targetConf)
	require.NoError(t, err)
	t.Cleanup(func() { targetCluster.Cleanup(t.Failed()) })

	// Start the target cluster (both Zero and Alphas)
	require.NoError(t, targetCluster.Start())

	targetGc, targetCleanup, err := targetCluster.Client()
	require.NoError(t, err)
	t.Cleanup(targetCleanup)
	require.NoError(t, targetGc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))
	return targetGc
}
