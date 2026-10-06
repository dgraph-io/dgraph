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
	// baseShard is the first shard unpinned predicates round-robin over: with tablet
	// placement active and spare map shards available, shards below it are designated
	// for groups (reserved + pinned data), so unpinned predicates stay out of them and
	// remain eligible for size packing in the merge step.
	baseShard int
	// pinned maps a namespaced predicate to its map shard, from --tablet_placement.
	// Immutable after construction, so shardFor reads it without holding the lock.
	pinned map[string]int
}

func newShardMap(numShards, baseShard int, pinned map[string]int) *shardMap {
	return &shardMap{
		numShards:   numShards,
		predToShard: make(map[string]int),
		nextShard:   baseShard,
		baseShard:   baseShard,
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
	m.nextShard++
	if m.nextShard >= m.numShards {
		m.nextShard = m.baseShard
	}
	return shard
}
