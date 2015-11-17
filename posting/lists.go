/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgryski/go-farm"
)

type entry struct {
	l *List
}

type counters struct {
	added  uint64
	merged uint64
}

func (c *counters) periodicLog() {
	for _ = range time.Tick(time.Second) {
		added := atomic.LoadUint64(&c.added)
		merged := atomic.LoadUint64(&c.merged)
		pending := added - merged

		glog.WithFields(logrus.Fields{
			"added":   added,
			"merged":  merged,
			"pending": pending,
		}).Info("Merge counters")
	}
}

var lmutex sync.RWMutex
var lcache map[uint64]*entry
var pstore *store.Store
var clog *commit.Logger
var ch chan uint64

func Init(posting *store.Store, log *commit.Logger) {
	lmutex.Lock()
	defer lmutex.Unlock()

	lcache = make(map[uint64]*entry)
	pstore = posting
	clog = log
	ch = make(chan uint64, 10000)
}

func get(k uint64) *List {
	lmutex.RLock()
	defer lmutex.RUnlock()
	if e, ok := lcache[k]; ok {
		return e.l
	}
	return nil
}

func Get(key []byte) *List {
	// Acquire read lock and check if list is available.
	lmutex.RLock()
	uid := farm.Fingerprint64(key)
	if e, ok := lcache[uid]; ok {
		lmutex.RUnlock()
		return e.l
	}
	lmutex.RUnlock()

	// Couldn't find it. Acquire write lock.
	lmutex.Lock()
	defer lmutex.Unlock()
	// Check again after acquiring write lock.
	if e, ok := lcache[uid]; ok {
		return e.l
	}

	e := new(entry)
	e.l = new(List)
	e.l.init(key, pstore, clog)
	lcache[uid] = e
	return e.l
}

func queueForProcessing(c *counters) {
	lmutex.RLock()
	for eid, e := range lcache {
		if len(ch) >= cap(ch) {
			break
		}
		if e.l.IsDirty() {
			ch <- eid
			atomic.AddUint64(&c.added, 1)
		}
	}
	lmutex.RUnlock()
}

func periodicQueueForProcessing(c *counters) {
	ticker := time.NewTicker(time.Minute)
	for _ = range ticker.C {
		queueForProcessing(c)
	}
}

func process(c *counters, wg *sync.WaitGroup) {
	for eid := range ch {
		l := get(eid)
		if l == nil {
			continue
		}
		atomic.AddUint64(&c.merged, 1)
		if err := l.MergeIfDirty(); err != nil {
			glog.WithError(err).Error("While commiting dirty list.")
		}
	}
	if wg != nil {
		wg.Done()
	}
}

func periodicProcess(c *counters) {
	ticker := time.NewTicker(100 * time.Millisecond)
	for _ = range ticker.C {
		process(c, nil)
	}
}

func queueAll(c *counters) {
	lmutex.RLock()
	for hid, _ := range lcache {
		ch <- hid
		atomic.AddUint64(&c.added, 1)
	}
	close(ch)
	lmutex.RUnlock()
}

func StartPeriodicMerging() {
	ctr := new(counters)
	go periodicQueueForProcessing(ctr)
	go periodicProcess(ctr)
}

func MergeLists(numRoutines int) {
	c := new(counters)
	go c.periodicLog()
	go queueAll(c)

	wg := new(sync.WaitGroup)
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go process(c, wg)
	}
	wg.Wait()
}
