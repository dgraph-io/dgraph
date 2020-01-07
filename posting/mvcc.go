/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v2"
	bpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

// IncRollupi is used to batch keys for rollup incrementally.
type IncRollupi struct {
	// keysCh is populated with batch of 64 keys that needs to be rolled up during reads
	keysCh chan *[][]byte
	// keysPool is sync.Pool to share the batched keys to rollup.
	keysPool sync.Pool
}

var (
	// ErrTsTooOld is returned when a transaction is too old to be applied.
	ErrTsTooOld = errors.Errorf("Transaction is too old")

	// IncrRollup is used to batch keys for rollup incrementally.
	IncrRollup = IncRollupi{
		keysCh: make(chan *[][]byte),
		keysPool: sync.Pool{
			New: func() interface{} {
				return new([][]byte)
			},
		},
	}
)

// rollUpKey takes the given key's posting lists, rolls it up and writes back to badger
func (ir *IncRollupi) rollUpKey(writer *TxnWriter, key []byte) error {
	l, err := GetNoStore(key)
	if err != nil {
		return err
	}

	kvs, err := l.Rollup()
	if err != nil {
		return err
	}

	return writer.Write(&bpb.KVList{Kv: kvs})
}

func (ir *IncRollupi) addKeyToBatch(key []byte) {
	batch := ir.keysPool.Get().(*[][]byte)
	*batch = append(*batch, key)
	if len(*batch) < 64 {
		ir.keysPool.Put(batch)
		return
	}

	select {
	case ir.keysCh <- batch:
	default:
		// Drop keys and build the batch again. Lossy behavior.
		*batch = (*batch)[:0]
		ir.keysPool.Put(batch)
	}
}

// Process will rollup batches of 64 keys in a go routine.
func (ir *IncRollupi) Process() {
	m := make(map[uint64]int64) // map hash(key) to ts. hash(key) to limit the size of the map.
	limiter := time.NewTicker(100 * time.Millisecond)
	writer := NewTxnWriter(pstore)

	for batch := range ir.keysCh {
		currTs := time.Now().Unix()
		for _, key := range *batch {
			hash := z.MemHash(key)
			if elem, ok := m[hash]; !ok || (currTs-elem >= 10) {
				// Key not present or Key present but last roll up was more than 10 sec ago.
				// Add/Update map and rollup.
				m[hash] = currTs
				err := ir.rollUpKey(writer, key)
				if err != nil {
					glog.Warningf("Error %v rolling up key %v\n", err, key)
					continue
				}
			}
		}
		// clear the batch and put it back in Sync keysPool
		*batch = (*batch)[:0]
		ir.keysPool.Put(batch)

		// throttle to 1 batch = 64 rollups per 100 ms.
		<-limiter.C
	}
	// keysCh is closed. This should never happen.
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
		if !x.HasString(ctx.Keys, fps) {
			ctx.Keys = append(ctx.Keys, fps)
		}
	}
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

func unmarshalOrCopy(plist *pb.PostingList, item *badger.Item) error {
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
	l := new(List)
	l.key = key
	l.plist = new(pb.PostingList)
	const maxDeltaCount = 2
	deltaCount := 0

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
				x.Check(pl.Unmarshal(val))
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

	if deltaCount >= maxDeltaCount {
		IncrRollup.addKeyToBatch(key)
	}

	return l, nil
}

// TODO: We should only create a posting list with a specific readTs.
func getNew(key []byte, pstore *badger.DB) (*List, error) {
	txn := pstore.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()

	// When we do rollups, an older version would go to the top of the LSM tree, which can cause
	// issues during txn.Get. Therefore, always iterate.
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	iterOpts.PrefetchValues = false
	itr := txn.NewKeyIterator(key, iterOpts)
	defer itr.Close()
	itr.Seek(key)
	return ReadPostingList(key, itr)
}
