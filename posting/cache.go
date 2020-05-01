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
	"encoding/binary"

	"github.com/dgraph-io/dgraph/protos/pb"

	"github.com/dgraph-io/ristretto"
)

const (
	numShards = 16
)

func generateKey(key []byte, version uint64) []byte {
	if len(key) == 0 {
		return nil
	}

	cacheKey := make([]byte, len(key)+8)
	copy(cacheKey, key)
	binary.BigEndian.PutUint64(cacheKey[len(key):], version)
	return cacheKey
}

type PlCache struct {
	cache *ristretto.Cache
}

func NewPlCache() (*PlCache, error) {
	cache := &PlCache{}
	return cache, cache.init()
}

func (c *PlCache) init() error {
	if c == nil {
		return nil
	}

	var err error
	c.cache, err = ristretto.NewCache(&ristretto.Config{
		NumCounters: 200e6,
		MaxCost:     int64(Config.AllottedMemory * 1024 * 1024),
		BufferItems: 64,
		Metrics:     true,
		Cost: func(val interface{}) int64 {
			pl, ok := val.(*pb.PostingList)
			if !ok {
				return int64(0)
			}
			return int64(calculatePostingListSize(pl))
		},
	})
	return err
}

func (c *PlCache) Get(key []byte, version uint64) *pb.PostingList {
	if c == nil || len(key) == 0 {
		return nil
	}

	cacheKey := generateKey(key, version)
	cachedVal, ok := c.cache.Get(cacheKey)
	if !ok {
		return nil
	}
	pl, ok := cachedVal.(*pb.PostingList)
	if !ok {
		return nil
	}

	return pl
}

func (c *PlCache) Set(key []byte, version uint64, pl *pb.PostingList) {
	if c == nil || len(key) == 0 || version == 0 || pl == nil {
		return
	}

	_ = c.cache.Set(generateKey(key, version), pl, 0)
}

func (c *PlCache) Clear() {
	if c == nil {
		return
	}
	c.cache.Clear()
}

// ClearCache will clear the entire list cache.
func ClearCache() {
	plCache.Clear()
}
