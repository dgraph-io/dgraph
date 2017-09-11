package main

import (
	"sync"
)

type uidMap struct {
	sync.Mutex
	lastUID uint64
	uids    map[string]uint64
}

func newUIDMap() *uidMap {
	return &uidMap{
		lastUID: 1,
		uids:    map[string]uint64{},
	}
}

// assignUID would assume that str is an external ID, and would assign a new internal Dgraph ID for
// this.
func (m *uidMap) assignUID(str string) uint64 {
	m.Lock()
	defer m.Unlock()

	uid, ok := m.uids[str]
	if ok {
		return uid
	}
	m.lastUID++
	m.uids[str] = m.lastUID
	return m.lastUID
}

func (m *uidMap) lease() uint64 {
	// The lease is the last UID that we've used (as opposed to the next UID
	// that could be assigned).
	return m.lastUID
}
