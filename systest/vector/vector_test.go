//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/x"
)

const (
	testSchema             = `project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .`
	testSchemaWithoutIndex = `project_description_v: float32vector .`
	pred                   = "project_description_v"
	// numClusters is deliberately small relative to the ~1000-vector test
	// datasets (~125 vectors/cluster). numClusters == numVectors would put ~1
	// vector per cluster, so a default numProbes (numClusters/25) could only
	// gather a handful of candidates and topK-count assertions would fail.
	schemaVecDimension10 = `project_description_v: float32vector @index(hnsw(numClusters: "8", partitionStratOpt: "kmeans", vectorDimension: "10", metric: "euclidean")) .`
)

var schemas = map[string]string{
	"hnsw":            `project_description_v: float32vector @index(hnsw(exponent: "5", metric: "euclidean")) .`,
	"partitionedhnsw": `project_description_v: float32vector @index(hnsw(numClusters: "8", partitionStratOpt: "kmeans", vectorDimension: "100", metric: "euclidean")) .`,
}

func testVectorQuery(t *testing.T, gc *dgraphapi.GrpcClient, vectors [][]float32, rdfs, pred string, topk int) {
	for i, vector := range vectors {
		triple := strings.Split(rdfs, "\n")[i]
		uid := strings.Split(triple, " ")[0]
		queriedVector, err := gc.QuerySingleVectorsUsingUid(uid, pred)
		require.NoError(t, err)
		require.Equal(t, vectors[i], queriedVector[0])

		similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, topk)
		require.NoError(t, err)
		for _, similarVector := range similarVectors {
			require.Contains(t, vectors, similarVector)
		}
	}
}

func (vsuite *VectorTestSuite) TestVectorDropAll() {
	t := vsuite.T()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
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

	numVectors := 50

	testVectorSimilarTo := func(vectors [][]float32) {
		for _, vector := range vectors {
			_, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 100)
			require.ErrorContains(t, err, "is not indexed")
			break
		}
	}

	for i := 0; i < 10; i++ {
		require.NoError(t, gc.SetupSchema(vsuite.schema))
		rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
		mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err = gc.Mutate(mu)
		require.NoError(t, err)

		query := `{
			vector(func: has(project_description_v)) {
				   count(uid)
				}
		}`
		result, err := gc.Query(query)
		require.NoError(t, err)
		require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

		testVectorQuery(t, gc, vectors, rdfs, pred, numVectors)
		t.Log("dropping data \n")

		require.NoError(t, gc.DropAll())

		result, err = gc.Query(query)
		require.NoError(t, err)
		require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, 0), string(result.GetJson()))
		testVectorSimilarTo(vectors)
	}
}

func (vsuite *VectorTestSuite) TestVectorSnapshot() {
	t := vsuite.T()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(3).WithReplicas(3).WithACL(time.Hour)
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

	require.NoError(t, c.KillAlpha(1))

	hc, err = c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	gc, cleanup, err = c.AlphaClient(0)
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	require.NoError(t, gc.SetupSchema(vsuite.schema))

	prevSnapshotTs, err := hc.GetCurrentSnapshotTs(1)
	require.NoError(t, err)

	numVectors := 500
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	query := `{
		vector(func: has(project_description_v)) {
			   count(uid)
			}
	}`

	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	_, err = hc.WaitForSnapshot(1, prevSnapshotTs)
	require.NoError(t, err)

	require.NoError(t, c.StartAlpha(1))
	require.NoError(t, c.HealthCheck(false))

	time.Sleep(time.Second)

	gc, cleanup, err = c.AlphaClient(1)
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.Login(context.Background(), dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword))

	result, err = gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	testVectorQuery(t, gc, vectors, rdfs, pred, numVectors)

	// Self-recall on the restarted alpha: every indexed vector must find
	// itself. If the restart's replayed index rebuild raced the replayed
	// mutations, some vectors are permanently unreachable via similar_to
	// (the known reindex-vs-mutation restart race).
	for i, vector := range vectors {
		similar, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, numVectors)
		require.NoError(t, err)
		require.Containsf(t, similar, vector,
			"vector %d not found via similar_to after restart (reindex-vs-mutation race?)", i)
	}
}

