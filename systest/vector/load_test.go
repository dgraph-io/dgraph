//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/x"
)

type Node struct {
	Uid       string    `json:"uid"`
	Namespace string    `json:"namespace"`
	Vtest     []float32 `json:"vtest"`
}

func (vsuite *VectorTestSuite) TestLiveLoadAndExportRDFFormat() {
	testExportAndLiveLoad(vsuite.T(), "rdf", vsuite.schemaVecDimesion10)
}

func testExportAndLiveLoad(t *testing.T, exportFormat string, schema string) {
	gc, hc := setupTest(t)
	exportDir := testExportDir(t)

	require.NoError(t, gc.SetupSchema(schema))

	numVectors := 1000
	pred := "project_description_v"
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 10, pred)

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err := gc.Mutate(mu)
	require.NoError(t, err)

	require.NoError(t, hc.Export(exportDir, exportFormat, -1))

	require.NoError(t, gc.DropAll())

	query := `{
		vector(func: has(project_description_v)) {
			   count(uid)
			}
	}`

	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, 0), string(result.GetJson()))

	require.NoError(t, sharedCluster.LiveLoadFromExport(exportDir))

	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	result, err = gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	testVectorQuery(t, gc, vectors, rdfs, pred, numVectors)
}

func TestBulkLoadVectorIndex(t *testing.T) {
	numVectors := 1000

	// Run the same bulk-load round trip for both the monolithic HNSW index and
	// the partitioned (numClusters) index. numClusters == 4 over 1000 vectors
	// keeps clustering meaningful (~250 vectors/cluster). Partitioned search is
	// approximate (IVF), so its recall thresholds are lower than monolithic's.
	modes := []struct {
		name             string
		schema           string
		minSourceSimilar int
		minTargetSimilar int
	}{
		{"monolithic", testSchema, 3, 4},
		{
			"partitioned",
			`project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean", numClusters: "4")) .`,
			2, 2,
		},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			// Step 1: Load vectors into the shared cluster (bulk-load source)
			gc, hc := setupTest(t)
			exportDir := testExportDir(t)

			// Set up vector schema and load vectors
			require.NoError(t, gc.SetupSchema(mode.schema))

			rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 10, pred)

			mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
			_, err := gc.Mutate(mu)
			require.NoError(t, err)

			// Verify vectors are loaded and queryable in source cluster
			for _, vector := range vectors[:5] { // Test first 5 vectors
				similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 5)
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(similarVectors), mode.minSourceSimilar,
					"similar_to query should return results")
			}

			// Step 2: Export the data from source cluster
			require.NoError(t, hc.Export(exportDir, "rdf", -1))

			// Step 3+4: Bulk load the export and start a target cluster on it
			targetGc := setupBulkTarget(t, exportDir, 1)

			// Step 5: Verify vector count
			query := `{
				vector(func: has(project_description_v)) {
					count(uid)
				}
			}`
			result, err := targetGc.Query(query)
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

			// Step 6: Verify vector similarity queries work (tests that vector index was built correctly)
			for i, vector := range vectors {
				similarVectors, err := targetGc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 5)
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(similarVectors), mode.minTargetSimilar,
					"similar_to query should return results for vector %d", i)
			}
		})
	}
}

