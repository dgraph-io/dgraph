//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/tok/kmeans"
	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

func (vsuite *VectorTestSuite) TestVectorIncrBackupRestore() {
	t := vsuite.T()
	// The 5-round restore verification below queries every vector of each
	// restored batch (topk == numVectors), which takes ~35min per index mode;
	// the assertions stay at full strength and the test runs nightly.
	skipUnlessNightlyLane(t)
	gc, hc := vsuite.setup()
	backupDir := testBackupDir(t)

	numVectors := 1500
	allVectors := make([][][]float32, 0, 5)
	allRdfs := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		var rdfs string
		var vectors [][]float32
		rdfs, vectors = dgraphapi.GenerateRandomVectors(numVectors*(i-1), numVectors*i, 10, pred)
		allVectors = append(allVectors, vectors)
		allRdfs = append(allRdfs, rdfs)
		mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err := gc.Mutate(mu)
		require.NoError(t, err)
		require.NoError(t, gc.SetupSchema(vsuite.schemaVecDimesion10))

		t.Logf("taking backup #%v\n", i)
		require.NoError(t, hc.Backup(sharedCluster, i == 1, backupDir))
	}

	for i := 1; i <= 5; i++ {
		t.Logf("restoring backup #%v\n", i)

		incrFrom := i - 1
		require.NoError(t, hc.Restore(sharedCluster, backupDir, "", incrFrom, i))
		require.NoError(t, dgraphapi.WaitForRestore(sharedCluster))
		query := `{
			vector(func: has(project_description_v)) {
				   count(uid)
				}
		}`
		result, err := gc.Query(query)
		require.NoError(t, err)

		require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors*i), string(result.GetJson()))

		// Only the batches restored so far exist (the count above proves it);
		// similar_to results must come from exactly this set.
		var restoredVecs [][]float32
		for _, vecArr := range allVectors[:i] {
			restoredVecs = append(restoredVecs, vecArr...)
		}
		for p, vector := range allVectors[i-1] {
			triple := strings.Split(allRdfs[i-1], "\n")[p]
			uid := strings.Split(triple, " ")[0]
			queriedVector, err := gc.QuerySingleVectorsUsingUid(uid, pred)
			require.NoError(t, err)

			require.Equal(t, allVectors[i-1][p], queriedVector[0])
			similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, numVectors)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(similarVectors), 10)
			for _, similarVector := range similarVectors {
				require.Contains(t, restoredVecs, similarVector)
			}
		}
	}
}

func (vsuite *VectorTestSuite) TestVectorBackupRestore() {
	t := vsuite.T()
	gc, hc := vsuite.setup()
	backupDir := testBackupDir(t)

	numVectors := 1001
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err := gc.Mutate(mu)
	require.NoError(t, err)
	require.NoError(t, gc.SetupSchema(vsuite.schema))

	t.Log("taking backup \n")
	require.NoError(t, hc.Backup(sharedCluster, false, backupDir))

	t.Log("restoring backup \n")
	require.NoError(t, hc.Restore(sharedCluster, backupDir, "", 0, 0))
	require.NoError(t, dgraphapi.WaitForRestore(sharedCluster))

	for _, vector := range vectors {
		similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 100)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(similarVectors), 100)
		for _, similarVector := range similarVectors {
			require.Contains(t, vectors, similarVector)
		}
	}
}

