package posting

import (
	"sort"
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

type listSetShard struct {
	sync.RWMutex
	m map[uint64]struct{}
}

type listSet struct {
	numShards int
	shard     []*listSetShard
}

type shardSize struct {
	size, idx int
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

func newListSetShard() *listSetShard {
	return &listSetShard{m: make(map[uint64]struct{})}
}

// Size returns size of map.
func (s *listSetShard) Size() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.m)
}

// Get returns true if found.
func (s *listSetShard) Get(key uint64) bool {
	s.RLock()
	defer s.RUnlock()
	_, found := s.m[key]
	return found
}

// Put puts key into set.
func (s *listSetShard) Put(key uint64) {
	s.Lock()
	defer s.Unlock()
	s.m[key] = struct{}{}
}

// Delete deletes key from hash set.
func (s *listSetShard) Delete(key uint64) {
	s.Lock()
	defer s.Unlock()
	delete(s.m, key)
}

// MultiGet returns number of items written to buf. Number of items written is
// guaranteed to be <= size.
func (s *listSetShard) MultiGet(buf []uint64, size int) int {
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

func newShardedListSet(numShards int) *listSet {
	out := &listSet{
		numShards: numShards,
		shard:     make([]*listSetShard, numShards),
	}
	for i := 0; i < numShards; i++ {
		out.shard[i] = newListSetShard()
	}
	return out
}

// Size returns size of map.
func (s *listSet) Size() int {
	var size int
	for i := 0; i < s.numShards; i++ {
		size += s.shard[i].Size()
	}
	return size
}

// Get returns true if found.
func (s *listSet) Get(key uint64) bool {
	return s.shard[getShard(s.numShards, key)].Get(key)
}

// Put puts key into set.
func (s *listSet) Put(key uint64) {
	s.shard[getShard(s.numShards, key)].Put(key)
}

// Delete deletes key from hash set.
func (s *listSet) Delete(key uint64) {
	s.shard[getShard(s.numShards, key)].Delete(key)
}

type BySize []shardSize

func (s BySize) Len() int           { return len(s) }
func (s BySize) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s BySize) Less(i, j int) bool { return s[i].size > s[j].size }

// GetShardSizes return shard sizes in descending order of sizes.
func (s *listSet) GetShardSizes() []shardSize {
	out := make([]shardSize, s.numShards)
	for i := 0; i < dirtymap.numShards; i++ {
		out[i] = shardSize{dirtymap.shard[i].Size(), i}
	}
	sort.Sort(BySize(out))
	return out
}
