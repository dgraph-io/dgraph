/*
* Copyright 2017-2023 Dgraph Labs, Inc. and Contributors
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
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	"github.com/dgraph-io/badger/v4"
	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
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

type GlobalCache struct {
	sync.RWMutex

	count      map[string]int
	list       map[string]*List
	lastUpdate map[string]uint64
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

	globalCache = &GlobalCache{count: make(map[string]int), list: make(map[string]*List), lastUpdate: make(map[string]uint64)}
)

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

	//if len(kvs) > 0 {
	//	pl := new(pb.PostingList)
	//	x.Check(pl.Unmarshal(kvs[0].Value))
	//	l.Lock()
	//	l.plist = pl
	//	l.mutationMap = nil
	//	l.maxTs = kvs[0].Version
	//	l.Unlock()
	//}

	//l.RLock()
	//lCache.Set(key, l, int64(l.DeepSize()))
	//l.RUnlock()

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
			if elem := m[hash]; currTs-elem >= 2 {
				// Key not present or Key present but last roll up was more than 10 sec ago.
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
func (txn *Txn) FillContext(ctx *api.TxnContext, gid uint32) {
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
	txn.Update()
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
	globalCache.Lock()
	globalCache.count = make(map[string]int)
	globalCache.list = make(map[string]*List)
	globalCache.lastUpdate = make(map[string]uint64)
	globalCache.Unlock()
	lCache.Clear()
}

// RemoveCachedKeys will delete the cached list by this txn.
func (txn *Txn) UpdateCachedKeys(commitTs uint64) {
	if txn == nil || txn.cache == nil {
		return
	}

	for key, delta := range txn.cache.deltas {
		pk, _ := x.Parse([]byte(key))
		p := new(pb.PostingList)
		x.Check(p.Unmarshal(delta))
		fmt.Println("[TXN] UPDATING0", pk, p, commitTs, "start_ts:", txn.cache.startTs)
		globalCache.Lock()
		globalCache.lastUpdate[key] = commitTs
		val, ok := globalCache.count[key]
		if !ok {
			globalCache.Unlock()
			fmt.Println("[TXN] UPDATING1", pk, p, commitTs, "start_ts:", txn.cache.startTs)
			continue
		}
		globalCache.count[key] = val - 1
		if val == 1 {
			delete(globalCache.count, key)
			delete(globalCache.list, key)
			globalCache.Unlock()
			fmt.Println("[TXN] UPDATING2", pk, p, commitTs, "start_ts:", txn.cache.startTs)
			continue
		}

		if commitTs != 0 {
			pl, ok := globalCache.list[key]
			if ok {
				pl.setMutationAfterCommit(txn.StartTs, commitTs, delta)
				fmt.Println("[TXN] UPDATING3", pk, p, commitTs, pl.mutationMap)
			}
		}
		globalCache.Unlock()
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
		return plist.Unmarshal(val)
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
			l.minTs = item.Version()
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
				if err := pl.Unmarshal(val); err != nil {
					return err
				}
				pl.CommitTs = item.Version()
				for _, mpost := range pl.Postings {
					// commitTs, startTs are meant to be only in memory, not
					// stored on disk.
					mpost.CommitTs = item.Version()
				}
				if l.mutationMap == nil {
					l.mutationMap = make(map[uint64]*pb.PostingList)
				}
				l.mutationMap[pl.CommitTs] = pl
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
	lCopy.mutationMap = make(map[uint64]*pb.PostingList, len(l.mutationMap))
	for k, v := range l.mutationMap {
		lCopy.mutationMap[k] = proto.Clone(v).(*pb.PostingList)
	}
	return lCopy
}

func getNew(key []byte, pstore *badger.DB, readTs uint64) (*List, error) {
	if pstore.IsClosed() {
		return nil, badger.ErrDBClosed
	}

	pk, _ := x.Parse(key)
	globalCache.Lock()
	globalCache.count[string(key)]++

	// We use badger subscription to invalidate the cache. For every write we make the value
	// corresponding to the key in the cache to nil. So, if we get some non-nil value from the cache
	// then it means that no  writes have happened after the last set of this key in the cache.
	if l, ok := globalCache.list[string(key)]; ok {
		if l != nil && l.minTs <= readTs {
			l.RLock()
			lCopy := copyList(l)
			fmt.Println("[TXN] getting data", pk, l.mutationMap, l.plist, readTs)
			l.RUnlock()
			//var o ListOptions
			//o.ReadTs = readTs
			//uids, _ := lCopy.Uids(o)
			//vals, _ := lCopy.AllValues(readTs)
			//fmt.Println("GETTING", pk, uids, vals, lCopy.mutationMap, l.mutationMap)
			globalCache.Unlock()
			return lCopy, nil
		}
	}
	//if val, ok := lCache.Get(key); ok {
	//	switch val.(type) {
	//	case *List:
	//		l := val.(*List)
	//		// l.maxTs can be greater than readTs. We might have the latest
	//		// version cached, while readTs is looking for an older version.
	//		if l != nil {
	//			//fmt.Println(l.maxTs, readTs)
	//		}
	//		if l != nil && l.maxTs > 0 {
	//			l.RLock()
	//			lCopy := copyList(l)
	//			l.RUnlock()
	//			return lCopy, nil
	//		}
	//	}
	//}
	globalCache.Unlock()

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

	// Only set l to the cache if readTs >= latestTs, which implies that l is
	// the latest version of the PL. We also check that we're reading a version
	// from Badger, which is higher than the write registered by the cache.
	if !pk.IsData() && readTs >= l.maxTs {
		//l.RLock()
		//defer l.RUnlock()
		//fmt.Println("Setting", l.maxTs)
		//lCache.Set(key, copyList(l), int64(l.DeepSize()))
		globalCache.Lock()

		l.RLock()
		cacheList, ok := globalCache.list[string(key)]
		if !ok || (ok && cacheList.maxTs < l.maxTs) {
			if lastUpdateTs, k := globalCache.lastUpdate[string(key)]; !k || (k && lastUpdateTs < readTs) {
				fmt.Println("[TXN] setting data", pk, l.mutationMap, l.plist, readTs, lastUpdateTs)
				globalCache.list[string(key)] = copyList(l)
			}
		}
		l.RUnlock()

		globalCache.Unlock()
	}
	return l, nil
}
