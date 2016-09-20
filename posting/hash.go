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
func (s *listMapShard) Size() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.m)
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
func (s *listMapShard) Get(key uint64) (*List, bool) {
	s.RLock()
	defer s.RUnlock()
	val, found := s.m[key]
	return val, found
}

// Get returns value for given key. Returns true if found.
func (s *listMap) Get(key uint64) (*List, bool) {
	return s.shard[getShard(s.numShards, key)].Get(key)
}

// PutIfMissing puts item into list. If key is missing, insertion happens and we
// return the new value. Otherwise, nothing happens and we return the old value.
func (s *listMapShard) PutIfMissing(key uint64, val *List) *List {
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
	return s.shard[getShard(s.numShards, key)].PutIfMissing(key, val)
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

// Delete deletes item from list if it is present. If not present, we return
// nil, false. Otherwise, we return the deleted value, true.
func (s *listMap) Delete(key uint64) (*List, bool) {
	return s.shard[getShard(s.numShards, key)].Delete(key)
}

// StreamUntilCap pushes keys into channel until it reaches capacity. Returns true
// if we reach cap.
func (s *listMapShard) StreamUntilCap(out chan uint64) bool {
	s.RLock()
	defer s.RUnlock()
	for key := range s.m {
		out <- key
		if len(out) == cap(out) {
			return true
		}
	}
	return false
}

// StreamUntilCap pushes keys into channel until it reaches capacity. Returns true
// if we reach cap.
func (s *listMap) StreamUntilCap(out chan uint64) {
	if len(out) == cap(out) {
		return
	}
	// We have tried using multiple goroutines to push into the channel, but has
	// seen little to no improvement in running time.
	for _, shard := range s.shard {
		if shard.StreamUntilCap(out) {
			break
		}
	}
}
