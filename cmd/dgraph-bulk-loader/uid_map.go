package main

import (
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

const (
	// Memory used is about numShards * avgKeySize * lruSize
	numShards = 1 << 12
	lruSize   = 1 << 9
)

type uidMap struct {
	lease  uint64
	shards [numShards]shard

	kv      *badger.KV
	batch   []*badger.Entry
	batchMu []*sync.Mutex
}

type shard struct {
	sync.Mutex
	cache    lruCache
	lastUsed uint64
	lease    uint64
}

func newUIDMap() *uidMap {
	um := &uidMap{
		lease: 1,
	}
	for i := range um.shards {
		um.shards[i].cache = lruCache{m: make(map[string]uint64)}
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

	uid, ok := sh.cache.m[str]
	if ok {
		// In a normal LRU cache, this would reset the position of the element.
		// We can't easily do that with a circular buffer though.
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
		sh.cache.add(str, uid)
		return uid
	}

	const leaseChunk = 1e5
	if sh.lastUsed == sh.lease {
		sh.lease = atomic.AddUint64(&m.lease, leaseChunk)
		sh.lastUsed = sh.lease - leaseChunk
	}

	sh.lastUsed++
	lck := sh.cache.add(str, sh.lastUsed)
	lck.Lock() // Stop from being evicted until unlocked.

	var valBuf [binary.MaxVarintLen64]byte
	m.batch = append(m.batch, &badger.Entry{
		Key:   []byte(str),
		Value: valBuf[:binary.PutUvarint(valBuf[:], uid)],
	})
	m.batchMu = append(m.batchMu, lck)
	if len(m.batch) > 1e3 {
		batch := m.batch
		m.batch = nil
		batchMu := m.batchMu
		m.batchMu = nil
		m.kv.BatchSetAsync(m.batch, func(err error) {
			x.Check(err)
			for _, e := range batch {
				x.Check(e.Error)
			}
			for _, mu := range batchMu {
				// Allow entries to be evicted from LRU cache.
				mu.Unlock()
			}
		})
	}

	return sh.lastUsed
}

type lruCache struct {
	// Circular buffer. LRU data is held in keys. EvictLocks control when
	// eviction can occur - eviction must not take place until the keys have
	// been persisted to badger.
	keys       [lruSize]string
	evictLocks [lruSize]sync.Mutex
	head, tail int // Put at head, get at tail.

	m map[string]uint64
}

func (c *lruCache) add(k string, v uint64) *sync.Mutex {
	const n = lruSize
	if (c.head-c.tail+n)%n == n-1 {
		// LRU is full, so evict oldest element. Make sure the element lock can
		// be held before the eviction. Being able to hold the lock proves that
		// the element has been accepted by badger.
		c.evictLocks[c.tail].Lock()
		c.evictLocks[c.tail].Unlock()
		delete(c.m, c.keys[c.tail])
		c.tail = (c.tail + 1) % n
	}

	c.keys[c.head] = k
	lck := &c.evictLocks[c.head]
	c.head = (c.head + 1) % n
	c.m[k] = v
	return lck
}
