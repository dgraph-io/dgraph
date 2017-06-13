/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package posting

import (
	"context"
	"crypto/md5"
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

	"github.com/dgraph-io/badger/badger"
	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var (
	maxmemory = flag.Int("stw_ram_mb", 4096,
		"If RAM usage exceeds this, we stop the world, and flush our buffers.")

	commitFraction   = flag.Float64("gentlecommit", 0.10, "Fraction of dirty posting lists to commit every few seconds.")
	lhmapNumShards   = flag.Int("lhmap", 32, "Number of shards for lhmap.")
	dummyPostingList []byte // Used for indexing.
)

// syncMarks stores the watermark for synced RAFT proposals. Each RAFT proposal consists
// of many individual mutations, which could be applied to many different posting lists.
// Thus, each PL when being mutated would send an undone Mark, and each list would
// accumulate all such pending marks. When the PL is synced to RocksDB, it would
// mark all the pending ones as done.
// This ideally belongs to RAFT node struct (where committed watermark is being tracked),
// but because the logic of mutations is
// present here and to avoid a circular dependency, we've placed it here.
// Note that there's one watermark for each RAFT node/group.
// This watermark would be used for taking snapshots, to ensure that all the data and
// index mutations have been syned to RocksDB, before a snapshot is taken, and previous
// RAFT entries discarded.
type syncMarks struct {
	sync.RWMutex
	m map[uint32]*x.WaterMark
}

func init() {
	x.AddInit(func() {
		h := md5.New()
		pl := protos.PostingList{
			Checksum: h.Sum(nil),
		}
		var err error
		dummyPostingList, err = pl.Marshal()
		x.Check(err)
	})
}

func (g *syncMarks) create(group uint32) *x.WaterMark {
	g.Lock()
	defer g.Unlock()
	if g.m == nil {
		g.m = make(map[uint32]*x.WaterMark)
	}

	if prev, present := g.m[group]; present {
		return prev
	}
	w := &x.WaterMark{Name: fmt.Sprintf("Synced: Group %d", group)}
	w.Init()
	g.m[group] = w
	return w
}

func (g *syncMarks) Get(group uint32) *x.WaterMark {
	g.RLock()
	if w, present := g.m[group]; present {
		g.RUnlock()
		return w
	}
	g.RUnlock()
	return g.create(group)
}

// SyncMarkFor returns the synced watermark for the given RAFT group.
// We use this to determine the index to use when creating a new snapshot.
func SyncMarkFor(group uint32) *x.WaterMark {
	return marks.Get(group)
}

type counters struct {
	ticker  *time.Ticker
	done    uint64
	noop    uint64
	lastVal uint64
}

func (c *counters) periodicLog() {
	for range c.ticker.C {
		c.log()
	}
}

func (c *counters) log() {
	done := atomic.LoadUint64(&c.done)
	noop := atomic.LoadUint64(&c.noop)
	lastVal := atomic.LoadUint64(&c.lastVal)
	if done == lastVal {
		// Ignore.
		return
	}
	atomic.StoreUint64(&c.lastVal, done)

	log.Printf("Commit counters. done: %5d noop: %5d\n", done, noop)
}

func newCounters() *counters {
	c := new(counters)
	c.ticker = time.NewTicker(time.Second)
	go c.periodicLog()
	return c
}

type listMaps struct {
	x.SafeMutex
	m map[uint32]*listMap
}

func (l *listMaps) create(group uint32) *listMap {
	l.Lock()
	defer l.Unlock()
	if l.m == nil {
		l.m = make(map[uint32]*listMap)
	}

	if prev, present := l.m[group]; present {
		return prev
	}
	lhmap := newShardedListMap(*lhmapNumShards)
	l.m[group] = lhmap
	return lhmap
}

func (l *listMaps) get(group uint32) *listMap {
	// Don't store anything in group zero since we never compact
	// group 0 logs
	x.AssertTruef(group != 0, "group id is 0 for lhmap")
	l.RLock()
	if lhmap, present := l.m[group]; present {
		l.RUnlock()
		return lhmap
	}
	l.RUnlock()
	return l.create(group)
}

func (l *listMaps) groups() []uint32 {
	l.RLock()
	defer l.RUnlock()
	var groups []uint32
	for k := range l.m {
		groups = append(groups, k)
	}
	return groups
}

func lhmapFor(group uint32) *listMap {
	return lhmaps.get(group)
}

func aggressivelyEvict() {
	stopTheWorld.AssertLock()
	log.Println("Aggressive evict, committing to RocksDB")

	// To evict entries belonging to a group no longer served by the server
	// CommitLists shouldn't have any affect as the entries should have been synced before
	// we stopped serving the group
	var wg sync.WaitGroup
	for _, gid := range lhmaps.groups() {
		wg.Add(1)
		go func(gid uint32, wg *sync.WaitGroup) {
			EvictGroup(gid)
			wg.Done()
		}(gid, &wg)
	}
	wg.Wait()

	log.Println("Trying to free OS memory")
	// Forces garbage collection followed by returning as much memory to the OS
	// as possible.
	debug.FreeOSMemory()

	megs := getMemUsage()
	log.Printf("EVICT DONE! Memory usage after calling GC. Allocated MB: %v", megs)
}

func gentleCommit(dirtyMap map[fingerPrint]struct{}, pending chan struct{}) {
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
	keysBuffer := make([]fingerPrint, 0, n)

	for key := range dirtyMap {
		delete(dirtyMap, key)
		keysBuffer = append(keysBuffer, key)
		if len(keysBuffer) >= n {
			// We don't want to process the entire dirtyMap in one go.
			break
		}
	}

	go func(keys []fingerPrint) {
		defer func() { <-pending }()
		if len(keys) == 0 {
			return
		}
		ctr := newCounters()
		defer ctr.ticker.Stop()

		for _, key := range keys {
			l := lhmapFor(key.gid).Get(key.fp)
			if l == nil {
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
	dirtyMap := make(map[fingerPrint]struct{}, 1000)
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
			// Okay, we exceed the max memory threshold.
			// Stop the world, and deal with this first.
			log.Printf("Memory usage over threshold. STW. Allocated MB: %v\n", totMemory)
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

type fingerPrint struct {
	fp  uint64
	gid uint32
}

var (
	stopTheWorld x.SafeMutex
	pstore       *badger.KV
	syncCh       chan syncEntry
	dirtyChan    chan fingerPrint // All dirty posting list keys are pushed here.
	marks        *syncMarks
	lhmaps       *listMaps
)

// Init initializes the posting lists package, the in memory and dirty list hash.
func Init(ps *badger.KV) {
	marks = new(syncMarks)
	pstore = ps
	lhmaps = new(listMaps)
	dirtyChan = make(chan fingerPrint, 10000)
	fmt.Println("Starting commit routine.")
	syncCh = make(chan syncEntry, 10000)

	go periodicCommit()
	go batchSync()
}

// GetOrCreate stores the List corresponding to key, if it's not there already.
// to lhmap and returns it. It also returns a reference decrement function to be called by caller.
//
// plist, decr := GetOrCreate(key, store)
// defer decr()
// ... // Use plist
// TODO: This should take a node id and index. And just append all indices to a list.
// When doing a commit, it should update all the sync index watermarks.
// worker pkg would push the indices to the watermarks held by lists.
// And watermark stuff would have to be located outside worker pkg, maybe in x.
// That way, we don't have a dependency conflict.
func GetOrCreate(key []byte, group uint32) (rlist *List, decr func()) {
	fp := farm.Fingerprint64(key)

	stopTheWorld.RLock()
	defer stopTheWorld.RUnlock()

	lp := lhmapFor(group).Get(fp)
	if lp != nil {
		lp.incr()
		return lp, lp.decr
	}

	// Any initialization for l must be done before PutIfMissing. Once it's added
	// to the map, any other goroutine can retrieve it.
	l := getNew(key, pstore) // This retrieves a new *List and sets refcount to 1.
	l.water = marks.Get(group)

	lp = lhmapFor(group).PutIfMissing(fp, l)
	// We are always going to return lp to caller, whether it is l or not. So, let's
	// increment its reference counter.
	lp.incr()

	if lp != l {
		// Undo the increment in getNew() call above.
		l.decr()
	}
	pk := x.Parse(key)

	// This replaces "TokensTable". The idea is that we want to quickly add the
	// index key to the data store, with essentially an empty value. We just need
	// the keys for filtering / sorting.
	if l == lp && pk.IsIndex() {
		// Lock before entering goroutine. Otherwise, some tests in query will fail.
		l.Lock()
		go func(key []byte) {
			defer l.Unlock()
			var item badger.KVItem
			err := pstore.Get(key, &item)
			x.Check(err)
			val := item.Value()
			if len(val) == 0 {
				pstore.Set(key, dummyPostingList)
			}
		}(key)
	}
	return lp, lp.decr
}

// GetOrUnmarshal takes a key, value and a groupID. It checks if the in-memory map has an
// updated value and returns it if it exists or it unmarshals the value passed and returns
// the list.
func GetOrUnmarshal(key, val []byte, gid uint32) (rlist *List, decr func()) {
	fp := farm.Fingerprint64(key)

	stopTheWorld.RLock()
	lp := lhmapFor(gid).Get(fp)
	stopTheWorld.RUnlock()

	if lp != nil {
		lp.incr()
		return lp, lp.decr
	}

	var pl protos.PostingList
	pl.Unmarshal(val)
	lp = getNew(key, pstore) // This retrieves a new *List and sets refcount to 1.
	lp.mlayer = pl.Postings

	return lp, lp.decr
}

func commitOne(l *List, c *counters) {
	if l == nil {
		return
	}
	if merged, err := l.SyncIfDirty(context.Background()); err != nil {
		log.Printf("Error while committing dirty list: %v\n", err)
	} else if merged {
		atomic.AddUint64(&c.done, 1)
	} else {
		atomic.AddUint64(&c.noop, 1)
	}
}

func CommitLists(numRoutines int, group uint32) {
	if group == 0 {
		return
	}
	c := newCounters()
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
				commitOne(l, c)
			}
		}()
	}

	lhmapFor(group).Each(func(k uint64, l *List) {
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

// EvictAll removes all pl's stored in memory for given group
func EvictGroup(group uint32) {
	// This is serialized by raft so no need to worry about race condition from getOrCreate
	// request from same group
	CommitLists(1, group)
	lhmapFor(group).EachWithDelete(func(k uint64, l *List) {
		l.SetForDeletion()
		l.decr()
	})
}

// The following logic is used to batch up all the writes to RocksDB.
type syncEntry struct {
	key     []byte
	val     []byte
	water   *x.WaterMark
	pending []uint64
	sw      *x.SafeWait
}

func batchSync() {
	var entries []syncEntry
	var loop uint64
	wb := make([]*badger.Entry, 0, 100)

	for {
		select {
		case e := <-syncCh:
			entries = append(entries, e)

		default:
			// default is executed if no other case is ready.
			start := time.Now()
			if len(entries) > 0 {
				loop++
				fmt.Printf("[%4d] Writing batch of size: %v\n", loop, len(entries))
				for _, e := range entries {
					if e.val == nil {
						wb = badger.EntriesDelete(wb, e.key)
					} else {
						wb = badger.EntriesSet(wb, e.key, e.val)
					}
				}
				pstore.BatchSet(wb)
				wb = wb[:0]

				for _, e := range entries {
					e.sw.Done()
					if e.water != nil {
						e.water.Ch <- x.Mark{Indices: e.pending, Done: true}
					}
				}
				entries = entries[:0]
			}
			// Add a sleep clause to avoid a busy wait loop if there's no input to commitCh.
			sleepFor := 10*time.Millisecond - time.Since(start)
			time.Sleep(sleepFor)
		}
	}
}
