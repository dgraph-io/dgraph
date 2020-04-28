/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package posting

import (
	"sync"

	"github.com/dgraph-io/dgraph/protos/pb"

	"github.com/dgraph-io/ristretto"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/protobuf/proto"
)

func generateKey(key []byte, ts uint64) []byte {
	return nil
}

func copyList(l *List) *List {
	if l == nil {
		return nil
	}

	// No need to clone the immutable layer or the key since mutations will not modify it.
	lCopy := &List{
		key:   l.key,
		maxTs: l.maxTs,
		minTs: l.minTs,
		plist: l.plist,
	}

	if l.mutationMap != nil {
		lCopy.mutationMap = make(map[uint64]*pb.PostingList, len(l.mutationMap))
		for ts, pl := range l.mutationMap {
			lCopy.mutationMap[ts] = proto.Clone(pl).(*pb.PostingList)
		}
	}
	return lCopy
}

type PlCache struct {
	sync.RWMutex
	tsMap map[uint64]uint64
	cache *ristretto.Cache
}

func NewPlCache() (*PlCache, error) {
	cache := &PlCache{
		tsMap: make(map[uint64]uint64),
	}
	var err error
	cache.cache, err = ristretto.NewCache(&ristretto.Config{
		NumCounters: 200e6,
		MaxCost:     int64(Config.AllottedMemory * 1024 * 1024),
		BufferItems: 64,
		Metrics:     true,
		Cost: func(val interface{}) int64 {
			l, ok := val.(*List)
			if !ok {
				return int64(0)
			}
			return int64(l.DeepSize())
		},
	})
	if err != nil {
		return nil, err
	}
	return cache, nil
}

func (c *PlCache) Get(key []byte, ts uint64) *List {
	if c == nil || len(key) == 0 || ts == 0 {
		return nil
	}

	c.RLock()
	defer c.RUnlock()

	hash := z.MemHash(key)
	maxTs, ok := c.tsMap[hash]
	if !ok {
		return nil
	}
	if ts < maxTs {
		return nil
	}

	cacheKey := generateKey(key, ts)
	cachedVal, ok := c.cache.Get(cacheKey)
	if !ok {
		return nil
	}
	l, ok := cachedVal.(*List)
	if !ok {
		return nil
	}

	return copyList(l)
}

func (c *PlCache) Set(key []byte, ts uint64, pl *List) {
	if c == nil || len(key) == 0 || ts == 0 || pl == nil {
		return
	}

	c.Lock()
	defer c.Unlock()

	hash := z.MemHash(key)
	prevTs := c.tsMap[hash]
	if ts <= prevTs {
		return
	}

	prevKey := generateKey(key, prevTs)
	cacheKey := generateKey(key, ts)
	_ = c.cache.Set(cacheKey, copyList(pl), 0)
	c.cache.Del(prevKey)
}

func (c *PlCache) Del(key []byte) {
	if c == nil || len(key) == 0 {
		return
	}

	c.Lock()
	defer c.Unlock()
	hash := z.MemHash(key)
	prevTs, ok := c.tsMap[hash]
	if !ok {
		return
	}

	delete(c.tsMap, hash)
	c.cache.Del(generateKey(key, prevTs))
}

func (c *PlCache) Clear() {
	if c == nil {
		return
	}

	c.Lock()
	defer c.Unlock()
	c.tsMap = make(map[uint64]uint64)
	c.cache.Clear()
}

// ClearCache will clear the entire list cache.
func ClearCache() {
	plCache.Clear()
}

// RemoveCachedKeys will delete the cached list by this transaction.
func (txn *Txn) RemoveCachedKeys() {
	if txn == nil || txn.cache == nil {
		return
	}

	for key := range txn.cache.deltas {
		plCache.Del([]byte(key))
	}
}
