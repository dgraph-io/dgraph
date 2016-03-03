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

var maxmemory = flag.Uint64("stw_ram_mb", 4096,
	"If RAM usage exceeds this, we stop the world, and flush our buffers.")

type mergeRoutines struct {
	sync.RWMutex
	count int
}

func (mr *mergeRoutines) Count() int {
	mr.RLock()
	defer mr.RUnlock()
	return mr.count
}

func (mr *mergeRoutines) Add(delta int) {
	mr.Lock()
	mr.count += delta
	mr.Unlock()
}

type counters struct {
	ticker *time.Ticker
	added  uint64
	merged uint64
	clean  uint64
}

func (c *counters) periodicLog() {
	for _ = range c.ticker.C {
		c.log()
	}
}

func (c *counters) log() {
	added := atomic.LoadUint64(&c.added)
	merged := atomic.LoadUint64(&c.merged)
	var pending uint64
	if added > merged {
		pending = added - merged
	}

	glog.WithFields(logrus.Fields{
		"added":     added,
		"merged":    merged,
		"clean":     atomic.LoadUint64(&c.clean),
		"pending":   pending,
		"mapsize":   lhmap.Size(),
		"dirtysize": dirtymap.Size(),
	}).Info("List Merge counters")
}

func NewCounters() *counters {
	c := new(counters)
	c.ticker = time.NewTicker(time.Second)
	return c
}

var MIB, MAX_MEMORY uint64

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

func gentlyMerge(mr *mergeRoutines) {
	defer mr.Add(-1)
	ctr := NewCounters()
	defer ctr.ticker.Stop()

	// Pick 5% of the dirty map or 400 keys, whichever is higher.
	pick := int(float64(dirtymap.Size()) * 0.05)
	if pick < 400 {
		pick = 400
	}
	// We should start picking up elements from a randomly selected index,
	// otherwise, the same keys would keep on getting merged, while the
	// rest would never get a chance.
	var start int
	n := dirtymap.Size() - pick
	if n <= 0 {
		start = 0
	} else {
		start = rand.Intn(n)
	}

	var hs []gotomic.Hashable
	idx := 0
	dirtymap.Each(func(k gotomic.Hashable, v gotomic.Thing) bool {
		if idx < start {
			idx += 1
			return false
		}

		hs = append(hs, k)
		return len(hs) >= pick
	})

	for _, hid := range hs {
		dirtymap.Delete(hid)

		ret, ok := lhmap.Get(hid)
		if !ok || ret == nil {
			continue
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
	ctr.log()
}

func checkMemoryUsage() {
	MIB = 1 << 20
	MAX_MEMORY = *maxmemory * MIB

	var mr mergeRoutines
	for _ = range time.Tick(5 * time.Second) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if ms.Alloc > MAX_MEMORY {
			aggressivelyEvict(ms)

		} else {
			// If merging is slow, we don't want to end up having too many goroutines
			// merging the dirty list. This should keep them in check.
			if mr.Count() > 25 {
				glog.Info("Skipping gentle merging.")
				continue
			}
			mr.Add(1)
			// gentlyMerge can take a while to finish. So, run it in a goroutine.
			go gentlyMerge(&mr)
		}
	}
}

var stopTheWorld sync.RWMutex
var lhmap *gotomic.Hash
var dirtymap *gotomic.Hash
var clog *commit.Logger

func Init(log *commit.Logger) {
	lhmap = gotomic.NewHash()
	dirtymap = gotomic.NewHash()
	clog = log
	go checkMemoryUsage()
}

func GetOrCreate(key []byte, pstore *store.Store) *List {
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
	// No need to go through dirtymap, because we're going through
	// everything right now anyways.
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
	// We're merging all the lists, so just create a new dirtymap.
	dirtymap = gotomic.NewHash()

	ch := make(chan gotomic.Hashable, 10000)
	c := NewCounters()
	go c.periodicLog()
	defer c.ticker.Stop()
	go queueAll(ch, c)

	wg := new(sync.WaitGroup)
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go process(ch, c, wg)
	}
	wg.Wait()
	c.ticker.Stop()
}
