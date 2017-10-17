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
	"bytes"
	"encoding/binary"
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
	// Ask group zero/primary key regarding the status of transaction.
	return 0, false, nil
}

type delta struct {
	key     []byte
	posting *protos.Posting
}
type Txn struct {
	StartTs       uint64
	PrimaryAttr   string
	ServesPrimary bool

	// atomic
	aborted uint32
	// Fields which can changed after init
	sync.Mutex
	deltas []delta
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
	t.deltas = append(t.deltas, delta{key: key, posting: p})
}

func (t *Txn) Fill(ctx *protos.TxnContext) {
	t.Lock()
	defer t.Unlock()
	ctx.StartTs = t.StartTs
	ctx.Primary = t.PrimaryAttr
	for _, d := range t.deltas {
		ctx.Keys = append(ctx.Keys, string(d.key))
	}
}

// Write All deltas per transaction at once.
// Called after all mutations are applied in memory and checked for locks/conflicts.
func (t *Txn) WriteDeltas() error {
	if t == nil {
		return nil
	}
	// TODO: Avoid delta for schema mutations, directly commit.
	t.Lock()
	defer t.Unlock()
	if t.Aborted() != 0 {
		return errConflict
	}
	txn := pstore.NewTransactionAt(t.StartTs, true)
	defer txn.Discard()

	if t.ServesPrimary {
		lk := x.LockKey(t.PrimaryAttr)
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], 0) // Indicates pending or aborted.
		// This write is only for determining the status of the transaction. Nothing else.
		if err := txn.Set(lk, buf[:], 0); err != nil {
			return err
		}
	}

	for _, d := range t.deltas {
		var pl protos.PostingList
		item, err := txn.Get([]byte(d.key))

		if err == nil && item.Version() == t.StartTs {
			val, err := item.Value()
			if err != nil {
				return err
			}
			x.Check(pl.Unmarshal(val))
			x.AssertTrue(pl.PrimaryAttr == t.PrimaryAttr)
		} else if err != badger.ErrKeyNotFound {
			return err
		}

		var meta byte
		pl.PrimaryAttr = t.PrimaryAttr
		if d.posting.Op == Del && bytes.Equal(d.posting.Value, []byte(x.Star)) {
			pl.Postings = pl.Postings[:0]
			meta = 0 // Indicates that this is the full posting list.
		} else {
			pl.Postings = append(pl.Postings, d.posting)
			meta = bitDeltaPosting
		}

		val, err := pl.Marshal()
		x.Check(err)
		if err = txn.Set([]byte(d.key), val, meta); err != nil {
			return err
		}
	}
	return txn.CommitAt(t.StartTs, nil)
}

// clean deletes the key with startTs after txn is aborted.
func clean(key []byte, startTs uint64) {
	txn := pstore.NewTransactionAt(startTs, true)
	txn.Delete(key)
	if err := txn.CommitAt(startTs, func(err error) {}); err != nil {
		x.Printf("Error while cleaning key %q %v\n", key, err)
	}
}

// checks the status and aborts/cleanups based on the response.
func checkCommitStatusHelper(key []byte, vs uint64) (uint64, bool, error) {
	commitTs, aborted, err := commitTimestamp(vs)
	if err != nil {
		return 0, false, err
	}
	if aborted {
		clean(key, vs)
		return 0, true, nil
	}
	if commitTs > 0 {
		tctx := &protos.TxnContext{CommitTs: commitTs}
		tctx.Keys = []string{string(key)}
		err := CommitMutations(tctx, false)
		if err == nil {
			clean(key, vs)
		}
		return commitTs, false, err
	}
	// uncommitted
	return 0, false, nil
}

// Writes all commit keys of the transaction.
// Called after all mutations are committed in memory.
func CommitMutations(tx *protos.TxnContext, writeLock bool) error {
	if writeLock {
		// First update the primary key to indicate the status of transaction.
		txn := pstore.NewTransaction(true)
		defer txn.Discard()

		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], tx.CommitTs)
		if err := txn.Set(x.LockKey(tx.Primary), buf[:], 0); err != nil {
			return err
		}
		if err := txn.CommitAt(tx.StartTs, nil); err != nil {
			return err
		}
	}

	// Now write the commit markers.
	txn := pstore.NewTransaction(true)
	defer txn.Discard()
	for _, k := range tx.Keys {
		if err := txn.Set([]byte(k), nil, bitCommitMarker); err != nil {
			return err
		}
	}
	return txn.CommitAt(tx.CommitTs, nil)
}

// Delete all deltas we wrote to badger.
// Called after mutations are aborted in memory.
func AbortMutations(keys []string, startTs uint64) error {
	txn := pstore.NewTransaction(true)
	defer txn.Discard()

	for _, k := range keys {
		err := txn.Delete([]byte(k))
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
	// CommitMarkers and Deltas are always interleaved.
	for it.Valid() {
		item := it.Item()
		if !bytes.Equal(item.Key(), l.key) {
			break
		}
		if item.UserMeta()&bitCommitMarker != 0 {
			// It's a commit key.
			commitTs = item.Version()
			it.Next()
			continue
		}

		// Found some uncommitted entry
		if commitTs == 0 {
			var aborted bool
			var err error
			// There would be at most one uncommitted entry per pl
			commitTs, aborted, err = checkCommitStatusHelper(item.Key(), item.Version())
			if err != nil {
				return nil, err
			} else if aborted {
				continue
			} else if commitTs == 0 {
				// not yet committed.
				l.startTs = item.Version()
			} else if commitTs > 0 {
				l.commitTs = commitTs
			}
		} else if l.commitTs == 0 {
			// First comitted entry
			l.commitTs = commitTs
		}

		val, err := item.Value()
		if err != nil {
			return nil, err
		}
		if item.UserMeta()&bitDeltaPosting == 0 {
			if len(val) == 0 {
				l.minTs = item.Version()
				break
			}
			// Found complete pl, no needn't iterate more
			// TODO: Move it at commit marker.
			if item.UserMeta()&bitUidPostings != 0 {
				l.plist.Uids = make([]byte, len(val))
				copy(l.plist.Uids, val)
			} else if len(val) > 0 {
				x.Check(l.plist.Unmarshal(val))
			}
			l.minTs = item.Version()
			break
		}
		// It's delta
		var pl protos.PostingList
		x.Check(pl.Unmarshal(val))
		for _, mpost := range pl.Postings {
			if commitTs > 0 {
				mpost.Commit = commitTs
			}
			// else delta with corresponding commitTs
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
