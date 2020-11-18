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
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/skl"
	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
)

// XidMap allocates and tracks mappings between Xids and Uids in a threadsafe
// manner. It's memory friendly because the mapping is stored on disk, but fast
// because it uses an LRU cache.
type XidMap struct {
	shards     []*shard
	newRanges  chan *pb.AssignedIds
	zc         pb.ZeroClient
	maxUidSeen uint64

	// Optionally, these can be set to persist the mappings.
	writer *badger.WriteBatch
}

type shard struct {
	sync.RWMutex
	block

	skiplist *skl.Skiplist
}

type block struct {
	start, end uint64
}

// assign assumes the write lock is already acquired.
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
// assignment operations. XidMap creates the temporary buffers inside dir directory. The caller must
// ensure that the dir exists.
func New(zero *grpc.ClientConn, db *badger.DB, dir string) *XidMap {
	numShards := 32
	xm := &XidMap{
		newRanges: make(chan *pb.AssignedIds, numShards),
		shards:    make([]*shard, numShards),
	}
	for i := range xm.shards {
		buf, err := z.NewBufferWithDir(math.MaxUint32, math.MaxUint32, z.UseMmap, dir)
		x.Check(err)
		xm.shards[i] = &shard{
			skiplist: skl.NewSkiplistWithBuffer(buf, false),
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
					sh.skiplist.PutUint64([]byte(key), uid)
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
	xm.zc = pb.NewZeroClient(zero)

	go func() {
		const initBackoff = 10 * time.Millisecond
		const maxBackoff = 5 * time.Second
		backoff := initBackoff
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			assigned, err := xm.zc.AssignUids(ctx, &pb.Num{Val: 1e5})
			glog.V(2).Infof("Assigned Uids: %+v. Err: %v", assigned, err)
			cancel()
			if err == nil {
				backoff = initBackoff
				xm.updateMaxSeen(assigned.EndId)
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
	fp := z.MemHashString(xid)
	idx := fp % uint64(len(m.shards))
	return m.shards[idx]
}

func (m *XidMap) CheckUid(xid string) bool {
	sh := m.shardFor(xid)
	sh.RLock()
	defer sh.RUnlock()
	uid, _ := sh.skiplist.GetUint64([]byte(xid))
	return uid != 0
}

func (m *XidMap) SetUid(xid string, uid uint64) {
	sh := m.shardFor(xid)
	sh.Lock()
	defer sh.Unlock()
	sh.skiplist.PutUint64([]byte(xid), uid)
}

// AssignUid creates new or looks up existing XID to UID mappings. It also returns if
// UID was created.
func (m *XidMap) AssignUid(xid string) (uint64, bool) {
	sh := m.shardFor(xid)
	sh.RLock()
	uid, _ := sh.skiplist.GetUint64([]byte(xid))
	sh.RUnlock()
	if uid > 0 {
		return uid, false
	}

	sh.Lock()
	defer sh.Unlock()

	uid, _ = sh.skiplist.GetUint64([]byte(xid))
	if uid > 0 {
		return uid, false
	}

	newUid := sh.assign(m.newRanges)
	sh.skiplist.PutUint64([]byte(xid), newUid)

	return newUid, true
}

func (sh *shard) Current() uint64 {
	sh.RLock()
	defer sh.RUnlock()
	return sh.start
}

func (m *XidMap) updateMaxSeen(max uint64) {
	for {
		prev := atomic.LoadUint64(&m.maxUidSeen)
		if prev >= max {
			return
		}
		if atomic.CompareAndSwapUint64(&m.maxUidSeen, prev, max) {
			return
		}
	}
}

// BumpTo can be used to make Zero allocate UIDs up to this given number. Attempts are made to
// ensure all future allocations of UIDs be higher than this one, but results are not guaranteed.
func (m *XidMap) BumpTo(uid uint64) {
	curMax := atomic.LoadUint64(&m.maxUidSeen)
	if uid <= curMax {
		return
	}

	for {
		glog.V(1).Infof("Bumping up to %v", uid)
		num := x.Max(uid-curMax, 1e4)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		assigned, err := m.zc.AssignUids(ctx, &pb.Num{Val: num})
		cancel()
		if err == nil {
			glog.V(1).Infof("Requested bump: %d. Got assigned: %v", uid, assigned)
			m.updateMaxSeen(assigned.EndId)
			return
		}
		glog.Errorf("While requesting AssignUids(%d): %v", num, err)
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
	glog.Infof("Writing xid map to DB")
	defer func() {
		glog.Infof("Finished writing xid map to DB")
	}()

	for _, shard := range m.shards {
		var err error
		if m.writer != nil {
			shard.Lock()
			it := shard.skiplist.NewIterator()
			var uidBuf [8]byte
			for it.SeekToFirst(); it.Valid(); it.Next() {
				curKey := it.Key()
				key := make([]byte, len(curKey))
				copy(key, curKey)
				binary.BigEndian.PutUint64(uidBuf[:], it.ValueUint64())
				err = m.writer.Set(key, uidBuf[:])
				y.Check(err)
			}
			it.Close()
			shard.Unlock()
		}
		shard.skiplist.DecrRef()
		if err != nil {
			return err
		}
	}

	if m.writer == nil {
		return nil
	}
	return m.writer.Flush()
}
