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
	"math/rand"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
	"github.com/golang/glog"
)

// XidMap allocates and tracks mappings between Xids and Uids in a threadsafe
// manner. It's memory friendly because the mapping is stored on disk, but fast
// because it uses an LRU cache.
type XidMap struct {
	shards    []*shard
	newRanges chan *pb.AssignedIds

	// Optionally, these can be set to persist the mappings.
	writer *badger.WriteBatch
}

type shard struct {
	sync.RWMutex
	block

	uidMap map[string]uint64
}

type block struct {
	start, end uint64
}

// This must already have a write lock.
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

// New creates an XidMap. zero conn must be valid for UID allocations to happen. Optionally, a
// badger.DB can be provided to persist the xid to uid allocations. This would add latency to the
// assignment operations.
func New(zero *grpc.ClientConn, db *badger.DB) *XidMap {
	xm := &XidMap{
		newRanges: make(chan *pb.AssignedIds, 10),
		shards:    make([]*shard, runtime.GOMAXPROCS(0)),
	}
	for i := range xm.shards {
		xm.shards[i] = &shard{
			uidMap: make(map[string]uint64),
		}
	}
	if db != nil {
		// If DB is provided, let's load up all the xid -> uid mappings in memory.
		xm.writer = db.NewWriteBatch()

		err := db.View(func(txn *badger.Txn) error {
			var count int
			opt := badger.DefaultIteratorOptions
			opt.PrefetchValues = false
			itr := txn.NewIterator(opt)
			defer itr.Close()
			for itr.Rewind(); itr.Valid(); itr.Next() {
				item := itr.Item()
				key := string(item.Key())
				sh := xm.shardFor(key)
				err := item.Value(func(val []byte) error {
					uid := binary.BigEndian.Uint64(val)
					// No need to acquire a lock. This is all serial access.
					sh.uidMap[key] = uid
					return nil
				})
				if err != nil {
					return err
				}
				count++
			}
			glog.Infof("Loaded up %d xid to uid mappings", count)
			return nil
		})
		x.Check(err)
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

func (m *XidMap) shardFor(xid string) *shard {
	fp := farm.Fingerprint32([]byte(xid))
	idx := fp % uint32(len(m.shards))
	return m.shards[idx]
}

// AssignUid creates new or looks up existing XID to UID mappings.
func (m *XidMap) AssignUid(xid string) uint64 {
	sh := m.shardFor(xid)
	sh.RLock()
	uid := sh.uidMap[xid]
	sh.RUnlock()
	if uid > 0 {
		return uid
	}

	sh.Lock()
	defer sh.Unlock()

	uid = sh.uidMap[xid]
	if uid > 0 {
		return uid
	}

	newUid := sh.assign(m.newRanges)
	sh.uidMap[xid] = newUid

	if m.writer != nil {
		var uidBuf [8]byte
		binary.BigEndian.PutUint64(uidBuf[:], newUid)
		if err := m.writer.Set([]byte(xid), uidBuf[:], 0); err != nil {
			panic(err)
		}
	}
	return newUid
}

func (sh *shard) End() uint64 {
	sh.RLock()
	defer sh.RUnlock()
	return sh.end
}

// BumpTo can be used to make Zero allocate UIDs up to this given number. No guarantees are made
// about the next UID allocated by XidMap. It can be lower or higher than the uid given here.
func (m *XidMap) BumpTo(uid uint64) {
	for _, sh := range m.shards {
		if uid <= sh.End() {
			return
		}
	}
	for {
		r := <-m.newRanges
		if uid <= r.EndId {
			return
		}
	}
}

// AllocateUid gives a single uid without creating an xid to uid mapping.
func (m *XidMap) AllocateUid() uint64 {
	sh := m.shards[rand.Intn(len(m.shards))]
	sh.Lock()
	defer sh.Unlock()
	return sh.assign(m.newRanges)
}

// Flush must be called if DB is provided to XidMap.
func (m *XidMap) Flush() error {
	if m.writer == nil {
		return nil
	}
	return m.writer.Flush()
}
