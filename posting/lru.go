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
	"sync/atomic"
	"time"

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
	cache   map[string]*list.Element
	done    int32
}

type CacheStats struct {
	Length    int
	Size      uint64
	NumEvicts uint64
}

type entry struct {
	key  string
	pl   *List
	size uint64
}

// New creates a new Cache.
func newListCache(maxSize uint64) *listCache {
	lc := &listCache{
		ctx:     context.Background(),
		MaxSize: maxSize,
		ll:      list.New(),
		cache:   make(map[string]*list.Element),
	}
	go lc.removeOldestLoop()
	return lc
}

func (c *listCache) UpdateMaxSize() {
	c.Lock()
	defer c.Unlock()
	if c.curSize < (50 << 20) {
		c.MaxSize = 50 << 20
		x.Println("LRU cache max size is being set to 50 MB")
		x.LcacheCapacity.Set(50 << 20)
		return
	}
	x.LcacheCapacity.Set(int64(c.curSize))
	c.MaxSize = c.curSize
}

// Add adds a value to the cache.
func (c *listCache) PutIfMissing(key string, pl *List) (res *List) {
	c.Lock()
	defer c.Unlock()

	if ee, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ee)
		res = ee.Value.(*entry).pl
		return res
	}

	e := &entry{
		key:  key,
		pl:   pl,
		size: uint64(pl.EstimatedSize()),
	}
	if e.size < 100 {
		e.size = 100
	}
	c.curSize += e.size
	ele := c.ll.PushFront(e)
	c.cache[key] = ele

	return e.pl
}

func (c *listCache) removeOldestLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		c.removeOldest()
		if atomic.LoadInt32(&c.done) > 0 {
			return
		}
	}
}

func (c *listCache) removeOldest() {
	c.Lock()
	defer c.Unlock()
	ele := c.ll.Back()
	for c.curSize > c.MaxSize && atomic.LoadInt32(&c.done) == 0 {
		if ele == nil {
			if c.curSize < 0 {
				c.curSize = 0
			}
			break
		}
		e := ele.Value.(*entry)

		if !e.pl.SetForDeletion() {
			ele = ele.Prev()
			continue
		}
		// If length of mutation layer is zero, then we won't call pstore.SetAsync and the
		// key wont be deleted from cache. So lets delete it now if SyncIfDirty returns false.
		if committed, err := e.pl.SyncIfDirty(true); err != nil {
			ele = ele.Prev()
			continue
		} else if !committed {
			delete(c.cache, e.key)
		}

		c.ll.Remove(ele)
		c.evicts++
		c.curSize -= e.size
		ele = ele.Prev()
	}
}

// Get looks up a key's value from the cache.
func (c *listCache) Get(key string) (pl *List) {
	c.Lock()
	defer c.Unlock()

	if ele, hit := c.cache[key]; hit {
		c.ll.MoveToFront(ele)
		e := ele.Value.(*entry)
		est := uint64(e.pl.EstimatedSize())
		c.curSize += est - e.size
		e.size = est
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

func (c *listCache) Each(f func(key []byte, val *List)) {
	c.Lock()
	defer c.Unlock()

	ele := c.ll.Front()
	for ele != nil {
		e := ele.Value.(*entry)
		f(e.pl.key, e.pl)
		ele = ele.Next()
	}
}

func (c *listCache) Reset() {
	c.Lock()
	defer c.Unlock()
	c.ll = list.New()
	c.cache = make(map[string]*list.Element)
	c.curSize = 0
}

func (c *listCache) iterate(cont func(l *List) bool) {
	c.Lock()
	defer c.Unlock()
	for _, e := range c.cache {
		kv := e.Value.(*entry)
		if !cont(kv.pl) {
			return
		}
	}
}

// Doesn't sync to disk, call this function only when you are deleting the pls.
func (c *listCache) clear(remove func(key []byte) bool) {
	c.Lock()
	defer c.Unlock()
	for k, e := range c.cache {
		kv := e.Value.(*entry)
		if !remove(kv.pl.key) {
			continue
		}

		c.ll.Remove(e)
		delete(c.cache, k)
	}
}

// delete removes a key from cache
func (c *listCache) delete(key []byte) {
	c.Lock()
	defer c.Unlock()

	if ele, ok := c.cache[string(key)]; ok {
		c.ll.Remove(ele)
		delete(c.cache, string(key))
	}
}
