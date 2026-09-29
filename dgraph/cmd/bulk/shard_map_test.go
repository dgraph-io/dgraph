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
	m := newShardMap(4, map[string]int{"0-payload": 2, "5-friend": 3})

	require.Equal(t, 2, m.shardFor("0-payload"))
	require.Equal(t, 3, m.shardFor("5-friend"))
	// The same predicate in another namespace is a different tablet and is not pinned.
	require.Equal(t, 0, m.shardFor("1-payload"))
}

func TestShardForReservedBeatsPin(t *testing.T) {
	// The placement parser rejects reserved predicates, so this map cannot be built from a
	// placement file; the check order still guarantees reserved predicates stay on shard 0.
	m := newShardMap(4, map[string]int{"0-dgraph.type": 3})
	require.Equal(t, 0, m.shardFor("0-dgraph.type"))
}

func TestShardForPinsDoNotAdvanceRoundRobin(t *testing.T) {
	pinned := map[string]int{"0-pinned": 2}
	m := newShardMap(3, pinned)

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

func TestShardForPinnedDeterministicUnderConcurrency(t *testing.T) {
	pinned := map[string]int{"0-p1": 1, "0-p2": 7, "3-p1": 4}
	m := newShardMap(8, pinned)

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