func (vsuite *VectorTestSuite) TestVectorDropNamespace() {
	t := vsuite.T()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
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

	numVectors := 500
	for i := 0; i < 6; i++ {
		ns, err := hc.AddNamespace()
		require.NoError(t, err)
		require.NoError(t, gc.SetupSchema(vsuite.schema))
		rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
		mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err = gc.Mutate(mu)
		require.NoError(t, err)

		query := `{
			vector(func: has(project_description_v)) {
				   count(uid)
				}
		}`

		result, err := gc.Query(query)
		require.NoError(t, err)
		require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

		for _, vector := range vectors {
			similarVectors, err := gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, numVectors)
			require.NoError(t, err)
			for _, similarVector := range similarVectors {
				require.Contains(t, vectors, similarVector)
			}
		}
		_, err = hc.DeleteNamespace(ns)
		require.NoError(t, err)
	}
}

func (vsuite *VectorTestSuite) TestVectorIndexRebuilding() {
	t := vsuite.T()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
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

	require.NoError(t, gc.SetupSchema(vsuite.schema))

	numVectors := 1000
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)
	query := `{
		vector(func: has(project_description_v)) {
			   count(uid)
			}
	}`

	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	testVectorQuery(t, gc, vectors, rdfs, pred, numVectors)

	// drop index
	require.NoError(t, gc.SetupSchema(testSchemaWithoutIndex))

	// rebuild index
	require.NoError(t, gc.SetupSchema(vsuite.schema))

	// Rebuilding the HNSW index over pre-existing data is async; poll until ready.
	require.Eventually(t, func() bool {
		res, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[0], pred, 100)
		return err == nil && len(res) == 100
	}, 30*time.Second, 500*time.Millisecond, "vector index not ready after 30s")

	result, err = gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	testVectorQuery(t, gc, vectors, rdfs, pred, numVectors)
}

func (vsuite *VectorTestSuite) TestVectorIndexOnVectorPredWithoutData() {
	t := vsuite.T()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
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

	require.NoError(t, gc.SetupSchema(vsuite.schema))

	vector := []float32{1.0, 2.0, 3.0}
	_, err = gc.QueryMultipleVectorsUsingSimilarTo(vector, pred, 10)
	require.NoError(t, err)
}

func (vsuite *VectorTestSuite) TestVectorIndexDropPredicate() {
	t := vsuite.T()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
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

	numVectors := 1000

	// add vectors
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	require.NoError(t, gc.SetupSchema(vsuite.schema))

	for _, vect := range vectors {
		similarVects, err := gc.QueryMultipleVectorsUsingSimilarTo(vect, pred, 2)
		require.NoError(t, err)
		require.Equal(t, 2, len(similarVects))
	}

	query := `{
		vector(func: has(project_description_v)) {
			   count(uid)
			}
	}`

	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	// remove index from vector predicate
	require.NoError(t, gc.SetupSchema(testSchemaWithoutIndex))

	// drop predicate
	op := &api.Operation{
		DropAttr: pred,
	}
	require.NoError(t, gc.Alter(context.Background(), op))

	// generate random vectors
	rdfs, vectors = dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
	mu = &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	// add index back
	require.NoError(t, gc.SetupSchema(vsuite.schema))

	// Rebuilding the HNSW index over pre-existing data is async; poll until ready.
	require.Eventually(t, func() bool {
		res, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[0], pred, 100)
		return err == nil && len(res) == 100
	}, 30*time.Second, 500*time.Millisecond, "vector index not ready after 30s")

	result, err = gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	for _, vect := range vectors {
		similarVects, err := gc.QueryMultipleVectorsUsingSimilarTo(vect, pred, 100)
		require.NoError(t, err)
		require.Equal(t, 100, len(similarVects))
	}
}

