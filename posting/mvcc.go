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
	"math"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

var (
	ErrTsTooOld = x.Errorf("Transaction is too old")
	txns        *transactions
	txnMarks    *x.WaterMark // Used to find out till which index we can snapshot.
)

func init() {
	txns = new(transactions)
	txns.m = make(map[uint64]*Txn)
	txnMarks = &x.WaterMark{Name: "Transaction watermark"}
	txnMarks.Init()
}

func TxnMarks() *x.WaterMark {
	return txnMarks
}

func Txns() *transactions {
	return txns
}

type delta struct {
	key     []byte
	posting *intern.Posting
}
type Txn struct {
	StartTs uint64

	// atomic
	shouldAbort uint32
	// Fields which can changed after init
	sync.Mutex
	deltas []delta
	// Stores list of proposal indexes belonging to the transaction, the watermark would
	// be marked as done only when it's committed.
	Indices []uint64
}

type transactions struct {
	x.SafeMutex
	m map[uint64]*Txn
}

func (t *transactions) MinTs() uint64 {
	t.Lock()
	defer t.Unlock()
	var minTs uint64
	for ts := range t.m {
		if ts < minTs || minTs == 0 {
			minTs = ts
		}
	}
	return minTs
}

// Returns startTs of all pending transactions started upto 10000 raft log
// entries after last snapshot if the memory consumed by all raft log entries
// is high.
func (t *transactions) TxnsSinceSnapshot() []uint64 {
	lastSnapshotIdx := TxnMarks().DoneUntil()
	var timestamps []uint64
	t.Lock()
	defer t.Unlock()
	numKeys := 0
	for _, txn := range t.m {
		lenDelta, index := txn.lenAndStartIdx()
		if index-lastSnapshotIdx <= 10000 {
			timestamps = append(timestamps, txn.StartTs)
		}
		numKeys += lenDelta
	}
	// Users can do transactions which mutates few edges, numKeys gives us
	// a good estimation of space consumed by raft log entries
	if numKeys < 10<<20 { // 500MB considering average size of posting to be 50bytes.
		return nil
	}
	return timestamps
}

func (t *transactions) Reset() {
	t.Lock()
	defer t.Unlock()
	for _, txn := range t.m {
		txn.done()
	}
	t.m = make(map[uint64]*Txn)
}

func (t *transactions) Iterate(ok func(key []byte) bool) []uint64 {
	t.RLock()
	defer t.RUnlock()
	var timestamps []uint64
	for _, txn := range t.m {
		if txn.conflicts(ok) {
			timestamps = append(timestamps, txn.StartTs)
		}
	}
	return timestamps
}

func (t *Txn) lenAndStartIdx() (int, uint64) {
	t.Lock()
	defer t.Unlock()
	x.AssertTrue(len(t.Indices) > 0)
	return len(t.deltas), t.Indices[0]
}

func (t *Txn) conflicts(ok func(key []byte) bool) bool {
	t.Lock()
	defer t.Unlock()
	for _, d := range t.deltas {
		if ok(d.key) {
			return true
		}
	}
	return false
}

func (t *transactions) Get(startTs uint64) *Txn {
	t.RLock()
	defer t.RUnlock()
	return t.m[startTs]
}

func (t *transactions) Done(startTs uint64) {
	t.Lock()
	defer t.Unlock()
	txn, ok := t.m[startTs]
	if !ok {
		return
	}
	txn.done()
	delete(t.m, startTs)
}

func (t *Txn) done() {
	t.Lock()
	defer t.Unlock()
	// All indices should have been added by  now.
	TxnMarks().DoneMany(t.Indices)
}

// LastIndex returns the index of last prewrite proposal associated with
// the transaction.
func (t *Txn) LastIndex() uint64 {
	t.Lock()
	defer t.Unlock()
	if l := len(t.Indices); l > 0 {
		return t.Indices[l-1]
	}
	return 0
}

func (t *transactions) PutOrMergeIndex(src *Txn) *Txn {
	t.Lock()
	defer t.Unlock()
	dst := t.m[src.StartTs]
	if dst == nil {
		t.m[src.StartTs] = src
		return src
	}
	x.AssertTrue(src.StartTs == dst.StartTs)
	dst.Indices = append(dst.Indices, src.Indices...)
	return dst
}

func (t *Txn) SetAbort() {
	atomic.StoreUint32(&t.shouldAbort, 1)
}

func (t *Txn) ShouldAbort() bool {
	return atomic.LoadUint32(&t.shouldAbort) > 0
}

func (t *Txn) AddDelta(key []byte, p *intern.Posting) {
	t.Lock()
	defer t.Unlock()
	t.deltas = append(t.deltas, delta{key: key, posting: p})
}

func (t *Txn) Fill(ctx *api.TxnContext) {
	t.Lock()
	defer t.Unlock()
	t.fill(ctx)
}

func (t *Txn) fill(ctx *api.TxnContext) {
	ctx.StartTs = t.StartTs
	for _, d := range t.deltas {
		fp := farm.Fingerprint64(d.key)
		ctx.Keys = append(ctx.Keys, strconv.FormatUint(fp, 36))
	}
}

