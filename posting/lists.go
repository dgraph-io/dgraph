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
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	ostats "go.opencensus.io/stats"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/ristretto"
	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

const (
	mb = 1 << 20
)

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
		glog.Errorf("Can't read the proc file. Err: %v\n", err)
		return 0
	}

	cont := strings.Split(string(contents), " ")
	// 24th entry of the file is the RSS which denotes the number of pages
	// used by the process.
	if len(cont) < 24 {
		glog.Errorln("Error in RSS from stat")
		return 0
	}

	rss, err := strconv.Atoi(cont[23])
	if err != nil {
		glog.Errorln(err)
		return 0
	}

	return rss * os.Getpagesize()
}

func updateMemoryMetrics(lc *y.Closer) {
	defer lc.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	update := func() {
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
			x.MemoryProc.M(int64(getMemUsage())))
	}
	// Call update immediately so that Dgraph reports memory stats without
	// having to wait for the first tick.
	update()

	for {
		select {
		case <-lc.HasBeenClosed():
			return
		case <-ticker.C:
			update()
		}
	}
}

var (
	pstore *badger.DB
	closer *y.Closer
	lCache *ristretto.Cache
)

// Init initializes the posting lists package, the in memory and dirty list hash.
func Init(ps *badger.DB) {
	pstore = ps
	closer = y.NewCloser(1)
	go updateMemoryMetrics(closer)

	// Initialize cache.
	var err error
	lCache, err = ristretto.NewCache(&ristretto.Config{
		NumCounters: 200e6,
		MaxCost:     int64(Config.AllottedMemory * 1024 * 1024),
		BufferItems: 64,
		Metrics:     true,
		Cost: func(val interface{}) int64 {
			l := val.(*List)
			return int64(l.DeepSize())
		},
	})
	x.Check(err)
	go func() {
		m := lCache.Metrics

		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			ostats.Record(context.Background(), x.CacheInUse.M(int64(m.CostAdded()-m.CostEvicted())),
				x.CacheAddedKeys.M(int64(m.KeysAdded())),
				x.CacheEvictedKeys.M(int64(m.KeysEvicted())),
				x.CacheUpdatedKeys.M(int64(m.KeysUpdated())),
				x.CacheHits.M(int64(m.Hits())),
				x.CacheHitRatio.M(m.Ratio()),
				x.CacheMiss.M(int64(m.Misses())),
				x.CacheAddedBytes.M(int64(m.CostAdded())),
				x.CacheEvictedBytes.M(int64(m.CostEvicted())),
				x.CacheDroppedSet.M(int64(m.SetsDropped())),
				x.CacheRejectedSet.M(int64(m.SetsRejected())),
				x.CacheDroppedGets.M(int64(m.GetsDropped())),
				x.CacheKeptGets.M(int64(m.GetsKept())))
		}
	}()
}

// Cleanup waits until the closer has finished processing.
func Cleanup() {
	closer.SignalAndWait()
}

// GetNoStore returns the list stored in the key or creates a new one if it doesn't exist.
// It does not store the list in any cache.
func GetNoStore(key []byte) (rlist *List, err error) {
	return getNew(key, pstore)
}

// LocalCache stores a cache of posting lists and deltas.
// This doesn't sync, so call this only when you don't care about dirty posting lists in
// memory(for example before populating snapshot) or after calling syncAllMarks
type LocalCache struct {
	sync.RWMutex

	startTs uint64

	// The keys for these maps is a string representation of the Badger key for the posting list.
	// deltas keep track of the updates made by txn. These must be kept around until written to disk
	// during commit.
	deltas map[string][]byte

	// max committed timestamp of the read posting lists.
	maxVersions map[string]uint64

	// plists are posting lists in memory. They can be discarded to reclaim space.
	plists map[string]*List
}

// NewLocalCache returns a new LocalCache instance.
func NewLocalCache(startTs uint64) *LocalCache {
	return &LocalCache{
		startTs:     startTs,
		deltas:      make(map[string][]byte),
		plists:      make(map[string]*List),
		maxVersions: make(map[string]uint64),
	}
}

func (lc *LocalCache) getNoStore(key string) *List {
	lc.RLock()
	defer lc.RUnlock()
	if l, ok := lc.plists[key]; ok {
		return l
	}
	return nil
}

// SetIfAbsent adds the list for the specified key to the cache. If a list for the same
// key already exists, the cache will not be modified and the existing list
// will be returned instead. This behavior is meant to prevent the goroutines
// using the cache from ending up with an orphaned version of a list.
func (lc *LocalCache) SetIfAbsent(key string, updated *List) *List {
	lc.Lock()
	defer lc.Unlock()
	if pl, ok := lc.plists[key]; ok {
		return pl
	}
	lc.plists[key] = updated
	return updated
}

func (lc *LocalCache) getInternal(key []byte, readFromDisk bool) (*List, error) {
	if lc == nil {
		return getNew(key, pstore)
	}
	skey := string(key)
	if pl := lc.getNoStore(skey); pl != nil {
		return pl, nil
	}

	var pl *List
	if readFromDisk {
		var err error
		pl, err = getNew(key, pstore)
		if err != nil {
			return nil, err
		}
	} else {
		pl = &List{
			key:   key,
			plist: new(pb.PostingList),
		}
	}

	// If we just brought this posting list into memory and we already have a delta for it, let's
	// apply it before returning the list.
	lc.RLock()
	if delta, ok := lc.deltas[skey]; ok && len(delta) > 0 {
		pl.setMutation(lc.startTs, delta)
	}
	lc.RUnlock()
	return lc.SetIfAbsent(skey, pl), nil
}

// Get retrieves the cached version of the list associated with the given key.
func (lc *LocalCache) Get(key []byte) (*List, error) {
	return lc.getInternal(key, true)
}

// GetFromDelta gets the cached version of the list without reading from disk
// and only applies the existing deltas. This is used in situations where the
// posting list will only be modified and not read (e.g adding index mutations).
func (lc *LocalCache) GetFromDelta(key []byte) (*List, error) {
	return lc.getInternal(key, false)
}

// UpdateDeltasAndDiscardLists updates the delta cache before removing the stored posting lists.
func (lc *LocalCache) UpdateDeltasAndDiscardLists() {
	lc.Lock()
	defer lc.Unlock()
	if len(lc.plists) == 0 {
		return
	}

	for key, pl := range lc.plists {
		data := pl.getMutation(lc.startTs)
		if len(data) > 0 {
			lc.deltas[key] = data
		}
		lc.maxVersions[key] = pl.maxVersion()
		// We can't run pl.release() here because LocalCache is still being used by other callers
		// for the same transaction, who might be holding references to posting lists.
		// TODO: Find another way to reuse postings via postingPool.
	}
	lc.plists = make(map[string]*List)
}

func (lc *LocalCache) fillPreds(ctx *api.TxnContext, gid uint32) {
	lc.RLock()
	defer lc.RUnlock()
	for key := range lc.deltas {
		pk, err := x.Parse([]byte(key))
		x.Check(err)
		if len(pk.Attr) == 0 {
			continue
		}
		// Also send the group id that the predicate was being served by. This is useful when
		// checking if Zero should allow a commit during a predicate move.
		predKey := fmt.Sprintf("%d-%s", gid, pk.Attr)
		ctx.Preds = append(ctx.Preds, predKey)
	}
	ctx.Preds = x.Unique(ctx.Preds)
}
