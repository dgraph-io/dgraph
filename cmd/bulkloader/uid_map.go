package main

import (
	"log"
	"strconv"
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

func (m *uidMap) assignUID(str string) uint64 {
	m.Lock()
	defer m.Unlock()

	hint, err := strconv.ParseUint(str, 10, 64)
	if err == nil {
		uid, ok := m.uids[str]
		if ok {
			if uid == hint {
				return uid
			} else {
				log.Fatalf("bad node hint: %v", str)
			}
		} else {
			m.uids[str] = hint
			return hint
		}
	}

	uid, ok := m.uids[str]
	if ok {
		return uid
	}
	m.lastUID++
	m.uids[str] = m.lastUID
	return m.lastUID
}
