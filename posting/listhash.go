package posting

import (
	"sync"

	"github.com/zond/gotomic"
)

type stdListHash struct {
	sync.RWMutex
	m map[gotomic.IntKey]gotomic.Thing
}

type shardedListHash struct {
	numShards int
	h         []*stdListHash
}

func newStdListHash() *stdListHash {
	return &stdListHash{
		m: make(map[gotomic.IntKey]gotomic.Thing),
	}
}

// Size returns size of map.
func (s *stdListHash) Size() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.m)
}

// Get returns value for given key. Returns true if found.
func (s *stdListHash) Get(key gotomic.IntKey) (gotomic.Thing, bool) {
	s.RLock()
	defer s.RUnlock()
	val, found := s.m[key]
	return val, found
}

// PutIfMissing puts item into list if it is missing. Otherwise, do nothing.
func (s *stdListHash) PutIfMissing(key gotomic.IntKey, val gotomic.Thing) bool {
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
func (s *stdListHash) Delete(key gotomic.IntKey) (gotomic.Thing, bool) {
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
func (s *stdListHash) MultiGet(buf []gotomic.Hashable, size int) int {
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

func newShardedListHash(numShards int) *shardedListHash {
	out := &shardedListHash{
		numShards: numShards,
		h:         make([]*stdListHash, numShards),
	}
	for i := 0; i < numShards; i++ {
		out.h[i] = newStdListHash()
	}
	return out
}

// Size returns size of map.
func (s *shardedListHash) Size() int {
	// For now, just ignore tempMap.
	var size int
	for i := 0; i < int(s.numShards); i++ {
		size += s.h[i].Size()
	}
	return size
}

func (s *shardedListHash) getShard(key gotomic.IntKey) int {
	return int(((key % gotomic.IntKey(s.numShards)) + gotomic.IntKey(s.numShards)) % gotomic.IntKey(s.numShards))
}

// Get returns value for given key. Returns true if found.
func (s *shardedListHash) Get(key gotomic.IntKey) (gotomic.Thing, bool) {
	return s.h[s.getShard(key)].Get(key)
}

// PutIfMissing puts item into list if it is missing. Otherwise, do nothing.
func (s *shardedListHash) PutIfMissing(key gotomic.IntKey, val gotomic.Thing) bool {
	return s.h[s.getShard(key)].PutIfMissing(key, val)
}

// Delete deletes item from list if it is present. If not present, we return
// nil, false. Otherwise, we return the deleted value, true.
func (s *shardedListHash) Delete(key gotomic.IntKey) (gotomic.Thing, bool) {
	return s.h[s.getShard(key)].Delete(key)
}

// MultiGet returns number of items written to buf. Number of items written is
// guaranteed to be <= size.
func (s *shardedListHash) MultiGet(buf []gotomic.Hashable, size int) int {
	var idx int
	for i := 0; i < s.numShards; i++ {
		idx += s.h[i].MultiGet(buf[idx:], size/s.numShards)
	}
	return idx
}
