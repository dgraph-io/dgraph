/*
 * Copyright 2015-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
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
	"sync/atomic"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/y"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptyPostingList []byte // Used for indexing.
	elog             trace.EventLog
)

const (
	MB = 1 << 20
)

// syncMarks stores the watermark for synced RAFT proposals. Each RAFT proposal consists
// of many individual mutations, which could be applied to many different posting lists.
// Thus, each PL when being mutated would send an undone Mark, and each list would
// accumulate all such pending marks. When the PL is synced to BadgerDB, it would
// mark all the pending ones as done.
// This ideally belongs to RAFT node struct (where committed watermark is being tracked),
// but because the logic of mutations is
// present here and to avoid a circular dependency, we've placed it here.
// Note that there's one watermark for each RAFT node/group.
// This watermark would be used for taking snapshots, to ensure that all the data and
// index mutations have been syned to BadgerDB, before a snapshot is taken, and previous
// RAFT entries discarded.
func init() {
	x.AddInit(func() {
		h := md5.New()
		pl := intern.PostingList{
			Checksum: h.Sum(nil),
		}
		var err error
		emptyPostingList, err = pl.Marshal()
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

func periodicUpdateStats(lc *y.Closer) {
	defer lc.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	setLruMemory := true
	var maxSize uint64
	var lastUse float64
	for {
		select {
		case <-lc.HasBeenClosed():
			return
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
			if setLruMemory {
				if inUse > 0.75*mem {
					maxSize = lcache.UpdateMaxSize(0)
					setLruMemory = false
					lastUse = inUse
				}
				break
			}

			// If memory has not changed by 100MB.
			if math.Abs(inUse-lastUse) < 100 {
				break
			}

			delta := maxSize / 10
			if delta > 50<<20 {
				delta = 50 << 20 // Change lru cache size by max 50mb.
			}
			if inUse > 0.85*mem { // Decrease max Size by 10%
				maxSize -= delta
				maxSize = lcache.UpdateMaxSize(maxSize)
				lastUse = inUse
			} else if inUse < 0.65*mem { // Increase max Size by 10%
				maxSize += delta
				maxSize = lcache.UpdateMaxSize(maxSize)
				lastUse = inUse
			}
		}
	}
}

func updateMemoryMetrics(lc *y.Closer) {
	defer lc.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-lc.HasBeenClosed():
			return
		case <-ticker.C:
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
}

var (
	pstore *badger.ManagedDB
	lcache *listCache
	btree  *BTree
	closer *y.Closer
)

// Init initializes the posting lists package, the in memory and dirty list hash.
func Init(ps *badger.ManagedDB) {
	pstore = ps
	lcache = newListCache(math.MaxUint64)
	btree = newBTree(2)
	x.LcacheCapacity.Set(math.MaxInt64)

	closer = y.NewCloser(2)

	go periodicUpdateStats(closer)
	go updateMemoryMetrics(closer)
}

func Cleanup() {
	closer.SignalAndWait()
}

func StopLRUEviction() {
	atomic.StoreInt32(&lcache.done, 1)
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
func Get(key []byte) (rlist *List, err error) {
	lp := lcache.Get(string(key))
	if lp != nil {
		x.CacheHit.Add(1)
		return lp, nil
	}
	x.CacheMiss.Add(1)

	// Any initialization for l must be done before PutIfMissing. Once it's added
	// to the map, any other goroutine can retrieve it.
	l, err := getNew(key, pstore)
	if err != nil {
		return nil, err
	}
	// We are always going to return lp to caller, whether it is l or not
	lp = lcache.PutIfMissing(string(key), l)
	if lp != l {
		x.CacheRace.Add(1)
	} else if atomic.LoadInt32(&l.onDisk) == 0 {
		btree.Insert(l.key)
	}
	return lp, nil
}

// GetLru checks the lru map and returns it if it exits
func GetLru(key []byte) *List {
	return lcache.Get(string(key))
}

// GetNoStore takes a key. It checks if the in-memory map has an updated value and returns it if it exists
// or it gets from the store and DOES NOT ADD to lru cache.
func GetNoStore(key []byte) (*List, error) {
	lp := lcache.Get(string(key))
	if lp != nil {
		return lp, nil
	}
	return getNew(key, pstore) // This retrieves a new *List and sets refcount to 1.
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

	// The following hack ensures that all the asynchrously run commits above would have been done
	// before this completes. Badger now actually gets rid of keys, which are deleted. So, we can
	// use the Delete function.
	txn := pstore.NewTransactionAt(1, true)
	defer txn.Discard()
	x.Check(txn.Delete(x.DataKey("_dummy_", 1)))
	// Nothing is being read, so there can't be an ErrConflict. This should go to disk.
	if err := txn.CommitAt(1, nil); err != nil {
		x.Printf("Commit unexpectedly failed with error: %v", err)
	}
}
