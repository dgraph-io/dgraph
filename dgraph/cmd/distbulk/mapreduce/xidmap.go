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
	"container/list"
	"encoding/binary"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
	// farm "github.com/dgryski/go-farm"
)

type opts struct {
	// NumShards controls the number of shards the XidMap is broken into. More
	// shards reduces lock contention.
	NumShards int
	// LRUSize controls the total size of the LRU cache. The LRU is split
	// between all shards, so with 4 shards and an LRUSize of 100, each shard
	// receives 25 LRU slots.
	LRUSize int
}

// XidMap allocates and tracks mappings between Xids and Uids in a threadsafe
// manner. It's memory friendly because the mapping is stored on disk, but fast
// because it uses an LRU cache.
type XidMap struct {
    lru shard
	kv     *badger.DB
	opt    opts
}

type shard struct {
	elems        map[string]*list.Element
	queue        *list.List
	beingEvicted map[string]uint64
	xm           *XidMap
}

type mapping struct {
	xid       string
	uid       uint64
}

// New creates an XidMap with given badger and uid provider.
func NewXidmap(badgerPath string) *XidMap {
	badgeropts := badger.DefaultOptions
	badgeropts.ReadOnly = true
	badgeropts.Dir = badgerPath
	badgeropts.ValueDir = badgerPath
	kv, err := badger.Open(badgeropts)
	x.Check(err)

	opt := opts{
		NumShards: 1 << 10,
		LRUSize:   1 << 19,
	}

	xm := &XidMap{
        kv:     kv,
        opt:    opt,
		lru:    shard{
            elems: make(map[string]*list.Element),
            queue: list.New(),
        },
	}
    xm.lru.xm = xm

	return xm
}

func (m *XidMap) LookupUid(xid string) (uid uint64, ok bool) {
	uid, ok = m.lru.lookup(xid)
	if ok {
		return uid, true
	}

	x.Check(m.kv.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(xid))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		x.Check(err)
		return item.Value(func(uidBuf []byte) error {
			x.AssertTrue(len(uidBuf) > 0)
			var n int
			uid, n = binary.Uvarint(uidBuf)
			x.AssertTrue(n == len(uidBuf))
			ok = true
			return nil
		})
	}))
	if ok {
		m.lru.add(xid, uid)
		return uid, true
	}
	return 0, false
}

func (s *shard) lookup(xid string) (uint64, bool) {
	elem, ok := s.elems[xid]
	if ok {
		s.queue.MoveToBack(elem)
		return elem.Value.(*mapping).uid, true
	}
	if uid, ok := s.beingEvicted[xid]; ok {
		s.add(xid, uid)
		return uid, true
	}
	return 0, false
}

func (s *shard) add(xid string, uid uint64) {
	if s.queue.Len() >= s.xm.opt.LRUSize && len(s.beingEvicted) == 0 {
		s.evict(0.5)
	}

	m := &mapping{
		xid:       xid,
		uid:       uid,
	}
	elem := s.queue.PushBack(m)
	s.elems[xid] = elem
}

func (m *XidMap) Close() {
    if m.kv != nil {
        m.kv.Close()
    }
}

func (s *shard) evict(ratio float64) {
	evict := int(float64(s.queue.Len()) * ratio)
	s.beingEvicted = make(map[string]uint64)
	for i := 0; i < evict; i++ {
		m := s.queue.Remove(s.queue.Front()).(*mapping)
		delete(s.elems, m.xid)
		s.beingEvicted[m.xid] = m.uid

	}
	s.beingEvicted = nil
}
