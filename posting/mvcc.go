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
	"fmt"
	"math"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
	"github.com/golang/glog"
)

var (
	ErrTsTooOld = x.Errorf("Transaction is too old")
)

func (txn *Txn) SetAbort() {
	atomic.StoreUint32(&txn.shouldAbort, 1)
}

func (txn *Txn) ShouldAbort() bool {
	if txn == nil {
		return false
	}
	return atomic.LoadUint32(&txn.shouldAbort) > 0
}

func (txn *Txn) AddKeys(key, conflictKey string) {
	txn.Lock()
	defer txn.Unlock()
	if txn.deltas == nil || txn.conflicts == nil {
		txn.deltas = make(map[string]struct{})
		txn.conflicts = make(map[string]struct{})
	}
	txn.deltas[key] = struct{}{}
	if len(conflictKey) > 0 {
		txn.conflicts[conflictKey] = struct{}{}
	}
}

func (txn *Txn) Fill(ctx *api.TxnContext, gid uint32) {
	txn.Lock()
	defer txn.Unlock()
	ctx.StartTs = txn.StartTs
	for key := range txn.conflicts {
		// We don'txn need to send the whole conflict key to Zero. Solving #2338
		// should be done by sending a list of mutating predicates to Zero,
		// along with the keys to be used for conflict detection.
		fps := strconv.FormatUint(farm.Fingerprint64([]byte(key)), 36)
		if !x.HasString(ctx.Keys, fps) {
			ctx.Keys = append(ctx.Keys, fps)
		}
	}
	for key := range txn.deltas {
		pk := x.Parse([]byte(key))
		// Also send the group id that the predicate was being served by. This is useful when
		// checking if Zero should allow a commit during a predicate move.
		predKey := fmt.Sprintf("%d-%s", gid, pk.Attr)
		if !x.HasString(ctx.Preds, predKey) {
			ctx.Preds = append(ctx.Preds, predKey)
		}
	}
}

// Don't call this for schema mutations. Directly commit them.
// This function only stores deltas to the commit timestamps. It does not try to generate a state.
// State generation is done via rollups, which happen when a snapshot is created.
func (txn *Txn) CommitToDisk(writer *TxnWriter, commitTs uint64) error {
	if commitTs == 0 {
		return nil
	}
	var keys []string
	txn.Lock()
	// TODO: We can remove the deltas here. Now that we're using txn local cache.
	for key := range txn.deltas {
		keys = append(keys, key)
	}
	txn.Unlock()

	var idx int
	for idx < len(keys) {
		// writer.Update can return early from the loop in case we encounter badger.ErrTxnTooBig. On
		// that error, writer.Update would still commit the transaction and return any error. If
		// nil, we continue to process the remaining keys.
		err := writer.Update(commitTs, func(btxn *badger.Txn) error {
			for ; idx < len(keys); idx++ {
				key := keys[idx]
				plist, err := txn.Get([]byte(key))
				if err != nil {
					return err
				}
				data := plist.GetMutation(txn.StartTs)
				if data == nil {
					continue
				}
				if plist.maxVersion() >= commitTs {
					pk := x.Parse([]byte(key))
					glog.Warningf("Existing >= Commit [%d >= %d]. Skipping write: %v",
						plist.maxVersion(), commitTs, pk)
					continue
				}
				if err := btxn.SetWithMeta([]byte(key), data, BitDeltaPosting); err != nil {
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

func (txn *Txn) CommitToMemory(commitTs uint64) error {
	txn.Lock()
	defer txn.Unlock()
	// TODO: Figure out what shouldAbort is for, and use it correctly. This should really be
	// shouldDiscard.
	// defer func() {
	// 	atomic.StoreUint32(&txn.shouldAbort, 1)
	// }()
	for key := range txn.deltas {
	inner:
		for {
			plist, err := txn.Get([]byte(key))
			if err != nil {
				return err
			}
			err = plist.CommitMutation(txn.StartTs, commitTs)
			switch err {
			case nil:
				break inner
			case ErrRetry:
				time.Sleep(5 * time.Millisecond)
			default:
				glog.Warningf("Error while committing to memory: %v\n", err)
				return err
			}
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

// constructs the posting list from the disk using the passed iterator.
// Use forward iterator with allversions enabled in iter options.
//
// key would now be owned by the posting list. So, ensure that it isn't reused
// elsewhere.
func ReadPostingList(key []byte, it *badger.Iterator) (*List, error) {
	l := new(List)
	l.key = key
	l.mutationMap = make(map[uint64]*pb.PostingList)
	l.plist = new(pb.PostingList)

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
				l.mutationMap[pl.CommitTs] = pl
				return nil
			})
			if err != nil {
				return nil, err
			}
		case BitSchemaPosting:
			return nil, x.Errorf(
				"Trying to read schema in ReadPostingList for key: %s", hex.Dump(key))
		default:
			return nil, x.Errorf(
				"Unexpected meta: %d for key: %s", item.UserMeta(), hex.Dump(key))
		}
		if item.DiscardEarlierVersions() {
			break
		}
		it.Next()
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
