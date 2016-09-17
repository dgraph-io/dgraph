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
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgryski/go-farm"
	"github.com/zond/gotomic"

	"github.com/dgraph-io/dgraph/store"
)

var maxmemory = flag.Int("stw_ram_mb", 4096,
	"If RAM usage exceeds this, we stop the world, and flush our buffers.")

var maxLists = flag.Int("maxlists", 50000, "We clear lhmap until we have less than this number of posting lists left.")
var bufferedKeysSize = flag.Int("bufferedkeysize", 1000, "Number of keys to be buffered for deletion.")

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
	ticker  *time.Ticker
	added   uint64
	merged  uint64
	clean   uint64
	lastVal uint64
}

func (c *counters) periodicLog() {
	for _ = range c.ticker.C {
		c.log()
	}
}

func (c *counters) log() {
	added := atomic.LoadUint64(&c.added)
	merged := atomic.LoadUint64(&c.merged)
	lastVal := atomic.LoadUint64(&c.lastVal)
	if merged == lastVal {
		// Ignore.
		return
	}
	atomic.StoreUint64(&c.lastVal, merged)

	var pending uint64
	if added > merged {
		pending = added - merged
	}

	log.Printf("List merge counters. added: %d merged: %d clean: %d"+
		" pending: %d mapsize: %d dirtysize: %d\n",
		added, merged, atomic.LoadUint64(&c.clean),
		pending, lhmap.Size(), dirtymap.Size())
}

func newCounters() *counters {
	c := new(counters)
	c.ticker = time.NewTicker(time.Second)
	return c
}

func aggressivelyEvict() {
	// Okay, we exceed the max memory threshold.
	// Stop the world, and deal with this first.
	stopTheWorld.Lock()
	defer stopTheWorld.Unlock()

	megs := getMemUsage()
	log.Printf("Memory usage over threshold. STW. Allocated MB: %v\n", megs)

	log.Println("Calling merge on all lists.")
	MergeLists(100 * runtime.GOMAXPROCS(-1))

	log.Println("Trying to free OS memory")
	// Forces garbage collection followed by returning as much memory to the OS
	// as possible.
	debug.FreeOSMemory()

	megs = getMemUsage()
	log.Printf("Memory usage after calling GC. Allocated MB: %v", megs)
}

func gentlyMerge(mr *mergeRoutines) {
	defer mr.Add(-1)
	ctr := newCounters()
	defer ctr.ticker.Stop()

	// Pick 7% of the dirty map or 400 keys, whichever is higher.
	pick := int(float64(dirtymap.Size()) * 0.07)
	if pick < 400 {
		pick = 400
	}
	// We should start picking up elements from a randomly selected index,
	// otherwise, the same keys would keep on getting merged, while the
	// rest would never get a chance.
	var start int
	n := dirtymap.Size() - pick
	if n > 0 {
		start = rand.Intn(n)
	}

	idx := 0
	count := 0
	dirtymap.Each(func(k gotomic.Hashable, v gotomic.Thing) bool {
		if idx < start {
			idx++
			return false
		}

		dirtymap.Delete(k)

		ret, ok := lhmap.Get(k.(gotomic.IntKey))
		if !ok || ret == nil {
			return false
		}
		// Not calling processOne, because we don't want to
		// remove the postings list from the map, to avoid
		// a race condition, where another caller re-creates the
		// posting list before a merge happens.
		l := ret.(*List)
		if l == nil {
			return false
		}
		mergeAndUpdate(l, ctr)
		count++
		return count >= pick
	})

	ctr.log()
}

// getMemUsage returns the amount of memory used by the process in MB
func getMemUsage() int {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	megs := ms.Alloc / (1 << 20)
	return int(megs)

	// Sticking to ms.Alloc temoprarily.
	// TODO(Ashwin): Switch to total Memory(RSS) once we figure out
	// how to release memory to OS (Currently only a small chunk
	// is returned)
	if runtime.GOOS != "linux" {
		pid := os.Getpid()
		cmd := fmt.Sprintf("ps -ao rss,pid | grep %v", pid)
		c1, err := exec.Command("bash", "-c", cmd).Output()
		if err != nil {
			// In case of error running the command, resort to go way
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			megs := ms.Alloc / (1 << 20)
			return int(megs)
		}

		rss := strings.Split(string(c1), " ")[0]
		kbs, err := strconv.Atoi(rss)
		if err != nil {
			return 0
		}

		megs := kbs / (1 << 10)
		return megs
	}

	contents, err := ioutil.ReadFile("/proc/self/stat")
	if err != nil {
		log.Println("Can't read the proc file", err)
		return 0
	}

	cont := strings.Split(string(contents), " ")
	// 24th entry of the file is the RSS which denotes the number of pages
	// used by the process.
	if len(cont) < 24 {
		log.Println("Error in RSS from stat")
		return 0
	}

	rss, err := strconv.Atoi(cont[23])
	if err != nil {
		log.Println(err)
		return 0
	}

	return rss * os.Getpagesize() / (1 << 20)
}

