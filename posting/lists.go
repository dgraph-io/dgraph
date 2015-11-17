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
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgryski/go-farm"
)

type entry struct {
	l *List
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
	ch = make(chan uint64, 1000)
	go queueForProcessing()
	go process()
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

func queueForProcessing() {
	ticker := time.NewTicker(time.Minute)
	for _ = range ticker.C {
		count := 0
		skipped := 0
		lmutex.RLock()
		now := time.Now()
		for eid, e := range lcache {
			if len(ch) >= cap(ch) {
				break
			}
			if len(ch) < int(0.3*float32(cap(ch))) && e.l.IsDirty() {
				// Let's add some work here.
				ch <- eid
				count += 1
			} else if now.Sub(e.l.LastCompactionTs()) > 10*time.Minute {
				// Only queue lists which haven't been processed for a while.
				ch <- eid
				count += 1
			} else {
				skipped += 1
			}
		}
		lmutex.RUnlock()
		glog.WithFields(logrus.Fields{
			"added":   count,
			"skipped": skipped,
			"pending": len(ch),
		}).Info("Added for compaction")
	}
}

func process() {
	ticker := time.NewTicker(100 * time.Millisecond)
	for _ = range ticker.C {
		eid := <-ch // blocking.
		l := get(eid)
		if l == nil {
			continue
		}
		glog.WithField("eid", eid).WithField("pending", len(ch)).
			Info("Commiting list")
		if err := l.MergeIfDirty(); err != nil {
			glog.WithError(err).Error("While commiting dirty list.")
		}
	}
}
