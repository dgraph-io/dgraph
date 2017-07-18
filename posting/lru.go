/*
Copyright 2013 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Modified by Dgraph Labs, Inc.

// Package lru implements an LRU cache.
package posting

import (
	"container/list"
	"context"
	"sync"

	"github.com/dgraph-io/dgraph/x"
)

// listCache is an LRU cache.
type listCache struct {
	sync.Mutex

	ctx context.Context
	// MaxSize is the maximum size of cache before an item is evicted.
	MaxSize uint64

	curSize uint64
	evicts  uint64
	ll      *list.List
	cache   map[uint64]*list.Element
}

type CacheStats struct {
	Length    int
	Size      uint64
	NumEvicts uint64
}

type entry struct {
	key  uint64
	pl   *List
	size uint64
}

// New creates a new Cache.
func newListCache(maxSize uint64) *listCache {
	return &listCache{
		ctx:     context.Background(),
		MaxSize: maxSize,
		ll:      list.New(),
		cache:   make(map[uint64]*list.Element),
	}
}

func (c *listCache) UpdateMaxSize() {
	c.Lock()
	defer c.Unlock()
	if c.curSize < (50 << 20) {
		c.MaxSize = 50 << 20
		x.Println("LRU cache max size is being set to 50 MB")
		return
	}
	x.LcacheCapacity.Set(int64(c.curSize))
	c.MaxSize = c.curSize
}

// TODO: fingerprint can collide
// Add adds a value to the cache.
func (c *listCache) PutIfMissing(key uint64, pl *List) (res *List) {
	c.Lock()
	defer c.Unlock()

	if ee, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ee)
		res = ee.Value.(*entry).pl
		res.incr()
		return res
	}

	e := &entry{
		key:  key,
		pl:   pl,
		size: uint64(pl.plist.Size()),
	}
	if e.size < 100 {
		e.size = 100
	}
	c.curSize += e.size
	ele := c.ll.PushFront(e)
	c.cache[key] = ele
	c.removeOldest()

	e.pl.incr()
	return e.pl
}

func (c *listCache) removeOldest() {
	for c.curSize > c.MaxSize {
		ele := c.ll.Back()
		if ele == nil {
			c.curSize = 0
			break
		}
		c.ll.Remove(ele)
		c.evicts++

		e := ele.Value.(*entry)
		c.curSize -= e.size

		e.pl.SetForDeletion()
		// If length of mutation layer is zero, then we won't call pstore.SetAsync and the
		// key wont be deleted from cache. So lets delete it now if SyncIfDirty returns false.
		if committed, _ := e.pl.SyncIfDirty(true); !committed {
			delete(c.cache, e.key)
			e.pl.decr()
		}
	}
}

// Get looks up a key's value from the cache.
func (c *listCache) Get(key uint64) (pl *List) {
	c.Lock()
	defer c.Unlock()

	if ele, hit := c.cache[key]; hit {
		c.ll.MoveToFront(ele)
		e := ele.Value.(*entry)
		est := uint64(e.pl.EstimatedSize())
		c.curSize += est - e.size
		e.size = est
		e.pl.incr()
		return e.pl
	}
	return nil
}

// Len returns the number of items in the cache.
func (c *listCache) Stats() CacheStats {
	c.Lock()
	defer c.Unlock()

	return CacheStats{
		Length:    c.ll.Len(),
		Size:      c.curSize,
		NumEvicts: c.evicts,
	}
}

func (c *listCache) Each(f func(key uint64, val *List)) {
	c.Lock()
	defer c.Unlock()

	ele := c.ll.Front()
	for ele != nil {
		e := ele.Value.(*entry)
		f(e.key, e.pl)
		ele = ele.Next()
	}
}

func (c *listCache) Reset() {
	c.Lock()
	defer c.Unlock()
	c.ll = list.New()
	c.cache = make(map[uint64]*list.Element)
	c.curSize = 0
}

// TODO: Remove it later
// Clear purges all stored items from the cache.
func (c *listCache) Clear() error {
	c.Lock()
	defer c.Unlock()
	for _, e := range c.cache {
		kv := e.Value.(*entry)
		kv.pl.SetForDeletion()
		if committed, _ := kv.pl.SyncIfDirty(true); !committed {
			delete(c.cache, kv.pl.ghash)
			kv.pl.decr()
		}
	}
	c.ll = list.New()
	c.cache = make(map[uint64]*list.Element)
	c.curSize = 0
	return nil
}

// This is called by DeletePredicate. We don't need to flush PL's to disk and can directly delete
// them from cache.
func (c *listCache) clear(attr string) error {
	c.Lock()
	defer c.Unlock()
	for k, e := range c.cache {
		kv := e.Value.(*entry)
		pk := x.Parse(kv.pl.key)
		if pk.Attr != attr {
			continue
		}
		c.ll.Remove(e)
		delete(c.cache, k)
		kv.pl.water.Ch <- x.Mark{Indices: kv.pl.pending, Done: true}
		kv.pl.decr()
	}
	return nil
}

// delete removes a key from cache
func (c *listCache) delete(key uint64) {
	c.Lock()
	defer c.Unlock()

	if ele, ok := c.cache[key]; ok {
		c.ll.Remove(ele)
		delete(c.cache, key)
		kv := ele.Value.(*entry)
		kv.pl.decr()
	}
}
