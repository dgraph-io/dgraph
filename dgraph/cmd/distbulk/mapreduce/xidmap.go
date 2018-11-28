/*
* Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"encoding/binary"
	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
    lru "github.com/hashicorp/golang-lru"
)

// XidMap allocates and tracks mappings between Xids and Uids in a threadsafe
// manner. It's memory friendly because the mapping is stored on disk, but fast
// because it uses an LRU cache.
type XidMap struct {
    cache  *lru.Cache
	kv     *badger.DB
}

// New creates an XidMap with given badger and uid provider.
func NewXidmap(badgerPath string, lruSize int) *XidMap {
	badgeropts := badger.DefaultOptions
	badgeropts.ReadOnly = true
	badgeropts.Dir = badgerPath
	badgeropts.ValueDir = badgerPath
	kv, err := badger.Open(badgeropts)
	x.Check(err)

    cache, err := lru.New(lruSize)
    x.Check(err)

	return &XidMap{
        kv:    kv,
        cache: cache,
	}
}

func (m *XidMap) LookupUid(xid string) (uid uint64, ok bool) {
    val, ok := m.cache.Get(xid)
	if ok {
		return val.(uint64), true
	}

	x.Check(m.kv.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(xid))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		x.Check(err)
		return item.Value(func(uidBuf []byte) error {
			uid, _ = binary.Uvarint(uidBuf)
			ok = true
			return nil
		})
	}))
	if ok {
		m.cache.Add(xid, uid)
		return uid, true
	}
	return 0, false
}

func (m *XidMap) Close() {
    if m.kv != nil {
        m.kv.Close()
    }
    if m.cache != nil {
        m.cache.Purge()
    }
}