func (vsuite *VectorTestSuite) TestVectorIndexWithoutSchema() {
	t := vsuite.T()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)

	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	numVectors := 1000

	// add vectors
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	require.NoError(t, gc.SetupSchema(vsuite.schema))

	// Building the HNSW index over pre-existing nodes is async. Poll a
	// sample query until the index is ready rather than sleeping a fixed
	// duration, which is unreliable on slower CI runners.
	require.Eventually(t, func() bool {
		res, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[0], pred, 100)
		return err == nil && len(res) == 100
	}, 30*time.Second, 500*time.Millisecond, "vector index not ready after 30s")

	for _, vect := range vectors {
		similarVects, err := gc.QueryMultipleVectorsUsingSimilarTo(vect, pred, 100)
		require.NoError(t, err)
		require.Equal(t, 100, len(similarVects))
	}

	query := `{
		vector(func: has(project_description_v)) {
			   count(uid)
			}
	}`

	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))
}

func (vsuite *VectorTestSuite) TestIndexRebuildingWithoutSchema() {
	t := vsuite.T()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, c.Start())

	defer func() { c.Cleanup(t.Failed()) }()

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	require.NoError(t, gc.DropAll())
	require.NoError(t, gc.SetupSchema(testSchemaWithoutIndex))

	numVectors := 1000
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)
	require.NoError(t, gc.SetupSchema(vsuite.schema))

	query := `{
		vector(func: has(project_description_v)) {
			   count(uid)
			}
	}`

	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))

	for _, vect := range vectors {
		similarVects, err := gc.QueryMultipleVectorsUsingSimilarTo(vect, pred, 100)
		require.NoError(t, err)
		require.Equal(t, 100, len(similarVects))
	}
}

func (vsuite *VectorTestSuite) TestVectorIndexWithoutSchemaWithoutIndex() {
	t := vsuite.T()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c, err := dgraphtest.NewLocalCluster(conf)

	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	numVectors := 1000

	// add vectors
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	require.NoError(t, gc.SetupSchema(vsuite.schema))

	for i, vect := range vectors {
		triple := strings.Split(rdfs, "\n")[i]
		uid := strings.Split(triple, " ")[0]
		queriedVector, err := gc.QuerySingleVectorsUsingUid(uid, pred)
		require.NoError(t, err)
		require.Equal(t, vect, queriedVector[0])
	}

	query := `{
		vector(func: has(project_description_v)) {
			   count(uid)
			}
	}`

	result, err := gc.Query(query)
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`{"vector":[{"count":%v}]}`, numVectors), string(result.GetJson()))
}

func (vsuite *VectorTestSuite) TestPartitionedHNSWIndex() {
	t := vsuite.T()

	if !vsuite.isForPartitionedIndex {
		t.Skip("Skipping TestPartitionedHNSWIndex for non partitioned index")
	}
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)

	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	defer cleanup()
	require.NoError(t, err)

	schemaWithoutIndex := `project_description_v: float32vector .`

	t.Run("with more than 1000 vectors", func(t *testing.T) {
		require.NoError(t, gc.DropAll())

		numVectors := 5000

		require.NoError(t, gc.SetupSchema(schemaWithoutIndex))
		rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
		mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err = gc.Mutate(mu)
		require.NoError(t, err)

		err = gc.SetupSchema(vsuite.schema)
		require.NoError(t, err)

		testVectorQuery(t, gc, vectors, rdfs, pred, 5)
	})

	t.Run("without providing vector dimension", func(t *testing.T) {
		require.NoError(t, gc.DropAll())

		numVectors := 1001

		require.NoError(t, gc.SetupSchema(schemaWithoutIndex))

		rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
		mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err = gc.Mutate(mu)
		require.NoError(t, err)

		s := `project_description_v: float32vector @index(hnsw` +
			`(numClusters:"1000", partitionStratOpt: "kmeans",metric: "euclidean")) .`
		err = gc.SetupSchema(s)
		require.NoError(t, err)

		testVectorQuery(t, gc, vectors, rdfs, pred, 1000)
	})

	t.Run("with less than 1000 vectors", func(t *testing.T) {
		require.NoError(t, gc.DropAll())
		numVectors := 100
		require.NoError(t, gc.SetupSchema(schemaWithoutIndex))

		rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, 100, pred)
		mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err = gc.Mutate(mu)
		require.NoError(t, err)

		err = gc.SetupSchema(vsuite.schema)
		require.NoError(t, err)

		testVectorQuery(t, gc, vectors, rdfs, pred, numVectors)
	})

	t.Run("with different length of vectors", func(t *testing.T) {
		require.NoError(t, gc.DropAll())
		numVectors := 1100
		require.NoError(t, gc.SetupSchema(schemaWithoutIndex))

		q := `schema {}`
		result, err := gc.Query(q)
		require.NoError(t, err)

		rdfs, _ := dgraphapi.GenerateRandomVectors(0, numVectors, 8, pred)
		mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err = gc.Mutate(mu)
		require.NoError(t, err)

		// The predicate already holds dimension-8 vectors. A float32vector
		// predicate has exactly one dimension, so declaring an index with
		// vectorDimension:100 is a genuine contradiction and must be rejected at
		// alter time, leaving the schema unchanged.
		err = gc.SetupSchema(vsuite.schema)
		require.Error(t, err)
		require.ErrorContains(t, err, "contradicts the existing vector dimension")

		// here check schema it should not be changed
		q = `schema {}`
		result1, err := gc.Query(q)
		require.NoError(t, err)
		require.JSONEq(t, string(result.GetJson()), string(result1.GetJson()))
	})
}

