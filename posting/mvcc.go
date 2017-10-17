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
	"context"
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

		if err == nil {
			// We should either find a commit entry or prewrite at same ts
			if item.Version() == t.StartTs {
				val, err := item.Value()
				if err != nil {
					return err
				}
				x.Check(pl.Unmarshal(val))
				x.AssertTrue(pl.PrimaryAttr == t.PrimaryAttr)
			} else {
				x.AssertTrue(item.UserMeta()&bitCommitMarker != 0)
			}
		} else if err != badger.ErrKeyNotFound {
			return err
		}

		var meta byte
		pl.PrimaryAttr = t.PrimaryAttr
		if d.posting.Op == Del && bytes.Equal(d.posting.Value, []byte(x.Star)) {
			pl.Postings = pl.Postings[:0]
			meta = bitCompletePosting // Indicates that this is the full posting list.
		} else {
			midx := sort.Search(len(pl.Postings), func(idx int) bool {
				mp := pl.Postings[idx]
				return d.posting.Uid <= mp.Uid
			})
			if midx >= len(pl.Postings) {
				pl.Postings = append(pl.Postings, d.posting)
			} else if pl.Postings[midx].Uid == d.posting.Uid {
				// Replace
				pl.Postings[midx] = d.posting
			} else {
				pl.Postings = append(pl.Postings, nil)
				copy(pl.Postings[midx+1:], pl.Postings[midx:])
				pl.Postings[midx] = d.posting
			}
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
func checkCommitStatusHelper(key []byte, vs uint64) error {
	commitTs, aborted, err := commitTimestamp(vs)
	if err != nil {
		return err
	}
	if aborted {
		return AbortMutations(context.Background(), []string{string(key)}, vs)
	}

	if commitTs > 0 {
		tctx := &protos.TxnContext{CommitTs: commitTs, StartTs: vs}
		tctx.Keys = []string{string(key)}
		return CommitMutations(context.Background(), tctx, false)
	}
	// uncommitted
	return nil
}

// Writes all commit keys of the transaction.
// Called after all mutations are committed in memory.
func CommitMutations(ctx context.Context, tx *protos.TxnContext, writeLock bool) error {
	idx := 0
	for _, key := range tx.Keys {
		plist := Get([]byte(key))
		committed, err := plist.CommitMutation(ctx, tx.StartTs, tx.CommitTs)
		if err != nil {
			return err
		}
		// Ensures that we don't write commit keys if pl was already committed.
		if committed {
			tx.Keys[idx] = key
			idx++
		}
	}
	tx.Keys = tx.Keys[:idx]

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
func AbortMutations(ctx context.Context, keys []string, startTs uint64) error {
	idx := 0
	for _, key := range keys {
		plist := Get([]byte(key))
		aborted, err := plist.AbortTransaction(ctx, startTs)
		if err != nil {
			return err
		}
		if aborted {
			keys[idx] = key
			idx++
		}
	}
	keys = keys[:idx]

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

func unmarshalOrCopy(plist *protos.PostingList, item *badger.Item) error {
	// It's delta
	val, err := item.Value()
	if err != nil {
		return err
	}
	if len(val) == 0 {
		// empty pl
		return nil
	}
	// Found complete pl, no needn't iterate more
	if item.UserMeta()&bitUidPostings != 0 {
		plist.Uids = make([]byte, len(val))
		copy(plist.Uids, val)
	} else if len(val) > 0 {
		x.Check(plist.Unmarshal(val))
	}
	return nil
}

// constructs the posting list from the disk using the passed iterator.
// Use forward iterator with allversions enabled in iter options.
func readPostingList(key []byte, it *badger.Iterator) (*List, error) {
	l := new(List)
	l.key = key
	l.plist = new(protos.PostingList)

	var commitTs uint64
	// CommitMarkers and Deltas are always interleaved.
	// Iterates from highest Ts to lowest Ts
	for it.Valid() {
		item := it.Item()
		if !bytes.Equal(item.Key(), l.key) {
			break
		}
		if item.UserMeta()&bitCommitMarker > 0 {
			// It's a commit key.
			commitTs = item.Version()
			l.minTs = commitTs
			if l.commitTs == 0 { // highest commitTs
				l.commitTs = commitTs
			}

			// No posting is present here
			if item.UserMeta()&bitCompletePosting == 0 {
				it.Next()
				continue
			}
			if err := unmarshalOrCopy(l.plist, item); err != nil {
				return l, err
			}
			break
		}

		// It's delta
		val, err := item.Value()
		if err != nil {
			return nil, err
		}
		if commitTs == 0 {
			l.startTs = item.Version()
		}
		if item.UserMeta()&bitCompletePosting > 0 {
			x.Check(l.plist.Unmarshal(val))
			if commitTs == 0 {
				l.PrimayKey = l.plist.PrimaryAttr
			}
			break
		} else if item.UserMeta()&bitDeltaPosting > 0 {
			var pl protos.PostingList
			x.Check(pl.Unmarshal(val))
			for _, mpost := range pl.Postings {
				if commitTs > 0 {
					mpost.Commit = commitTs
				}
			}
			l.mlayer = append(l.mlayer, pl.Postings...)
			if commitTs == 0 {
				l.PrimayKey = pl.PrimaryAttr
			}
		} else {
			x.Fatalf("unexpected meta")
		}
		it.Next()
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
