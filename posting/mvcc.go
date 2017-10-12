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
	"sort"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var (
	errConflict = x.Errorf("Transaction aborted due to conflict")
	errTsTooOld = x.Errorf("Transaction is too old")
)

// Dummy function for now
func commitTimestamp(startTs uint64) (commitTs uint64, aborted bool, err error) {
	// TODO: wait and ask group zero about the status
	// Do some batching/caching using group zero stream.
	return 0, false, nil
}

type Txn struct {
	StartTs uint64
	// Fields which can changed after init
	sync.Mutex
	m map[string][]*protos.Posting
	// atomic
	aborted uint32
}

func (t *Txn) Abort() {
	atomic.StoreUint32(&t.aborted, 1)
}

func (t *Txn) Aborted() uint32 {
	return atomic.LoadUint32(&t.aborted)
}

func (t *Txn) AddDelta(key []byte, p *protos.Posting) {
	t.Lock()
	defer t.Unlock()
	if t.m == nil {
		t.m = make(map[string][]*protos.Posting)
	}
	t.m[string(key)] = append(t.m[string(key)], p)
}

// Write All deltas per transaction at once.
// Called after all mutations are applied in memory and checked for locks/conflicts.
func (t *Txn) CommitDeltas() error {
	t.Lock()
	defer t.Unlock()
	if t.Aborted() != 0 {
		return errConflict
	}
	txn := pstore.NewTransaction(true)
	defer txn.Discard()

	for k, v := range t.m {
		var pl protos.PostingList
		item, err := txn.Get([]byte(k))
		if err == nil {
			val, err := item.Value()
			if err != nil {
				return err
			}
			pl.Unmarshal(val)
		}

		pl.Postings = append(pl.Postings, v...)
		val, err := pl.Marshal()
		x.Check(err)
		err = txn.Set([]byte(k), val, bitDeltaPosting)
		if err != nil {
			return err
		}
	}
	return txn.CommitAt(t.StartTs, nil)
}

// clean deletes the key with startTs after txn is aborted.
func clean(key []byte, startTs uint64) {
	txn := pstore.NewTransactionAt(startTs, true)
	txn.Delete(key)
	// We don't care about the error
	txn.CommitAt(startTs, func(err error) {})
}

func checkCommitStatus(key []byte, vs uint64) (uint64, bool, error) {
	commitTs, aborted, err := commitTimestamp(vs)
	if err != nil {
		return 0, false, err
	}
	if aborted {
		clean(key, vs)
		return 0, true, nil
	}
	if commitTs > 0 {
		err := commitMutations([][]byte{key}, commitTs)
		if err == nil {
			clean(key, vs)
		}
		return commitTs, false, err
	}
	return 0, false, nil
}

// Writes all commit keys of the transaction.
// Called after all mutations are committed in memory.
func commitMutations(keys [][]byte, commitTs uint64) error {
	txn := pstore.NewTransaction(true)
	defer txn.Discard()

	for _, k := range keys {
		err := txn.Set(k, nil, bitCommitMarker)
		if err != nil {
			return nil
		}
	}
	return txn.CommitAt(commitTs, nil)
}

func abortMutations(keys [][]byte, startTs uint64) error {
	txn := pstore.NewTransaction(true)
	defer txn.Discard()

	for _, k := range keys {
		err := txn.Delete(k)
		if err != nil {
			return nil
		}
	}
	return txn.CommitAt(startTs, nil)
}

// constructs the posting list from the disk using the passed iterator.
// Use forward iterator with allversions enabled in iter options.
func readPostingList(key []byte, it *badger.Iterator) (*List, error) {
	l := new(List)
	l.key = key
	l.plist = new(protos.PostingList)

	var commitTs uint64
	for it.ValidForPrefix(l.key) {
		// TODO: Break when key changes.
		item := it.Item()
		if item.UserMeta()&bitCommitMarker != 0 {
			// CommitMarkers and Deltas are always interleaved.
			commitTs = item.Version()
			it.Next()
			continue
		}

		// Found some uncommitted entry
		if commitTs == 0 {
			var aborted bool
			var err error
			// There would be at most one uncommitted entry.
			commitTs, aborted, err = checkCommitStatus(item.Key(), item.Version())
			if err != nil {
				return nil, err
			} else if aborted {
				continue
			} else if commitTs == 0 {
				l.startTs = item.Version()
			}
		}

		val, err := item.Value()
		if err != nil {
			return nil, err
		}
		if item.UserMeta()&bitDeltaPosting == 0 {
			// Found complete pl, no needn't iterate more
			if item.UserMeta()&bitUidPostings != 0 {
				l.plist.Uids = make([]byte, len(val))
				copy(l.plist.Uids, val)
			} else if len(val) > 0 {
				x.Check(l.plist.Unmarshal(val))
			}
			l.commitTs = item.Version()
			break
		}
		// It's delta
		var pl protos.PostingList
		x.Check(pl.Unmarshal(val))
		for _, mpost := range pl.Postings {
			if commitTs > 0 {
				mpost.Commit = commitTs
			} else {
				mpost.Commit = item.Version()
			}
		}
		l.mlayer = append(l.mlayer, pl.Postings...)
	}
	// Sort by Uid, Ts
	sort.Slice(l.mlayer, func(i, j int) bool {
		if l.mlayer[i].Uid != l.mlayer[j].Uid {
			return l.mlayer[i].Uid < l.mlayer[j].Uid
		}
		return l.mlayer[i].Commit > l.mlayer[j].Commit
	})
	l.Lock()
	size := l.calculateSize()
	l.Unlock()
	x.BytesRead.Add(int64(size))
	atomic.StoreUint32(&l.estimatedSize, size)
	return l, nil
}

func getNew(key []byte, pstore *badger.DB) (*List, error) {
	txn := pstore.NewTransaction(false)
	defer txn.Discard()

	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := txn.NewIterator(iterOpts)
	defer it.Close()
	it.Seek(key)
	return readPostingList(key, it)
}
