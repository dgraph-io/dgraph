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

	"github.com/dgraph-io/dgraph/store"
)

var (
	maxmemory = flag.Int("stw_ram_mb", 4096,
		"If RAM usage exceeds this, we stop the world, and flush our buffers.")

	gentleMergeFrac = flag.Float64("gentlemerge", 0.10, "Fraction of dirty posting lists to merge every few seconds.")
	lhmapNumShards  = flag.Int("lhmap", 32, "Number of shards for lhmap.")

	dirtyMap        map[uint64]struct{} // Made global for log().
	dirtyChan       chan uint64         // Puts to dirtyMap has to go through here.
	dirtyMapOpChan  chan dirtyMapOp     // Ops on dirtyMap except puts go in here.
	gentleMergeChan chan struct{}
)

// dirtyMapOp is a bulk operation on dirtyMap.
type dirtyMapOp int

const (
	dirtyMapOpReset       = iota
	dirtyMapOpGentleMerge = iota
)

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
		pending, lhmap.Size(), len(dirtyMap))
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

	log.Println("Aggressive evict, committing to RocksDB")
	MergeLists(100 * runtime.GOMAXPROCS(-1))

	log.Println("Trying to free OS memory")
	// Forces garbage collection followed by returning as much memory to the OS
	// as possible.
	debug.FreeOSMemory()

	megs = getMemUsage()
	log.Printf("Memory usage after calling GC. Allocated MB: %v", megs)
}

// mergeAndUpdateKeys calls mergeAndUpdate for each key in array "keys".
func mergeAndUpdateKeys(keys []uint64) {
	defer func() { <-gentleMergeChan }()
	if len(keys) == 0 {
		return
	}
	ctr := newCounters()
	defer ctr.ticker.Stop()

	for _, key := range keys {
		l, ok := lhmap.Get(key)
		if !ok || l == nil {
			continue
		}
		// Not removing the postings list from the map, to avoid a race condition,
		// where another caller re-creates the posting list before a merge happens.
		mergeAndUpdate(l, ctr)
	}
	ctr.log()
}

// processDirtyChan passes contents from dirty channel into dirty map.
// All write operations to dirtyMap should be contained in this function.
func processDirtyChan() {
	keysBuffer := make([]uint64, 0, 10000)
	for {
		select {
		case key := <-dirtyChan:
			dirtyMap[key] = struct{}{}

		case op := <-dirtyMapOpChan:
			switch op {
			case dirtyMapOpGentleMerge:
				select {
				case gentleMergeChan <- struct{}{}:
					n := int(float64(len(dirtyMap)) * *gentleMergeFrac)
					if cap(keysBuffer) < n {
						// Resize keysBuffer to newCap := max(2*current cap, n).
						newCap := 2 * cap(keysBuffer)
						if newCap < n {
							newCap = n
						}
						keysBuffer = make([]uint64, 0, newCap)
					}
					for key := range dirtyMap {
						delete(dirtyMap, key)
						keysBuffer = append(keysBuffer, key)
						if len(keysBuffer) > n {
							break
						}
					}
					go mergeAndUpdateKeys(keysBuffer)
				default:
					log.Println("Skipping gentle merge")
				}

			case dirtyMapOpReset:
				for k := range dirtyMap {
					delete(dirtyMap, k)
				}
			}
		}
	}
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

// checkMemoryUsage monitors the memory usage by the running process and
// calls aggressively evict if the threshold is reached.
func checkMemoryUsage() {
	for _ = range time.Tick(5 * time.Second) {
		totMemory := getMemUsage()
		if totMemory > *maxmemory {
			// Although gentle merges cannot start after aggressive evict, some of them
			// might still be running and try to lock lhmap.
			aggressivelyEvict()
		} else {
			// Gentle merge should not happen during an aggressive evict.
			// This push into dirtyMapOpChan might be blocked by another gentle merge
			// (just the copying into keysBuffers but not the actual RocksDB work) or a
			// reset of dirtymap.
			dirtyMapOpChan <- dirtyMapOpGentleMerge
		}
	}
}

var (
	stopTheWorld sync.RWMutex
	lhmap        *listMap
)

// Init initializes the posting lists package, the in memory and dirty list hash.
func Init() {
	lhmap = newShardedListMap(*lhmapNumShards)
	dirtyChan = make(chan uint64, 10000)
	dirtyMap = make(map[uint64]struct{}, 1000)
	dirtyMapOpChan = make(chan dirtyMapOp, 1)
	// Capacity is max number of gentle merges that can happen in parallel.
	gentleMergeChan = make(chan struct{}, 18)
	go checkMemoryUsage()
	go processDirtyChan()
}

func getFromMap(key uint64) *List {
	lp, _ := lhmap.Get(key)
	if lp == nil {
		return nil
	}
	lp.incr()
	return lp
}

// GetOrCreate stores the List corresponding to key, if it's not there already.
// to lhmap and returns it. It also returns a reference decrement function to be called by caller.
//
// plist, decr := GetOrCreate(key, store)
// defer decr()
// ... // Use plist
func GetOrCreate(key []byte, pstore *store.Store) (rlist *List, decr func()) {
	fp := farm.Fingerprint64(key)

	stopTheWorld.RLock()
	defer stopTheWorld.RUnlock()

	lp, _ := lhmap.Get(fp)
	if lp != nil {
		lp.incr()
		return lp, lp.decr
	}

	l := getNew() // This retrieves a new *List and increments its ref count.
	lp = lhmap.PutIfMissing(fp, l)
	// We are always going to return lp to caller, whether it is l or not. So, let's
	// increment its reference counter.
	lp.incr()

	if lp == l {
		l.init(key, pstore)
	} else {
		// Undo the increment in getNew() call above.
		l.decr()
	}
	return lp, lp.decr
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

func MergeLists(numRoutines int) {
	// We're merging all the lists, so just create a new dirtymap.
	// This push into dirtyMapOpChan can be blocked by a gentle merge as the channel
	// has very low capacity.
	dirtyMapOpChan <- dirtyMapOpReset

	c := newCounters()
	go c.periodicLog()
	defer c.ticker.Stop()

	// We iterate over lhmap, deleting keys and pushing values (List) into this
	// channel. Then goroutines right below will commit these lists to data store.
	workChan := make(chan *List, 10000)

	var wg sync.WaitGroup
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for l := range workChan {
				l.SetForDeletion() // No more AddMutation.
				mergeAndUpdate(l, c)
				l.decr()
			}
		}()
	}

	lhmap.EachWithDelete(func(k uint64, l *List) {
		if l == nil { // To be safe. Check might be unnecessary.
			return
		}
		// We lose one reference for deletion from lhmap. But we gain one reference
		// for pushing into workChan. So no decr or incr here.
		workChan <- l
	})
	close(workChan)
	wg.Wait()
}