// checkMeomeryUsage monitors the memory usage by the running process and
// calls aggressively evict if the threshold is reached.
func checkMemoryUsage() {
	var mr mergeRoutines
	for _ = range time.Tick(5 * time.Second) {
		totMemory := getMemUsage()
		if totMemory > *maxmemory {
			aggressivelyEvict()
		} else {
			// If merging is slow, we don't want to end up having too many goroutines
			// merging the dirty list. This should keep them in check.
			// With a value of 18 and duration of 5 seconds, some goroutines are
			// taking over 1.5 mins to finish.
			if mr.Count() > 18 {
				log.Println("Skipping gentle merging.")
				continue
			}
			mr.Add(1)
			// gentlyMerge can take a while to finish. So, run it in a goroutine.
			go gentlyMerge(&mr)
		}
	}
}

var (
	stopTheWorld sync.RWMutex
	//	lhmap        *gotomic.Hash
	lhmap    *shardedListHash
	dirtymap *gotomic.Hash
)

// Init initializes the posting lists package, the in memory and dirty list hash.
func Init() {
	//	lhmap = gotomic.NewHash()
	lhmap = newShardedListHash(32)
	dirtymap = gotomic.NewHash()
	go checkMemoryUsage()
}

// GetOrCreate stores the List corresponding to key(if its not there already)
// to lhmap and returns it.
func GetOrCreate(key []byte, pstore *store.Store) *List {
	fp := farm.Fingerprint64(key)
	gotomicKey := gotomic.IntKey(fp)

	stopTheWorld.RLock()
	defer stopTheWorld.RUnlock()
	lp, _ := lhmap.Get(gotomicKey)
	if lp != nil {
		return lp.(*List)
	}

	l := NewList()
	if inserted := lhmap.PutIfMissing(gotomicKey, l); inserted {
		l.init(key, pstore)
		return l
	}
	lp, _ = lhmap.Get(gotomicKey)
	return lp.(*List)
}

func mergeAndUpdate(l *List, c *counters) {
	if l == nil {
		return
	}
	if merged, err := l.MergeIfDirty(context.Background()); err != nil {
		log.Printf("Error while commiting dirty list: %v\n", err)
	} else if merged {
		atomic.AddUint64(&c.merged, 1)
	} else {
		atomic.AddUint64(&c.clean, 1)
	}
}

func processOne(k gotomic.Hashable, c *counters) {
	ret, _ := lhmap.Delete(k.(gotomic.IntKey))
	if ret == nil {
		//		log.Printf("Deletion failed! %d\n", k)
		return
	}
	//	log.Println("Deletion successful!")
	l := ret.(*List)

	if l == nil {
		return
	}
	l.SetForDeletion() // No more AddMutation.
	mergeAndUpdate(l, c)
}

// For on-demand merging of all lists.
func process(bufferedKeys []gotomic.Hashable, idx *uint64, c *counters, wg *sync.WaitGroup) {
	// No need to go through dirtymap, because we're going through
	// everything right now anyways.
	const grainSize = 100
	var done bool
	for !done {
		last := atomic.AddUint64(idx, grainSize) // Take a batch of elements.
		start := last - grainSize
		if last >= uint64(len(bufferedKeys)) {
			// Do not exceeed buffer.
			last = uint64(len(bufferedKeys))
			done = true
		}
		for i := start; i < last; i++ {
			processOne(bufferedKeys[i], c)
		}
	}
	wg.Done()
}

func MergeLists(numRoutines int) {
	// We're merging all the lists, so just create a new dirtymap.
	dirtymap = gotomic.NewHash()
	c := newCounters()
	go c.periodicLog()
	defer c.ticker.Stop()

	// Could make global?
	bufferedKeys := make([]gotomic.Hashable, *bufferedKeysSize)
	for lhmap.Size() >= *maxLists {
		size := lhmap.MultiGet(bufferedKeys, *bufferedKeysSize)
		var idx uint64 // Only atomic adds allowed for this!
		var wg sync.WaitGroup
		for i := 0; i < numRoutines; i++ {
			wg.Add(1)
			go process(bufferedKeys[:size], &idx, c, &wg)
		}
		wg.Wait()
	}

	c.ticker.Stop()
}