func TestBulkLoadVectorIndexMultipleGroups(t *testing.T) {
	// Define 3 different vector predicates - each will potentially go to different shards
	predicates := []string{"vec_pred_alpha", "vec_pred_beta", "vec_pred_gamma"}
	numVectorsPerPred := 1000
	vectorDim := 10
	numShards := 3

	// Schema with 3 vector predicates. numClusters == 4 over 1000 vectors per
	// predicate keeps clustering meaningful; partitioned search is approximate
	// (IVF), so its recall thresholds are lower than monolithic's.
	monolithicSchema := `
		vec_pred_alpha: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		vec_pred_beta: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		vec_pred_gamma: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
	`
	partitionedSchema := `
		vec_pred_alpha: float32vector @index(hnsw(exponent: "5", metric: "euclidean", numClusters: "4")) .
		vec_pred_beta: float32vector @index(hnsw(exponent: "5", metric: "euclidean", numClusters: "4")) .
		vec_pred_gamma: float32vector @index(hnsw(exponent: "5", metric: "euclidean", numClusters: "4")) .
	`

	modes := []struct {
		name             string
		schema           string
		minSourceSimilar int
		minTargetSimilar int
	}{
		{"monolithic", monolithicSchema, 3, 4},
		{"partitioned", partitionedSchema, 2, 2},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			// Step 1: Load vectors into the shared cluster (bulk-load source)
			gc, hc := setupTest(t)
			exportDir := testExportDir(t)

			// Set up schema with multiple vector predicates
			require.NoError(t, gc.SetupSchema(mode.schema))

			// Generate and load vectors for each predicate
			allVectors := make(map[string][][]float32)
			for _, pred := range predicates {
				rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectorsPerPred, vectorDim, pred)
				allVectors[pred] = vectors

				mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
				_, err := gc.Mutate(mu)
				require.NoError(t, err)
			}

			// Verify vectors are loaded and queryable in source cluster
			for _, pred := range predicates {
				vectors := allVectors[pred]
				similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[0], pred, 5)
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(similarVectors), mode.minSourceSimilar,
					"similar_to query should return results for %s", pred)
			}

			// Step 2: Export the data from source cluster
			require.NoError(t, hc.Export(exportDir, "rdf", -1))

			// Step 3+4: Bulk load the export with multiple shards and start a
			// target cluster on the bulk-loaded p directories
			targetGc := setupBulkTarget(t, exportDir, numShards)

			// Step 5: Verify vector counts for each predicate
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
			}

			// Step 6: Verify vector similarity queries work for each predicate
			for _, pred := range predicates {
				vectors := allVectors[pred]

				// Test a sample of vectors from each predicate
				sampleSize := 10

				for i := 0; i < sampleSize; i++ {
					similarVectors, err := targetGc.QueryMultipleVectorsUsingSimilarTo(vectors[i], pred, 5)
					require.NoError(t, err)
					require.GreaterOrEqual(t, len(similarVectors), mode.minTargetSimilar,
						"similar_to query should return results for predicate %s vector %d", pred, i)
				}
			}
		})
	}
}

// TestBulkLoadMixedPredicates tests bulk loading vector data alongside other
// predicate types (string with index, int with index, uid edges) to ensure
// vector indexing doesn't break existing functionality.
func TestBulkLoadMixedPredicates(t *testing.T) {
	numVectors := 500
	vectorDim := 10

	// The vector predicate is exercised as both a monolithic and a partitioned
	// (numClusters) index; the surrounding non-vector predicates are unchanged.
	// numClusters == 4 over 500 vectors keeps clustering meaningful. The
	// similarity assertion here only checks the query returns a uid, which holds
	// for approximate (IVF) search too, so no recall loosening is needed.
	modes := []struct {
		name        string
		vectorIndex string
	}{
		{"monolithic", `project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .`},
		{"partitioned", `project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean", numClusters: "4")) .`},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			// Schema with vectors AND other indexed predicates
			mixedSchema := fmt.Sprintf(`
				%s
				name: string @index(term, fulltext) .
				age: int @index(int) .
				score: float .
				friend: [uid] @reverse .
				dgraph.type: [string] @index(exact) .
			`, mode.vectorIndex)

			// Step 1: Load mixed data into the shared cluster (bulk-load source)
			gc, hc := setupTest(t)
			exportDir := testExportDir(t)

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
			_, err := gc.Mutate(mu)
			require.NoError(t, err)

			// Verify source data
			query := `{ q(func: type(Person)) { count(uid) } }`
			result, err := gc.Query(query)
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{"q":[{"count":%d}]}`, numVectors), string(result.GetJson()))

			// Step 2: Export data
			require.NoError(t, hc.Export(exportDir, "rdf", -1))

			// Step 3+4: Bulk load the export and start a target cluster on it
			targetGc := setupBulkTarget(t, exportDir, 1)

			// Step 5: Verify all predicate types work

			// Verify count
			result, err = targetGc.Query(query)
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{"q":[{"count":%d}]}`, numVectors), string(result.GetJson()))

			// Verify string index (term search)
			termQuery := `{ q(func: anyofterms(name, "Person 50")) { name } }`
			result, err = targetGc.Query(termQuery)
			require.NoError(t, err)
			require.Contains(t, string(result.GetJson()), "Person 50")

			// Verify int index
			intQuery := `{ q(func: eq(age, 25)) { count(uid) } }`
			result, err = targetGc.Query(intQuery)
			require.NoError(t, err)
			require.Contains(t, string(result.GetJson()), "count")

			// Verify reverse edges
			reverseQuery := `{ q(func: has(~friend)) { count(uid) } }`
			result, err = targetGc.Query(reverseQuery)
			require.NoError(t, err)
			require.Contains(t, string(result.GetJson()), "count")

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
		})
	}
}

