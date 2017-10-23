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
	ErrConflict = x.Errorf("Transaction aborted due to conflict")
	errTsTooOld = x.Errorf("Transaction is too old")
	txns        *transactions
)

func init() {
	txns.m = make(map[uint64]*Txn)
}

func Txns() *transactions {
	return txns
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
	hasConflict uint32
	// Fields which can changed after init
	sync.Mutex
	deltas    []delta
	conflicts []*protos.TxnContext
}

type transactions struct {
	x.SafeMutex
	m map[uint64]*Txn
}

func (t *transactions) Get(startTs uint64) *Txn {
	t.RLock()
	defer t.RUnlock()
	return t.m[startTs]
}

func (t *transactions) Done(startTs uint64) {
	t.Lock()
	defer t.RUnlock()
	delete(t.m, startTs)
}

func (t *transactions) GetOrCreate(startTs uint64, primary string, servesPrimary bool) *Txn {
	if txn := t.Get(startTs); txn != nil {
		return txn
	}
	t.Lock()
	defer t.Unlock()
	if txn := t.m[startTs]; txn != nil {
		return txn
	}
	txn := &Txn{
		StartTs:       startTs,
		PrimaryAttr:   primary,
		ServesPrimary: servesPrimary,
	}
	t.m[startTs] = txn
	return txn
}

func (t *Txn) WriteLock(index uint64) error {
	if t == nil {
		return nil
	}
	if !t.ServesPrimary {
		return nil
	}
	txn := pstore.NewTransaction(true)
	defer txn.Discard()
	var buf [8]byte
	if err := txn.Set(x.LockKey(t.PrimaryAttr, t.StartTs), buf[:], 0); err != nil {
		return err
	}
	SyncMarks().Begin(index)
	return txn.Commit(func(err error) {
		// TODO: Retry
		// If this errors out txn won't be committed.
		SyncMarks().Done(index)
	})
}

func (t *Txn) AddConflict(conflict *protos.TxnContext) {
	atomic.StoreUint32(&t.hasConflict, 1)
	t.Lock()
	defer t.Unlock()
	t.conflicts = append(t.conflicts, conflict)
}

func (t *Txn) Conflicts() []*protos.TxnContext {
	if t == nil {
		return nil
	}
	t.Lock()
	defer t.Unlock()
	return t.conflicts
}

func (t *Txn) HasConflict() bool {
	return atomic.LoadUint32(&t.hasConflict) > 0
}

func (t *Txn) AddDelta(key []byte, p *protos.Posting) {
	t.Lock()
	defer t.Unlock()
	t.deltas = append(t.deltas, delta{key: key, posting: p})
}

func (t *Txn) fill(ctx *protos.TxnContext) {
	ctx.StartTs = t.StartTs
	ctx.Primary = t.PrimaryAttr
}

func (t *Txn) Fill(ctx *protos.TxnContext) {
	t.Lock()
	defer t.Unlock()
	t.fill(ctx)
}

// TODO: Use commitAsync
func (tx *Txn) CommitMutations(ctx context.Context, commitTs uint64, writeLock bool) error {
	if writeLock {
		lk := x.LockKey(tx.PrimaryAttr, tx.StartTs)
		// First update the primary key to indicate the status of transaction.
		txn := pstore.NewTransaction(true)
		defer txn.Discard()

		item, err := txn.Get(lk)
		if err == badger.ErrKeyNotFound {
			// Already aborted.
			return ErrInvalidTxn
		} else if err != nil {
			return err
		}
		val, err := item.Value()
		if err != nil {
			return err
		}
		ts := binary.BigEndian.Uint64(val)
		if ts > 0 && ts != commitTs {
			return ErrInvalidTxn
		}
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], commitTs)
		if err := txn.Set(lk, buf[:], 0); err != nil {
			return err
		}
		if err := txn.CommitAt(tx.StartTs, nil); err != nil {
			return err
		}
	}

	txn := pstore.NewTransaction(true)
	defer txn.Discard()
	for _, d := range tx.deltas {
		var pl protos.PostingList
		var meta byte
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
	if err := txn.CommitAt(commitTs, nil); err != nil {
		return err
	}

	for _, d := range tx.deltas {
		plist := Get([]byte(d.key))
		err := plist.CommitMutation(ctx, tx.StartTs, commitTs)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tx *Txn) AbortMutations(ctx context.Context, writeLock bool) error {
	if writeLock {
		lk := x.LockKey(tx.PrimaryAttr, tx.StartTs)
		// First update the primary key to indicate the status of transaction.
		txn := pstore.NewTransaction(true)
		defer txn.Discard()

		item, err := txn.Get(lk)
		if err == badger.ErrKeyNotFound {
			// We write lock key in async way, if lock key write fails no issue we can
			// still abort the transaction.
		} else if err != nil {
			return err
		}
		val, err := item.Value()
		if err != nil {
			return err
		}
		ts := binary.BigEndian.Uint64(val)
		if ts > 0 {
			// Already committed
			return ErrInvalidTxn
		}
		if err := txn.Delete(lk); err != nil {
			return err
		}
		if err := txn.CommitAt(tx.StartTs, nil); err != nil {
			return err
		}
	}
	for _, d := range tx.deltas {
		plist := Get([]byte(d.key))
		err := plist.AbortTransaction(ctx, tx.StartTs)
		if err != nil {
			return err
		}
	}
	return nil
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
func ReadPostingList(key []byte, it *badger.Iterator) (*List, error) {
	l := new(List)
	l.key = key
	l.plist = new(protos.PostingList)

	// Iterates from highest Ts to lowest Ts
	for it.Valid() {
		item := it.Item()
		if !bytes.Equal(item.Key(), l.key) {
			break
		}
		l.minTs = item.Version()
		if l.commitTs == 0 { // highest commitTs
			l.commitTs = item.Version()
		}

		val, err := item.Value()
		if err != nil {
			return nil, err
		}
		if item.UserMeta()&bitCompletePosting > 0 {
			x.Check(l.plist.Unmarshal(val))
			break
		} else if item.UserMeta()&bitDeltaPosting > 0 {
			var pl protos.PostingList
			x.Check(pl.Unmarshal(val))
			l.mlayer = append(l.mlayer, pl.Postings...)
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
	return ReadPostingList(key, it)
}
