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

var minmemory = flag.Uint64("min_ram_mb", 2048,
	"If RAM usage exceeds this, start periodically evicting posting lists"+
		" from memory.")
var maxmemory = flag.Uint64("max_ram_mb", 4096,
	"If RAM usage exceeds this, we stop the world, and flush our buffers.")

type counters struct {
	ticker *time.Ticker
	added  uint64
	merged uint64
	clean  uint64
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
			"clean":   atomic.LoadUint64(&c.clean),
			"pending": pending,
			"mapsize": mapSize,
		}).Info("List Merge counters")
	}
}

func NewCounters() *counters {
	c := new(counters)
	c.ticker = time.NewTicker(time.Second)
	go c.periodicLog()
	return c
}

var MIB, MAX_MEMORY, MIN_MEMORY uint64

func aggressivelyEvict(ms runtime.MemStats) {
	// Okay, we exceed the max memory threshold.
	// Stop the world, and deal with this first.
	stopTheWorld.Lock()
	defer stopTheWorld.Unlock()

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
}

func gentlyMerge(ms runtime.MemStats) {
	ctr := NewCounters()
	defer ctr.ticker.Stop()

	count := 0
	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()
	for _ = range t.C {
		count += 1
		if count > 400 {
			break // We're doing 100 per second. So, stop after 4 seconds.
		}
		ret, ok := dirtyList.Pop()
		if !ok || ret == nil {
			break
		}
		// Not calling processOne, because we don't want to
		// remove the postings list from the map, to avoid
		// a race condition, where another caller re-creates the
		// posting list before a merge happens.
		l := ret.(*List)
		if l == nil {
			continue
		}
		mergeAndUpdate(l, ctr)
	}
}

func checkMemoryUsage() {
	MIB = 1 << 20
	MAX_MEMORY = *maxmemory * MIB
	MIN_MEMORY = *minmemory * MIB // Not being used right now.

	for _ = range time.Tick(5 * time.Second) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if ms.Alloc > MAX_MEMORY {
			aggressivelyEvict(ms)

		} else {
			gentlyMerge(ms)
		}
	}
}

var stopTheWorld sync.RWMutex
var lhmap *gotomic.Hash
var dirtyList *gotomic.List
var pstore *store.Store
var clog *commit.Logger

func Init(posting *store.Store, log *commit.Logger) {
	lhmap = gotomic.NewHash()
	dirtyList = gotomic.NewList()
	pstore = posting
	clog = log
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
	if inserted := lhmap.PutIfMissing(ukey, l); inserted {
		l.init(key, pstore, clog)
		return l
	} else {
		lp, _ = lhmap.Get(ukey)
		return lp.(*List)
	}
}

func mergeAndUpdate(l *List, c *counters) {
	if l == nil {
		return
	}
	if merged, err := l.MergeIfDirty(); err != nil {
		glog.WithError(err).Error("While commiting dirty list.")
	} else if merged {
		atomic.AddUint64(&c.merged, 1)
	} else {
		atomic.AddUint64(&c.clean, 1)
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
	mergeAndUpdate(l, c)
}

// For on-demand merging of all lists.
func process(ch chan gotomic.Hashable, c *counters, wg *sync.WaitGroup) {
	for dirtyList.Size() > 0 {
		ret, ok := dirtyList.Pop()
		if !ok || ret == nil {
			continue
		}
		l := ret.(*List)
		mergeAndUpdate(l, c)
	}

	for k := range ch {
		processOne(k, c)
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
	c := NewCounters()
	go queueAll(ch, c)

	wg := new(sync.WaitGroup)
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go process(ch, c, wg)
	}
	wg.Wait()
	c.ticker.Stop()
}
