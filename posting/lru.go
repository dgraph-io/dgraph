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

// Add adds a value to the cache.
func (c *listCache) PutIfMissing(key uint64, pl *List) (res *List) {
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
		size: uint64(pl.plist.Size()),
	}
	c.curSize += e.size
	ele := c.ll.PushFront(e)
	c.cache[key] = ele

	for c.curSize > c.MaxSize {
		c.removeOldest()
	}
	return e.pl
}

// Get looks up a key's value from the cache.
func (c *listCache) Get(key uint64) (pl *List) {
	c.Lock()
	defer c.Unlock()

	if ele, hit := c.cache[key]; hit {
		c.ll.MoveToFront(ele)
		e := ele.Value.(*entry)
		est := e.pl.EstimatedSize()
		c.curSize += est - e.size
		e.size = est
		return e.pl
	}
	return nil
}

func (c *listCache) removeOldest() {
	ele := c.ll.Back()
	if ele != nil {
		c.evicts++

		e := ele.Value.(*entry)
		c.curSize -= e.pl.EstimatedSize()

		// TODO: Hanlde race later
		e.pl.SetForDeletion()
		e.pl.SyncIfDirty(true)
		e.pl.decr()
		c.ll.Remove(ele)
		delete(c.cache, e.key)
	}
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
}

// Clear purges all stored items from the cache.
// func (c *listCache) Clear() error {
// 	c.Lock()
// 	defer c.Unlock()
// 	for _, e := range c.cache {
// 		kv := e.Value.(*entry)
// 		_, err := kv.pl.SyncIfDirty(c.ctx)
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	c.ll = nil
// 	c.cache = nil
// 	c.curSize = 0
// 	return nil
// }
