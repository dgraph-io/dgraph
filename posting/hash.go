/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package posting

import "sync"

type listMapShard struct {
	sync.RWMutex
	m map[uint64]*List
}

type listMap struct {
	numShards int
	shard     []*listMapShard
}

func getShard(numShards int, key uint64) int {
	return int(key % uint64(numShards))
}

func newShardedListMap(numShards int) *listMap {
	out := &listMap{
		numShards: numShards,
		shard:     make([]*listMapShard, numShards),
	}
	for i := 0; i < numShards; i++ {
		out.shard[i] = &listMapShard{m: make(map[uint64]*List)}
	}
	return out
}

// Size returns size of map.
func (s *listMapShard) size() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.m)
}

// Size returns size of map.
func (s *listMap) Size() int {
	var size int
	for i := 0; i < s.numShards; i++ {
		size += s.shard[i].size()
	}
	return size
}

// Get returns value for given key. Returns true if found.
func (s *listMapShard) get(key uint64) *List {
	s.RLock()
	defer s.RUnlock()
	val := s.m[key]
	return val
}

// Get returns value for given key.
func (s *listMap) Get(key uint64) *List {
	return s.shard[getShard(s.numShards, key)].get(key)
}

// PutIfMissing puts item into list. If key is missing, insertion happens and we
// return the new value. Otherwise, nothing happens and we return the old value.
func (s *listMapShard) putIfMissing(key uint64, val *List) *List {
	s.Lock()
	defer s.Unlock()
	oldVal := s.m[key]
	if oldVal != nil {
		return oldVal
	}
	s.m[key] = val
	return val
}

// PutIfMissing puts item into list. If key is missing, insertion happens and we
// return the new value. Otherwise, nothing happens and we return the old value.
func (s *listMap) PutIfMissing(key uint64, val *List) *List {
	return s.shard[getShard(s.numShards, key)].putIfMissing(key, val)
}

func (s *listMapShard) eachWithDelete(f func(key uint64, val *List)) {
	s.Lock()
	defer s.Unlock()
	for k, v := range s.m {
		delete(s.m, k)
		f(k, v)
	}
}

// EachWithDelete iterates over listMap and for each key, value pair, deletes the
// key and calls the given function.
func (s *listMap) EachWithDelete(f func(key uint64, val *List)) {
	for _, shard := range s.shard {
		shard.eachWithDelete(f)
	}
}

func (s *listMapShard) each(f func(key uint64, val *List)) {
	s.Lock()
	defer s.Unlock()
	for k, v := range s.m {
		f(k, v)
	}
}

// Each iterates over listMap and for each key, value pair, and calls the
// given function.
func (s *listMap) Each(f func(key uint64, val *List)) {
	for _, shard := range s.shard {
		shard.each(f)
	}
}
