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
	"unsafe"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/concurrent"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgryski/go-farm"
)

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

var lcmap *concurrent.Map
var pstore *store.Store
var clog *commit.Logger
var ch chan uint64
var lc *lcounters

func Init(posting *store.Store, log *commit.Logger) {
	lcmap = concurrent.NewMap(1 << 20)
	pstore = posting
	clog = log
	ch = make(chan uint64, 10000)
	lc = new(lcounters)
	go lc.periodicLog()
}

type lcounters struct {
	hit  uint64
	miss uint64
}

func (lc *lcounters) periodicLog() {
	for _ = range time.Tick(10 * time.Second) {
		glog.WithFields(logrus.Fields{
			"hit":  atomic.LoadUint64(&lc.hit),
			"miss": atomic.LoadUint64(&lc.miss),
		}).Info("Lists counters")
	}
}

func Get(key []byte) *List {
	uid := farm.Fingerprint64(key)
	lp := lcmap.Get(uid)
	if lp == nil {
		l := NewList()
		l.init(key, pstore, clog)
		lcmap.Put(uid, unsafe.Pointer(l))
		return l
	}
	return (*List)(lp)
}

/*
func periodicQueueForProcessing(c *counters) {
	ticker := time.NewTicker(time.Minute)
	for _ = range ticker.C {
		lmap.StreamUntilCap(ch)
	}
}
*/

func process(c *counters, wg *sync.WaitGroup) {
	for eid := range ch {
		lp := lcmap.Get(eid)
		if lp == nil {
			continue
		}
		atomic.AddUint64(&c.merged, 1)
		l := (*List)(lp)
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
	lcmap.StreamAll(ch)
	close(ch)
}

/*
func StartPeriodicMerging() {
	ctr := new(counters)
	go periodicQueueForProcessing(ctr)
	go periodicProcess(ctr)
}
*/

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