func (vsuite *VectorTestSuite) TestVectorBackupRestoreDropIndex() {
	t := vsuite.T()
	gc, hc := vsuite.setup()
	backupDir := testBackupDir(t)

	// add vector predicate + index
	require.NoError(t, gc.SetupSchema(vsuite.schema))
	// add data to the vector predicate
	numVectors := 1000
	pred := "project_description_v"
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err := gc.Mutate(mu)
	require.NoError(t, err)

	t.Log("taking full backup \n")
	require.NoError(t, hc.Backup(sharedCluster, false, backupDir))

	// drop index
	require.NoError(t, gc.SetupSchema(testSchemaWithoutIndex))

	// add more data to the vector predicate
	rdfs, vectors2 := dgraphapi.GenerateRandomVectors(numVectors, numVectors+3, 100, pred)
	mu = &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	// delete some entries
	mu = &api.Mutation{DelNquads: []byte(strings.Split(rdfs, "\n")[1]), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	vectors2 = slices.Delete(vectors2, 1, 2)

	mu = &api.Mutation{DelNquads: []byte(strings.Split(rdfs, "\n")[0]), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)
	vectors2 = slices.Delete(vectors2, 0, 1)

	t.Log("taking first incr backup \n")
	require.NoError(t, hc.Backup(sharedCluster, false, backupDir))

	// add index
	require.NoError(t, gc.SetupSchema(vsuite.schema))

	t.Log("taking second incr backup \n")
	require.NoError(t, hc.Backup(sharedCluster, false, backupDir))

	// restore backup
	t.Log("restoring backup \n")
	require.NoError(t, hc.Restore(sharedCluster, backupDir, "", 0, 0))
	require.NoError(t, dgraphapi.WaitForRestore(sharedCluster))

	query := ` {
		vectors(func: has(project_description_v)) {
			   count(uid)
			 }
		}`
	resp, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, `{"vectors":[{"count":1001}]}`, string(resp.GetJson()))

	require.NoError(t, err)
	allVec := append(vectors, vectors2...)

	for _, vector := range allVec {

		similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 4)
		require.NoError(t, err)
		for _, similarVector := range similarVectors {
			require.Contains(t, allVec, similarVector)
		}
	}
}

func (vsuite *VectorTestSuite) TestVectorBackupRestoreReIndexing() {
	t := vsuite.T()
	gc, hc := vsuite.setup()
	backupDir := testBackupDir(t)

	require.NoError(t, gc.SetupSchema(vsuite.schema))

	numVectors := 1000
	pred := "project_description_v"
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err := gc.Mutate(mu)
	require.NoError(t, err)

	t.Log("taking backup \n")
	require.NoError(t, hc.Backup(sharedCluster, false, backupDir))

	rdfs2, vectors2 := dgraphapi.GenerateRandomVectors(numVectors, numVectors+300, 100, pred)

	mu = &api.Mutation{SetNquads: []byte(rdfs2), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)
	t.Log("restoring backup \n")
	require.NoError(t, hc.Restore(sharedCluster, backupDir, "", 2, 1))
	require.NoError(t, dgraphapi.WaitForRestore(sharedCluster))

	for i := 0; i < 5; i++ {
		// drop index
		require.NoError(t, gc.SetupSchema(testSchemaWithoutIndex))
		// add index
		require.NoError(t, gc.SetupSchema(vsuite.schema))
	}
	vectors = append(vectors, vectors2...)
	rdfs = rdfs + rdfs2
	testVectorQuery(t, gc, vectors, rdfs, pred, numVectors)
}

