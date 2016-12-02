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

	"github.com/dgraph-io/dgraph/rdb"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

var (
	maxmemory = flag.Int("stw_ram_mb", 4096,
		"If RAM usage exceeds this, we stop the world, and flush our buffers.")

	commitFraction = flag.Float64("gentlecommit", 0.10, "Fraction of dirty posting lists to commit every few seconds.")
	lhmapNumShards = flag.Int("lhmap", 32, "Number of shards for lhmap.")

	dirtyChan       chan uint64 // All dirty posting list keys are pushed here.
	startCommitOnce sync.Once
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

	log.Printf("List commit counters. added: %d commitd: %d clean: %d"+
		" pending: %d\n", added, merged, atomic.LoadUint64(&c.clean), pending)
}

func newCounters() *counters {
	c := new(counters)
	c.ticker = time.NewTicker(time.Second)
	return c
}

func aggressivelyEvict() {
	// Okay, we exceed the max memory threshold.
	// Stop the world, and deal with this first.
	megs := getMemUsage()
	log.Printf("Memory usage over threshold. STW. Allocated MB: %v\n", megs)

	log.Println("Aggressive evict, committing to RocksDB")
	CommitLists(100 * runtime.GOMAXPROCS(-1))

	log.Println("Trying to free OS memory")
	// Forces garbage collection followed by returning as much memory to the OS
	// as possible.
	debug.FreeOSMemory()

	megs = getMemUsage()
	log.Printf("EVICT DONE! Memory usage after calling GC. Allocated MB: %v", megs)
}

func gentleCommit(dirtyMap map[uint64]struct{}, pending chan struct{}) {
	select {
	case pending <- struct{}{}:
	default:
		fmt.Println("Skipping gentleCommit")
		return
	}

	// NOTE: No need to acquire read lock for stopTheWorld. This portion is being run
	// serially alongside aggressive commit.

	n := int(float64(len(dirtyMap)) * *commitFraction)
	if n < 1000 {
		// Have a min value of n, so we can merge small number of dirty PLs fast.
		n = 1000
	}
	keysBuffer := make([]uint64, 0, n)

	for key := range dirtyMap {
		delete(dirtyMap, key)
		keysBuffer = append(keysBuffer, key)
		if len(keysBuffer) >= n {
			// We don't want to process the entire dirtyMap in one go.
			break
		}
	}

	go func(keys []uint64) {
		defer func() { <-pending }()
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
			// where another caller re-creates the posting list before a commit happens.
			commitOne(l, ctr)
		}
		ctr.log()
	}(keysBuffer)
}

// periodicMerging periodically merges the dirty posting lists. It also checks our memory
// usage. If it exceeds a certain threshold, it would stop the world, and aggressively
// merge and evict all posting lists from memory.
func periodicCommit() {
	ticker := time.NewTicker(5 * time.Second)
	dirtyMap := make(map[uint64]struct{}, 1000)
	// pending is used to ensure that we only have up to 15 goroutines doing gentle commits.
	pending := make(chan struct{}, 15)
	dsize := 0 // needed for better reporting.
	for {
		select {
		case key := <-dirtyChan:
			dirtyMap[key] = struct{}{}

		case <-ticker.C:
			if len(dirtyMap) != dsize {
				dsize = len(dirtyMap)
				log.Printf("Dirty map size: %d\n", dsize)
			}

			totMemory := getMemUsage()
			if totMemory <= *maxmemory {
				gentleCommit(dirtyMap, pending)
				break
			}

			// Do aggressive commit, which would delete all the PLs from memory.
			// Acquire lock, so no new posting lists are given out.
			stopTheWorld.Lock()
		DIRTYLOOP:
			// Flush out the dirtyChan after acquiring lock. This allow posting lists which
			// are currently being processed to not get stuck on dirtyChan, which won't be
			// processed until aggressive evict finishes.
			for {
				select {
				case <-dirtyChan:
					// pass
				default:
					break DIRTYLOOP
				}
			}
			aggressivelyEvict()
			for k := range dirtyMap {
				delete(dirtyMap, k)
			}
			stopTheWorld.Unlock()
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

var (
	stopTheWorld sync.RWMutex
	lhmap        *listMap
	pstore       *store.Store
	commitCh     chan commitEntry
)

func StartCommit() {
	startCommitOnce.Do(func() {
		fmt.Println("Starting commit routine.")
		commitCh = make(chan commitEntry, 10000)
		go batchCommit()
	})
}

// Init initializes the posting lists package, the in memory and dirty list hash.
func Init(ps *store.Store) {
	pstore = ps
	initIndex()
	lhmap = newShardedListMap(*lhmapNumShards)
	dirtyChan = make(chan uint64, 10000)
	StartCommit()
	go periodicCommit()
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
func GetOrCreate(key []byte) (rlist *List, decr func()) {
	fp := farm.Fingerprint64(key)

	stopTheWorld.RLock()
	defer stopTheWorld.RUnlock()

	lp, _ := lhmap.Get(fp)
	if lp != nil {
		lp.incr()
		return lp, lp.decr
	}

	l := getNew(key, pstore) // This retrieves a new *List and sets refcount to 1.
	lp = lhmap.PutIfMissing(fp, l)
	// We are always going to return lp to caller, whether it is l or not. So, let's
	// increment its reference counter.
	lp.incr()

	if lp != l {
		// Undo the increment in getNew() call above.
		l.decr()
	}
	return lp, lp.decr
}

func commitOne(l *List, c *counters) {
	if l == nil {
		return
	}
	if merged, err := l.CommitIfDirty(context.Background()); err != nil {
		log.Printf("Error while commiting dirty list: %v\n", err)
	} else if merged {
		atomic.AddUint64(&c.merged, 1)
	} else {
		atomic.AddUint64(&c.clean, 1)
	}
}

func CommitLists(numRoutines int) {
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
				commitOne(l, c)
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

// The following logic is used to batch up all the writes to RocksDB.
type commitEntry struct {
	key []byte
	val []byte
	sw  *x.SafeWait
}

func batchCommit() {
	ticker := time.NewTicker(10 * time.Millisecond)

	var b *rdb.WriteBatch
	var sz int
	var waits []*x.SafeWait
	var loop uint64

	put := func(e commitEntry) {
		if b == nil {
			b = pstore.NewWriteBatch()
		}
		b.Put(e.key, e.val)
		sz++
		waits = append(waits, e.sw)
	}

	for {
		select {
		case e := <-commitCh:
			put(e)

		case <-ticker.C:
		PICKALL:
			for {
				select {
				case e := <-commitCh:
					put(e)
				default:
					break PICKALL
				}
			}
			if sz == 0 || b == nil {
				break
			}
			loop++
			fmt.Printf("[%4d] Writing batch of size: %v\n", loop, sz)
			x.Checkf(pstore.WriteBatch(b), "Error while writing to RocksDB.")
			for _, w := range waits {
				w.Done()
			}
			b.Destroy()
			b = nil
			sz = 0
			waits = waits[:0]
		}
	}
}
