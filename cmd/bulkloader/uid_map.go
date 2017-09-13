package main

import (
	"sync"
	"sync/atomic"

	farm "github.com/dgryski/go-farm"
)

type shard struct {
	sync.Mutex
	xidToUid map[string]uint64
	lastUsed uint64
	lease    uint64
}

type uidMap struct {
	lease  uint64
	shards [1 << 16]shard
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
	idx := fp & 0xffff
	sh := &m.shards[idx]

	sh.Lock()
	defer sh.Unlock()

	uid, ok := sh.xidToUid[str]
	if ok {
		return uid
	}
	const leaseChunk = 1e5
	if sh.lastUsed == sh.lease {
		sh.lease = atomic.AddUint64(&m.lease, leaseChunk)
		sh.lastUsed = sh.lease - leaseChunk
	}
	sh.lastUsed++
	sh.xidToUid[str] = sh.lastUsed
	return sh.lastUsed
}
