/*
 * Copyright 2015-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
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
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/trace"

	ostats "go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/y"
	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/protos/pb"
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
		pl := pb.PostingList{}
		var err error
		emptyPostingList, err = pl.Marshal()
		x.Check(err)
	})
	elog = trace.NewEventLog("Memory", "")
}

func getMemUsage(ctx context.Context) int {
        startTime := time.Now()
	ctx, _ = tag.New(ctx, tag.Upsert(x.KeyMethod, "getMemUsage"))
	defer func() {
                timeSpentMs := float64(time.Since(startTime))/1e6
		ostats.Record(ctx, x.LatencyMs.M(timeSpentMs))
	}()

	if runtime.GOOS != "linux" {
		pid := os.Getpid()
		cmd := fmt.Sprintf("ps -ao rss,pid | grep %v", pid)
		c1, err := exec.Command("bash", "-c", cmd).Output()
		if err != nil {
			// In case of error running the command, resort to go way
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			megs := ms.Alloc

			// Otherwise we succeeded in getting the Memory usage from the runtime.
			ctx, _ = tag.New(ctx, tag.Insert(x.KeyStatus, x.TagValueStatusOK))

			return int(megs)
		}

		rss := strings.Split(string(c1), " ")[0]
		kbs, err := strconv.Atoi(rss)
		if err != nil {
			ctx, _ = tag.New(ctx,
				tag.Insert(x.KeyError, err.Error()),
				tag.Insert(x.KeyStatus, x.TagValueStatusError))
			return 0
		}

		megs := kbs << 10

		// Otherwise we succeeded in getting the Memory usage
		ctx, _ = tag.New(ctx, tag.Insert(x.KeyStatus, x.TagValueStatusOK))

		return megs
	}

	contents, err := ioutil.ReadFile("/proc/self/stat")
	if err != nil {
		errMsg := fmt.Sprintf("Can't read the proc file. Err: %v\n", err)
		ctx, _ = tag.New(ctx,
			tag.Insert(x.KeyError, errMsg),
			tag.Insert(x.KeyStatus, x.TagValueStatusError))
		glog.Errorf(errMsg)
		return 0
	}

	cont := strings.Split(string(contents), " ")
	// 24th entry of the file is the RSS which denotes the number of pages
	// used by the process.
	if len(cont) < 24 {
		errMsg := "Error in RSS from stat"
		ctx, _ = tag.New(ctx,
			tag.Insert(x.KeyError, errMsg),
			tag.Insert(x.KeyStatus, x.TagValueStatusError))
		glog.Errorln(errMsg)
		return 0
	}

	rss, err := strconv.Atoi(cont[23])
	if err != nil {
		ctx, _ = tag.New(ctx,
			tag.Insert(x.KeyError, err.Error()),
			tag.Insert(x.KeyStatus, x.TagValueStatusError))
		glog.Errorln(err)
		return 0
	}

	// Otherwise we succeeded in getting the Memory usage
	ctx, _ = tag.New(ctx, tag.Insert(x.KeyStatus, x.TagValueStatusOK))

	return rss * os.Getpagesize()
}

func periodicUpdateStats(ctx context.Context, lc *y.Closer) {
	defer lc.Done()

	period := 10 * time.Second
	ctx, _ = tag.New(ctx,
		tag.Insert(x.KeyMethod, "periodicUpdateStats"),
		tag.Insert(x.KeyPeriod, period.String()))

	ticker := time.NewTicker(period)
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

			ostats.Record(ctx,
				x.LcacheEvicts.M(int64(stats.NumEvicts)),
				x.LcacheSize.M(int64(stats.Size)),
				x.LcacheLen.M(int64(stats.Length)),
				x.NumGoRoutines.M(int64(runtime.NumGoroutine())))

			// Okay, we exceed the max memory threshold.
			// Stop the world, and deal with this first.
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

func updateMemoryMetrics(ctx context.Context, lc *y.Closer) {
	defer lc.Done()
	period := time.Minute
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	ctx, _ = tag.New(ctx,
		tag.Insert(x.KeyMethod, "updateMemoryMetrics"),
		tag.Insert(x.KeyPeriod, period.String()))

	for {
		select {
		case <-lc.HasBeenClosed():
			return
		case <-ticker.C:
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)

			inUse := ms.HeapInuse + ms.StackInuse
			// From runtime/mstats.go:
			// HeapIdle minus HeapReleased estimates the amount of memory
			// that could be returned to the OS, but is being retained by
			// the runtime so it can grow the heap without requesting more
			// memory from the OS. If this difference is significantly
			// larger than the heap size, it indicates there was a recent
			// transient spike in live heap size.
			idle := ms.HeapIdle - ms.HeapReleased

			ostats.Record(context.Background(),
				x.MemoryInUse.M(int64(inUse)),
				x.MemoryIdle.M(int64(idle)),
				x.MemoryProc.M(int64(getMemUsage(ctx))))
		}
	}
}

var (
	pstore *badger.DB
	lcache *listCache
	closer *y.Closer
)

// Init initializes the posting lists package, the in memory and dirty list hash.
func Init(ps *badger.DB) {
	pstore = ps
	lcache = newListCache(math.MaxUint64)

	closer = y.NewCloser(2)

	// At the beginning add some distinguishing information
	// to the context as tags that will be propagated when
	// collecting metrics.
	ctx := x.ObservabilityEnabledParentContext()

	go periodicUpdateStats(ctx, closer)
	go updateMemoryMetrics(ctx, closer)
}

func Cleanup() {
	closer.SignalAndWait()
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
	ctx, _ := tag.New(context.Background(), tag.Upsert(x.KeyMethod, "lcache.Get"),
		// For majority of the cases, the status is OK,
		// if an error occurs this will be changed anyways.
		tag.Upsert(x.KeyStatus, x.TagValueStatusOK))

	lp := lcache.Get(string(key))
	if lp != nil {
		ostats.Record(ctx, x.LcacheHit.M(1))
		return lp, nil
	}

	// From this point on we encountered a cache miss.

	// Any initialization for l must be done before PutIfMissing. Once it's added
	// to the map, any other goroutine can retrieve it.
	l, err := getNew(key, pstore)
	if err != nil {
		ctx, _ = tag.New(ctx, tag.Upsert(x.KeyStatus, x.TagValueStatusError), tag.Upsert(x.KeyError, err.Error()))
		ostats.Record(ctx, x.LcacheMiss.M(1))
		return nil, err
	}
	// We are always going to return lp to caller, whether it is l or not
	lp = lcache.PutIfMissing(string(key), l)
	if lp != l {
		ostats.Record(ctx, x.LcacheRace.M(1), x.LcacheMiss.M(1))
	} else { // Otherwise we didn't race, so record the previously encountered cache niss.
		ostats.Record(ctx, x.LcacheMiss.M(1))
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

// This doesn't sync, so call this only when you don't care about dirty posting lists in
// memory(for example before populating snapshot) or after calling syncAllMarks
func EvictLRU() {
	lcache.Reset()
}
