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
	"flag"
	"math/rand"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgryski/go-farm"
	"github.com/zond/gotomic"
)

var maxmemory = flag.Uint64("threshold_ram_mb", 3072,
	"If RAM usage exceeds this, we stop the world, and flush our buffers.")

type counters struct {
	ticker *time.Ticker
	added  uint64
	merged uint64
}

func (c *counters) periodicLog() {
	for _ = range c.ticker.C {
		mapSize := lhmap.Size()
		added := atomic.LoadUint64(&c.added)
		merged := atomic.LoadUint64(&c.merged)
		pending := added - merged

		glog.WithFields(logrus.Fields{
			"added":   added,
			"merged":  merged,
			"pending": pending,
			"mapsize": mapSize,
		}).Info("List Merge counters")
	}
}

var MAX_MEMORY uint64
var MIB uint64

func checkMemoryUsage() {
	MIB = 1 << 20
	MAX_MEMORY = *maxmemory * (1 << 20)

	for _ = range time.Tick(5 * time.Second) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if ms.Alloc < MAX_MEMORY {
			continue
		}

		// Okay, we exceed the max memory threshold.
		// Stop the world, and deal with this first.
		stopTheWorld.Lock()
		megs := ms.Alloc / MIB
		glog.WithField("allocated_MB", megs).
			Info("Memory usage over threshold. STOPPED THE WORLD!")

		glog.Info("Calling merge on all lists.")
		MergeLists(100 * runtime.GOMAXPROCS(-1))

		glog.Info("Merged lists. Calling GC.")
		runtime.GC() // Call GC to do some cleanup.
		glog.Info("Trying to free OS memory")
		debug.FreeOSMemory()

		runtime.ReadMemStats(&ms)
		megs = ms.Alloc / MIB
		glog.WithField("allocated_MB", megs).
			Info("Memory Usage after calling GC.")
		stopTheWorld.Unlock()
	}
}

var stopTheWorld sync.RWMutex
var lhmap *gotomic.Hash
var pstore *store.Store
var clog *commit.Logger

func Init(posting *store.Store, log *commit.Logger) {
	lhmap = gotomic.NewHash()
	pstore = posting
	clog = log
	lc = new(lcounters)
	go checkMemoryUsage()
}

func GetOrCreate(key []byte) *List {
	stopTheWorld.RLock()
	defer stopTheWorld.RUnlock()

	uid := farm.Fingerprint64(key)
	ukey := gotomic.IntKey(uid)
	lp, _ := lhmap.Get(ukey)
	if lp != nil {
		return lp.(*List)
	}

	l := NewList()
	l.init(key, pstore, clog)
	if inserted := lhmap.PutIfMissing(ukey, l); inserted {
		return l
	} else {
		lp, _ = lhmap.Get(ukey)
		return lp.(*List)
	}
}

func processOne(k gotomic.Hashable, c *counters) {
	ret, _ := lhmap.Delete(k)
	if ret == nil {
		return
	}
	l := ret.(*List)
	if l == nil {
		return
	}

	l.SetForDeletion() // No more AddMutation.
	if err := l.MergeIfDirty(); err != nil {
		glog.WithError(err).Error("While commiting dirty list.")
	}
	atomic.AddUint64(&c.merged, 1)
}

// For on-demand merging of all lists.
func process(ch chan gotomic.Hashable, c *counters, wg *sync.WaitGroup) {
	for l := range ch {
		processOne(l, c)
	}
	if wg != nil {
		wg.Done()
	}
}

func queueAll(ch chan gotomic.Hashable, c *counters) {
	lhmap.Each(func(k gotomic.Hashable, v gotomic.Thing) bool {
		ch <- k
		atomic.AddUint64(&c.added, 1)
		return false // If this returns true, Each would break.
	})
	close(ch)
}

func MergeLists(numRoutines int) {
	ch := make(chan gotomic.Hashable, 10000)
	c := new(counters)
	c.ticker = time.NewTicker(time.Second)
	go c.periodicLog()
	go queueAll(ch, c)

	wg := new(sync.WaitGroup)
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go process(ch, c, wg)
	}
	wg.Wait()
	c.ticker.Stop()
}

// For periodic merging of lists.
func queueRandomLists(ch chan gotomic.Hashable, c *counters) {
	var buf []gotomic.Hashable
	var count int
	needed := cap(ch) - len(ch)
	if needed < 100 {
		return
	}

	// Generate a random list of
	lhmap.Each(func(k gotomic.Hashable, v gotomic.Thing) bool {
		if count < needed {
			buf = append(buf, k)

		} else {
			j := rand.Intn(count)
			if j < len(buf) {
				buf[j] = k
			}
		}
		count += 1
		return false
	})

	for _, k := range buf {
		ch <- k
		atomic.AddUint64(&c.added, 1)
	}
}

func periodicQueueForProcessing(ch chan gotomic.Hashable, c *counters) {
	ticker := time.NewTicker(time.Minute)
	for _ = range ticker.C {
		queueRandomLists(ch, c)
	}
}

func periodicProcess(ch chan gotomic.Hashable, c *counters) {
	ticker := time.NewTicker(100 * time.Millisecond)
	for _ = range ticker.C {
		hid := <-ch
		processOne(hid, c)
	}
}

func StartPeriodicMerging() {
	ctr := new(counters)
	ch := make(chan gotomic.Hashable, 10000)
	go periodicQueueForProcessing(ch, ctr)
	go periodicProcess(ch, ctr)
}
