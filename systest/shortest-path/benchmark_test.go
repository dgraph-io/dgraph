//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/x"
)

// This file benchmarks Dgraph's `shortest` block under different
// `maxfrontiersize` and `numpaths` settings. It is the CI-tier benchmark for
// comparing the four open shortest-path PRs (#9576, #9599, #9607, #9678) that
// all target issue #9577 (k-shortest-path returns wrong paths when the
// frontier cap is hit).
//
// Two kinds of signal live here:
//
//  1. BenchmarkShortestPath_*           — latency / allocs across a matrix of
//     (numpaths, maxfrontiersize) values, on the shared `graph.rdf.gz`
//     fixture. Run via `go test -tags integration2 -bench=. -run=^$
//     ./systest/shortest-path/...`.
//  2. TestShortestPath_CapBoundCorrectness — diffs the path returned with a
//     tight cap against the no-cap ground truth. This is what fails on
//     buggy/regressed implementations (e.g. min-heap Pop eviction or
//     backpressure that converges to a suboptimal path). Skipped unless
//     DGRAPH_SHORTEST_PATH_CORRECTNESS=1 because it is expected to fail on
//     unfixed main; reviewers comparing PRs opt in explicitly.

const (
	// GUIDs from the existing graph.rdf.gz fixture; same pair used by
	// TestShortestPath in shortest_test.go.
	benchSourceGUID = "85270d10-560e-4cc8-8703-4b4c563a2f4e"
	benchDestGUID   = "4a520068-80b6-42f2-9019-4e6ef8a02bb3"

	correctnessEnv = "DGRAPH_SHORTEST_PATH_CORRECTNESS"
)

var (
	benchCluster  *dgraphtest.LocalCluster
	benchGC       *dgraphapi.GrpcClient
	benchOnce     sync.Once
	benchSetupErr error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if benchCluster != nil {
		benchCluster.Cleanup(false)
	}
	os.Exit(code)
}

// ensureCluster lazily brings up the shared LocalCluster and live-loads the
// shortest-path fixture. Called from each benchmark/test that needs it; the
// existing TestShortestPath in this package is untouched.
func ensureCluster(tb testing.TB) {
	tb.Helper()
	benchOnce.Do(func() {
		benchSetupErr = setupBenchCluster()
	})
	if benchSetupErr != nil {
		tb.Fatalf("bench cluster setup: %v", benchSetupErr)
	}
}

func setupBenchCluster() error {
	conf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).
		WithNumZeros(1).
		WithReplicas(1).
		WithACL(time.Hour)

	c, err := dgraphtest.NewLocalCluster(conf)
	if err != nil {
		return fmt.Errorf("NewLocalCluster: %w", err)
	}
	benchCluster = c

	if err := c.Start(); err != nil {
		return fmt.Errorf("Start: %w", err)
	}

	if err := c.LiveLoad(dgraphtest.LiveOpts{
		DataFiles:      []string{"graph.rdf.gz"},
		SchemaFiles:    []string{"graph.schema.gz"},
		GqlSchemaFiles: []string{},
	}); err != nil {
		return fmt.Errorf("LiveLoad: %w", err)
	}

	gc, _, err := c.Client()
	if err != nil {
		return fmt.Errorf("Client: %w", err)
	}
	if err := gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace); err != nil {
		return fmt.Errorf("LoginIntoNamespace: %w", err)
	}
	benchGC = gc
	return nil
}

// shortestPathQuery returns a DQL query that finds the shortest path between
// benchSourceGUID and benchDestGUID. maxFrontier <= 0 omits the cap entirely
// (ground-truth mode).
func shortestPathQuery(numPaths, maxFrontier int) string {
	frontier := ""
	if maxFrontier > 0 {
		frontier = fmt.Sprintf(", maxfrontiersize: %d", maxFrontier)
	}
	return fmt.Sprintf(`
{
    src(func: eq(guid, %q)) { a as uid }
    dst(func: eq(guid, %q)) { b as uid }
    p as shortest(from: uid(a), to: uid(b), numpaths: %d%s) {
        connected_to @facets(weight)
    }
    path(func: uid(p)) {
        uid
    }
}`, benchSourceGUID, benchDestGUID, numPaths, frontier)
}

