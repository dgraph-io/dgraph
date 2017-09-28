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
	numShards = 1 << 12
	lruSize   = 1 << 9
	lruEvict  = lruSize / 4
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

	elems   map[string]*list.Element
	queue   *list.List
	evicted map[string]uint64 // Evicted but not yet persisted.
}

type keyval struct {
	key string
	val uint64
}

func newUIDMap(kv *badger.KV) *uidMap {
	um := &uidMap{
		lease: 1,
		kv:    kv,
	}
	for i := range um.shards {
		um.shards[i].elems = make(map[string]*list.Element)
		um.shards[i].queue = list.New()
	}
	return um
}

// assignUID would assume that str is an external ID, and would assign a new
// internal Dgraph ID for this.
func (m *uidMap) assignUID(str string) uint64 {
	fp := farm.Fingerprint64([]byte(str))
	idx := fp % numShards
	sh := &m.shards[idx]

	sh.Lock()
	defer sh.Unlock()

	uid, ok := sh.lookup(str)
	if ok {
		return uid
	}

	var item badger.KVItem
	x.Check(m.kv.Get([]byte(str), &item))
	x.Check(item.Value(func(v []byte) error {
		if v == nil {
			return nil
		}
		var n int
		uid, n = binary.Uvarint(v)
		x.AssertTrue(n == len(v))
		ok = true
		return nil
	}))
	if ok {
		sh.add(str, uid, m.kv)
		return uid
	}

	const leaseChunk = 1e5
	if sh.lastUsed == sh.lease {
		sh.lease = atomic.AddUint64(&m.lease, leaseChunk)
		sh.lastUsed = sh.lease - leaseChunk
	}
	sh.lastUsed++
	sh.add(str, sh.lastUsed, m.kv)
	return sh.lastUsed
}

func (s *shard) lookup(k string) (v uint64, ok bool) {
	var elem *list.Element
	elem, ok = s.elems[k]
	if ok {
		s.queue.MoveToBack(elem)
		return elem.Value.(*keyval).val, true
	}
	if v, ok := s.evicted[k]; ok {
		// TODO: Possible to move from evicted back to main part of cache?
		return v, true
	}
	return 0, false
}

func (s *shard) add(k string, v uint64, kv *badger.KV) {
	if s.queue.Len()+1 > lruSize && len(s.evicted) == 0 {
		s.evicted = make(map[string]uint64, lruEvict)
		batch := make([]*badger.Entry, 0, lruEvict)
		for s.queue.Len()+1 > lruSize {
			elem := s.queue.Front()
			entry := elem.Value.(*keyval)
			s.queue.Remove(elem)
			delete(s.elems, entry.key)
			s.evicted[entry.key] = entry.val

			var valBuf [binary.MaxVarintLen64]byte
			batch = append(batch, &badger.Entry{
				Key:   []byte(entry.key),
				Value: valBuf[:binary.PutUvarint(valBuf[:], entry.val)],
			})
		}
		kv.BatchSetAsync(batch, func(err error) {
			x.Check(err)
			for _, e := range batch {
				x.Check(e.Error)
			}

			s.Lock()
			s.evicted = nil
			s.Unlock()
		})
	}

	entry := &keyval{
		key: k,
		val: v,
	}
	elem := s.queue.PushBack(entry)
	s.elems[k] = elem
}
