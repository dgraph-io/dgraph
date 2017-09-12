package main

import (
	"sync"
	"sync/atomic"

	farm "github.com/dgryski/go-farm"
)

type shard struct {
	sync.Mutex
	m       map[string]uint64
	lastUID uint64
	lease   uint64
}

type uidMap struct {
	lease  uint64
	shards [256]shard
}

func newUIDMap() *uidMap {
	um := &uidMap{
		lease: 1,
	}
	for i := range um.shards {
		um.shards[i].m = make(map[string]uint64)
	}
	return um
}

// assignUID would assume that str is an external ID, and would assign a new
// internal Dgraph ID for this.
func (m *uidMap) assignUID(str string) uint64 {
	fp := farm.Fingerprint64([]byte(str))
	idx := fp & 0xff
	sh := &m.shards[idx]

	sh.Lock()
	defer sh.Unlock()

	uid, ok := sh.m[str]
	if ok {
		return uid
	}
	if sh.lastUID == sh.lease {
		sh.lease = atomic.AddUint64(&m.lease, 10000)
	}
	sh.lastUID++
	sh.m[str] = sh.lastUID
	return sh.lastUID
}
