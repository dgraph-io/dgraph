/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package posting

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
)

const NBUCKETS = 32

type latency struct {
	n1   uint64
	u1   uint64
	u10  uint64
	u100 uint64
	m1   uint64
	s1   uint64
}

type bucket struct {
	sync.RWMutex
	m   map[uint64]*List
	lat *latency
}

func (l *latency) update(s time.Time) {
	e := time.Now().Sub(s)
	micros := e.Nanoseconds() / 1000
	if micros > 1000000 {
		atomic.AddUint64(&l.s1, 1)
	} else if micros > 1000 {
		atomic.AddUint64(&l.m1, 1)
	} else if micros > 100 {
		atomic.AddUint64(&l.u100, 1)
	} else if micros > 10 {
		atomic.AddUint64(&l.u10, 1)
	} else if micros > 1 {
		atomic.AddUint64(&l.u1, 1)
	} else {
		atomic.AddUint64(&l.n1, 1)
	}
}

func (l *latency) log() {
	for _ = range time.Tick(5 * time.Second) {
		glog.WithFields(logrus.Fields{
			"n1":   atomic.LoadUint64(&l.n1),
			"u1":   atomic.LoadUint64(&l.u1),
			"u10":  atomic.LoadUint64(&l.u10),
			"u100": atomic.LoadUint64(&l.u100),
			"m1":   atomic.LoadUint64(&l.m1),
			"s1":   atomic.LoadUint64(&l.s1),
		}).Info("Lmap latency")
	}
}

func (b *bucket) get(key uint64) (*List, bool) {
	if b.lat != nil {
		n := time.Now()
		defer b.lat.update(n)
	}

	b.RLock()
	if l, ok := b.m[key]; ok {
		b.RUnlock()
		return l, false
	}
	b.RUnlock()

	b.Lock()
	defer b.Unlock()
	if l, ok := b.m[key]; ok {
		return l, false
	}

	l := NewList()
	b.m[key] = l
	return l, true
}

type Map struct {
	buckets []*bucket
}

func NewMap(withLog bool) *Map {
	var lat *latency
	if withLog {
		lat = new(latency)
		go lat.log()
	} else {
		lat = nil
	}

	m := new(Map)
	m.buckets = make([]*bucket, NBUCKETS)
	for i := 0; i < NBUCKETS; i++ {
		m.buckets[i] = new(bucket)
		m.buckets[i].lat = lat
		m.buckets[i].m = make(map[uint64]*List)
	}
	return m
}

func (m *Map) Get(key uint64) (*List, bool) {
	bi := key % NBUCKETS
	return m.buckets[bi].get(key)
}

func (m *Map) StreamUntilCap(ch chan uint64) {
	bi := rand.Intn(NBUCKETS)
	b := m.buckets[bi]
	b.RLock()
	defer b.RUnlock()
	for u := range b.m {
		if len(ch) >= cap(ch) {
			break
		}
		ch <- u
	}
}

func (m *Map) StreamAllKeys(ch chan uint64) {
	for i := 0; i < len(m.buckets); i++ {
		b := m.buckets[i]
		b.RLock()
		for u := range b.m {
			ch <- u
		}
		b.RUnlock()
	}
}