// Don't call this for schema mutations. Directly commit them.
func (tx *Txn) CommitMutations(ctx context.Context, commitTs uint64) error {
	tx.Lock()
	defer tx.Unlock()
	if tx.ShouldAbort() {
		return ErrInvalidTxn
	}

	txn := pstore.NewTransactionAt(commitTs, true)
	defer txn.Discard()
	// Sort by keys so that we have all postings for same pl side by side.
	sort.SliceStable(tx.deltas, func(i, j int) bool {
		return bytes.Compare(tx.deltas[i].key, tx.deltas[j].key) < 0
	})
	var prevKey []byte
	var pl *intern.PostingList
	var plist *List
	i := 0
	for i < len(tx.deltas) {
		d := tx.deltas[i]
		if !bytes.Equal(prevKey, d.key) {
			plist = Get(d.key)
			if plist.AlreadyCommitted(tx.StartTs) {
				// Delta already exists, so skip the key
				// There won't be any race from lru eviction, because we don't
				// commit in memory unless we write delta to disk.
				i++
				for i < len(tx.deltas) && bytes.Equal(tx.deltas[i].key, d.key) {
					i++
				}
				continue
			}
			pl = new(intern.PostingList)
		}
		prevKey = d.key
		var meta byte
		if d.posting.Op == Del && bytes.Equal(d.posting.Value, []byte(x.Star)) {
			pl.Postings = pl.Postings[:0]
			meta = BitCompletePosting // Indicates that this is the full posting list.
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

		// delta postings are pointers to the postings present in the Pl present in lru.
		// commitTs is accessed using RLock & atomics except in marshal so no RLock.
		// TODO: Fix this hack later
		plist.Lock()
		val, err := pl.Marshal()
		plist.Unlock()
		x.Check(err)
		if err = txn.SetWithMeta([]byte(d.key), val, meta); err == badger.ErrTxnTooBig {
			if err := txn.CommitAt(commitTs, nil); err != nil {
				return err
			}
			txn = pstore.NewTransactionAt(commitTs, true)
			if err := txn.SetWithMeta([]byte(d.key), val, meta); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		i++
	}
	if err := txn.CommitAt(commitTs, nil); err != nil {
		return err
	}
	return tx.commitMutationsMemory(ctx, commitTs)
}

func (tx *Txn) CommitMutationsMemory(ctx context.Context, commitTs uint64) error {
	tx.Lock()
	defer tx.Unlock()
	return tx.commitMutationsMemory(ctx, commitTs)
}

func (tx *Txn) commitMutationsMemory(ctx context.Context, commitTs uint64) error {
	for _, d := range tx.deltas {
		plist := Get(d.key)
		err := plist.CommitMutation(ctx, tx.StartTs, commitTs)
		for err == ErrRetry {
			time.Sleep(5 * time.Millisecond)
			plist = Get(d.key)
			err = plist.CommitMutation(ctx, tx.StartTs, commitTs)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (tx *Txn) AbortMutations(ctx context.Context) error {
	tx.Lock()
	defer tx.Unlock()
	for _, d := range tx.deltas {
		plist := Get([]byte(d.key))
		err := plist.AbortTransaction(ctx, tx.StartTs)
		for err == ErrRetry {
			time.Sleep(5 * time.Millisecond)
			plist = Get(d.key)
			err = plist.AbortTransaction(ctx, tx.StartTs)
		}
		if err != nil {
			return err
		}
	}
	atomic.StoreUint32(&tx.shouldAbort, 1)
	return nil
}

func unmarshalOrCopy(plist *intern.PostingList, item *badger.Item) error {
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
	if item.UserMeta()&BitUidPosting != 0 {
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
	l.activeTxns = make(map[uint64]struct{})
	l.plist = new(intern.PostingList)

	// Iterates from highest Ts to lowest Ts
	for it.Valid() {
		item := it.Item()
		if !bytes.Equal(item.Key(), l.key) {
			break
		}
		if l.commitTs == 0 {
			l.commitTs = item.Version()
		}

		val, err := item.Value()
		if err != nil {
			return nil, err
		}
		if item.UserMeta()&BitCompletePosting > 0 {
			if err := unmarshalOrCopy(l.plist, item); err != nil {
				return nil, err
			}
			l.minTs = item.Version()
			it.Next()
			break
		} else if item.UserMeta()&bitDeltaPosting > 0 {
			var pl intern.PostingList
			x.Check(pl.Unmarshal(val))
			for _, mpost := range pl.Postings {
				// commitTs, startTs are meant to be only in memory, not
				// stored on disk.
				mpost.CommitTs = item.Version()
				l.mlayer = append(l.mlayer, mpost)
			}
		} else {
			x.Fatalf("unexpected meta: %d", item.UserMeta())
		}
		it.Next()
	}

	// Sort by Uid, Ts
	sort.Slice(l.mlayer, func(i, j int) bool {
		if l.mlayer[i].Uid != l.mlayer[j].Uid {
			return l.mlayer[i].Uid < l.mlayer[j].Uid
		}
		return l.mlayer[i].CommitTs >= l.mlayer[j].CommitTs
	})
	return l, nil
}

func getNew(key []byte, pstore *badger.ManagedDB) (*List, error) {
	l := new(List)
	l.key = key
	l.activeTxns = make(map[uint64]struct{})
	l.plist = new(intern.PostingList)
	txn := pstore.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()

	item, err := txn.Get(key)
	if err == badger.ErrKeyNotFound {
		return l, nil
	}
	if err != nil {
		return l, err
	}
	if item.UserMeta()&BitCompletePosting > 0 {
		err = unmarshalOrCopy(l.plist, item)
		l.minTs = item.Version()
		l.commitTs = item.Version()
	} else {
		iterOpts := badger.DefaultIteratorOptions
		iterOpts.AllVersions = true
		it := txn.NewIterator(iterOpts)
		defer it.Close()
		it.Seek(key)
		l, err = ReadPostingList(key, it)
	}

	l.Lock()
	size := l.calculateSize()
	l.Unlock()
	x.BytesRead.Add(int64(size))
	atomic.StoreInt32(&l.estimatedSize, size)
	return l, err
}