// readBackupManifest reads manifest.json from the backup directory inside the container.
func readBackupManifest(t *testing.T, c *dgraphtest.LocalCluster, dir string) worker.MasterManifest {
	t.Helper()
	b, err := c.ReadFileFromContainer(filepath.Join(dir, "manifest.json"))
	require.NoError(t, err)
	var m worker.MasterManifest
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

// allUserPreds returns all non-internal predicates across all groups from a manifest entry.
func allUserPreds(m *worker.Manifest) []string {
	var preds []string
	for _, ps := range m.Groups {
		for _, p := range ps {
			if !strings.HasPrefix(p, "dgraph.") {
				preds = append(preds, p)
			}
		}
	}
	return preds
}

// userPredsPerGroup returns non-internal predicates grouped by their group ID.
func userPredsPerGroup(m *worker.Manifest) map[uint32][]string {
	result := make(map[uint32][]string)
	for gid, ps := range m.Groups {
		for _, p := range ps {
			if !strings.HasPrefix(p, "dgraph.") {
				result[gid] = append(result[gid], p)
			}
		}
	}
	return result
}

func assertVecSupportingPreds(t *testing.T, preds []string, vecPreds []string, ctx string) {
	t.Helper()
	for _, vecPred := range vecPreds {
		require.Containsf(t, preds, vecPred, "%s: missing base predicate %q", ctx, vecPred)
		require.Containsf(t, preds, vecPred+hnsw.VecEntry, "%s: missing %s for %q", ctx, hnsw.VecEntry, vecPred)
		require.Containsf(t, preds, vecPred+hnsw.VecKeyword, "%s: missing %s for %q", ctx, hnsw.VecKeyword, vecPred)
		require.Containsf(t, preds, vecPred+hnsw.VecDead, "%s: missing %s for %q", ctx, hnsw.VecDead, vecPred)
	}
}

// assertPartitionedVecSupportingPreds checks that a partitioned (numClusters)
// vector predicate's backup predicate set carries every internal key family:
// the centroid set, the per-predicate dimension metadata (__vector_meta_), and
// the per-cluster split attrs. Restore is byte-replay with no rebuild, so any
// one of these missing from the manifest silently vanishes on restore.
func assertPartitionedVecSupportingPreds(t *testing.T, preds []string, vecPred string,
	numClusters int, ctx string) {
	t.Helper()
	require.Containsf(t, preds, vecPred, "%s: missing base predicate %q", ctx, vecPred)
	require.Containsf(t, preds, vecPred+kmeans.CentroidPrefix, "%s: missing centroid for %q", ctx, vecPred)
	require.Containsf(t, preds, vecPred+hnsw.VecMeta, "%s: missing %s for %q", ctx, hnsw.VecMeta, vecPred)
	for i := 0; i < numClusters; i++ {
		require.Containsf(t, preds, vecPred+fmt.Sprintf("%s_%d", hnsw.VecEntry, i),
			"%s: missing per-cluster %s_%d for %q", ctx, hnsw.VecEntry, i, vecPred)
		require.Containsf(t, preds, vecPred+fmt.Sprintf("%s_%d", hnsw.VecKeyword, i),
			"%s: missing per-cluster %s_%d for %q", ctx, hnsw.VecKeyword, i, vecPred)
		require.Containsf(t, preds, vecPred+fmt.Sprintf("%s_%d", hnsw.VecDead, i),
			"%s: missing per-cluster %s_%d for %q", ctx, hnsw.VecDead, i, vecPred)
	}
}

func assertNoDuplicates(t *testing.T, preds []string, ctx string) {
	t.Helper()
	seen := make(map[string]int)
	for _, p := range preds {
		seen[p]++
	}
	for p, count := range seen {
		require.Equalf(t, 1, count, "%s: predicate %q appears %d times, expected 1", ctx, p, count)
	}
}

func insertVectors(t *testing.T, gc *dgraphapi.GrpcClient, vecPreds []string, startIdx, endIdx int) {
	t.Helper()
	for _, p := range vecPreds {
		rdfs, _ := dgraphapi.GenerateRandomVectors(startIdx, endIdx, 1, p)
		mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err := gc.Mutate(mu)
		require.NoError(t, err)
	}
}

// findGroupForPred returns the group ID that contains the given predicate.
func findGroupForPred(m *worker.Manifest, pred string) (uint32, bool) {
	for gid, ps := range m.Groups {
		for _, p := range ps {
			if p == pred {
				return gid, true
			}
		}
	}
	return 0, false
}

func TestVectorBackupManifestPredicates(t *testing.T) {
	gc, hc := setupTest(t)
	// Each subtest uses its own backup subdirectory (under a base unique to
	// this test) so manifest counts are deterministic and independent of how
	// many earlier subtests or other tests ran.
	baseDir := testBackupDir(t)

	t.Run("single vector predicate", func(t *testing.T) {
		require.NoError(t, gc.DropAll())

		dir := filepath.Join(baseDir, "single")

		schema := `vec1: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
			name: string @index(exact) .`
		require.NoError(t, gc.SetupSchema(schema))

		insertVectors(t, gc, []string{"vec1"}, 0, 5)

		require.NoError(t, hc.Backup(sharedCluster, true, dir))

		manifest := readBackupManifest(t, sharedCluster, dir)
		require.Len(t, manifest.Manifests, 1)
		preds := allUserPreds(manifest.Manifests[0])

		fmt.Println("preds", preds)

		require.Contains(t, preds, "0-name")
		assertVecSupportingPreds(t, preds, []string{"0-vec1"}, "0-single-vec")
		assertNoDuplicates(t, preds, "single-vec")
	})

	t.Run("partitioned vector predicate", func(t *testing.T) {
		require.NoError(t, gc.DropAll())

		dir := filepath.Join(baseDir, "partitioned")

		const numClusters = 4
		schema := fmt.Sprintf(`vecp: float32vector @index(hnsw(metric: "euclidean", numClusters: "%d")) .
			name: string @index(exact) .`, numClusters)
		require.NoError(t, gc.SetupSchema(schema))

		insertVectors(t, gc, []string{"vecp"}, 0, 20)

		require.NoError(t, hc.Backup(sharedCluster, true, dir))

		manifest := readBackupManifest(t, sharedCluster, dir)
		preds := allUserPreds(manifest.Manifests[len(manifest.Manifests)-1])

		require.Contains(t, preds, "0-name")
		// A partitioned index's backup set must carry the centroid, the
		// dimension metadata (__vector_meta_), and every per-cluster split attr.
		assertPartitionedVecSupportingPreds(t, preds, "0-vecp", numClusters, "partitioned-vec")
		assertNoDuplicates(t, preds, "partitioned-vec")
	})

	t.Run("multiple vector predicates in same group", func(t *testing.T) {
		require.NoError(t, gc.DropAll())

		dir := filepath.Join(baseDir, "multi")

		schema := `vec_a: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
			vec_b: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
			vec_c: float32vector @index(hnsw(exponent: "5", metric: "cosine")) .
			age: int .`
		require.NoError(t, gc.SetupSchema(schema))

		insertVectors(t, gc, []string{"vec_a", "vec_b", "vec_c"}, 0, 3)

		require.NoError(t, hc.Backup(sharedCluster, true, dir))

		manifest := readBackupManifest(t, sharedCluster, dir)
		require.GreaterOrEqual(t, len(manifest.Manifests), 1)
		latest := manifest.Manifests[len(manifest.Manifests)-1]
		preds := allUserPreds(latest)

		require.Contains(t, preds, "0-age")
		assertVecSupportingPreds(t, preds, []string{"0-vec_a", "0-vec_b", "0-vec_c"}, "multi-vec")
		assertNoDuplicates(t, preds, "multi-vec")
	})

	t.Run("incremental backup preserves vector predicates", func(t *testing.T) {
		require.NoError(t, gc.DropAll())

		dir := filepath.Join(baseDir, "incremental")

		schema := `vec_x: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
			vec_y: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .`
		require.NoError(t, gc.SetupSchema(schema))

		insertVectors(t, gc, []string{"vec_x", "vec_y"}, 0, 3)
		require.NoError(t, hc.Backup(sharedCluster, true, dir))

		insertVectors(t, gc, []string{"vec_x", "vec_y"}, 3, 6)
		require.NoError(t, hc.Backup(sharedCluster, false, dir))

		// Isolated backup dir => exactly two manifests: the full backup (index 0)
		// and the incremental (index 1). Both must carry the vector supporting
		// predicates.
		manifest := readBackupManifest(t, sharedCluster, dir)
		require.Len(t, manifest.Manifests, 2)

		for i := 0; i < 2; i++ {
			m := manifest.Manifests[i]
			preds := allUserPreds(m)
			ctx := fmt.Sprintf("manifest[%d]", i)
			assertVecSupportingPreds(t, preds, []string{"0-vec_x", "0-vec_y"}, ctx)
			assertNoDuplicates(t, preds, ctx)
		}
	})
}

func TestVectorBackupManifestMultiGroup(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(2).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
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

	t.Run("vector predicates across multiple groups", func(t *testing.T) {
		require.NoError(t, gc.DropAll())

		schema := `vec_p: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
			vec_q: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
			vec_r: float32vector @index(hnsw(exponent: "5", metric: "cosine")) .
			vec_s: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
			title: string @index(exact) .
			score: int .
			description: string .
			rating: float .
			active: bool @index(bool) .
			tags: [string] @index(exact) .`
		require.NoError(t, gc.SetupSchema(schema))

		insertVectors(t, gc, []string{"vec_p", "vec_q", "vec_r", "vec_s"}, 0, 5)

		require.NoError(t, hc.Backup(c, true, dgraphtest.DefaultBackupDir))

		manifest := readBackupManifest(t, c, dgraphtest.DefaultBackupDir)
		latest := manifest.Manifests[len(manifest.Manifests)-1]

		// Verify we have multiple groups.
		require.GreaterOrEqual(t, len(latest.Groups), 2, "expected at least 2 groups with 2 alphas")

		// For each vector predicate, verify its supporting predicates are in the SAME group.
		for _, vecPred := range []string{"0-vec_p", "0-vec_q", "0-vec_r", "0-vec_s"} {
			gid, found := findGroupForPred(latest, vecPred)
			require.Truef(t, found, "base predicate %q not found in any group", vecPred)

			groupPreds := latest.Groups[gid]
			require.Containsf(t, groupPreds, vecPred+hnsw.VecEntry,
				"group %d: missing %s for %q", gid, hnsw.VecEntry, vecPred)
			require.Containsf(t, groupPreds, vecPred+hnsw.VecKeyword,
				"group %d: missing %s for %q", gid, hnsw.VecKeyword, vecPred)
			require.Containsf(t, groupPreds, vecPred+hnsw.VecDead,
				"group %d: missing %s for %q", gid, hnsw.VecDead, vecPred)
		}

		// Verify no duplicates within each group independently.
		for gid, preds := range userPredsPerGroup(latest) {
			assertNoDuplicates(t, preds, fmt.Sprintf("group-%d", gid))
		}

		// Verify no duplicates across all groups combined.
		assertNoDuplicates(t, allUserPreds(latest), "all-groups")
	})

	t.Run("partitioned vector predicates across multiple groups", func(t *testing.T) {
		require.NoError(t, gc.DropAll())

		// A partitioned index legitimately expects a LARGER supporting-predicate
		// set (centroid, __vector_meta_, and per-cluster split attrs), so this is
		// not a loosening of the monolithic case above. numClusters == 4 over 100
		// vectors keeps clustering meaningful.
		const numClusters = 4
		schema := fmt.Sprintf(`vecp_p: float32vector @index(hnsw(metric: "euclidean", numClusters: "%d")) .
			vecp_q: float32vector @index(hnsw(metric: "euclidean", numClusters: "%d")) .
			vecp_r: float32vector @index(hnsw(metric: "cosine", numClusters: "%d")) .
			vecp_s: float32vector @index(hnsw(metric: "euclidean", numClusters: "%d")) .
			title: string @index(exact) .
			score: int .`, numClusters, numClusters, numClusters, numClusters)
		require.NoError(t, gc.SetupSchema(schema))

		insertVectors(t, gc, []string{"vecp_p", "vecp_q", "vecp_r", "vecp_s"}, 0, 100)

		require.NoError(t, hc.Backup(c, true, dgraphtest.DefaultBackupDir))

		manifest := readBackupManifest(t, c, dgraphtest.DefaultBackupDir)
		latest := manifest.Manifests[len(manifest.Manifests)-1]

		// Verify we have multiple groups.
		require.GreaterOrEqual(t, len(latest.Groups), 2, "expected at least 2 groups with 2 alphas")

		// For each partitioned vector predicate, verify its full supporting
		// predicate set is present in the SAME group as its base predicate.
		for _, vecPred := range []string{"0-vecp_p", "0-vecp_q", "0-vecp_r", "0-vecp_s"} {
			gid, found := findGroupForPred(latest, vecPred)
			require.Truef(t, found, "base predicate %q not found in any group", vecPred)

			assertPartitionedVecSupportingPreds(t, latest.Groups[gid], vecPred, numClusters,
				fmt.Sprintf("partitioned-multi-group-%d", gid))
		}

		// Verify no duplicates within each group independently.
		for gid, preds := range userPredsPerGroup(latest) {
			assertNoDuplicates(t, preds, fmt.Sprintf("group-%d", gid))
		}

		// Verify no duplicates across all groups combined.
		assertNoDuplicates(t, allUserPreds(latest), "all-groups")
	})

	t.Run("incremental backup multi group", func(t *testing.T) {
		require.NoError(t, gc.DropAll())

		schema := `vec_m: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
			vec_n: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
			vec_o: float32vector @index(hnsw(exponent: "5", metric: "cosine")) .
			label: string @index(exact) .
			category: string .
			weight: float .
			enabled: bool @index(bool) .
			status: string @index(exact) .`
		require.NoError(t, gc.SetupSchema(schema))

		insertVectors(t, gc, []string{"vec_m", "vec_n", "vec_o"}, 0, 3)
		require.NoError(t, hc.Backup(c, true, dgraphtest.DefaultBackupDir))

		insertVectors(t, gc, []string{"vec_m", "vec_n", "vec_o"}, 3, 6)
		require.NoError(t, hc.Backup(c, false, dgraphtest.DefaultBackupDir))

		manifest := readBackupManifest(t, c, dgraphtest.DefaultBackupDir)
		require.GreaterOrEqual(t, len(manifest.Manifests), 3)

		// Verify the last 2 entries (full + incremental from this subtest).
		for i := len(manifest.Manifests) - 2; i < len(manifest.Manifests); i++ {
			m := manifest.Manifests[i]
			require.GreaterOrEqual(t, len(m.Groups), 2,
				"manifest[%d]: expected at least 2 groups", i)

			for _, vecPred := range []string{"0-vec_m", "0-vec_n", "0-vec_o"} {
				gid, found := findGroupForPred(m, vecPred)
				require.Truef(t, found, "manifest[%d]: predicate %q not found", i, vecPred)

				groupPreds := m.Groups[gid]
				require.Containsf(t, groupPreds, vecPred+hnsw.VecEntry,
					"manifest[%d] group %d: missing %s for %q", i, gid, hnsw.VecEntry, vecPred)
				require.Containsf(t, groupPreds, vecPred+hnsw.VecKeyword,
					"manifest[%d] group %d: missing %s for %q", i, gid, hnsw.VecKeyword, vecPred)
				require.Containsf(t, groupPreds, vecPred+hnsw.VecDead,
					"manifest[%d] group %d: missing %s for %q", i, gid, hnsw.VecDead, vecPred)
			}

			for gid, preds := range userPredsPerGroup(m) {
				assertNoDuplicates(t, preds, fmt.Sprintf("manifest[%d]-group-%d", i, gid))
			}
			assertNoDuplicates(t, allUserPreds(m), fmt.Sprintf("manifest[%d]-all", i))
		}
	})
}

// TestVectorBackupAfterRestore verifies that taking a backup after a restore
// does not produce duplicate supporting predicates in the manifest. The restore
// code skips ForceTablet for supporting predicates so they remain absent from
// Zero's membership state (consistent with normal operation). The backup code
// discovers them via GetSchemaOverNetwork and adds them exactly once.
func TestVectorBackupAfterRestore(t *testing.T) {
	gc, hc := setupTest(t)
	backupDir := testBackupDir(t)

	schema := `vec_r1: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		vec_r2: float32vector @index(hnsw(exponent: "5", metric: "cosine")) .
		label: string @index(exact) .`
	require.NoError(t, gc.SetupSchema(schema))

	insertVectors(t, gc, []string{"vec_r1", "vec_r2"}, 0, 5)

	// Take the initial backup.
	require.NoError(t, hc.Backup(sharedCluster, true, backupDir))

	// Restore from the backup. Supporting predicates are NOT registered as
	// tablets in Zero (ForceTablet is skipped for them), keeping the state
	// consistent with normal operation.
	require.NoError(t, hc.Restore(sharedCluster, backupDir, "", 0, 0))
	require.NoError(t, dgraphapi.WaitForRestore(sharedCluster))

	// Re-login after restore since sessions are invalidated.
	gc, cleanup, err := sharedCluster.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err = sharedCluster.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	// Insert more data and take another backup. Supporting predicates are
	// not in Zero's membership state, so the backup code discovers them
	// via GetSchemaOverNetwork and adds them exactly once.
	insertVectors(t, gc, []string{"vec_r1", "vec_r2"}, 5, 10)
	require.NoError(t, hc.Backup(sharedCluster, true, backupDir))

	manifest := readBackupManifest(t, sharedCluster, backupDir)
	require.GreaterOrEqual(t, len(manifest.Manifests), 2)

	latest := manifest.Manifests[len(manifest.Manifests)-1]
	preds := allUserPreds(latest)

	assertVecSupportingPreds(t, preds, []string{"0-vec_r1", "0-vec_r2"}, "post-restore-backup")
	assertNoDuplicates(t, preds, "post-restore-backup")
}

// TestVectorBackupAfterRestorePartitioned is the partitioned (numClusters)
// counterpart of TestVectorBackupAfterRestore. Restore is a byte-replay with no
// index rebuild, so a partitioned index's LARGER supporting-predicate set
// (centroid, __vector_meta_, and per-cluster split attrs) must survive the
// backup -> restore -> backup round trip and be discovered exactly once by the
// post-restore backup. This is not a loosening of the monolithic case; the
// supporting-predicate set is legitimately larger and every member is asserted.
func TestVectorBackupAfterRestorePartitioned(t *testing.T) {
	gc, hc := setupTest(t)
	backupDir := testBackupDir(t)

	// numClusters == 2 keeps clustering meaningful for the vector counts inserted
	// below (50 before the first backup, 100 total by the second).
	const numClusters = 2
	schema := fmt.Sprintf(`vec_r1: float32vector @index(hnsw(exponent: "5", metric: "euclidean", numClusters: "%d")) .
		vec_r2: float32vector @index(hnsw(exponent: "5", metric: "cosine", numClusters: "%d")) .
		label: string @index(exact) .`, numClusters, numClusters)
	require.NoError(t, gc.SetupSchema(schema))

	insertVectors(t, gc, []string{"vec_r1", "vec_r2"}, 0, 50)

	// Take the initial backup.
	require.NoError(t, hc.Backup(sharedCluster, true, backupDir))

	// Restore from the backup. Supporting predicates are NOT registered as
	// tablets in Zero (ForceTablet is skipped for them), keeping the state
	// consistent with normal operation.
	require.NoError(t, hc.Restore(sharedCluster, backupDir, "", 0, 0))
	require.NoError(t, dgraphapi.WaitForRestore(sharedCluster))

	// Re-login after restore since sessions are invalidated.
	gc, cleanup, err := sharedCluster.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err = sharedCluster.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	// Insert more data and take another backup. The partitioned supporting
	// predicates are not in Zero's membership state, so the backup code discovers
	// them via GetSchemaOverNetwork and adds them exactly once.
	insertVectors(t, gc, []string{"vec_r1", "vec_r2"}, 50, 100)
	require.NoError(t, hc.Backup(sharedCluster, true, backupDir))

	manifest := readBackupManifest(t, sharedCluster, backupDir)
	require.GreaterOrEqual(t, len(manifest.Manifests), 2)

	latest := manifest.Manifests[len(manifest.Manifests)-1]
	preds := allUserPreds(latest)

	assertPartitionedVecSupportingPreds(t, preds, "0-vec_r1", numClusters, "post-restore-backup")
	assertPartitionedVecSupportingPreds(t, preds, "0-vec_r2", numClusters, "post-restore-backup")
	assertNoDuplicates(t, preds, "post-restore-backup")
}
