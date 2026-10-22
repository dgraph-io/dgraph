/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bulk

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShardForPinned(t *testing.T) {
	m := newShardMap(4, 0, map[string]int{"0-payload": 2, "5-friend": 3})

	require.Equal(t, 2, m.shardFor("0-payload"))
	require.Equal(t, 3, m.shardFor("5-friend"))
	// The same predicate in another namespace is a different tablet and is not pinned.
	require.Equal(t, 0, m.shardFor("1-payload"))
}

func TestShardForReservedBeatsPin(t *testing.T) {
	// The placement parser rejects reserved predicates, so this map cannot be built from a
	// placement file; the check order still guarantees reserved predicates stay on shard 0.
	m := newShardMap(4, 0, map[string]int{"0-dgraph.type": 3})
	require.Equal(t, 0, m.shardFor("0-dgraph.type"))
}

func TestShardForPinsDoNotAdvanceRoundRobin(t *testing.T) {
	pinned := map[string]int{"0-pinned": 2}
	m := newShardMap(3, 0, pinned)

	// Unpinned predicates get the same round-robin assignment they would get with no pins.
	require.Equal(t, 0, m.shardFor("0-a"))
	require.Equal(t, 2, m.shardFor("0-pinned"))
	require.Equal(t, 1, m.shardFor("0-b"))
	require.Equal(t, 2, m.shardFor("0-pinned"))
	require.Equal(t, 2, m.shardFor("0-c"))
	require.Equal(t, 0, m.shardFor("0-d"))
	// Memoized assignments are stable.
	require.Equal(t, 0, m.shardFor("0-a"))
}

func TestShardForUnpinnedStayOutOfGroupShards(t *testing.T) {
	// With placement active and spare map shards (map_shards=6, reduce_shards=3), unpinned
	// predicates round-robin over shards [3, 6) only, so they stay eligible for size
	// packing instead of riding a group-designated shard by arrival order.
	m := newShardMap(6, 3, map[string]int{"0-payload": 1})

	require.Equal(t, 3, m.shardFor("0-a"))
	require.Equal(t, 1, m.shardFor("0-payload")) // pinned, untouched by the base
	require.Equal(t, 4, m.shardFor("0-b"))
	require.Equal(t, 5, m.shardFor("0-c"))
	require.Equal(t, 3, m.shardFor("0-d")) // wraps to base, not to 0
	require.Equal(t, 0, m.shardFor("0-dgraph.type"))
	require.Equal(t, 3, m.shardFor("0-a")) // memoized
}

func TestUnpinnedBaseShard(t *testing.T) {
	pins := map[string]int{"0-p": 0}
	for _, tc := range []struct {
		opt  BulkOptions
		want int
	}{
		{BulkOptions{MapShards: 6, ReduceShards: 3, tabletPlacement: pins}, 3},
		{BulkOptions{MapShards: 3, ReduceShards: 3, tabletPlacement: pins}, 0}, // no spare shards
		{BulkOptions{MapShards: 6, ReduceShards: 3}, 0},                        // no placement
	} {
		require.Equal(t, tc.want, unpinnedBaseShard(&tc.opt))
	}
}

func TestShardForPinnedDeterministicUnderConcurrency(t *testing.T) {
	pinned := map[string]int{"0-p1": 1, "0-p2": 7, "3-p1": 4}
	m := newShardMap(8, 0, pinned)

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				require.Equal(t, 1, m.shardFor("0-p1"))
				require.Equal(t, 7, m.shardFor("0-p2"))
				require.Equal(t, 4, m.shardFor("3-p1"))
				m.shardFor("0-unpinned")
			}
		}()
	}
	wg.Wait()
	// Unpinned predicates still land within range and stay memoized.
	got := m.shardFor("0-unpinned")
	require.GreaterOrEqual(t, got, 0)
	require.Less(t, got, 8)
}
