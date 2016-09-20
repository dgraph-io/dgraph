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

	keysBufferFrac = flag.Float64("keysbuffer", 0.07, "Number of keys to be buffered for deletion as fraction of lhmap.")
	lhmapNumShards = flag.Int("lhmap", 32, "Number of shards for lhmap.")
	dirtyMap       map[uint64]struct{} // Made global for log().
	dirtyChannel   chan uint64         // Puts to dirtyMap has to go through this channel.
	resetDirtyChan chan struct{}       // To clear dirtyMap, push to this channel.
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
func mergeAndUpdateKeys(keys []uint64, gentleMergeChan chan struct{}) {
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
		// Not calling processOne, because we don't want to
		// remove the postings list from the map, to avoid
		// a race condition, where another caller re-creates the
		// posting list before a merge happens.
		mergeAndUpdate(l, ctr)
	}
	ctr.log()
}

// processDirtyChan passes contents from dirty channel into dirty map.
// All write operations to dirtyMap should be contained in this function.
func processDirtyChan() {
	// Max number of gentle merges in parallel.
	gentleMergeChan := make(chan struct{}, 18)
	timer := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-timer.C:
			select {
			case gentleMergeChan <- struct{}{}:
				n := int(float64(len(dirtyMap)) * 0.10) // Clear 10% of dirty lists.
				keysBuffer := make([]uint64, 0, n)
				for key := range dirtyMap {
					delete(dirtyMap, key)
					keysBuffer = append(keysBuffer, key)
					if len(keysBuffer) > n {
						break
					}
				}
				go mergeAndUpdateKeys(keysBuffer, gentleMergeChan)
			default:
				log.Println("Skipping gentle merge")
			}

		case key := <-dirtyChannel:
			dirtyMap[key] = struct{}{}

		case <-resetDirtyChan:
			for k := range dirtyMap {
				delete(dirtyMap, k)
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
			aggressivelyEvict()
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
	dirtyChannel = make(chan uint64, 10000)
	dirtyMap = make(map[uint64]struct{}, 1000)
	resetDirtyChan = make(chan struct{}, 1)
	go checkMemoryUsage()
	go processDirtyChan()
}

// GetOrCreate stores the List corresponding to key(if its not there already)
// to lhmap and returns it.
func GetOrCreate(key []byte, pstore *store.Store) *List {
	fp := farm.Fingerprint64(key)

	stopTheWorld.RLock()
	defer stopTheWorld.RUnlock()
	lp, _ := lhmap.Get(fp)
	if lp != nil {
		return lp
	}

	l := NewList()
	lp = lhmap.PutIfMissing(fp, l)
	if lp == l {
		l.init(key, pstore)
		return l
	}
	return lp
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

func processOne(k uint64, c *counters) {
	l, _ := lhmap.Delete(k)
	if l == nil {
		return
	}
	l.SetForDeletion() // No more AddMutation.
	mergeAndUpdate(l, c)
}

// For on-demand merging of all lists, keysBuffer is read-only - do not touch.
func process(keysBuffer []uint64, idx *uint64, c *counters, wg *sync.WaitGroup) {
	// No need to go through dirtymap, because we're going through
	// everything right now anyways.
	const grainSize = 100
	for {
		last := atomic.AddUint64(idx, grainSize) // Take a batch of elements.
		start := last - grainSize
		if start >= uint64(len(keysBuffer)) {
			break
		}
		if last > uint64(len(keysBuffer)) {
			last = uint64(len(keysBuffer))
		}
		for i := start; i < last; i++ {
			processOne(keysBuffer[i], c)
		}
	}
	wg.Done()
}

func MergeLists(numRoutines int) {
	// We're merging all the lists, so just create a new dirtymap.
	resetDirtyChan <- struct{}{}

	c := newCounters()
	go c.periodicLog()
	defer c.ticker.Stop()

	// Read n items each time by iterating over list map.
	n := int(*keysBufferFrac * float64(lhmap.Size()))
	if n < 1000 {
		n = 1000
	}

	// Main idea: While iterating, we do not call processOne at all. The reason is
	// that when iterating, we lock big parts of the map, and we want to do it as
	// fast as possible, and without distraction from processOne which is also
	// trying to lock parts of the map. On one machine, the loading times goes from
	// 8 mins to <5 mins. We have also tried pushing to keysChan while consuming it
	// with processOne calls.  That seems to increase running time back to 7 mins.

	keysChan := make(chan uint64, n) // Allocate channel only once.
	defer close(keysChan)
	for lhmap.Size() > 0 {
		lhmap.StreamUntilCap(keysChan) // Don't close keysChan yet. We can reuse it.

		var wg sync.WaitGroup
		for i := 0; i < numRoutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Note: we don't do a range loop here because it will block when channel
				// becomes empty. Instead, we exit the goroutine once channel is empty.
				for {
					select {
					case k := <-keysChan:
						processOne(k, c)
					default:
						return
					}
				}
			}()
		}
		wg.Wait()
	}
}
