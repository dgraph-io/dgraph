//go:build integration || cloud || upgrade

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Integration tests for MaxFrontierSize in shortest path queries.
//
// These tests verify the three bugs fixed in the frontier eviction logic:
//   1. Old code called pq.Pop() (removes arbitrary last element) instead of
//      removeMax() (removes highest-cost node).
//   2. Eviction happened BEFORE push, so the new node was never considered
//      for eviction — a high-cost newcomer could displace a low-cost existing node.
//   3. The boundary check used ">" instead of accounting for the push-then-evict
//      pattern, allowing the queue to temporarily exceed the limit.
//
// Test data uses the "connects" predicate with weight facets (nodes 51-55)
// and the hop test chain (nodes 56-60) from common_test.go.

// TestShortestPath_MaxFrontierSize_FindsOptimalPath verifies that with a
// frontier size limit, the shortest (lowest cost) path is still found.
// The optimal path 51→53→54→55 has cost 3.
// With maxfrontiersize=3, the frontier is tight but the optimal path should
// still be discoverable because removeMax evicts the highest-cost candidates.
func TestShortestPath_MaxFrontierSize_FindsOptimalPath(t *testing.T) {
	query := `
		{
			shortest(from: 51, to: 55, maxfrontiersize: 3) {
				connects @facets(weight)
			}
		}`
	js := processQueryNoErr(t, query)
	// The optimal path is 51→53→54→55 with total weight 3.
	require.JSONEq(t, `
		{
			"data": {
				"_path_": [
					{
						"connects": {
							"connects": {
								"connects": {
									"uid": "0x37",
									"connects|weight": 1
								},
								"uid": "0x36",
								"connects|weight": 1
							},
							"uid": "0x35",
							"connects|weight": 1
						},
						"uid": "0x33",
						"_weight_": 3
					}
				]
			}
		}
	`, js)
}

// TestShortestPath_MaxFrontierSize_VerySmall verifies behavior with
// maxfrontiersize=1 (the tightest possible constraint). Only one candidate
// is kept at a time, so the algorithm may not find the optimal path.
// We just verify it doesn't crash and returns a valid result.
func TestShortestPath_MaxFrontierSize_VerySmall(t *testing.T) {
	query := `
		{
			shortest(from: 51, to: 55, maxfrontiersize: 1) {
				connects @facets(weight)
			}
		}`
	// With frontier=1, the algorithm is extremely constrained.
	// It should either find a path or return empty — but not crash.
	_, err := processQuery(context.Background(), t, query)
	require.NoError(t, err)
}

// TestShortestPath_MaxFrontierSize_LargeEnough verifies that when the
// frontier is large enough to hold all candidates, the result is identical
// to an unconstrained query.
func TestShortestPath_MaxFrontierSize_LargeEnough(t *testing.T) {
	unconstrained := `
		{
			shortest(from: 51, to: 55) {
				connects @facets(weight)
			}
		}`
	constrained := `
		{
			shortest(from: 51, to: 55, maxfrontiersize: 1000) {
				connects @facets(weight)
			}
		}`
	jsUnconstrained := processQueryNoErr(t, unconstrained)
	jsConstrained := processQueryNoErr(t, constrained)
	require.JSONEq(t, jsUnconstrained, jsConstrained)
}

// TestKShortestPath_MaxFrontierSize_FindsOptimalPath verifies that
// k-shortest-path with a frontier limit still finds the optimal path.
func TestKShortestPath_MaxFrontierSize_FindsOptimalPath(t *testing.T) {
	query := `
		{
			shortest(from: 51, to: 55, numpaths: 2, maxfrontiersize: 5) {
				connects @facets(weight)
			}
		}`
	js := processQueryNoErr(t, query)
	// Should find at least the optimal path 51→53→54→55 with weight 3.
	require.Contains(t, js, `"_weight_":3`)
}

// TestKShortestPath_MaxFrontierSize_VerySmall verifies k-shortest-path
// doesn't crash with an extremely small frontier.
func TestKShortestPath_MaxFrontierSize_VerySmall(t *testing.T) {
	query := `
		{
			shortest(from: 51, to: 55, numpaths: 3, maxfrontiersize: 1) {
				connects @facets(weight)
			}
		}`
	_, err := processQuery(context.Background(), t, query)
	require.NoError(t, err)
}

