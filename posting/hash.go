package posting

import (
	"sync"
)

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

func newListMapShard() *listMapShard {
	return &listMapShard{m: make(map[uint64]*List)}
}

// Size returns size of map.
func (s *listMapShard) Size() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.m)
}

// Get returns value for given key. Returns true if found.
func (s *listMapShard) Get(key uint64) (*List, bool) {
	s.RLock()
	defer s.RUnlock()
	val, found := s.m[key]
	return val, found
}

// PutIfMissing puts item into list if it is missing. Otherwise, do nothing.
func (s *listMapShard) PutIfMissing(key uint64, val *List) bool {
	s.Lock()
	defer s.Unlock()
	// Remark: Go maps feel inefficient as this feels like two calls when we should
	// do only one!
	_, found := s.m[key]
	if found {
		// Found, don't insert!
		return false
	}
	s.m[key] = val
	return true
}

// Delete deletes item from list if it is present. If not present, we return
// nil, false. Otherwise, we return the deleted value, true.
func (s *listMapShard) Delete(key uint64) (*List, bool) {
	s.Lock()
	defer s.Unlock()
	val, found := s.m[key]
	if !found {
		// Not found, nothing to delete!
		return nil, false
	}
	delete(s.m, key)
	return val, true
}

// MultiGet returns number of items written to buf. Number of items written is
// guaranteed to be <= size.
func (s *listMapShard) MultiGet(buf []uint64, size int) int {
	s.RLock()
	defer s.RUnlock()
	var idx int
	for k := range s.m {
		buf[idx] = k
		idx++
		if idx >= size {
			break
		}
	}
	return idx
}

func newShardedListMap(numShards int) *listMap {
	out := &listMap{
		numShards: numShards,
		shard:     make([]*listMapShard, numShards),
	}
	for i := 0; i < numShards; i++ {
		out.shard[i] = newListMapShard()
	}
	return out
}

// Size returns size of map.
func (s *listMap) Size() int {
	var size int
	for i := 0; i < s.numShards; i++ {
		size += s.shard[i].Size()
	}
	return size
}

// Get returns value for given key. Returns true if found.
func (s *listMap) Get(key uint64) (*List, bool) {
	return s.shard[getShard(s.numShards, key)].Get(key)
}

// PutIfMissing puts item into list if it is missing. Otherwise, do nothing.
func (s *listMap) PutIfMissing(key uint64, val *List) bool {
	return s.shard[getShard(s.numShards, key)].PutIfMissing(key, val)
}

// Delete deletes item from list if it is present. If not present, we return
// nil, false. Otherwise, we return the deleted value, true.
func (s *listMap) Delete(key uint64) (*List, bool) {
	return s.shard[getShard(s.numShards, key)].Delete(key)
}

// MultiGet returns number of items written to buf. Number of items written is
// guaranteed to be <= size.
func (s *listMap) MultiGet(buf []uint64, size int) int {
	var idx int
	for i := 0; i < s.numShards; i++ {
		idx += s.shard[i].MultiGet(buf[idx:], size/s.numShards)
	}
	return idx
}
