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
	"context"
	"encoding/binary"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

// XidMap allocates and tracks mappings between Xids and Uids in a threadsafe
// manner. It's memory friendly because the mapping is stored on disk, but fast
// because it uses an LRU cache.
type XidMap struct {
	sync.Mutex
	kv *badger.DB
	// opt       Options
	newRanges chan *pb.AssignedIds
	uidMap    sync.Map
	writer    *badger.WriteBatch

	block *block
}

type block struct {
	sync.Mutex
	start, end uint64
}

func (b *block) assign(ch <-chan *pb.AssignedIds) uint64 {
	b.Lock()
	defer b.Unlock()

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
func New(kv *badger.DB, zero *grpc.ClientConn) *XidMap {
	// x.AssertTrue(opt.LRUSize != 0)
	// x.AssertTrue(opt.NumShards != 0)
	xm := &XidMap{
		kv:        kv,
		newRanges: make(chan *pb.AssignedIds, 10),
		writer:    kv.NewWriteBatch(),
		block:     new(block),
	}
	go func() {
		zc := pb.NewZeroClient(zero)
		const initBackoff = 10 * time.Millisecond
		const maxBackoff = 5 * time.Second
		backoff := initBackoff
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			assigned, err := zc.AssignUids(ctx, &pb.Num{Val: 1e5})
			glog.V(1).Infof("Assigned Uids: %+v. Err: %v", assigned, err)
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
func (m *XidMap) AssignUid(xid string) (uid uint64) {
	val, ok := m.uidMap.Load(xid)
	if ok {
		return val.(uint64)
	}
	// TODO: Load up the xid->uid map upfront.

	// fp := farm.Fingerprint64([]byte(xid))
	// idx := fp % uint64(m.opt.NumShards)
	// sh := &m.shards[idx]

	// sh.Lock()
	// defer sh.Unlock()

	// var ok bool
	// uid, ok = sh.lookup(xid)
	// if ok {
	// 	return uid, false
	// }

	// x.Check(m.kv.View(func(txn *badger.Txn) error {
	// 	item, err := txn.Get([]byte(xid))
	// 	if err == badger.ErrKeyNotFound {
	// 		return nil
	// 	}
	// 	x.Check(err)
	// 	return item.Value(func(uidBuf []byte) error {
	// 		x.AssertTrue(len(uidBuf) > 0)
	// 		var n int
	// 		uid, n = binary.Uvarint(uidBuf)
	// 		x.AssertTrue(n == len(uidBuf))
	// 		ok = true
	// 		return nil
	// 	})
	// }))
	// if ok {
	// 	sh.add(xid, uid, true)
	// 	return uid, false
	// }

	uid = m.block.assign(m.newRanges)
	val, loaded := m.uidMap.LoadOrStore(xid, uid)
	uid = val.(uint64)
	if !loaded {
		// This uid was stored.
		var uidBuf [binary.MaxVarintLen64]byte
		n := binary.PutUvarint(uidBuf[:], uid)
		if err := m.writer.Set([]byte(xid), uidBuf[:n], 0); err != nil {
			panic(err)
		}
	}
	return uid
}

// AllocateUid gives a single uid without creating an xid to uid mapping.
func (m *XidMap) AllocateUid() uint64 {
	return m.block.assign(m.newRanges)
}
