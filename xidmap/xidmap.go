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

package xidmap

import (
	"container/list"
	"context"
	"encoding/binary"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
	"github.com/golang/glog"
)

// Options controls the performance characteristics of the XidMap.
type Options struct {
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
	shards    []shard
	kv        *badger.DB
	opt       Options
	newRanges chan *pb.AssignedIds

	noMapMu sync.Mutex
	noMap   block // block for allocating uids without an xid to uid mapping
}

type shard struct {
	sync.Mutex
	block

	elems        map[string]*list.Element
	queue        *list.List
	beingEvicted map[string]uint64

	xm *XidMap
}

type mapping struct {
	xid       string
	uid       uint64
	persisted bool
}

type block struct {
	start, end uint64
}

func (b *block) assign(ch <-chan *pb.AssignedIds) uint64 {
	if b.end == 0 || b.start > b.end {
		newRange := <-ch
		b.start, b.end = newRange.StartId, newRange.EndId
	}
	x.AssertTrue(b.start <= b.end)
	uid := b.start
	b.start++
	return uid
}

// New creates an XidMap with given badger and uid provider.
func New(kv *badger.DB, zero *grpc.ClientConn, opt Options) *XidMap {
	x.AssertTrue(opt.LRUSize != 0)
	x.AssertTrue(opt.NumShards != 0)
	xm := &XidMap{
		shards:    make([]shard, opt.NumShards),
		kv:        kv,
		opt:       opt,
		newRanges: make(chan *pb.AssignedIds),
	}
	for i := range xm.shards {
		xm.shards[i].elems = make(map[string]*list.Element)
		xm.shards[i].queue = list.New()
		xm.shards[i].xm = xm
	}
	go func() {
		zc := pb.NewZeroClient(zero)
		const initBackoff = 10 * time.Millisecond
		const maxBackoff = 5 * time.Second
		backoff := initBackoff
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			assigned, err := zc.AssignUids(ctx, &pb.Num{Val: 10000})
			cancel()
			if err == nil {
				backoff = initBackoff
				xm.newRanges <- assigned
				continue
			}
			glog.Errorf("Error while getting lease: %v\n", err)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			time.Sleep(backoff)
		}

	}()
	return xm
}

// AssignUid creates new or looks up existing XID to UID mappings.
func (m *XidMap) AssignUid(xid string) (uid uint64, isNew bool) {
	fp := farm.Fingerprint64([]byte(xid))
	idx := fp % uint64(m.opt.NumShards)
	sh := &m.shards[idx]

	sh.Lock()
	defer sh.Unlock()

    // Check if the XID exists in the LRU cache map
	var ok bool
	uid, ok = sh.lookup(xid)
	if ok {
		return uid, false
	}

    // Check if the XID exists in the Badger
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

    // If found in the Badger, add it to the LRU cache
	if ok {
		sh.add(xid, uid, true)
		return uid, false
	}

    // Otherwise, it's a brand new XID, so we need to assign a UID to it
	uid = sh.assign(m.newRanges)
	sh.add(xid, uid, false)
	return uid, true
}

// AllocateUid gives a single uid without creating an xid to uid mapping.
func (m *XidMap) AllocateUid() uint64 {
	m.noMapMu.Lock()
	defer m.noMapMu.Unlock()
	return m.noMap.assign(m.newRanges)
}

func (s *shard) lookup(xid string) (uint64, bool) {
	elem, ok := s.elems[xid]
	if ok {
		s.queue.MoveToBack(elem)
		return elem.Value.(*mapping).uid, true
	}
	if uid, ok := s.beingEvicted[xid]; ok {
		s.add(xid, uid, true)
		return uid, true
	}
	return 0, false
}

func (s *shard) add(xid string, uid uint64, persisted bool) {
	lruSizePerShard := s.xm.opt.LRUSize / s.xm.opt.NumShards
	if s.queue.Len() >= lruSizePerShard && len(s.beingEvicted) == 0 {
		s.evict(0.5)
	}

	m := &mapping{
		xid:       xid,
		uid:       uid,
		persisted: persisted,
	}
	elem := s.queue.PushBack(m)
	s.elems[xid] = elem
}

func (m *XidMap) EvictAll() {
	for i := range make([]struct{}, len(m.shards)) {
		m.shards[i].Lock()
		m.shards[i].evict(1.0)
		m.shards[i].Unlock()
	}
}

func (s *shard) evict(ratio float64) {
	evict := int(float64(s.queue.Len()) * ratio)
	s.beingEvicted = make(map[string]uint64)
	txn := s.xm.kv.NewTransaction(true)
	defer txn.Discard()
	for i := 0; i < evict; i++ {
		m := s.queue.Remove(s.queue.Front()).(*mapping)
		delete(s.elems, m.xid)
		s.beingEvicted[m.xid] = m.uid
		if !m.persisted {
			var uidBuf [binary.MaxVarintLen64]byte
			n := binary.PutUvarint(uidBuf[:], m.uid)
			txn.Set([]byte(m.xid), uidBuf[:n])
		}

	}
	x.Check(txn.Commit())
	s.beingEvicted = nil
}
