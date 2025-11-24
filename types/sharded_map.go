/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Package types contains some very common utilities used by Dgraph. These utilities
// are of "miscellaneous" nature, e.g., error checking.
package types

import (
	"sync"
)

type ShardedMap struct {
	shards []map[uint64]Val
}

const NumShards = 30

func NewShardedMap() *ShardedMap {
	shards := make([]map[uint64]Val, NumShards)
	for i := range shards {
		shards[i] = make(map[uint64]Val)
	}
	return &ShardedMap{shards: shards}
}

func (s *ShardedMap) Merge(other *ShardedMap, ag func(a, b Val) Val) {
	// TODO: ideally othermap should be the one which is smaller in size.
	var wg sync.WaitGroup
	for i := range s.shards {
		wg.Add(1)
		go func(i int) {
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
	return len(s.shards) == 0
}

func (s *ShardedMap) GetShardOrNil(key int) map[uint64]Val {
	if s == nil {
		return make(map[uint64]Val)
	}
	return s.shards[key]
}

func (s *ShardedMap) init() {
	if s == nil {
		*s = *NewShardedMap()
	}
}

func (s *ShardedMap) getShard(key uint64) map[uint64]Val {
	return s.shards[key%NumShards]
}

func (s *ShardedMap) Set(key uint64, value Val) {
	if s == nil {
		s.init()
	}
	shard := s.getShard(key)
	shard[key] = value
}

func (s *ShardedMap) Get(key uint64) (Val, bool) {
	if s == nil {
		return Val{}, false
	}
	shard := s.getShard(key)
	val, ok := shard[key]
	return val, ok
}

func (s *ShardedMap) Len() int {
	if s == nil {
		return 0
	}
	var count int
	for _, shard := range s.shards {
		count += len(shard)
	}
	return count
}

func (s *ShardedMap) Iterate(f func(uint64, Val) error) error {
	if s == nil {
		return nil
	}
	for _, shard := range s.shards {
		for k, v := range shard {
			if err := f(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
