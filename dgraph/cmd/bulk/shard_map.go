/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bulk

import (
	"sync"

	"github.com/dgraph-io/dgraph/v25/x"
)

type shardMap struct {
	sync.RWMutex
	numShards   int
	predToShard map[string]int
	nextShard   int
	// pinned maps a namespaced predicate to its map shard, from --tablet_placement.
	// Immutable after construction, so shardFor reads it without holding the lock.
	pinned map[string]int
}

func newShardMap(numShards int, pinned map[string]int) *shardMap {
	return &shardMap{
		numShards:   numShards,
		predToShard: make(map[string]int),
		pinned:      pinned,
	}
}

func (m *shardMap) shardFor(pred string) int {
	// Always assign NQuads with reserved predicates to the first map shard.
	if x.IsReservedPredicate(pred) {
		return 0
	}
	// Pinned predicates bypass round-robin and never advance nextShard, so the
	// assignment of unpinned predicates is unaffected by pins.
	if shard, ok := m.pinned[pred]; ok {
		return shard
	}

	m.RLock()
	shard, ok := m.predToShard[pred]
	m.RUnlock()
	if ok {
		return shard
	}

	m.Lock()
	defer m.Unlock()
	shard, ok = m.predToShard[pred]
	if ok {
		return shard
	}

	shard = m.nextShard
	m.predToShard[pred] = shard
	m.nextShard = (m.nextShard + 1) % m.numShards
	return shard
}
