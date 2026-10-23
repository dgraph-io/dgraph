/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Package types contains some very common utilities used by Dgraph. These utilities
// are of "miscellaneous" nature, e.g., error checking.
package types

import (
	"sync"
)

const NumShards = 30

type ShardedMap struct {
	// shards is a fixed-size array, not a slice, so allocating a ShardedMap
	// performs exactly one heap allocation. Individual shard maps are created
	// lazily on first write, so workloads that touch few shards (or none)
	// pay near-zero allocation cost.
	shards [NumShards]map[uint64]Val
}

func NewShardedMap() *ShardedMap {
	return &ShardedMap{}
}

func (s *ShardedMap) Merge(other *ShardedMap, ag func(a, b Val) Val) {
	// TODO: ideally othermap should be the one which is smaller in size.
	var wg sync.WaitGroup
	for i := range s.shards {
		// Skip shards that are empty in both maps — no work, no goroutine launch.
		if len(other.shards[i]) == 0 {
			continue
		}
		wg.Add(1)
		go func(i int) {
			if s.shards[i] == nil {
				s.shards[i] = make(map[uint64]Val, len(other.shards[i]))
			}
			for k, v := range other.shards[i] {
				if _, ok := s.shards[i][k]; ok {
					s.shards[i][k] = ag(s.shards[i][k], v)
				} else {
					s.shards[i][k] = v
				}
			}
			wg.Done()
		}(i)
	}

	wg.Wait()
}

func (s *ShardedMap) IsEmpty() bool {
	if s == nil {
		return true
	}
	for i := range s.shards {
		if len(s.shards[i]) > 0 {
			return false
		}
	}
	return true
}

// GetShardOrNil returns the underlying map for the given shard, creating it
// if necessary. Callers may write through the returned reference; those writes
// persist in the ShardedMap. (The original implementation eagerly created all
// shards, so callers depended on the returned map being a live reference.)
func (s *ShardedMap) GetShardOrNil(key int) map[uint64]Val {
	if s == nil {
		return make(map[uint64]Val)
	}
	if s.shards[key] == nil {
		s.shards[key] = make(map[uint64]Val)
	}
	return s.shards[key]
}

// PeekShard returns the underlying shard map without allocating one if it does
// not yet exist. Callers MUST NOT write to the returned map — use GetShardOrNil
// for that. This is the right call for iterate-only / range-only access.
func (s *ShardedMap) PeekShard(key int) map[uint64]Val {
	if s == nil {
		return nil
	}
	return s.shards[key]
}

func (s *ShardedMap) init() {
	if s == nil {
		*s = *NewShardedMap()
	}
}

func (s *ShardedMap) getShardIdx(key uint64) int {
	return int(key % NumShards)
}

func (s *ShardedMap) Set(key uint64, value Val) {
	if s == nil {
		s.init()
	}
	idx := s.getShardIdx(key)
	if s.shards[idx] == nil {
		s.shards[idx] = make(map[uint64]Val)
	}
	s.shards[idx][key] = value
}

func (s *ShardedMap) Get(key uint64) (Val, bool) {
	if s == nil {
		return Val{}, false
	}
	idx := s.getShardIdx(key)
	if s.shards[idx] == nil {
		return Val{}, false
	}
	val, ok := s.shards[idx][key]
	return val, ok
}

func (s *ShardedMap) Len() int {
	if s == nil {
		return 0
	}
	var count int
	for i := range s.shards {
		count += len(s.shards[i])
	}
	return count
}

func (s *ShardedMap) Iterate(f func(uint64, Val) error) error {
	if s == nil {
		return nil
	}
	for i := range s.shards {
		if s.shards[i] == nil {
			continue
		}
		for k, v := range s.shards[i] {
			if err := f(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
