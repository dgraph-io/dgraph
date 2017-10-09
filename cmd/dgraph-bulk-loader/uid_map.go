package main

import (
	"container/list"
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

const (
	// Memory used is in the order of numShards * (avgKeySize + constant
	// overhead per key) * lruSize.
	numShards = 1 << 10
	lruSize   = 1 << 9
)

type uidMap struct {
	lease  uint64
	shards [numShards]shard
	kv     *badger.KV
}

type shard struct {
	sync.Mutex

	lastUsed uint64
	lease    uint64

	elems        map[string]*list.Element
	queue        *list.List
	beingEvicted map[string]uint64

	kv *badger.KV
}

type mapping struct {
	xid       string
	uid       uint64
	persisted bool
}

func newUIDMap(kv *badger.KV) *uidMap {
	um := &uidMap{
		lease: 1,
		kv:    kv,
	}
	for i := range um.shards {
		um.shards[i].elems = make(map[string]*list.Element)
		um.shards[i].queue = list.New()
		um.shards[i].kv = kv
	}
	return um
}

// assignUID creates new or looks up existing XID to UID mappings.
func (m *uidMap) assignUID(xid string) (uid uint64, isNew bool) {
	fp := farm.Fingerprint64([]byte(xid))
	idx := fp % numShards
	sh := &m.shards[idx]

	sh.Lock()
	defer sh.Unlock()

	var ok bool
	uid, ok = sh.lookup(xid)
	if ok {
		return uid, false
	}

	var item badger.KVItem
	x.Check(m.kv.Get([]byte(xid), &item))
	x.Check(item.Value(func(uidBuf []byte) error {
		if uidBuf == nil {
			return nil
		}
		var n int
		uid, n = binary.Uvarint(uidBuf)
		x.AssertTrue(n == len(uidBuf))
		ok = true
		return nil
	}))
	if ok {
		sh.add(xid, uid, true)
		return uid, false
	}

	const leaseChunk = 1e5
	if sh.lastUsed == sh.lease {
		sh.lease = atomic.AddUint64(&m.lease, leaseChunk)
		sh.lastUsed = sh.lease - leaseChunk
	}
	sh.lastUsed++
	sh.add(xid, sh.lastUsed, false)
	return sh.lastUsed, true
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
	if s.queue.Len() >= lruSize && len(s.beingEvicted) == 0 {
		s.evict()
	}

	m := &mapping{
		xid:       xid,
		uid:       uid,
		persisted: persisted,
	}
	elem := s.queue.PushBack(m)
	s.elems[xid] = elem
}

func (s *shard) evict() {
	const evictRatio = 0.5
	evict := int(float64(s.queue.Len()) * evictRatio)
	s.beingEvicted = make(map[string]uint64)
	batch := make([]*badger.Entry, 0, evict)
	for i := 0; i < evict; i++ {
		m := s.queue.Remove(s.queue.Front()).(*mapping)
		delete(s.elems, m.xid)
		s.beingEvicted[m.xid] = m.uid
		if !m.persisted {
			var uidBuf [binary.MaxVarintLen64]byte
			n := binary.PutUvarint(uidBuf[:], m.uid)
			batch = append(batch, &badger.Entry{
				Key:   []byte(m.xid),
				Value: uidBuf[:n],
			})
		}

	}
	s.kv.BatchSetAsync(batch, func(err error) {
		x.Check(err)
		for _, e := range batch {
			x.Check(e.Error)
		}

		s.Lock()
		s.beingEvicted = nil
		s.Unlock()
	})
}