// TestKShortestPath_MaxFrontierSize_LargeEnough verifies that a generous
// frontier limit produces the same result as no limit.
func TestKShortestPath_MaxFrontierSize_LargeEnough(t *testing.T) {
	unconstrained := `
		{
			shortest(from: 51, to: 55, numpaths: 5) {
				connects @facets(weight)
			}
		}`
	constrained := `
		{
			shortest(from: 51, to: 55, numpaths: 5, maxfrontiersize: 10000) {
				connects @facets(weight)
			}
		}`
	jsUnconstrained := processQueryNoErr(t, unconstrained)
	jsConstrained := processQueryNoErr(t, constrained)
	require.JSONEq(t, jsUnconstrained, jsConstrained)
}

// TestShortestPath_MaxFrontierSize_LinearChain tests frontier eviction on
// the linear chain 56→58→59→60 (each edge weight=1). With frontier=2,
// the algorithm must still find the 3-hop path.
func TestShortestPath_MaxFrontierSize_LinearChain(t *testing.T) {
	query := `
		{
			shortest(from: 56, to: 60, maxfrontiersize: 2) {
				connects @facets(weight)
			}
		}`
	js := processQueryNoErr(t, query)
	// Expected path: 56→58→59→60 with total weight 3.
	require.Contains(t, js, `"_weight_":3`)
	require.Contains(t, js, `"uid":"0x3c"`) // 60 = 0x3c
}

// TestShortestPath_MaxFrontierSize_EvictsHighCostNotLowCost is the key
// regression test for the old bug. In the graph from 51, the neighbors are:
//
//	51→53 (cost 1) — on the optimal path
//	51→52 (cost 11) — expensive
//	51→54 (cost 10) — expensive
//
// With maxfrontiersize=2, after exploring 51, the frontier has 3 candidates.
// The OLD code (pq.Pop()) removed an arbitrary element — potentially 53
// (cost 1), killing the optimal path.
// The NEW code (removeMax()) correctly evicts the highest-cost node,
// keeping the optimal path alive.
func TestShortestPath_MaxFrontierSize_EvictsHighCostNotLowCost(t *testing.T) {
	query := `
		{
			shortest(from: 51, to: 55, maxfrontiersize: 2) {
				connects @facets(weight)
			}
		}`
	js := processQueryNoErr(t, query)
	// With correct eviction, the optimal path 51→53→54→55 (cost 3) should
	// be found even with a very tight frontier. The key assertion is that
	// the result is non-empty (the old bug could cause path loss).
	require.Contains(t, js, `"_path_"`)
	require.Contains(t, js, `"uid":"0x37"`) // 55 = 0x37
}

// TestShortestPath_MaxFrontierSize_PushThenEvictOrder verifies the
// push-then-evict fix. If eviction happened BEFORE push (old bug), a
// high-cost new node would always be admitted. With push-then-evict,
// a high-cost newcomer is itself evicted if it's the worst candidate.
//
// We test this by using maxfrontiersize=2 on the 51-55 graph. Node 51
// expands to three neighbors (costs 1, 10, 11). With push-then-evict:
//
//	Push 53(1) → frontier [53(1)]
//	Push 54(10) → frontier [53(1), 54(10)]
//	Push 52(11) → frontier [53(1), 54(10), 52(11)] → evict 52(11)
//
// With the OLD evict-then-push:
//
//	frontier [53(1), 54(10)] → evict arbitrary → push 52(11)
//	Could end up with [54(10), 52(11)] — losing the optimal path node 53.
func TestShortestPath_MaxFrontierSize_PushThenEvictOrder(t *testing.T) {
	query := `
		{
			shortest(from: 51, to: 55, maxfrontiersize: 2) {
				connects @facets(weight)
			}
		}`
	js := processQueryNoErr(t, query)
	// If push-then-evict works correctly, the optimal path with weight 3
	// should be found.
	require.Contains(t, js, `"_weight_":3`, "optimal path should be found with push-then-evict")
}
