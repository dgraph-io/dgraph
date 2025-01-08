/*
 * Copyright 2017-2025 Hypermode Inc. and Contributors
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
	"bytes"
	"context"
	"encoding/hex"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	ostats "go.opencensus.io/stats"
	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/badger/v4"
	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/hypermodeinc/dgraph/v24/protos/pb"
	"github.com/hypermodeinc/dgraph/v24/x"
)

type pooledKeys struct {
	// keysCh is populated with batch of 64 keys that needs to be rolled up during reads
	keysCh chan *[][]byte
	// keysPool is sync.Pool to share the batched keys to rollup.
	keysPool *sync.Pool
}

// incrRollupi is used to batch keys for rollup incrementally.
type incrRollupi struct {
	// We are using 2 priorities with now, idx 0 represents the high priority keys to be rolled
	// up while idx 1 represents low priority keys to be rolled up.
	priorityKeys []*pooledKeys
	count        uint64

	// Get Timestamp function gets a new timestamp to store the rollup at. This makes sure that
	// we are not overwriting any transaction. If there are transactions that are ongoing,
	// which modify the item, rollup wouldn't affect the data, as a delta would be written
	// later on
	getNewTs func(bool) uint64
	closer   *z.Closer
}

type CachePL struct {
	count      int
	list       *List
	lastUpdate uint64
	lastRead   time.Time
}

var (
	// ErrTsTooOld is returned when a transaction is too old to be applied.
	ErrTsTooOld = errors.Errorf("Transaction is too old")
	// ErrInvalidKey is returned when trying to read a posting list using
	// an invalid key (e.g the key to a single part of a larger multi-part list).
	ErrInvalidKey = errors.Errorf("cannot read posting list using multi-part list key")
	// ErrHighPriorityOp is returned when rollup is cancelled so that operations could start.
	ErrHighPriorityOp = errors.New("Cancelled rollup to make way for high priority operation")

	// IncrRollup is used to batch keys for rollup incrementally.
	IncrRollup = &incrRollupi{
		priorityKeys: make([]*pooledKeys, 2),
	}
)

var memoryLayer *MemoryLayer

func init() {
	x.AssertTrue(len(IncrRollup.priorityKeys) == 2)
	for i := range IncrRollup.priorityKeys {
		IncrRollup.priorityKeys[i] = &pooledKeys{
			keysCh: make(chan *[][]byte, 16),
			keysPool: &sync.Pool{
				New: func() interface{} {
					return new([][]byte)
				},
			},
		}
	}
}

// rollUpKey takes the given key's posting lists, rolls it up and writes back to badger
func (ir *incrRollupi) rollUpKey(writer *TxnWriter, key []byte) error {
	// Get a new non read only ts. This makes sure that no other txn would write at this
	// ts, overwriting some data. Wait to read the Posting list until ts-1 have been applied
	// to badger. This helps us prevent issues with wal replay, as we now have a timestamp
	// where nothing was writen to dgraph.
	ts := ir.getNewTs(false)

	// Get a wait channel from oracle. Can't use WaitFromTs as we also need to check if other
	// operations need to start. If ok is not true, that means we have already passed the ts,
	// and we don't need to wait.
	waitCh, ok := o.addToWaiters(ts)
	if ok {
		select {
		case <-ir.closer.HasBeenClosed():
			return ErrHighPriorityOp

		case <-waitCh:
		}
	}

	l, err := GetNoStore(key, ts)
	if err != nil {
		return err
	}

	kvs, err := l.Rollup(nil, ts)
	if err != nil {
		return err
	}

	RemoveCacheFor(key)
	memoryLayer.del(key)
	// TODO Update cache with rolled up results
	// If we do a rollup, we typically won't need to update the key in cache.
	// The only caveat is that the key written by rollup would be written at +1
	// timestamp, hence bumping the latest TS for the key by 1. The cache should
	// understand that.
	const N = uint64(1000)
	if glog.V(2) {
		if count := atomic.AddUint64(&ir.count, 1); count%N == 0 {
			glog.V(2).Infof("Rolled up %d keys", count)
		}
	}
	return writer.Write(&bpb.KVList{Kv: kvs})
}

// TODO: When the opRollup is not running the keys from keysPool of ir are dropped. Figure out some
// way to handle that.
func (ir *incrRollupi) addKeyToBatch(key []byte, priority int) {
	rki := ir.priorityKeys[priority]
	batch := rki.keysPool.Get().(*[][]byte)
	*batch = append(*batch, key)
	if len(*batch) < 16 {
		rki.keysPool.Put(batch)
		return
	}

	select {
	case rki.keysCh <- batch:
	default:
		// Drop keys and build the batch again. Lossy behavior.
		*batch = (*batch)[:0]
		rki.keysPool.Put(batch)
	}
}

// Process will rollup batches of 64 keys in a go routine.
func (ir *incrRollupi) Process(closer *z.Closer, getNewTs func(bool) uint64) {
	ir.getNewTs = getNewTs
	ir.closer = closer

	defer closer.Done()

	writer := NewTxnWriter(pstore)
	defer writer.Flush()

	m := make(map[uint64]int64) // map hash(key) to ts. hash(key) to limit the size of the map.
	limiter := time.NewTicker(time.Millisecond)
	defer limiter.Stop()
	cleanupTick := time.NewTicker(5 * time.Minute)
	defer cleanupTick.Stop()
	forceRollupTick := time.NewTicker(500 * time.Millisecond)
	defer forceRollupTick.Stop()

	doRollup := func(batch *[][]byte, priority int) {
		currTs := time.Now().Unix()
		for _, key := range *batch {
			hash := z.MemHash(key)
			if elem := m[hash]; currTs-elem >= 10 {
				// Key not present or Key present but last roll up was more than 2 sec ago.
				// Add/Update map and rollup.
				m[hash] = currTs
				if err := ir.rollUpKey(writer, key); err != nil {
					glog.Warningf("Error %v rolling up key %v\n", err, key)
				}
			}
		}
		*batch = (*batch)[:0]
		ir.priorityKeys[priority].keysPool.Put(batch)
	}

	for {
		select {
		case <-closer.HasBeenClosed():
			return
		case <-cleanupTick.C:
			currTs := time.Now().UnixNano()
			for hash, ts := range m {
				// Remove entries from map which have been there for there more than 10 seconds.
				if currTs-ts >= int64(10*time.Second) {
					delete(m, hash)
				}
			}
		case <-forceRollupTick.C:
			batch := ir.priorityKeys[0].keysPool.Get().(*[][]byte)
			if len(*batch) > 0 {
				doRollup(batch, 0)
			} else {
				ir.priorityKeys[0].keysPool.Put(batch)
			}
		case batch := <-ir.priorityKeys[0].keysCh:
			doRollup(batch, 0)
			// We don't need a limiter here as we don't expect to call this function frequently.
		case batch := <-ir.priorityKeys[1].keysCh:
			doRollup(batch, 1)
			// throttle to 1 batch = 16 rollups per 1 ms.
			<-limiter.C
		}
	}
}

// ShouldAbort returns whether the transaction should be aborted.
func (txn *Txn) ShouldAbort() bool {
	if txn == nil {
		return false
	}
	return atomic.LoadUint32(&txn.shouldAbort) > 0
}

func (txn *Txn) addConflictKey(conflictKey uint64) {
	txn.Lock()
	defer txn.Unlock()
	if txn.conflicts == nil {
		txn.conflicts = make(map[uint64]struct{})
	}
	if conflictKey > 0 {
		txn.conflicts[conflictKey] = struct{}{}
	}
}

// FillContext updates the given transaction context with data from this transaction.
func (txn *Txn) FillContext(ctx *api.TxnContext, gid uint32, isErrored bool) {
	txn.Lock()
	ctx.StartTs = txn.StartTs

	for key := range txn.conflicts {
		// We don'txn need to send the whole conflict key to Zero. Solving #2338
		// should be done by sending a list of mutating predicates to Zero,
		// along with the keys to be used for conflict detection.
		fps := strconv.FormatUint(key, 36)
		ctx.Keys = append(ctx.Keys, fps)
	}
	ctx.Keys = x.Unique(ctx.Keys)

	txn.Unlock()
	// If the trasnaction has errored out, we don't need to update it, as these values will never be read.
	// Sometimes, the transaction might have failed due to timeout. If we let this trasnactino update, there
	// could be deadlock with the running transaction.
	if !isErrored {
		txn.Update()
	}
	txn.cache.fillPreds(ctx, gid)
}

// CommitToDisk commits a transaction to disk.
// This function only stores deltas to the commit timestamps. It does not try to generate a state.
// State generation is done via rollups, which happen when a snapshot is created.
// Don't call this for schema mutations. Directly commit them.
func (txn *Txn) CommitToDisk(writer *TxnWriter, commitTs uint64) error {
	if commitTs == 0 {
		return nil
	}

	cache := txn.cache
	cache.Lock()
	defer cache.Unlock()

	var keys []string
	for key := range cache.deltas {
		keys = append(keys, key)
	}

	defer func() {
		// Add these keys to be rolled up after we're done writing. This is the right place for them
		// to be rolled up, because we just pushed these deltas over to Badger.
		for _, key := range keys {
			IncrRollup.addKeyToBatch([]byte(key), 1)
		}
	}()

	var idx int
	for idx < len(keys) {
		// writer.update can return early from the loop in case we encounter badger.ErrTxnTooBig. On
		// that error, writer.update would still commit the transaction and return any error. If
		// nil, we continue to process the remaining keys.
		err := writer.update(commitTs, func(btxn *badger.Txn) error {
			for ; idx < len(keys); idx++ {
				key := keys[idx]
				data := cache.deltas[key]
				if len(data) == 0 {
					continue
				}
				if ts := cache.maxVersions[key]; ts >= commitTs {
					// Skip write because we already have a write at a higher ts.
					// Logging here can cause a lot of output when doing Raft log replay. So, let's
					// not output anything here.
					continue
				}
				err := btxn.SetEntry(&badger.Entry{
					Key:      []byte(key),
					Value:    data,
					UserMeta: BitDeltaPosting,
				})
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func ResetCache() {
	memoryLayer.clear()
}

// RemoveCacheFor will delete the list corresponding to the given key.
func RemoveCacheFor(key []byte) {
	memoryLayer.del(key)
}

type Cache struct {
	data *ristretto.Cache[[]byte, *CachePL]

	numCacheRead      int
	numCacheReadFails int
	numCacheSave      int
}

func (c *Cache) wait() {
	if c == nil {
		return
	}
	c.data.Wait()
}

func (c *Cache) get(key []byte) (*CachePL, bool) {
	if c == nil {
		return nil, false
	}
	val, ok := c.data.Get(key)
	if !ok {
		c.numCacheReadFails += 1
		return val, ok
	}
	if val.list == nil {
		c.numCacheReadFails += 1
		return nil, false
	}
	c.numCacheRead += 1
	return val, true
}

func (c *Cache) set(key []byte, i *CachePL) {
	if c == nil {
		return
	}
	c.numCacheSave += 1
	c.data.Set(key, i, 1)
}

func (c *Cache) del(key []byte) {
	if c == nil {
		return
	}
	c.data.Del(key)
}

func (c *Cache) clear() {
	if c == nil {
		return
	}
	c.data.Clear()
}

type MemoryLayer struct {
	keepUpdates bool
	cache       *Cache

	numDisksRead int
}

func (ml *MemoryLayer) clear() {
	ml.cache.clear()
}
func (ml *MemoryLayer) del(key []byte) {
	ml.cache.del(key)
}

func initMemoryLayer(cacheSize int64, keepUpdates bool) *MemoryLayer {
	ml := &MemoryLayer{}
	ml.keepUpdates = keepUpdates
	if cacheSize > 0 {
		cache, err := ristretto.NewCache[[]byte, *CachePL](&ristretto.Config[[]byte, *CachePL]{
			// Use 5% of cache memory for storing counters.
			NumCounters: int64(float64(cacheSize) * 0.05 * 2),
			MaxCost:     int64(float64(cacheSize) * 0.95),
			BufferItems: 16,
			Metrics:     true,
			Cost: func(val *CachePL) int64 {
				return 1
			},
			ShouldUpdate: func(cur, prev *CachePL) bool {
				return !(cur.list != nil && prev.list != nil && prev.list.maxTs > cur.list.maxTs)
			},
		})
		x.Check(err)
		go func() {
			m := cache.Metrics
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				// Record the posting list cache hit ratio
				ostats.Record(context.Background(), x.PLCacheHitRatio.M(m.Ratio()))
			}
		}()

		ml.cache = &Cache{data: cache}
	}
	return ml
}

func NewCachePL() *CachePL {
	return &CachePL{
		count:      0,
		list:       nil,
		lastUpdate: 0,
	}
}

func checkForRollup(key []byte, l *List) {
	deltaCount := l.mutationMap.len()
	// If deltaCount is high, send it to high priority channel instead.
	if deltaCount > 500 {
		IncrRollup.addKeyToBatch(key, 0)
	}
}

func (ml *MemoryLayer) wait() {
	ml.cache.wait()
}

func (ml *MemoryLayer) updateItemInCache(key string, delta []byte, startTs, commitTs uint64) {
	if commitTs == 0 {
		return
	}

	if !ml.keepUpdates {
		// TODO We should mark the key as deleted instead of directly deleting from the cache.
		ml.del([]byte(key))
		return
	}

	val, ok := ml.cache.get([]byte(key))
	if !ok {
		return
	}

	val.lastUpdate = commitTs
	val.count -= 1

	if val.list != nil && ml.keepUpdates {
		p := new(pb.PostingList)
		x.Check(proto.Unmarshal(delta, p))

		val.list.setMutationAfterCommit(startTs, commitTs, p, true)
		checkForRollup([]byte(key), val.list)
	}
}

// RemoveCachedKeys will delete the cached list by this txn.
func (txn *Txn) UpdateCachedKeys(commitTs uint64) {
	if txn == nil || txn.cache == nil {
		return
	}

	memoryLayer.wait()
	for key, delta := range txn.cache.deltas {
		memoryLayer.updateItemInCache(key, delta, txn.StartTs, commitTs)
	}
}

func unmarshalOrCopy(plist *pb.PostingList, item *badger.Item) error {
	if plist == nil {
		return errors.Errorf("cannot unmarshal value to a nil posting list of key %s",
			hex.Dump(item.Key()))
	}

	return item.Value(func(val []byte) error {
		if len(val) == 0 {
			// empty pl
			return nil
		}
		return proto.Unmarshal(val, plist)
	})
}

// ReadPostingList constructs the posting list from the disk using the passed iterator.
// Use forward iterator with allversions enabled in iter options.
// key would now be owned by the posting list. So, ensure that it isn't reused elsewhere.
func ReadPostingList(key []byte, it *badger.Iterator) (*List, error) {
	// Previously, ReadPostingList was not checking that a multi-part list could only
	// be read via the main key. This lead to issues during rollup because multi-part
	// lists ended up being rolled-up multiple times. This issue was caught by the
	// uid-set Jepsen test.
	pk, err := x.Parse(key)
	if err != nil {
		return nil, errors.Wrapf(err, "while reading posting list with key [%v]", key)
	}
	if pk.HasStartUid {
		// Trying to read a single part of a multi part list. This type of list
		// should be read using using the main key because the information needed
		// to access the whole list is stored there.
		// The function returns a nil list instead. This is safe to do because all
		// public methods of the List object are no-ops and the list is being already
		// accessed via the main key in the places where this code is reached (e.g rollups).
		return nil, ErrInvalidKey
	}

	l := new(List)
	l.key = key
	l.plist = new(pb.PostingList)
	l.minTs = 0

	// We use the following block of code to trigger incremental rollup on this key.
	deltaCount := 0
	defer func() {
		if deltaCount > 0 {
			// If deltaCount is high, send it to high priority channel instead.
			if deltaCount > 500 {
				IncrRollup.addKeyToBatch(key, 0)
			} else {
				IncrRollup.addKeyToBatch(key, 1)
			}
		}
	}()

	// Iterates from highest Ts to lowest Ts
	for it.Valid() {
		item := it.Item()
		if !bytes.Equal(item.Key(), l.key) {
			break
		}
		l.maxTs = x.Max(l.maxTs, item.Version())
		if item.IsDeletedOrExpired() {
			// Don't consider any more versions.
			break
		}

		switch item.UserMeta() {
		case BitEmptyPosting:
			return l, nil
		case BitCompletePosting:
			if err := unmarshalOrCopy(l.plist, item); err != nil {
				return nil, err
			}

			l.minTs = item.Version()
			// No need to do Next here. The outer loop can take care of skipping
			// more versions of the same key.
			return l, nil
		case BitDeltaPosting:
			err := item.Value(func(val []byte) error {
				pl := &pb.PostingList{}
				if err := proto.Unmarshal(val, pl); err != nil {
					return err
				}
				pl.CommitTs = item.Version()
				if l.mutationMap == nil {
					l.mutationMap = newMutableLayer()
				}
				l.mutationMap.insertCommittedPostings(pl)
				return nil
			})
			if err != nil {
				return nil, err
			}
			deltaCount++
		case BitSchemaPosting:
			return nil, errors.Errorf(
				"Trying to read schema in ReadPostingList for key: %s", hex.Dump(key))
		default:
			return nil, errors.Errorf(
				"Unexpected meta: %d for key: %s", item.UserMeta(), hex.Dump(key))
		}
		if item.DiscardEarlierVersions() {
			break
		}
		it.Next()
	}
	return l, nil
}

func copyList(l *List) *List {
	l.AssertRLock()
	// No need to clone the immutable layer or the key since mutations will not modify it.
	lCopy := &List{
		minTs: l.minTs,
		maxTs: l.maxTs,
		key:   l.key,
		plist: l.plist,
	}
	lCopy.mutationMap = l.mutationMap.clone()
	return lCopy
}

func (c *CachePL) Set(l *List, readTs uint64) {
	if c.lastUpdate < readTs && (c.list == nil || c.list.maxTs < l.maxTs) {
		c.list = l
	}
}

func (ml *MemoryLayer) readFromCache(key []byte, readTs uint64) *List {
	cacheItem, ok := ml.cache.get(key)

	if ok {
		cacheItem.count += 1
		cacheItem.lastRead = time.Now()
		if cacheItem.list != nil && cacheItem.list.minTs <= readTs {
			cacheItem.list.RLock()
			lCopy := copyList(cacheItem.list)
			cacheItem.list.RUnlock()
			checkForRollup(key, lCopy)
			return lCopy
		}
	}
	return nil
}

func (ml *MemoryLayer) readFromDisk(key []byte, pstore *badger.DB, readTs uint64) (*List, error) {
	ml.numDisksRead += 1
	txn := pstore.NewTransactionAt(readTs, false)
	defer txn.Discard()

	// When we do rollups, an older version would go to the top of the LSM tree, which can cause
	// issues during txn.Get. Therefore, always iterate.
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	iterOpts.PrefetchValues = false
	itr := txn.NewKeyIterator(key, iterOpts)
	defer itr.Close()
	itr.Seek(key)
	l, err := ReadPostingList(key, itr)
	if err != nil {
		return l, err
	}
	return l, nil
}

// Saves the data in the cache. The caller must ensure that the list provided is the latest possible.
func (ml *MemoryLayer) saveInCache(key []byte, l *List) {
	l.RLock()
	defer l.RUnlock()
	cacheItem := NewCachePL()
	cacheItem.count = 1
	cacheItem.list = copyList(l)
	cacheItem.lastUpdate = l.maxTs
	ml.cache.set(key, cacheItem)
}

func (ml *MemoryLayer) ReadData(key []byte, pstore *badger.DB, readTs uint64) (*List, error) {
	// We first try to read the data from cache, if it is present. If it's not present, then we would read the
	// latest data from the disk. This would get stored in the cache. If this read has a minTs > readTs then
	// we would have to read the correct timestamp from the disk.
	l := ml.readFromCache(key, readTs)
	if l != nil {
		l.mutationMap.setTs(readTs)
		return l, nil
	}
	l, err := ml.readFromDisk(key, pstore, math.MaxUint64)
	if err != nil {
		return nil, err
	}
	ml.saveInCache(key, l)
	if l.minTs == 0 || readTs >= l.minTs {
		l.mutationMap.setTs(readTs)
		return l, nil
	}

	l, err = ml.readFromDisk(key, pstore, readTs)
	if err != nil {
		return nil, err
	}

	l.mutationMap.setTs(readTs)
	return l, nil
}

func GetNew(key []byte, pstore *badger.DB, readTs uint64) (*List, error) {
	return getNew(key, pstore, readTs)
}

func getNew(key []byte, pstore *badger.DB, readTs uint64) (*List, error) {
	if pstore.IsClosed() {
		return nil, badger.ErrDBClosed
	}

	l, err := memoryLayer.ReadData(key, pstore, readTs)
	if err != nil {
		return l, err
	}
	return l, nil
}
