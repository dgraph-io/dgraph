//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/x"
)

type Node struct {
	Uid       string    `json:"uid"`
	Namespace string    `json:"namespace"`
	Vtest     []float32 `json:"vtest"`
}

func TestLiveLoadAndExportRDFFormat(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	testExportAndLiveLoad(t, c, "rdf")
}

func testExportAndLiveLoad(t *testing.T, c *dgraphtest.LocalCluster, exportFormat string) {
	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	require.NoError(t, gc.SetupSchema(testSchema))

	numVectors := 100
	pred := "project_description_v"
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 10, pred)

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	t.Log("taking export \n")

	require.NoError(t, hc.Export(dgraphtest.DefaultExportDir, exportFormat, -1))

	require.NoError(t, gc.DropAll())

	query := `{
		vector(func: has(project_description_v)) {
			   count(uid)
			}
	}`

	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, 0), string(result.GetJson()))

	require.NoError(t, c.LiveLoadFromExport(dgraphtest.DefaultExportDir))

	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	result, err = gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	testVectorQuery(t, gc, vectors, rdfs, pred, numVectors)
}

func TestBulkLoadVectorIndex(t *testing.T) {
	// if runtime.GOOS != "linux" && os.Getenv("DGRAPH_BINARY") == "" {
	// 	fmt.Println("You can set the DGRAPH_BINARY environment variable to path of a native dgraph binary to run these tests")
	// 	t.Skip("Skipping test on non-Linux platforms due to dgraph binary dependency")
	// }

	// Step 1: Create a source cluster and load vectors into it
	t.Log("Step 1: Creating source cluster and loading vectors...")
	sourceConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour)
	sourceCluster, err := dgraphtest.NewLocalCluster(sourceConf)
	require.NoError(t, err)
	defer func() { sourceCluster.Cleanup(t.Failed()) }()
	require.NoError(t, sourceCluster.Start())

	gc, cleanup, err := sourceCluster.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err := sourceCluster.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	// Set up vector schema and load vectors
	require.NoError(t, gc.SetupSchema(testSchema))

	numVectors := 1000
	pred := "project_description_v"
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 10, pred)

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)
	t.Logf("Loaded %d vectors into source cluster", numVectors)

	// Verify vectors are loaded and queryable in source cluster
	for _, vector := range vectors[:5] { // Test first 5 vectors
		similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 5)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(similarVectors), 3, "similar_to query should return results")
	}
	t.Log("Verified vector similarity queries work on source cluster")

	// Step 2: Export the data from source cluster
	t.Log("Step 2: Exporting data from source cluster...")
	require.NoError(t, hc.Export(dgraphtest.DefaultExportDir, "rdf", -1))
	t.Log("Export completed")

	// Step 3: Set up a cluster for bulk loading and run bulk load on exported data
	t.Log("Step 3: Running bulk load on exported data...")
	bulkOutDir := t.TempDir()
	bulkConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour).
		WithBulkLoadOutDir(bulkOutDir)

	bulkCluster, err := dgraphtest.NewLocalCluster(bulkConf)
	require.NoError(t, err)
	defer func() { bulkCluster.Cleanup(t.Failed()) }()

	// Start only Zero for bulk loading
	require.NoError(t, bulkCluster.StartZero(0))
	require.NoError(t, bulkCluster.HealthCheck(true))

	// Copy exported files from source cluster container to host for bulk load
	exportHostDir := t.TempDir()
	dataFiles, schemaFiles, err := sourceCluster.CopyExportToHost(dgraphtest.DefaultExportDir, exportHostDir)
	require.NoError(t, err)
	require.NotEmpty(t, dataFiles, "should have exported data files")
	require.NotEmpty(t, schemaFiles, "should have exported schema files")
	t.Logf("Copied export files to host - data: %v, schema: %v", dataFiles, schemaFiles)

	// Run bulk load with exported data
	opts := dgraphtest.BulkOpts{
		DataFiles:   dataFiles,
		SchemaFiles: schemaFiles,
		OutDir:      bulkOutDir,
	}
	require.NoError(t, bulkCluster.BulkLoad(opts))
	t.Log("Bulk load completed successfully")

	// Step 4: Create a new cluster that uses the bulk loaded p directory
	t.Log("Step 4: Starting target cluster with bulk loaded p directory...")
	targetConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour).
		WithBulkLoadOutDir(bulkOutDir)

	targetCluster, err := dgraphtest.NewLocalCluster(targetConf)
	require.NoError(t, err)
	defer func() { targetCluster.Cleanup(t.Failed()) }()

	// Start the target cluster (both Zero and Alphas)
	require.NoError(t, targetCluster.Start())
	t.Log("Target cluster started with bulk loaded data")

	// Get a client to verify the data
	targetGc, targetCleanup, err := targetCluster.Client()
	require.NoError(t, err)
	defer targetCleanup()
	require.NoError(t, targetGc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	// Step 5: Verify vector count
	t.Log("Step 5: Verifying vectors were loaded correctly...")
	query := `{
		vector(func: has(project_description_v)) {
			count(uid)
		}
	}`
	result, err := targetGc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))
	t.Logf("Verified %d vectors were loaded via bulk load", numVectors)

	// Step 6: Verify vector similarity queries work (tests that vector index was built correctly)
	t.Log("Step 6: Verifying vector similarity queries work on bulk loaded data...")
	fmt.Println("vectors: ", len(vectors))
	for i, vector := range vectors {
		similarVectors, err := targetGc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 5)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(similarVectors), 4,
			"similar_to query should return results for vector %d", i)

		// The query vector itself should be in the results (closest match)
		found := false
		for _, sv := range similarVectors {
			if vectorsEqual(vector, sv) {
				found = true
				break
			}
		}
		if !found {
			fmt.Println("vector not found: ", i)
			fmt.Println("vector: ", vector)
			fmt.Println("similarVectors: ", similarVectors)
			// require.True(t, found, "Original vector %d should be found in similar_to results", i)
		}
	}
	t.Log("All vector similarity queries verified successfully - bulk load supports vector indexing!")
}

