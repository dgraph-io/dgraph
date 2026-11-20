//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

// reachabilityAfterEntryDeletion builds a vector index over numVectors random
// vectors, deletes the lowest-uid vector, restarts the alpha, then checks every
// surviving vector and returns how many can no longer find themselves at a wide
// beam (i.e. are unreachable in the graph).
//
// The lowest-uid vector matters: the build seats the first vector it inserts
// into a graph as that graph's entry node, the node every search for that graph
// starts from. Deleting an entry node must not make the rest of its graph
// unreachable — entry recovery has to reseat the entry on another live member
// of the same graph. For the partitioned index this holds per cluster: each
// cluster is an independent HNSW graph with its own entry node, so losing one
// cluster's entry must not cut that cluster off from search.
//
// The restart drives the full rebuild-and-replay path: the alpha replays the
// index alter from the WAL (re-running the build) while the delete is drained
// into the rebuilt index, which leaves the graph's entry pointing at a node
// whose vector is gone — the state entry recovery must handle.
func reachabilityAfterEntryDeletion(t *testing.T, idxSchema string, numVectors, dim int) int {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	const pred = "project_description_v"
	require.NoError(t, gc.DropAll())
	require.NoError(t, gc.SetupSchema(pred+`: float32vector .`))
	rdfs, vectors := dgraphapi.GenerateRandomVectors(0, numVectors, dim, pred)
	_, err = gc.Mutate(&api.Mutation{SetNquads: []byte(rdfs), CommitNow: true})
	require.NoError(t, err)
	require.NoError(t, gc.SetupSchema(idxSchema))

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	waitIndexed := func() {
		require.Eventually(t, func() bool {
			h, err := hc.HealthForInstance()
			return err == nil && !strings.Contains(string(h), "opIndexing")
		}, 120*time.Second, 500*time.Millisecond, "index build did not finish")
	}
	waitIndexed()

	// Delete the lowest-uid vector, which the build seats as a graph entry node.
	delUID := strings.Split(strings.Split(strings.TrimSpace(rdfs), "\n")[0], " ")[0]
	_, err = gc.Mutate(&api.Mutation{
		DelNquads: []byte(fmt.Sprintf("%s <%s> * .", delUID, pred)), CommitNow: true})
	require.NoError(t, err)

	// Restart: the alpha replays the index alter and drains the delete.
	require.NoError(t, c.StopAlpha(0))
	require.NoError(t, c.StartAlpha(0))
	require.NoError(t, c.HealthCheck(false))
	gc, cleanup2, err := c.Client()
	require.NoError(t, err)
	defer cleanup2()
	hc, err = c.HTTPClient()
	require.NoError(t, err)
	waitIndexed()
	require.Eventually(t, func() bool {
		top, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[1], pred, 1)
		return err == nil && len(top) == 1
	}, 60*time.Second, 500*time.Millisecond, "index not serving after restart")

	// vectors[0] was deleted; every other vector must find itself at a wide
	// beam. A vector missing from its own wide result set is unreachable.
	orphaned := 0
	for i := 1; i < len(vectors); i++ {
		wide, err := gc.QueryMultipleVectorsUsingSimilarTo(vectors[i], pred, 100)
		require.NoError(t, err)
		found := false
		for _, v := range wide {
			if fmt.Sprint(v) == fmt.Sprint(vectors[i]) {
				found = true
				break
			}
		}
		if !found {
			orphaned++
		}
	}
	return orphaned
}

// TestRestartReachabilityAfterEntryDeletion guards against a deleted entry node
// orphaning the rest of its graph after a restart. It covers both index
// layouts: monolithic, where the single graph's vectors are the whole index,
// and partitioned, where each cluster is its own graph but all clusters share
// one vector keyspace — so recovering a cluster's deleted entry has to walk that
// cluster's own graph rather than the shared vectors.
func TestRestartReachabilityAfterEntryDeletion(t *testing.T) {
	const (
		pred       = "project_description_v"
		dim        = 16
		numVectors = 1200
	)
	cases := []struct {
		name   string
		schema string
	}{
		{"monolithic", pred + `: float32vector @index(hnsw(efConstruction: "150", metric: "euclidean")) .`},
		{"partitioned", pred + `: float32vector @index(hnsw(numClusters: "32", numProbes: "8", partitionStratOpt: "kmeans", metric: "euclidean")) .`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orphaned := reachabilityAfterEntryDeletion(t, tc.schema, numVectors, dim)
			require.Zerof(t, orphaned,
				"%d vectors unreachable after deleting a graph entry node and restarting", orphaned)
		})
	}
}