type pathNode struct {
	UID string `json:"uid"`
}

type queryResult struct {
	Path []pathNode `json:"path"`
}

// pathUIDSet returns the sorted set of UIDs in the `path` block of a
// shortest-path response. Two correct implementations finding the same
// shortest path on a graph with a unique optimum will produce identical sets.
func pathUIDSet(respJSON []byte) ([]string, error) {
	var qr queryResult
	if err := json.Unmarshal(respJSON, &qr); err != nil {
		return nil, fmt.Errorf("decode response: %w (raw=%s)", err, string(respJSON))
	}
	if len(qr.Path) == 0 {
		return nil, nil
	}
	uids := make([]string, 0, len(qr.Path))
	seen := make(map[string]struct{}, len(qr.Path))
	for _, n := range qr.Path {
		if _, ok := seen[n.UID]; ok {
			continue
		}
		seen[n.UID] = struct{}{}
		uids = append(uids, n.UID)
	}
	sort.Strings(uids)
	return uids, nil
}

// benchCase parameterises a single benchmark sub-run.
type benchCase struct {
	name        string
	numPaths    int
	maxFrontier int // <= 0 disables the cap
}

// benchMatrix is the workload table shared by BenchmarkShortestPath. The cap
// values are mixed deliberately: small caps (5, 10) are tight enough to
// exercise the eviction path repeatedly; large caps (1000, 10000) are
// effectively unbounded for this fixture and serve as a baseline.
var benchMatrix = []benchCase{
	{"numpaths1_noCap", 1, 0},
	{"numpaths1_cap10", 1, 10},
	{"numpaths1_cap1000", 1, 1000},
	{"numpaths5_noCap", 5, 0},
	{"numpaths5_cap10", 5, 10},
	{"numpaths5_cap1000", 5, 1000},
	{"numpaths5_cap10000", 5, 10000},
}

func BenchmarkShortestPath(b *testing.B) {
	ensureCluster(b)
	for _, tc := range benchMatrix {
		q := shortestPathQuery(tc.numPaths, tc.maxFrontier)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := benchGC.Query(q); err != nil {
					b.Fatalf("query: %v", err)
				}
			}
		})
	}
}

// TestShortestPath_CapBoundCorrectness asserts that a tight maxfrontiersize
// does not change which nodes are returned in the shortest path. Skipped by
// default (see file-level comment); set DGRAPH_SHORTEST_PATH_CORRECTNESS=1
// to enable.
func TestShortestPath_CapBoundCorrectness(t *testing.T) {
	if os.Getenv(correctnessEnv) == "" {
		t.Skipf("skipped; set %s=1 to enable cap-bound correctness gate", correctnessEnv)
	}
	ensureCluster(t)

	type capCase struct {
		numPaths int
		caps     []int
	}
	cases := []capCase{
		{numPaths: 1, caps: []int{5, 10, 50, 100, 1000}},
		{numPaths: 5, caps: []int{10, 50, 100, 1000}},
	}

	for _, cc := range cases {
		truthResp, err := benchGC.Query(shortestPathQuery(cc.numPaths, 0))
		require.NoError(t, err, "ground-truth query (numpaths=%d) failed", cc.numPaths)
		truthSet, err := pathUIDSet(truthResp.Json)
		require.NoError(t, err)
		require.NotEmpty(t, truthSet,
			"ground-truth path empty for numpaths=%d — src/dst may not be connected", cc.numPaths)

		for _, cap := range cc.caps {
			cap := cap
			t.Run(fmt.Sprintf("numpaths=%d/cap=%d", cc.numPaths, cap), func(t *testing.T) {
				resp, err := benchGC.Query(shortestPathQuery(cc.numPaths, cap))
				require.NoError(t, err)
				got, err := pathUIDSet(resp.Json)
				require.NoError(t, err)
				require.Equal(t, truthSet, got,
					"cap=%d numpaths=%d diverged from ground truth\n"+
						"this means the implementation either evicted optimal nodes "+
						"or throttled exploration to a suboptimal path",
					cap, cc.numPaths)
			})
		}
	}
}