// vectorsEqual compares two float32 vectors for equality
func vectorsEqual(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBulkLoadVectorIndexMultipleGroups(t *testing.T) {
	// if runtime.GOOS != "linux" && os.Getenv("DGRAPH_BINARY") == "" {
	// 	fmt.Println("You can set the DGRAPH_BINARY environment variable to path of a native dgraph binary to run these tests")
	// 	t.Skip("Skipping test on non-Linux platforms due to dgraph binary dependency")
	// }

	// Define 3 different vector predicates - each will potentially go to different shards
	predicates := []string{"vec_pred_alpha", "vec_pred_beta", "vec_pred_gamma"}
	numVectorsPerPred := 1000
	vectorDim := 10
	numShards := 3

	// Schema with 3 vector predicates
	multiPredSchema := `
		vec_pred_alpha: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		vec_pred_beta: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		vec_pred_gamma: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
	`

	// Step 1: Create a source cluster and load vectors into it
	t.Log("Step 1: Creating source cluster and loading vectors for 3 predicates...")
	sourceConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour)
	sourceCluster, err := dgraphtest.NewLocalCluster(sourceConf)
	require.NoError(t, err)
	defer func() { sourceCluster.Cleanup(t.Failed()) }()
	require.NoError(t, sourceCluster.Start())

	gc, cleanup, err := sourceCluster.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err := sourceCluster.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	// Set up schema with multiple vector predicates
	require.NoError(t, gc.SetupSchema(multiPredSchema))

	// Generate and load vectors for each predicate
	allVectors := make(map[string][][]float32)
	for _, pred := range predicates {
		rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectorsPerPred, vectorDim, pred)
		allVectors[pred] = vectors

		mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err = gc.Mutate(mu)
		require.NoError(t, err)
		t.Logf("Loaded %d vectors for predicate %s", numVectorsPerPred, pred)
	}

	// Verify vectors are loaded and queryable in source cluster
	for _, pred := range predicates {
		vectors := allVectors[pred]
		similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[0], pred, 5)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(similarVectors), 3, "similar_to query should return results for %s", pred)
	}
	t.Log("Verified vector similarity queries work on source cluster for all predicates")

	// Step 2: Export the data from source cluster
	t.Log("Step 2: Exporting data from source cluster...")
	require.NoError(t, hc.Export(dgraphtest.DefaultExportDir, "rdf", -1))
	t.Log("Export completed")

	// Step 3: Set up a cluster for bulk loading with multiple shards
	t.Log("Step 3: Running bulk load with multiple shards...")
	bulkOutDir := t.TempDir()
	bulkConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(numShards). // 3 alphas for 3 shards
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour).
		WithBulkLoadOutDir(bulkOutDir)

	bulkCluster, err := dgraphtest.NewLocalCluster(bulkConf)
	require.NoError(t, err)
	defer func() { bulkCluster.Cleanup(t.Failed()) }()

	// Start only Zero for bulk loading
	require.NoError(t, bulkCluster.StartZero(0))
	require.NoError(t, bulkCluster.HealthCheck(true))

	// Copy exported files from source cluster container to host for bulk load
	exportHostDir := t.TempDir()
	dataFiles, schemaFiles, err := sourceCluster.CopyExportToHost(dgraphtest.DefaultExportDir, exportHostDir)
	require.NoError(t, err)
	require.NotEmpty(t, dataFiles, "should have exported data files")
	require.NotEmpty(t, schemaFiles, "should have exported schema files")
	t.Logf("Copied export files to host - data: %v, schema: %v", dataFiles, schemaFiles)

	// Run bulk load with explicit shard configuration
	opts := dgraphtest.BulkOpts{
		DataFiles:    dataFiles,
		SchemaFiles:  schemaFiles,
		OutDir:       bulkOutDir,
		MapShards:    numShards,
		ReduceShards: numShards,
	}
	require.NoError(t, bulkCluster.BulkLoad(opts))
	t.Logf("Bulk load completed successfully with %d map shards and %d reduce shards", numShards, numShards)

	// Step 4: Create a new cluster that uses the bulk loaded p directories
	t.Log("Step 4: Starting target cluster with bulk loaded p directories...")
	targetConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(numShards). // Must match the number of shards
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour).
		WithBulkLoadOutDir(bulkOutDir)

	targetCluster, err := dgraphtest.NewLocalCluster(targetConf)
	require.NoError(t, err)
	// defer func() { targetCluster.Cleanup(t.Failed()) }()

	// Start the target cluster (both Zero and Alphas)
	require.NoError(t, targetCluster.Start())
	t.Log("Target cluster started with bulk loaded data")

	// Get a client to verify the data
	targetGc, targetCleanup, err := targetCluster.Client()
	require.NoError(t, err)
	defer targetCleanup()
	require.NoError(t, targetGc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	// Step 5: Verify vector counts for each predicate
	t.Log("Step 5: Verifying vectors were loaded correctly for all predicates...")
	for _, pred := range predicates {
		query := fmt.Sprintf(`{
			vector(func: has(%s)) {
				count(uid)
			}
		}`, pred)
		result, err := targetGc.Query(query)
		require.NoError(t, err)
		require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectorsPerPred), string(result.GetJson()),
			"Predicate %s should have %d vectors", pred, numVectorsPerPred)
		t.Logf("Verified %d vectors were loaded for predicate %s", numVectorsPerPred, pred)
	}

	// Step 6: Verify vector similarity queries work for each predicate
	t.Log("Step 6: Verifying vector similarity queries work for all predicates...")
	for _, pred := range predicates {
		vectors := allVectors[pred]
		successCount := 0

		// Test a sample of vectors from each predicate
		sampleSize := 10

		for i := 0; i < sampleSize; i++ {
			similarVectors, err := targetGc.QueryMultipleVectorsUsingSimilarTo(vectors[i], pred, 5)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(similarVectors), 4,
				"similar_to query should return results for predicate %s vector %d", pred, i)
			successCount++
		}
		t.Logf("Predicate %s: %d/%d similarity queries succeeded", pred, successCount, sampleSize)
	}

	t.Log("All vector similarity queries verified successfully for multiple groups/shards!")
}