func TestBulkLoadVectorDimensions(t *testing.T) {
	// Test different dimension sizes: small (3D), medium (128D), large (512D)
	// against both the monolithic HNSW index and the partitioned (numClusters)
	// index. numClusters is kept small relative to the vector count so clustering
	// stays meaningful (4 for >=100 vectors, 2 otherwise). Partitioned search is
	// approximate (IVF), so its recall threshold (minSimilar) is lower than
	// monolithic's.
	testCases := []struct {
		name       string
		dimension  int
		numVecs    int
		schema     string
		minSimilar int
	}{
		{"small_3d/monolithic", 3, 100,
			`project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .`, 4},
		{"small_3d/partitioned", 3, 100,
			`project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean", numClusters: "4")) .`, 2},
		{"medium_128d/monolithic", 128, 100,
			`project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .`, 4},
		{"medium_128d/partitioned", 128, 100,
			`project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean", numClusters: "4")) .`, 2},
		{"large_512d/monolithic", 512, 50, // Fewer vectors for large dimensions
			`project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .`, 4},
		{"large_512d/partitioned", 512, 50, // 50 vectors -> 2 clusters keeps clustering meaningful
			`project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean", numClusters: "2")) .`, 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			predName := "project_description_v"

			// Step 1: Load vectors into the shared cluster (bulk-load source)
			gc, hc := setupTest(t)
			exportDir := testExportDir(t)

			require.NoError(t, gc.SetupSchema(tc.schema))

			// Generate vectors with specific dimension
			rdfs, vectors := dgraphapi.GenerateRandomVectors(0, tc.numVecs, tc.dimension, predName)
			mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
			_, err := gc.Mutate(mu)
			require.NoError(t, err)

			// Step 2: Export
			require.NoError(t, hc.Export(exportDir, "rdf", -1))

			// Step 3+4: Bulk load the export and start a target cluster on it
			targetGc := setupBulkTarget(t, exportDir, 1)

			// Verify count
			query := fmt.Sprintf(`{ q(func: has(%s)) { count(uid) } }`, predName)
			result, err := targetGc.Query(query)
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{"q":[{"count":%d}]}`, tc.numVecs), string(result.GetJson()))

			// Verify similarity query works. Partitioned (IVF) search is
			// approximate, so its recall threshold is lower than monolithic's;
			// the count equality above stays strict for both.
			for _, vector := range vectors {
				similarVectors, err := targetGc.QueryMultipleVectorsUsingSimilarTo(vector, predName, 5)
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(similarVectors), tc.minSimilar,
					"similar_to query should return results for vector")
			}
		})
	}
}

func TestBulkLoadVectorEdgeCases(t *testing.T) {
	// Schema with multiple vector predicates - some will have data, some won't
	schema := `
		vec_with_data: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		vec_single: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		vec_empty: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .
		name: string @index(exact) .
	`

	vectorDim := 10

	// Step 1: Load edge-case data into the shared cluster (bulk-load source)
	gc, hc := setupTest(t)
	exportDir := testExportDir(t)

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
	_, err := gc.Mutate(mu)
	require.NoError(t, err)

	// Step 2: Export
	require.NoError(t, hc.Export(exportDir, "rdf", -1))

	// Step 3+4: Bulk load the export and start a target cluster on it
	targetGc := setupBulkTarget(t, exportDir, 1)

	// Step 5: Verify edge cases

	// Verify vec_with_data (100 vectors)
	query := `{ q(func: has(vec_with_data)) { count(uid) } }`
	result, err := targetGc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"count":100}]}`, string(result.GetJson()))

	// Verify similarity query works for vec_with_data
	similarQuery := fmt.Sprintf(`{
		vector(func: similar_to(vec_with_data, 5, "%v")) {
			uid
		}
	}`, vectorsWithData[0])
	result, err = targetGc.Query(similarQuery)
	require.NoError(t, err)
	require.Contains(t, string(result.GetJson()), "uid")

	// Verify vec_single (1 vector)
	query = `{ q(func: has(vec_single)) { count(uid) } }`
	result, err = targetGc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"count":1}]}`, string(result.GetJson()))

	// Verify similarity query works for single vector (should return itself)
	singleSimilarQuery := fmt.Sprintf(`{
		vector(func: similar_to(vec_single, 5, "%v")) {
			uid
		}
	}`, singleVec)
	result, err = targetGc.Query(singleSimilarQuery)
	require.NoError(t, err)
	require.Contains(t, string(result.GetJson()), "0x3e8") // 0x3e8 = 1000 in hex

	// Verify vec_empty (0 vectors)
	query = `{ q(func: has(vec_empty)) { count(uid) } }`
	result, err = targetGc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, `{"q":[{"count":0}]}`, string(result.GetJson()))

}

func TestBulkLoadPartitionedVectorIndex(t *testing.T) {
	// Partitioned vector index schema (numClusters set)
	partitionedSchema := `
		project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean", numClusters: "4")) .
	`

	// Step 1: Load vectors into the shared cluster (bulk-load source)
	gc, hc := setupTest(t)
	exportDir := testExportDir(t)

	// Load the vectors FIRST (predicate typed but not indexed), then alter to
	// add the partitioned index. The alter-with-data path is the one that used
	// to leak an internal vectorDimension option into the persisted schema.
	require.NoError(t, gc.SetupSchema(`project_description_v: float32vector .`))

	numVectors := 1000
	pred := "project_description_v"
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 10, pred)

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err := gc.Mutate(mu)
	require.NoError(t, err)

	require.NoError(t, gc.SetupSchema(partitionedSchema))

	// Verify vectors are loaded and queryable in source cluster
	for _, vector := range vectors[:3] { // Test first 3 vectors
		similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 5)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(similarVectors), 2, "similar_to query should return results")
	}

	// Step 2: Export the data from source cluster
	require.NoError(t, hc.Export(exportDir, "rdf", -1))

	// Step 3+4: Bulk load the export and start a target cluster on it
	targetGc := setupBulkTarget(t, exportDir, 1)

	// Step 5: Verify vector count
	query := `{
		vector(func: has(project_description_v)) {
			count(uid)
		}
	}`
	result, err := targetGc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	// Regression lock: the round-tripped schema (source alter -> export -> bulk
	// -> target) must keep the user's numClusters and must NOT carry the
	// internal, derived vectorDimension option.
	schemaResp, err := targetGc.Query(`schema(pred: [project_description_v]) {type index tokenizer}`)
	require.NoError(t, err)
	require.Contains(t, string(schemaResp.GetJson()), "numClusters")
	require.NotContains(t, string(schemaResp.GetJson()), "vectorDimension")

	// Step 6: Verify vector similarity queries work (tests that partitioned index was built correctly)
	for i, vector := range vectors {
		similarVectors, err := targetGc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 5)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(similarVectors), 2,
			"similar_to query should return results for vector %d", i)
	}
}

// TestPartitionedVectorDimensionValidation pins the schema-alter validation of
// a user-set vectorDimension: a value contradicting the existing data must be
// rejected, while a matching value (and an absent one) is accepted.
func TestPartitionedVectorDimensionValidation(t *testing.T) {
	gc, _ := setupTest(t)

	const pred = "project_description_v"
	const dim = 10
	require.NoError(t, gc.SetupSchema(pred+`: float32vector .`))
	rdfs, _ := dgraphapi.GenerateRandomVectors(0, 200, dim, pred)
	_, err := gc.Mutate(&api.Mutation{SetNquads: []byte(rdfs), CommitNow: true})
	require.NoError(t, err)

	idx := func(opts string) string {
		return pred + `: float32vector @index(hnsw(metric: "euclidean", numClusters: "4"` + opts + `)) .`
	}

	// Contradicts the 10-d data → rejected.
	err = gc.SetupSchema(idx(`, vectorDimension: "7"`))
	require.Error(t, err, "a vectorDimension contradicting existing data must be rejected")

	// Non-positive → rejected.
	require.Error(t, gc.SetupSchema(idx(`, vectorDimension: "0"`)),
		"a non-positive vectorDimension must be rejected")

	// Matching the data → accepted.
	require.NoError(t, gc.SetupSchema(idx(`, vectorDimension: "10"`)),
		"a vectorDimension matching the data must be accepted")
}

// TestPartitionedVectorDimensionValidationUntyped is the regression lock for the
// "insert-before-declare" path: vectors are mutated BEFORE the predicate is typed
// float32vector, so they are stored in their raw text form, not packed float32.
// ExistingVectorDimension must NOT treat those text bytes as a float array (which
// would yield a bogus dimension of len(text)/4 and wrongly reject the alter); it
// must leave the dimension unknown and let the alter through, so the build can
// establish the true dimension. Before the fix this failed with a spurious
// "contradicts the existing vector dimension" error.
func TestPartitionedVectorDimensionValidationUntyped(t *testing.T) {
	gc, _ := setupTest(t)

	const pred = "project_description_v"
	const dim = 100

	// Mutate BEFORE declaring the predicate as float32vector: the values are
	// stored as their default text form, e.g. "[0.5, 7.8, ...]".
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, 500, dim, pred)
	_, err := gc.Mutate(&api.Mutation{SetNquads: []byte(rdfs), CommitNow: true})
	require.NoError(t, err)

	// Now attach the partitioned index declaring the matching vectorDimension.
	// This must be accepted (the text-stored data must not be misread as
	// dimension len(text)/4).
	idx := pred + `: float32vector @index(hnsw(metric: "euclidean", numClusters: "4", vectorDimension: "100")) .`
	require.NoError(t, gc.SetupSchema(idx),
		"attaching the index after loading untyped vectors must not be rejected as a dimension contradiction")

	// The index built over the (now typed) data must be queryable.
	for _, vector := range vectors {
		similar, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 10)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(similar), 1)
	}
}
