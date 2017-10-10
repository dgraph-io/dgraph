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
	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"sort"
	"sync/atomic"
)

var (
	errConflict = x.Errorf("Transaction aborted due to conflict")
)

func commitTimestamp(startTs uint64) (commitTs uint64, aborted bool) {
	// TODO: wait and ask group zero about the status
	return 0, false
}

// Apply all mutations in memory and then write the delta to disk.
// TODO: Write delta per transaction at once, change it to map[string][]*posting
func addDeltaAt(key []byte, p []*protos.Posting, startTs uint64) error {
	txn := pstore.NewTransaction(true)
	defer txn.Discard()

	item, err := txn.Get(key)
	if err != nil && err != badger.ErrKeyNotFound {
		return err
	} else if err == nil && item.UserMeta()&bitUncommitedPosting != 0 {
		// If latest version is uncommited value.
		// acts like a lock
		if commitTs, aborted, err := checkCommitStatus(item); err != nil {
			return err
		} else if commitTs > startTs || (commitTs == 0 && !aborted) {
			return errConflict
		}
	}

	var pl protos.PostingList
	pl.Postings = p
	val, err := pl.Marshal()
	x.Check(err)
	err = txn.Set(key, val, bitDeltaPosting&bitUncommitedPosting)
	if err != nil {
		return err
	}
	return txn.CommitAt(startTs, nil)
}

// TODO: Can be done in background.
func clean(key []byte, startTs uint64) {
	txn := pstore.NewTransactionAt(startTs, true)
	txn.Delete(key)
	txn.CommitAt(startTs, nil)
}

func checkCommitStatus(item *badger.Item) (uint64, bool, error) {
	//  Found some uncomitted data
	vs := item.Version()
	// Can happen when client crashes after transaction is comitted but before the
	// commit keys are written on nodes.
	commitTs, aborted := commitTimestamp(vs)
	if aborted {
		clean(item.Key(), vs)
		return 0, true, nil
	}
	if commitTs > 0 {
		err := commitMutation(item.Key(), vs, commitTs)
		clean(item.Key(), vs)
		return commitTs, false, err
	}
	return 0, false, nil
}

// Probably this might not be needed, we can write delta when evicting from LRU.
func commitMutation(key []byte, startTs uint64, commitTs uint64) error {
	txn := pstore.NewTransaction(true)
	defer txn.Discard()

	item, err := txn.Get(key)
	if err != nil {
		return err
	}

	x.AssertTrue(item.Version() == startTs)
	val, err := item.Value()
	newVal := make([]byte, len(val))
	copy(newVal, val)

	// Writes the value with commitTimestamp
	err = txn.Set(key, val, bitDeltaPosting)
	if err != nil {
		return nil
	}
	return txn.CommitAt(commitTs, nil)
}

// reads the latest posting list from disk.(When not found in lru)
func getNew(key []byte, pstore *badger.DB) (*List, error) {
	txn := pstore.NewTransaction(false)
	defer txn.Discard()

	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := txn.NewIterator(iterOpts)
	defer it.Close()
	l := new(List)
	l.key = key
	l.plist = new(protos.PostingList)
	l.water = SyncMarks()

	for it.Seek(key); it.ValidForPrefix(key); it.Next() {
		item := it.Item()
		// There would be at most one uncommitted entry.
		if item.UserMeta()&bitUncommitedPosting != 0 {
			commitTs, aborted, err := checkCommitStatus(item)
			if err != nil {
				return nil, err
			} else if aborted {
				continue
			}
			val, err := item.Value()
			if err != nil {
				return nil, err
			}
			var pl protos.PostingList
			x.Check(pl.Unmarshal(val))
			if commitTs == 0 {
				l.uncommitted = pl.Postings
			}
			continue
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
		var p protos.Posting
		x.Check(p.Unmarshal(val))
		l.mlayer = append(l.mlayer, &p)
	}
	// Sort by Uid, Ts
	sort.Slice(l.mlayer, func(i, j int) bool {
		if l.mlayer[i].Uid != l.mlayer[j].Uid {
			return l.mlayer[i].Uid < l.mlayer[j].Uid
		}
		return l.mlayer[i].Commit > l.mlayer[j].Commit
	})
	size := l.calculateSize()
	x.BytesRead.Add(int64(size))
	atomic.StoreUint32(&l.estimatedSize, size)
	return l, nil
}