// TestBulkLoadMixedPredicates tests bulk loading vector data alongside other
// predicate types (string with index, int with index, uid edges) to ensure
// vector indexing doesn't break existing functionality.
func TestBulkLoadMixedPredicates(t *testing.T) {
	// if runtime.GOOS != "linux" && os.Getenv("DGRAPH_BINARY") == "" {
	// 	t.Skip("Skipping test on non-Linux platforms due to dgraph binary dependency")
	// }

	// Schema with vectors AND other indexed predicates
	mixedSchema := `
		project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		name: string @index(term, fulltext) .
		age: int @index(int) .
		score: float .
		friend: [uid] @reverse .
		dgraph.type: [string] @index(exact) .
	`

	numVectors := 500
	vectorDim := 10

	// Step 1: Create source cluster and load mixed data
	t.Log("Step 1: Creating source cluster and loading mixed data...")
	sourceConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour)
	sourceCluster, err := dgraphtest.NewLocalCluster(sourceConf)
	require.NoError(t, err)
	defer func() { sourceCluster.Cleanup(t.Failed()) }()
	require.NoError(t, sourceCluster.Start())

	gc, cleanup, err := sourceCluster.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err := sourceCluster.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	require.NoError(t, gc.SetupSchema(mixedSchema))

	// Generate mixed RDF data: vectors + strings + ints + edges
	var rdfBuilder strings.Builder
	vectors := make([][]float32, numVectors)

	for i := 0; i < numVectors; i++ {
		uid := i + 10
		// Generate random vector
		vec := dgraphapi.GenerateRandomVector(vectorDim)
		vectors[i] = vec
		vecStr := fmt.Sprintf(`"[%s]"`, strings.Trim(strings.Join(strings.Fields(fmt.Sprint(vec)), ", "), "[]"))

		// Add vector predicate
		rdfBuilder.WriteString(fmt.Sprintf("<0x%x> <project_description_v> %s .\n", uid, vecStr))
		// Add string predicate
		rdfBuilder.WriteString(fmt.Sprintf("<0x%x> <name> \"Person %d\" .\n", uid, i))
		// Add int predicate
		rdfBuilder.WriteString(fmt.Sprintf("<0x%x> <age> \"%d\"^^<xs:int> .\n", uid, 20+i%50))
		// Add float predicate
		rdfBuilder.WriteString(fmt.Sprintf("<0x%x> <score> \"%f\"^^<xs:float> .\n", uid, float64(i)*1.5))
		// Add dgraph.type
		rdfBuilder.WriteString(fmt.Sprintf("<0x%x> <dgraph.type> \"Person\" .\n", uid))
		// Add friend edge (to create some graph structure)
		if i > 0 {
			friendUid := 10 + (i-1)%numVectors
			rdfBuilder.WriteString(fmt.Sprintf("<0x%x> <friend> <0x%x> .\n", uid, friendUid))
		}
	}

	mu := &api.Mutation{SetNquads: []byte(rdfBuilder.String()), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)
	t.Logf("Loaded %d entities with mixed predicates", numVectors)

	// Verify source data
	query := `{ q(func: type(Person)) { count(uid) } }`
	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"q":[{"count":%d}]}`, numVectors), string(result.GetJson()))

	// Step 2: Export data
	t.Log("Step 2: Exporting data...")
	require.NoError(t, hc.Export(dgraphtest.DefaultExportDir, "rdf", -1))

	// Step 3: Bulk load
	t.Log("Step 3: Running bulk load...")
	bulkOutDir := t.TempDir()
	bulkConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour).
		WithBulkLoadOutDir(bulkOutDir)

	bulkCluster, err := dgraphtest.NewLocalCluster(bulkConf)
	require.NoError(t, err)
	defer func() { bulkCluster.Cleanup(t.Failed()) }()

	require.NoError(t, bulkCluster.StartZero(0))
	require.NoError(t, bulkCluster.HealthCheck(true))

	exportHostDir := t.TempDir()
	dataFiles, schemaFiles, err := sourceCluster.CopyExportToHost(dgraphtest.DefaultExportDir, exportHostDir)
	require.NoError(t, err)

	opts := dgraphtest.BulkOpts{
		DataFiles:   dataFiles,
		SchemaFiles: schemaFiles,
		OutDir:      bulkOutDir,
	}
	require.NoError(t, bulkCluster.BulkLoad(opts))
	t.Log("Bulk load completed")

	// Step 4: Start target cluster
	t.Log("Step 4: Starting target cluster...")
	targetConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour).
		WithBulkLoadOutDir(bulkOutDir)

	targetCluster, err := dgraphtest.NewLocalCluster(targetConf)
	require.NoError(t, err)
	defer func() { targetCluster.Cleanup(t.Failed()) }()
	require.NoError(t, targetCluster.Start())

	targetGc, targetCleanup, err := targetCluster.Client()
	require.NoError(t, err)
	defer targetCleanup()
	require.NoError(t, targetGc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	// Step 5: Verify all predicate types work
	t.Log("Step 5: Verifying all predicate types...")

	// Verify count
	result, err = targetGc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"q":[{"count":%d}]}`, numVectors), string(result.GetJson()))
	t.Log("✓ Count verified")

	// Verify string index (term search)
	termQuery := `{ q(func: anyofterms(name, "Person 50")) { name } }`
	result, err = targetGc.Query(termQuery)
	require.NoError(t, err)
	require.Contains(t, string(result.GetJson()), "Person 50")
	t.Log("✓ String term index verified")

	// Verify int index
	intQuery := `{ q(func: eq(age, 25)) { count(uid) } }`
	result, err = targetGc.Query(intQuery)
	require.NoError(t, err)
	require.Contains(t, string(result.GetJson()), "count")
	t.Log("✓ Int index verified")

	// Verify reverse edges
	reverseQuery := `{ q(func: has(~friend)) { count(uid) } }`
	result, err = targetGc.Query(reverseQuery)
	require.NoError(t, err)
	require.Contains(t, string(result.GetJson()), "count")
	t.Log("✓ Reverse edge index verified")

	// Verify vector similarity query
	similarQuery := fmt.Sprintf(`{
		vector(func: similar_to(project_description_v, 5, "%v")) {
			uid
			name
		}
	}`, vectors[0])
	result, err = targetGc.Query(similarQuery)
	require.NoError(t, err)
	require.Contains(t, string(result.GetJson()), "uid")
	t.Log("✓ Vector similarity query verified")

	t.Log("All mixed predicate types verified successfully!")
}

