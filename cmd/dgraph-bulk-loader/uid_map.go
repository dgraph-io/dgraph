package main

import (
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

type shard struct {
	sync.Mutex
	cache    lruCache
	lastUsed uint64
	lease    uint64
}

const numShards = 1 << 16

type uidMap struct {
	lease  uint64
	shards [numShards]shard

	kv    *badger.KV
	batch []*badger.Entry
}

func newUIDMap() *uidMap {
	um := &uidMap{
		lease: 1,
	}
	for i := range um.shards {
		um.shards[i].xidToUid = make(map[string]uint64)
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
		return uid
	}

	var item badger.KVItem
	x.Check(m.kv.Get([]byte(str), &item))
	x.Check(item.Value(func(v []byte) error {
		if v == nil {
			ok = true
			return nil
		}
		var n int
		uid, n = binary.Uvarint(v)
		x.AssertTrue(n == len(v))
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
	sh.cache.add(str, sh.lastUsed)

	// TODO: put into badger
	var valBuf [binary.MaxVarintLen64]byte
	m.batch = append(m.batch, &badger.Entry{
		Key:   []byte(str),
		Value: valBuf[:binary.PutUvarint(valBuf, uid)],
	})

	return sh.lastUsed
}

type lruCache struct {
	circKeys   []string
	circWgs    []sync.WaitGroup // wait for badger write
	head, tail int              // put at head, get at tail
	m          map[string]uint64
}

func (c *lruCache) add(k string, v uint64) {
	n := len(c.circWgs)
	if (c.head-c.tail+n)%n == n-1 {
		c.circWgs[c.tail].Wait()
		delete(c.m, c.circKeys[c.tail])
		c.tail = (c.tail + 1) % n
	}
	c.circWgs[c.head].Add(1)
	c.circKeys[c.head] = k
	c.head = (c.head + 1) % n
	c.m[k] = v
}
