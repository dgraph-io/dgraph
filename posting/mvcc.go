/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package posting

import (
	"bytes"
	"context"
	"encoding/base64"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
)

var (
	ErrTsTooOld = x.Errorf("Transaction is too old")
	txns        *transactions
	txnMarks    *x.WaterMark // Used to find out till what RAFT index we can snapshot entries.
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

// This structure is useful to keep track of which keys were updated, and whether they should be
// used for conflict detection or not. When a txn is marked committed or aborted, this is what we
// use to go fetch the posting lists and update the txn status in them.
type delta struct {
	key           []byte
	posting       *intern.Posting
	checkConflict bool // Check conflict detection.
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
	Indices    []uint64
	nextKeyIdx int
}

type transactions struct {
	x.SafeMutex
	m map[uint64]*Txn
}

func (t *transactions) MinTs() uint64 {
	t.Lock()
	var minTs uint64
	for ts := range t.m {
		if ts < minTs || minTs == 0 {
			minTs = ts
		}
	}
	t.Unlock()
	maxPending := Oracle().MaxPending()
	if minTs == 0 {
		// maxPending gives the guarantee that all commits with timestamp
		// less than maxPending should have been done and since nothing
		// is present in map, all transactions with commitTs below maxPending
		// have been written to disk.
		return maxPending
	} else if maxPending < minTs {
		// Not sure if needed, but just for safety
		return maxPending
	}
	return minTs
}

func (t *transactions) TxnsSinceSnapshot(pending uint64) []uint64 {
	lastSnapshotIdx := TxnMarks().DoneUntil()
	var timestamps []uint64
	t.Lock()
	defer t.Unlock()
	var oldest float64 = 0.2 * float64(pending)
	for _, txn := range t.m {
		index := txn.startIdx()
		// We abort oldest 20% of the transactions.
		if index-lastSnapshotIdx <= uint64(oldest) {
			timestamps = append(timestamps, txn.StartTs)
		}
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

func (t *Txn) startIdx() uint64 {
	t.Lock()
	defer t.Unlock()
	x.AssertTrue(len(t.Indices) > 0)
	return t.Indices[0]
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
	// All indices should have been added by now.
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
	if t == nil {
		return false
	}
	return atomic.LoadUint32(&t.shouldAbort) > 0
}

func (t *Txn) AddDelta(key []byte, p *intern.Posting, checkConflict bool) {
	t.Lock()
	defer t.Unlock()
	t.deltas = append(t.deltas, delta{key: key, posting: p, checkConflict: checkConflict})
}

func (t *Txn) Fill(ctx *api.TxnContext) {
	t.Lock()
	defer t.Unlock()
	ctx.StartTs = t.StartTs
	for i := t.nextKeyIdx; i < len(t.deltas); i++ {
		d := t.deltas[i]
		if d.checkConflict {
			// Instead of taking a fingerprint of the keys, send the whole key to Zero. So, Zero can
			// parse the key and check if that predicate is undergoing a move, hence avoiding #2338.
			k := base64.StdEncoding.EncodeToString(d.key)
			ctx.Keys = append(ctx.Keys, k)
		}
	}
	t.nextKeyIdx = len(t.deltas)
}

// Don't call this for schema mutations. Directly commit them.
func (tx *Txn) CommitMutations(ctx context.Context, commitTs uint64) error {
	tx.Lock()
	defer tx.Unlock()

	txn := pstore.NewTransactionAt(commitTs, true)
	defer txn.Discard()
	// Sort by keys so that we have all postings for same pl side by side.
	sort.SliceStable(tx.deltas, func(i, j int) bool {
		return bytes.Compare(tx.deltas[i].key, tx.deltas[j].key) < 0
	})
	var prevKey []byte
	var pl *intern.PostingList
	var plist *List
	var err error
	i := 0
	for i < len(tx.deltas) {
		d := tx.deltas[i]
		if !bytes.Equal(prevKey, d.key) {
			plist, err = Get(d.key)
			if err != nil {
				return err
			}
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
			// Indicates that this is the full posting list.
			meta = BitEmptyPosting
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
		plist, err := Get(d.key)
		if err != nil {
			return err
		}
		err = plist.CommitMutation(ctx, tx.StartTs, commitTs)
		for err == ErrRetry {
			time.Sleep(5 * time.Millisecond)
			plist, err = Get(d.key)
			if err != nil {
				return err
			}
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
		plist, err := Get([]byte(d.key))
		if err != nil {
			return err
		}
		err = plist.AbortTransaction(ctx, tx.StartTs)
		for err == ErrRetry {
			time.Sleep(5 * time.Millisecond)
			plist, err = Get(d.key)
			if err != nil {
				return err
			}
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
//
// key would now be owned by the posting list. So, ensure that it isn't reused
// elsewhere.
func ReadPostingList(key []byte, it *badger.Iterator) (*List, error) {
	l := new(List)
	l.key = key
	l.mutationMap = make(map[uint64]*intern.PostingList)
	l.activeTxns = make(map[uint64]struct{})
	l.plist = new(intern.PostingList)

	// Iterates from highest Ts to lowest Ts
	for it.Valid() {
		item := it.Item()
		if item.IsDeletedOrExpired() {
			// Don't consider any more versions.
			break
		}
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
			// No need to do Next here. The outer loop can take care of skipping more versions of
			// the same key.
			break
		}
		if item.UserMeta()&bitDeltaPosting > 0 {
			pl := &intern.PostingList{}
			x.Check(pl.Unmarshal(val))
			pl.Commit = item.Version()
			for _, mpost := range pl.Postings {
				// commitTs, startTs are meant to be only in memory, not
				// stored on disk.
				mpost.CommitTs = item.Version()
			}
			l.mutationMap[pl.Commit] = pl
		} else {
			x.Fatalf("unexpected meta: %d", item.UserMeta())
		}
		if item.DiscardEarlierVersions() {
			break
		}
		it.Next()
	}
	return l, nil
}

func getNew(key []byte, pstore *badger.ManagedDB) (*List, error) {
	l := new(List)
	l.key = key
	l.mutationMap = make(map[uint64]*intern.PostingList)
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

	if err != nil {
		return l, err
	}

	l.onDisk = 1
	l.Lock()
	size := l.calculateSize()
	l.Unlock()
	x.BytesRead.Add(int64(size))
	atomic.StoreInt32(&l.estimatedSize, size)
	return l, nil
}

type bTreeIterator struct {
	keys    [][]byte
	idx     int
	reverse bool
	prefix  []byte
}

func (bi *bTreeIterator) Next() {
	bi.idx++
}

func (bi *bTreeIterator) Key() []byte {
	x.AssertTrue(bi.Valid())
	return bi.keys[bi.idx]
}

func (bi *bTreeIterator) Valid() bool {
	return bi.idx < len(bi.keys)
}

func (bi *bTreeIterator) Seek(key []byte) {
	cont := func(key []byte) bool {
		if !bytes.HasPrefix(key, bi.prefix) {
			return false
		}
		bi.keys = append(bi.keys, key)
		return true
	}
	if !bi.reverse {
		btree.AscendGreaterOrEqual(key, cont)
	} else {
		btree.DescendLessOrEqual(key, cont)
	}
}

type TxnPrefixIterator struct {
	btreeIter  *bTreeIterator
	badgerIter *badger.Iterator
	prefix     []byte
	reverse    bool
	curKey     []byte
	userMeta   byte // userMeta stored as part of badger item, used to skip empty PL in has query.
}

func NewTxnPrefixIterator(txn *badger.Txn,
	iterOpts badger.IteratorOptions, prefix, key []byte) *TxnPrefixIterator {
	x.AssertTrue(iterOpts.PrefetchValues == false)
	txnIt := new(TxnPrefixIterator)
	txnIt.reverse = iterOpts.Reverse
	txnIt.prefix = prefix
	txnIt.btreeIter = &bTreeIterator{
		reverse: iterOpts.Reverse,
		prefix:  prefix,
	}
	txnIt.btreeIter.Seek(key)
	// Create iterator only after copying the keys from btree, or else there could
	// be race after creating iterator and before reading btree. Some keys might end up
	// getting deleted and iterator won't be initialized with new memtbales.
	txnIt.badgerIter = txn.NewIterator(iterOpts)
	txnIt.badgerIter.Seek(key)
	txnIt.Next()
	return txnIt
}

func (t *TxnPrefixIterator) Valid() bool {
	return len(t.curKey) > 0
}

func (t *TxnPrefixIterator) compare(key1 []byte, key2 []byte) int {
	if !t.reverse {
		return bytes.Compare(key1, key2)
	}
	return bytes.Compare(key2, key1)
}

func (t *TxnPrefixIterator) Next() {
	if len(t.curKey) > 0 {
		// Avoid duplicate keys during merging.
		for t.btreeIter.Valid() && t.compare(t.btreeIter.Key(), t.curKey) <= 0 {
			t.btreeIter.Next()
		}
		for t.badgerIter.ValidForPrefix(t.prefix) &&
			t.compare(t.badgerIter.Item().Key(), t.curKey) <= 0 {
			t.badgerIter.Next()
		}
	}

	t.userMeta = 0 // reset it.
	if !t.btreeIter.Valid() && !t.badgerIter.ValidForPrefix(t.prefix) {
		t.curKey = nil
		return
	} else if !t.badgerIter.ValidForPrefix(t.prefix) {
		t.storeKey(t.btreeIter.Key())
		t.btreeIter.Next()
	} else if !t.btreeIter.Valid() {
		t.userMeta = t.badgerIter.Item().UserMeta()
		t.storeKey(t.badgerIter.Item().Key())
		t.badgerIter.Next()
	} else { // Both are valid
		if t.compare(t.btreeIter.Key(), t.badgerIter.Item().Key()) < 0 {
			t.storeKey(t.btreeIter.Key())
			t.btreeIter.Next()
		} else {
			t.userMeta = t.badgerIter.Item().UserMeta()
			t.storeKey(t.badgerIter.Item().Key())
			t.badgerIter.Next()
		}
	}
}

func (t *TxnPrefixIterator) UserMeta() byte {
	return t.userMeta
}

func (t *TxnPrefixIterator) storeKey(key []byte) {
	if cap(t.curKey) < len(key) {
		t.curKey = make([]byte, 2*len(key))
	}
	t.curKey = t.curKey[:len(key)]
	copy(t.curKey, key)
}

func (t *TxnPrefixIterator) Key() []byte {
	return t.curKey
}

func (t *TxnPrefixIterator) Close() {
	t.badgerIter.Close()
}