// TestPartitionedPipelines drives the four supported partitioned-hnsw
// pipelines end to end on one cluster: index build (schema alter over
// existing data), query routing, live mutations and deletes after the build,
// alpha restart (centroid re-hydration from disk), and a numClusters change
// (rebuild with a different layout).
func (vsuite *VectorTestSuite) TestPartitionedPipelines() {
	t := vsuite.T()
	if !vsuite.isForPartitionedIndex {
		t.Skip("Skipping TestPartitionedPipelines for non partitioned index")
	}

	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	defer cleanup()
	require.NoError(t, err)

	const (
		dim                = 16
		numVectors         = 1200
		schemaWithoutIndex = `project_description_v: float32vector .`
		indexSchema        = `project_description_v: float32vector @index(hnsw` +
			`(numClusters: "32", numProbes: "8", partitionStratOpt: "kmeans", metric: "euclidean")) .`
		reindexSchema = `project_description_v: float32vector @index(hnsw` +
			`(numClusters: "8", numProbes: "4", partitionStratOpt: "kmeans", metric: "euclidean")) .`
	)

	// A vector must always find itself: insert and query routing use the
	// same centroids, so top-1 of similar_to(v) is v regardless of how well
	// the clusters fit the data.
	requireSelfRecall := func(t *testing.T, vecs [][]float32, step int) {
		for i := 0; i < len(vecs); i += step {
			similar, err := gc.QueryMultipleVectorsUsingSimilarTo(vecs[i], pred, 1)
			require.NoError(t, err)
			require.Lenf(t, similar, 1, "vector %d: no result", i)
			if len(similar[0]) == len(vecs[i]) && fmt.Sprint(similar[0]) == fmt.Sprint(vecs[i]) {
				continue
			}
			// Distinguish ranking noise (present at a wide beam) from a
			// genuinely unreachable — orphaned — graph node.
			wide, err := gc.QueryMultipleVectorsUsingSimilarTo(vecs[i], pred, 100)
			require.NoError(t, err)
			found := false
			for _, v := range wide {
				if fmt.Sprint(v) == fmt.Sprint(vecs[i]) {
					found = true
					break
				}
			}
			t.Fatalf("vector %d did not find itself: top1=%v, in top100=%v (false = orphaned, true = ranking miss)",
				i, similar[0], found)
		}
	}

	// The restart subtest replaces gc with a fresh client that later
	// subtests keep using; close it at the end of the whole test.
	var restartCleanup func()
	defer func() {
		if restartCleanup != nil {
			restartCleanup()
		}
	}()

	require.NoError(t, gc.DropAll())
	require.NoError(t, gc.SetupSchema(schemaWithoutIndex))
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, dim, pred)
	_, err = gc.Mutate(&api.Mutation{SetNquads: []byte(rdfs), CommitNow: true})
	require.NoError(t, err)
	require.NoError(t, gc.SetupSchema(indexSchema))

	t.Run("build then query", func(t *testing.T) {
		requireSelfRecall(t, vectors, 40)
	})

	var liveVectors [][]float32
	t.Run("live inserts after build", func(t *testing.T) {
		var liveRdfs string
		liveRdfs, liveVectors = dgraphapi.GenerateRandomVectors(numVectors, numVectors+50, dim, pred)
		_, err := gc.Mutate(&api.Mutation{SetNquads: []byte(liveRdfs), CommitNow: true})
		require.NoError(t, err)
		requireSelfRecall(t, liveVectors, 5)
	})

	t.Run("deletes disappear from results", func(t *testing.T) {
		triple := strings.Split(rdfs, "\n")[0]
		uid := strings.Split(triple, " ")[0]
		delNquad := fmt.Sprintf("%s <%s> * .", uid, pred)
		_, err := gc.Mutate(&api.Mutation{DelNquads: []byte(delNquad), CommitNow: true})
		require.NoError(t, err)

		similar, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[0], pred, 5)
		require.NoError(t, err)
		require.NotContainsf(t, similar, vectors[0],
			"deleted vector still returned by similar_to")
	})

	t.Run("alpha restart rehydrates routing", func(t *testing.T) {
		// An alpha restart replays the schema alter from the raft WAL,
		// which re-runs the whole index rebuild while the replayed data
		// mutations arrive behind it. The vector capture gate (see
		// posting.StartVectorRebuildCapture) suppresses their index writes
		// and replays them into the finished index, so the replayed
		// mutations — including deletes — survive the restart intact.
		// Centroid hydration itself is covered deterministically by
		// posting/vector_restart_test.go.

		require.NoError(t, c.StopAlpha(0))
		require.NoError(t, c.StartAlpha(0))
		require.NoError(t, c.HealthCheck(false))

		gcr, cleanup2, err := c.Client()
		restartCleanup = cleanup2
		require.NoError(t, err)
		gc = gcr

		// The restart replays the schema mutation from the raft WAL, which
		// re-runs the whole index rebuild; the replayed data mutations are
		// captured by the rebuild gate and drained into the finished index.
		// Wait for the replayed rebuild to finish (health reports the
		// ongoing opIndexing task), then for the index to serve results.
		hc, err := c.HTTPClient()
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			health, err := hc.HealthForInstance()
			return err == nil && !strings.Contains(string(health), "opIndexing")
		}, 60*time.Second, 500*time.Millisecond, "replayed index rebuild still running after 60s")
		// Readiness: a wide query returns a healthy result set and the
		// probe vector finds itself. The set may be one short of k: the
		// vector deleted before the restart is tombstoned and filtered
		// from results — asserting len == k here would demand the OLD
		// buggy behavior, where the replayed delete was lost to the
		// rebuild race and the deleted vector resurrected.
		require.Eventually(t, func() bool {
			res, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[1], pred, 100)
			if err != nil || len(res) < 90 {
				return false
			}
			top, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[1], pred, 1)
			return err == nil && len(top) == 1 && fmt.Sprint(top[0]) == fmt.Sprint(vectors[1])
		}, 60*time.Second, 500*time.Millisecond, "vector index not ready after restart")

		// The delete must survive the restart: the WAL-replayed delete is
		// captured while the replayed rebuild runs and drained into the
		// dead list afterwards, so the deleted vector stays invisible.
		// (Before the capture gate this was the data-loss race: the
		// tombstone write raced the rebuild and the deleted vector came
		// back from the dead.)
		res, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[0], pred, 100)
		require.NoError(t, err)
		require.NotContainsf(t, res, vectors[0],
			"vector deleted before restart must stay deleted after the replayed rebuild")

		// Search routing must come back from the persisted centroids, and —
		// with the capture gate replaying the WAL-replayed mutations — no
		// vector may be lost: absence at a wide beam means an orphaned
		// graph node (the old race's signature), which is a hard failure.
		// Top-1 ranking keeps a small tolerance: it is ANN approximation,
		// not integrity.
		checked, found := 0, 0
		var orphaned []int
		for i := 0; i < len(vectors)-1; i += 40 {
			checked++
			top, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[1+i], pred, 1)
			require.NoError(t, err)
			if len(top) == 1 && fmt.Sprint(top[0]) == fmt.Sprint(vectors[1+i]) {
				found++
				continue
			}
			wide, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[1+i], pred, 100)
			require.NoError(t, err)
			present := false
			for _, v := range wide {
				if fmt.Sprint(v) == fmt.Sprint(vectors[1+i]) {
					present = true
					break
				}
			}
			if !present {
				orphaned = append(orphaned, 1+i)
			}
		}
		// KNOWN ISSUE (pre-existing, tracked separately): the rebuild scans
		// with 16+ concurrent stream workers whose unsynchronized
		// read-modify-writes of shared adjacency rows can orphan a node or
		// two per build — independent of the capture gate (reproduced with
		// zero mutations in flight). Tolerate at most 2 of the ~30 sampled
		// originals; the gate's own guarantee (replayed mutations are never
		// lost) is asserted with zero tolerance via the liveVectors and
		// post-insert sweeps.
		require.LessOrEqualf(t, len(orphaned), 2,
			"vectors unreachable after restart (orphaned graph nodes): %v", orphaned)
		require.GreaterOrEqualf(t, float64(found)/float64(checked), 0.9,
			"post-restart top-1 self-recall collapsed: %d/%d — centroid hydration broken?", found, checked)

		// The gate's own guarantee, zero tolerance: every mutation the
		// restart's replayed rebuild captured and drained must be reachable.
		for i := 0; i < len(liveVectors); i += 5 {
			wide, err := gc.QueryMultipleVectorsUsingSimilarTo(liveVectors[i], pred, 100)
			require.NoError(t, err)
			foundSelf := false
			for _, v := range wide {
				if fmt.Sprint(v) == fmt.Sprint(liveVectors[i]) {
					foundSelf = true
					break
				}
			}
			require.Truef(t, foundSelf,
				"drained vector %d unreachable after restart — replayed mutation lost", i)
		}

		// Insert routing must be exact: these vectors arrive after the
		// replayed rebuild, so the pre-existing race cannot touch them.
		postRdfs, postVectors := dgraphapi.GenerateRandomVectors(
			numVectors+50, numVectors+100, dim, pred)
		_, err = gc.Mutate(&api.Mutation{SetNquads: []byte(postRdfs), CommitNow: true})
		require.NoError(t, err)
		requireSelfRecall(t, postVectors, 5)
	})

	t.Run("numClusters change rebuilds", func(t *testing.T) {
		require.NoError(t, gc.SetupSchema(reindexSchema))
		requireSelfRecall(t, vectors[1:], 40)
		requireSelfRecall(t, liveVectors, 10)
	})
}

type VectorTestSuite struct {
	suite.Suite
	schema                string
	schemaVecDimesion10   string
	isForPartitionedIndex bool
}

func TestVectorSuite(t *testing.T) {
	for _, schema := range schemas {
		var ssuite VectorTestSuite
		ssuite.schema = schema
		if strings.Contains(schema, "numClusters") {
			ssuite.schemaVecDimesion10 = schemaVecDimension10
			ssuite.isForPartitionedIndex = true
		} else {
			ssuite.schemaVecDimesion10 = schema
		}
		suite.Run(t, &ssuite)
	}
	// Panic only after every schema iteration has run so that a failure in one
	// index mode does not skip the remaining tests; the process still exits
	// loudly if anything failed.
	if t.Failed() {
		x.Panic(errors.New("vector tests failed"))
	}
}
