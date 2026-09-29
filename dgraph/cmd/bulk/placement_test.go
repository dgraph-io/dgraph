/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bulk

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/x"
)

func TestBuildTabletPlacement(t *testing.T) {
	opt := &BulkOptions{ReduceShards: 3, Namespace: math.MaxUint64}

	placement, err := buildTabletPlacement(opt, []x.TabletPlacement{
		{Predicate: "name", Group: 1},
		{Predicate: "payload", Group: 2},
		{Predicate: "payload", Group: 3, Namespace: 22},
	})
	require.NoError(t, err)
	// Keys are namespaced attrs (hex namespace), values are group-1 = shard index.
	require.Equal(t, map[string]int{"0-name": 0, "0-payload": 1, "16-payload": 2}, placement)
}

func TestBuildTabletPlacementGroupOutOfRange(t *testing.T) {
	opt := &BulkOptions{ReduceShards: 2, Namespace: math.MaxUint64}

	_, err := buildTabletPlacement(opt, []x.TabletPlacement{
		{Predicate: "name", Group: 2},
		{Predicate: "payload", Group: 3},
		{Predicate: "friend", Group: 7},
	})
	require.ErrorContains(t, err, "reduce_shards(2)")
	require.ErrorContains(t, err, "payload -> group 3")
	require.ErrorContains(t, err, "friend -> group 7")
	require.NotContains(t, err.Error(), "name")
}

func TestBuildTabletPlacementForceNamespaceConflict(t *testing.T) {
	opt := &BulkOptions{ReduceShards: 4, Namespace: 5}

	// All data is forced into namespace 5; entries for any other namespace can never match.
	_, err := buildTabletPlacement(opt, []x.TabletPlacement{
		{Predicate: "name", Group: 1, Namespace: 0},
		{Predicate: "payload", Group: 2, Namespace: 0},
		{Predicate: "friend", Group: 3, Namespace: 3},
		{Predicate: "payload", Group: 4, Namespace: 5},
	})
	require.ErrorContains(t, err, "--force-namespace=5")
	require.ErrorContains(t, err, "[0] name")
	require.ErrorContains(t, err, "[0] payload")
	require.ErrorContains(t, err, "[3] friend")

	// Entries matching the forced namespace are fine.
	placement, err := buildTabletPlacement(opt, []x.TabletPlacement{
		{Predicate: "payload", Group: 4, Namespace: 5},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]int{"5-payload": 3}, placement)
}
