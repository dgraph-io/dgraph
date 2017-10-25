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
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var (
	dummyPostingList []byte // Used for indexing.
	elog             trace.EventLog
)

const (
	MB = 1 << 20
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
	elog = trace.NewEventLog("Memory", "")
}

func getMemUsage() int {
	if runtime.GOOS != "linux" {
		pid := os.Getpid()
		cmd := fmt.Sprintf("ps -ao rss,pid | grep %v", pid)
		c1, err := exec.Command("bash", "-c", cmd).Output()
		if err != nil {
			// In case of error running the command, resort to go way
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			megs := ms.Alloc
			return int(megs)
		}

		rss := strings.Split(string(c1), " ")[0]
		kbs, err := strconv.Atoi(rss)
		if err != nil {
			return 0
		}

		megs := kbs << 10
		return megs
	}

	contents, err := ioutil.ReadFile("/proc/self/stat")
	if err != nil {
		x.Println("Can't read the proc file", err)
		return 0
	}

	cont := strings.Split(string(contents), " ")
	// 24th entry of the file is the RSS which denotes the number of pages
	// used by the process.
	if len(cont) < 24 {
		x.Println("Error in RSS from stat")
		return 0
	}

	rss, err := strconv.Atoi(cont[23])
	if err != nil {
		x.Println(err)
		return 0
	}

	return rss * os.Getpagesize()
}

func periodicUpdateStats() {
	ticker := time.NewTicker(time.Second)
	setLruMemory := true
	for {
		select {

		case <-ticker.C:

			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			megs := (ms.HeapInuse + ms.StackInuse) / (1 << 20)
			inUse := float64(megs)

			stats := lcache.Stats()
			x.EvictedPls.Set(int64(stats.NumEvicts))
			x.LcacheSize.Set(int64(stats.Size))
			x.LcacheLen.Set(int64(stats.Length))

			// Okay, we exceed the max memory threshold.
			// Stop the world, and deal with this first.
			x.NumGoRoutines.Set(int64(runtime.NumGoroutine()))
			Config.Mu.Lock()
			mem := Config.AllottedMemory
			Config.Mu.Unlock()
			if setLruMemory && inUse > 0.75*mem {
				lcache.UpdateMaxSize()
				setLruMemory = false
			}
		}
	}
}

func updateMemoryMetrics() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		megs := (ms.HeapInuse + ms.StackInuse)

		inUse := float64(megs)
		idle := float64(ms.HeapIdle - ms.HeapReleased)

		x.MemoryInUse.Set(int64(inUse))
		x.HeapIdle.Set(int64(idle))
		x.TotalOSMemory.Set(int64(getMemUsage()))
	}
}

var (
	pstore *badger.ManagedDB
	marks  *x.WaterMark
	lcache *listCache
)

func SyncMarks() *x.WaterMark {
	return marks
}

// Init initializes the posting lists package, the in memory and dirty list hash.
func Init(ps *badger.ManagedDB) {
	marks = &x.WaterMark{Name: "Synced watermark"}
	marks.Init()

	pstore = ps
	lcache = newListCache(math.MaxUint64)
	x.LcacheCapacity.Set(math.MaxInt64)

	go periodicUpdateStats()
	go updateMemoryMetrics()
}

// Get stores the List corresponding to key, if it's not there already.
// to lru cache and returns it.
//
// plist := Get(key, group)
// ... // Use plist
// TODO: This should take a node id and index. And just append all indices to a list.
// When doing a commit, it should update all the sync index watermarks.
// worker pkg would push the indices to the watermarks held by lists.
// And watermark stuff would have to be located outside worker pkg, maybe in x.
// That way, we don't have a dependency conflict.
func Get(key []byte) (rlist *List) {
	lp := lcache.Get(string(key))
	if lp != nil {
		x.CacheHit.Add(1)
		return lp
	}
	x.CacheMiss.Add(1)

	// Any initialization for l must be done before PutIfMissing. Once it's added
	// to the map, any other goroutine can retrieve it.
	l, _ := getNew(key, pstore)
	// We are always going to return lp to caller, whether it is l or not
	lp = lcache.PutIfMissing(string(key), l)
	if lp != l {
		x.CacheRace.Add(1)
	}
	return lp
}

// GetNoStore takes a key. It checks if the in-memory map has an updated value and returns it if it exists
// or it gets from the store and DOES NOT ADD to lru cache.
func GetNoStore(key []byte) (rlist *List) {
	lp := lcache.Get(string(key))
	if lp != nil {
		return lp
	}
	lp, _ = getNew(key, pstore) // This retrieves a new *List and sets refcount to 1.
	return lp
}

// This doesn't sync, so call this only when you don't care about dirty posting lists in // memory(for example before populating snapshot) or after calling syncAllMarks
func EvictLRU() {
	lcache.Reset()
}

func CommitLists(commit func(key []byte) bool) {
	// We iterate over lru and pushing values (List) into this
	// channel. Then goroutines right below will commit these lists to data store.
	workChan := make(chan *List, 10000)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for l := range workChan {
				l.SyncIfDirty(false)
			}
		}()
	}

	lcache.iterate(func(l *List) bool {
		if commit(l.key) {
			workChan <- l
		}
		return true
	})
	close(workChan)
	wg.Wait()
	// TODO(txn): Consider using sync in syncIfDirty instead of async.
	// Hacky solution for now, ensures that everything is flushed to disk before we return.
	txn := pstore.NewTransactionAt(1, true)
	defer txn.Discard()
	txn.Set(x.LockKey("_dummy_", 0), nil, 0)
	txn.CommitAt(1, nil)
}