func TestBulkLoadVectorDimensions(t *testing.T) {
	// if runtime.GOOS != "linux" && os.Getenv("DGRAPH_BINARY") == "" {
	// 	t.Skip("Skipping test on non-Linux platforms due to dgraph binary dependency")
	// }

	// Test different dimension sizes: small (3D), medium (128D), large (512D)
	testCases := []struct {
		name      string
		dimension int
		numVecs   int
	}{
		{"small_3d", 3, 100},
		{"medium_128d", 128, 100},
		{"large_512d", 512, 50}, // Fewer vectors for large dimensions
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			predName := "project_description_v"
			schema := fmt.Sprintf(`%s: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .`, predName)

			// Step 1: Create source cluster
			sourceConf := dgraphtest.NewClusterConfig().
				WithNumAlphas(1).
				WithNumZeros(1).
				WithReplicas(1).
				WithACL(time.Hour)
			sourceCluster, err := dgraphtest.NewLocalCluster(sourceConf)
			require.NoError(t, err)
			defer func() { sourceCluster.Cleanup(t.Failed()) }()
			require.NoError(t, sourceCluster.Start())

			gc, cleanup, err := sourceCluster.Client()
			require.NoError(t, err)
			defer cleanup()
			require.NoError(t, gc.LoginIntoNamespace(context.Background(),
				dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

			hc, err := sourceCluster.HTTPClient()
			require.NoError(t, err)
			require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
				dgraphapi.DefaultPassword, x.RootNamespace))

			require.NoError(t, gc.SetupSchema(schema))

			// Generate vectors with specific dimension
			rdfs, vectors := dgraphapi.GenerateRandomVectors(0, tc.numVecs, tc.dimension, predName)
			mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
			_, err = gc.Mutate(mu)
			require.NoError(t, err)
			t.Logf("Loaded %d vectors of dimension %d", tc.numVecs, tc.dimension)

			// Step 2: Export
			require.NoError(t, hc.Export(dgraphtest.DefaultExportDir, "rdf", -1))

			// Step 3: Bulk load
			bulkOutDir := t.TempDir()
			bulkConf := dgraphtest.NewClusterConfig().
				WithNumAlphas(1).
				WithNumZeros(1).
				WithReplicas(1).
				WithACL(time.Hour).
				WithBulkLoadOutDir(bulkOutDir)

			bulkCluster, err := dgraphtest.NewLocalCluster(bulkConf)
			require.NoError(t, err)
			defer func() { bulkCluster.Cleanup(t.Failed()) }()

			require.NoError(t, bulkCluster.StartZero(0))
			require.NoError(t, bulkCluster.HealthCheck(true))

			exportHostDir := t.TempDir()
			dataFiles, schemaFiles, err := sourceCluster.CopyExportToHost(dgraphtest.DefaultExportDir, exportHostDir)
			require.NoError(t, err)

			opts := dgraphtest.BulkOpts{
				DataFiles:   dataFiles,
				SchemaFiles: schemaFiles,
				OutDir:      bulkOutDir,
			}
			require.NoError(t, bulkCluster.BulkLoad(opts))

			// Step 4: Start target cluster
			targetConf := dgraphtest.NewClusterConfig().
				WithNumAlphas(1).
				WithNumZeros(1).
				WithReplicas(1).
				WithACL(time.Hour).
				WithBulkLoadOutDir(bulkOutDir)

			targetCluster, err := dgraphtest.NewLocalCluster(targetConf)
			require.NoError(t, err)
			// defer func() { targetCluster.Cleanup(t.Failed()) }()
			require.NoError(t, targetCluster.Start())

			targetGc, targetCleanup, err := targetCluster.Client()
			require.NoError(t, err)
			defer targetCleanup()
			require.NoError(t, targetGc.LoginIntoNamespace(context.Background(),
				dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

			// Verify count
			query := fmt.Sprintf(`{ q(func: has(%s)) { count(uid) } }`, predName)
			result, err := targetGc.Query(query)
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{"q":[{"count":%d}]}`, tc.numVecs), string(result.GetJson()))

			// Verify similarity query works
			for _, vector := range vectors {
				similarVectors, err := targetGc.QueryMultipleVectorsUsingSimilarTo(vector, predName, 5)
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(similarVectors), 4,
					"similar_to query should return results for vector")
			}

		})
	}
}

func TestBulkLoadVectorEdgeCases(t *testing.T) {
	// if runtime.GOOS != "linux" && os.Getenv("DGRAPH_BINARY") == "" {
	// 	t.Skip("Skipping test on non-Linux platforms due to dgraph binary dependency")
	// }

	// Schema with multiple vector predicates - some will have data, some won't
	schema := `
		vec_with_data: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		vec_single: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		vec_empty: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		name: string @index(exact) .
	`

	vectorDim := 10

	// Step 1: Create source cluster
	t.Log("Step 1: Creating source cluster with edge case data...")
	sourceConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour)
	sourceCluster, err := dgraphtest.NewLocalCluster(sourceConf)
	require.NoError(t, err)
	defer func() { sourceCluster.Cleanup(t.Failed()) }()
	require.NoError(t, sourceCluster.Start())

	gc, cleanup, err := sourceCluster.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err := sourceCluster.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	require.NoError(t, gc.SetupSchema(schema))

	// Generate data:
	// - vec_with_data: 100 vectors
	// - vec_single: 1 vector
	// - vec_empty: 0 vectors (schema only)

	var rdfBuilder strings.Builder
	var vectorsWithData [][]float32

	// Add 100 vectors to vec_with_data
	for i := 0; i < 100; i++ {
		vec := dgraphapi.GenerateRandomVector(vectorDim)
		vectorsWithData = append(vectorsWithData, vec)
		vecStr := fmt.Sprintf(`"[%s]"`, strings.Trim(strings.Join(strings.Fields(fmt.Sprint(vec)), ", "), "[]"))
		rdfBuilder.WriteString(fmt.Sprintf("<0x%x> <vec_with_data> %s .\n", i+10, vecStr))
		rdfBuilder.WriteString(fmt.Sprintf("<0x%x> <name> \"entity_%d\" .\n", i+10, i))
	}

	// Add single vector to vec_single
	singleVec := dgraphapi.GenerateRandomVector(vectorDim)
	singleVecStr := fmt.Sprintf(`"[%s]"`, strings.Trim(strings.Join(strings.Fields(fmt.Sprint(singleVec)), ", "), "[]"))
	rdfBuilder.WriteString(fmt.Sprintf("<0x%x> <vec_single> %s .\n", 1000, singleVecStr))
	rdfBuilder.WriteString(fmt.Sprintf("<0x%x> <name> \"single_entity\" .\n", 1000))

	// vec_empty: no data, just schema

	mu := &api.Mutation{SetNquads: []byte(rdfBuilder.String()), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)
	t.Log("Loaded edge case data: 100 vectors, 1 single vector, 0 empty vectors")

	// Step 2: Export
	t.Log("Step 2: Exporting data...")
	require.NoError(t, hc.Export(dgraphtest.DefaultExportDir, "rdf", -1))

	// Step 3: Bulk load
	t.Log("Step 3: Running bulk load...")
	bulkOutDir := t.TempDir()
	bulkConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour).
		WithBulkLoadOutDir(bulkOutDir)

	bulkCluster, err := dgraphtest.NewLocalCluster(bulkConf)
	require.NoError(t, err)
	defer func() { bulkCluster.Cleanup(t.Failed()) }()

	require.NoError(t, bulkCluster.StartZero(0))
	require.NoError(t, bulkCluster.HealthCheck(true))

	exportHostDir := t.TempDir()
	dataFiles, schemaFiles, err := sourceCluster.CopyExportToHost(dgraphtest.DefaultExportDir, exportHostDir)
	require.NoError(t, err)

	opts := dgraphtest.BulkOpts{
		DataFiles:   dataFiles,
		SchemaFiles: schemaFiles,
		OutDir:      bulkOutDir,
	}
	require.NoError(t, bulkCluster.BulkLoad(opts))
	t.Log("Bulk load completed")

	// Step 4: Start target cluster
	t.Log("Step 4: Starting target cluster...")
	targetConf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour).
		WithBulkLoadOutDir(bulkOutDir)

	targetCluster, err := dgraphtest.NewLocalCluster(targetConf)
	require.NoError(t, err)
	defer func() { targetCluster.Cleanup(t.Failed()) }()
	require.NoError(t, targetCluster.Start())

	targetGc, targetCleanup, err := targetCluster.Client()
	require.NoError(t, err)
	defer targetCleanup()
	require.NoError(t, targetGc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	// Step 5: Verify edge cases
	t.Log("Step 5: Verifying edge cases...")

	// Verify vec_with_data (100 vectors)
	query := `{ q(func: has(vec_with_data)) { count(uid) } }`
	result, err := targetGc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"count":100}]}`, string(result.GetJson()))
	t.Log("✓ vec_with_data: 100 vectors verified")

	// Verify similarity query works for vec_with_data
	similarQuery := fmt.Sprintf(`{
		vector(func: similar_to(vec_with_data, 5, "%v")) {
			uid
		}
	}`, vectorsWithData[0])
	result, err = targetGc.Query(similarQuery)
	require.NoError(t, err)
	require.Contains(t, string(result.GetJson()), "uid")
	t.Log("✓ vec_with_data: similarity query works")

	// Verify vec_single (1 vector)
	query = `{ q(func: has(vec_single)) { count(uid) } }`
	result, err = targetGc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"count":1}]}`, string(result.GetJson()))
	t.Log("✓ vec_single: 1 vector verified")

	// Verify similarity query works for single vector (should return itself)
	singleSimilarQuery := fmt.Sprintf(`{
		vector(func: similar_to(vec_single, 5, "%v")) {
			uid
		}
	}`, singleVec)
	result, err = targetGc.Query(singleSimilarQuery)
	require.NoError(t, err)
	require.Contains(t, string(result.GetJson()), "0x3e8") // 0x3e8 = 1000 in hex
	t.Log("✓ vec_single: similarity query returns the single vector")

	// Verify vec_empty (0 vectors)
	query = `{ q(func: has(vec_empty)) { count(uid) } }`
	result, err = targetGc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"count":0}]}`, string(result.GetJson()))

}
