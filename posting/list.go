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
	"fmt"
	"log"
	"math"
	"sort"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/net/trace"

	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/y"
)

var (
	// ErrRetry can be triggered if the posting list got deleted from memory due to a hard commit.
	// In such a case, retry.
	ErrRetry = fmt.Errorf("Temporary Error. Please retry.")
	// ErrNoValue would be returned if no value was found in the posting list.
	ErrNoValue     = fmt.Errorf("No value found")
	ErrInvalidTxn  = fmt.Errorf("Invalid transaction")
	errUncommitted = fmt.Errorf("Posting List has uncommitted data")
	emptyPosting   = &intern.Posting{}
	emptyList      = &intern.PostingList{}
)

const (
	// Set means overwrite in mutation layer. It contributes 0 in Length.
	Set uint32 = 0x01
	// Del means delete in mutation layer. It contributes -1 in Length.
	Del uint32 = 0x02

	// Metadata Bit which is stored to find out whether the stored value is pl or byte slice.
	BitUidPosting      byte = 0x01
	bitDeltaPosting    byte = 0x04
	BitCompletePosting byte = 0x08
)

type List struct {
	x.SafeMutex
	key           []byte
	plist         *intern.PostingList
	mlayer        []*intern.Posting // committed mutations, sorted by uid,ts
	minTs         uint64            // commit timestamp of immutable layer, reject reads before this ts.
	commitTs      uint64            // last commitTs of this pl
	activeTxns    map[uint64]struct{}
	deleteMe      int32 // Using atomic for this, to avoid expensive SetForDeletion operation.
	markdeleteAll uint64
	estimatedSize int32
	numCommits    int
	onDisk        int32 // Using atomic, Was written to disk atleast once.
}

// calculateSize would give you the size estimate. Does not consider elements in mutation layer.
// Expensive, so don't run it carefully.
func (l *List) calculateSize() int32 {
	sz := int(unsafe.Sizeof(l))
	sz += l.plist.Size()
	sz += cap(l.key)
	sz += cap(l.mlayer) * 8
	return int32(sz)
}

type PIterator struct {
	pl         *intern.PostingList
	uidPosting *intern.Posting
	pidx       int // index of postings
	plen       int
	valid      bool
	bi         bp128.BPackIterator
	uids       []uint64
	// Offset into the uids slice
	offset int
}

func (it *PIterator) Init(pl *intern.PostingList, afterUid uint64) {
	it.pl = pl
	it.uidPosting = &intern.Posting{}
	it.bi.Init(pl.Uids, afterUid)
	it.plen = len(pl.Postings)
	it.uids = it.bi.Uids()
	it.pidx = sort.Search(it.plen, func(idx int) bool {
		p := pl.Postings[idx]
		return afterUid < p.Uid
	})
	if it.bi.StartIdx() < it.bi.Length() {
		it.valid = true
	}
}

func (it *PIterator) Next() {
	it.offset++
	if it.offset < len(it.uids) {
		return
	}
	it.bi.Next()
	if !it.bi.Valid() {
		it.valid = false
		return
	}
	it.uids = it.bi.Uids()
	it.offset = 0
}

func (it *PIterator) Valid() bool {
	return it.valid
}

func (it *PIterator) Posting() *intern.Posting {
	uid := it.uids[it.offset]

	for it.pidx < it.plen {
		if it.pl.Postings[it.pidx].Uid > uid {
			break
		}
		if it.pl.Postings[it.pidx].Uid == uid {
			return it.pl.Postings[it.pidx]
		}
		it.pidx++
	}
	it.uidPosting.Uid = uid
	return it.uidPosting
}

// ListOptions is used in List.Uids (in posting) to customize our output list of
// UIDs, for each posting list. It should be intern.to this package.
type ListOptions struct {
	ReadTs    uint64
	AfterUID  uint64       // Any UID returned must be after this value.
	Intersect *intern.List // Intersect results with this list of UIDs.
}

// samePosting tells whether this is same posting depending upon operation of new posting.
// if operation is Del, we ignore facets and only care about uid and value.
// otherwise we match everything.
func samePosting(oldp *intern.Posting, newp *intern.Posting) bool {
	if oldp.Uid != newp.Uid {
		return false
	}
	if oldp.ValType != newp.ValType {
		return false
	}
	if !bytes.Equal(oldp.Value, newp.Value) {
		return false
	}
	if oldp.PostingType != newp.PostingType {
		return false
	}
	if bytes.Compare(oldp.LangTag, newp.LangTag) != 0 {
		return false
	}

	// Checking source might not be necessary.
	if oldp.Label != newp.Label {
		return false
	}
	if newp.Op == Del {
		return true
	}
	return facets.SameFacets(oldp.Facets, newp.Facets)
}

func NewPosting(t *intern.DirectedEdge) *intern.Posting {
	var op uint32
	if t.Op == intern.DirectedEdge_SET {
		op = Set
	} else if t.Op == intern.DirectedEdge_DEL {
		op = Del
	} else {
		x.Fatalf("Unhandled operation: %+v", t)
	}

	var postingType intern.Posting_PostingType
	if len(t.Lang) > 0 {
		postingType = intern.Posting_VALUE_LANG
	} else if t.ValueId == 0 {
		postingType = intern.Posting_VALUE
	} else {
		postingType = intern.Posting_REF
	}

	return &intern.Posting{
		Uid:         t.ValueId,
		Value:       t.Value,
		ValType:     intern.Posting_ValType(t.ValueType),
		PostingType: postingType,
		LangTag:     []byte(t.Lang),
		Label:       t.Label,
		Op:          op,
		Facets:      t.Facets,
	}
}

func (l *List) EstimatedSize() int32 {
	size := atomic.LoadInt32(&l.estimatedSize)
	if size < 0 {
		return 0
	}
	return size
}

// SetForDeletion will mark this List to be deleted, so no more mutations can be applied to this.
func (l *List) SetForDeletion() bool {
	l.Lock()
	defer l.Unlock()
	if len(l.activeTxns) > 0 {
		return false
	}
	atomic.StoreInt32(&l.deleteMe, 1)
	return true
}

// Ensure that you either abort the uncomitted postings or commit them before calling me.
func (l *List) updateMutationLayer(startTs uint64, mpost *intern.Posting) bool {
	l.AssertLock()
	x.AssertTrue(mpost.Op == Set || mpost.Op == Del)
	if mpost.Op == Del && bytes.Equal(mpost.Value, []byte(x.Star)) {
		l.markdeleteAll = startTs
		// Remove all mutations done in same transaction.
		midx := 0
		for _, mpost := range l.mlayer {
			if mpost.StartTs != startTs {
				l.mlayer[midx] = mpost
				midx++
			}
		}
		l.mlayer = l.mlayer[:midx]
		return true
	}

	// Check the mutable layer.
	midx := sort.Search(len(l.mlayer), func(idx int) bool {
		mp := l.mlayer[idx]
		if mpost.Uid != mp.Uid {
			return mpost.Uid < mp.Uid
		}
		return mpost.StartTs >= mp.StartTs
	})
	// Doesn't match what we already have in immutable layer. So, add to mutable layer.
	if midx >= len(l.mlayer) {
		// Add it at the end.
		l.mlayer = append(l.mlayer, mpost)
		return true
	}

	if l.mlayer[midx].Uid == mpost.Uid && l.mlayer[midx].StartTs == startTs {
		l.mlayer[midx] = mpost
		return true
	}
	// Otherwise, add it where midx is pointing to.
	l.mlayer = append(l.mlayer, nil)
	copy(l.mlayer[midx+1:], l.mlayer[midx:])
	l.mlayer[midx] = mpost
	return true
}

// AddMutation adds mutation to mutation layers. Note that it does not write
// anything to disk. Some other background routine will be responsible for merging
// changes in mutation layers to BadgerDB. Returns whether any mutation happens.
func (l *List) AddMutation(ctx context.Context, txn *Txn, t *intern.DirectedEdge) (bool, error) {
	t1 := time.Now()
	l.Lock()
	if dur := time.Since(t1); dur > time.Millisecond {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("acquired lock %v %v", dur, t.Attr)
		}
	}
	defer l.Unlock()
	return l.addMutation(ctx, txn, t)
}

// TypeID returns the typeid of destination vertex
func TypeID(edge *intern.DirectedEdge) types.TypeID {
	if edge.ValueId != 0 {
		return types.UidID
	}
	return types.TypeID(edge.ValueType)
}

func (l *List) addMutation(ctx context.Context, txn *Txn, t *intern.DirectedEdge) (bool, error) {
	if atomic.LoadInt32(&l.deleteMe) == 1 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("DELETEME set to true. Temporary error.")
		}
		return false, ErrRetry
	}

	if txn.ShouldAbort() {
		return false, y.ErrConflict
	}
	// We can have at max one pending <s> <p> * mutation.
	hasPendingDelete := (l.markdeleteAll != txn.StartTs) &&
		l.markdeleteAll > 0 && t.Op == intern.DirectedEdge_DEL &&
		bytes.Equal(t.Value, []byte(x.Star))
	doAbort := hasPendingDelete || txn.StartTs < l.commitTs
	ignoreConflict := false
	if t.Attr == "_predicate_" {
		doAbort = false
		ignoreConflict = true
	} else if txn.IgnoreIndexConflict && !x.Parse(l.key).IsData() {
		doAbort = false
		ignoreConflict = true
	}
	if doAbort {
		txn.SetAbort()
		return false, y.ErrConflict
	}

	mpost := NewPosting(t)

	if mpost.PostingType != intern.Posting_REF {
		// There could be a collision if the user gives us a value with Lang = "en" and later gives
		// us a value = "en" for the same predicate. We would end up overwritting his older lang
		// value.

		// Value with a lang type.
		if len(t.Lang) > 0 {
			t.ValueId = farm.Fingerprint64([]byte(t.Lang))
		} else if schema.State().IsList(t.Attr) {
			// TODO - When values are deleted for list type, then we should only delete the uid from
			// index if no other values produces that index token.
			// Value for list type.
			t.ValueId = farm.Fingerprint64(t.Value)
		} else {
			// All edges with a value without LANGTAG, have the same uid. In other words,
			// an (entity, attribute) can only have one untagged value.
			t.ValueId = math.MaxUint64
		}
	}

	mpost.Uid = t.ValueId
	mpost.StartTs = txn.StartTs
	t1 := time.Now()
	hasMutated := l.updateMutationLayer(txn.StartTs, mpost)
	atomic.AddInt32(&l.estimatedSize, int32(mpost.Size()+16 /* various overhead */))
	if dur := time.Since(t1); dur > time.Millisecond {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("updated mutation layer %v %v %v", dur, len(l.mlayer), len(l.plist.Uids))
		}
	}
	l.activeTxns[txn.StartTs] = struct{}{}
	txn.AddDelta(l.key, mpost, ignoreConflict)
	return hasMutated, nil
}

func (l *List) AbortTransaction(ctx context.Context, startTs uint64) error {
	l.Lock()
	defer l.Unlock()
	return l.abortTransaction(ctx, startTs)
}

func (l *List) abortTransaction(ctx context.Context, startTs uint64) error {
	if atomic.LoadInt32(&l.deleteMe) == 1 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("DELETEME set to true. Temporary error.")
		}
		return ErrRetry
	}
	l.AssertLock()
	midx := 0
	for _, mpost := range l.mlayer {
		if mpost.StartTs != startTs {
			l.mlayer[midx] = mpost
			midx++
		} else {
			atomic.AddInt32(&l.estimatedSize, -1*int32(mpost.Size()+16 /* various overhead */))
		}
	}
	l.mlayer = l.mlayer[:midx]
	delete(l.activeTxns, startTs)
	if l.markdeleteAll == startTs {
		// Reset it so that other transactions can perform S P * deletion.
		l.markdeleteAll = 0
	}
	return nil
}

func (l *List) AlreadyCommitted(startTs uint64) bool {
	l.RLock()
	defer l.RUnlock()
	_, ok := l.activeTxns[startTs]
	return !ok
}

func (l *List) CommitMutation(ctx context.Context, startTs, commitTs uint64) error {
	l.Lock()
	defer l.Unlock()
	return l.commitMutation(ctx, startTs, commitTs)
}

func (l *List) commitMutation(ctx context.Context, startTs, commitTs uint64) error {
	if atomic.LoadInt32(&l.deleteMe) == 1 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("DELETEME set to true. Temporary error.")
		}
		return ErrRetry
	}

	l.AssertLock()
	if _, ok := l.activeTxns[startTs]; !ok {
		// It was already committed, might be happening due to replay.
		return nil
	}
	if l.markdeleteAll > 0 {
		l.deleteHelper(ctx)
		l.minTs = commitTs
		l.markdeleteAll = 0
	} else {
		for _, mpost := range l.mlayer {
			if mpost.StartTs == startTs {
				atomic.StoreUint64(&mpost.CommitTs, commitTs)
				l.numCommits++
			}
		}
	}
	if commitTs > l.commitTs {
		l.commitTs = commitTs
	}
	delete(l.activeTxns, startTs)
	// Calculate 5% of immutable layer
	numUids := (bp128.NumIntegers(l.plist.Uids) * 5) / 100
	if numUids < 1000 {
		numUids = 1000
	}
	if l.numCommits > numUids {
		l.syncIfDirty(false)
	}
	return nil
}

func (l *List) deleteHelper(ctx context.Context) error {
	l.AssertLock()
	l.plist = emptyList
	midx := 0
	for _, mpost := range l.mlayer {
		if mpost.StartTs >= l.markdeleteAll {
			l.mlayer[midx] = mpost
			midx++
		}
	}
	l.mlayer = l.mlayer[:midx] // Clear the mutation layer.
	atomic.StoreInt32(&l.estimatedSize, l.calculateSize())
	return nil
}

// Iterate will allow you to iterate over this Posting List, while having acquired a read lock.
// So, please keep this iteration cheap, otherwise mutations would get stuck.
// The iteration will start after the provided UID. The results would not include this UID.
// The function will loop until either the Posting List is fully iterated, or you return a false
// in the provided function, which will indicate to the function to break out of the iteration.
//
// 	pl.Iterate(func(p *intern.Posting) bool {
//    // Use posting p
//    return true  // to continue iteration.
//    return false // to break iteration.
//  })
func (l *List) Iterate(readTs uint64, afterUid uint64, f func(obj *intern.Posting) bool) error {
	l.RLock()
	defer l.RUnlock()
	return l.iterate(readTs, afterUid, f)
}

func (l *List) Conflicts(readTs uint64) []uint64 {
	l.RLock()
	defer l.RUnlock()
	var conflicts []uint64
	for ts := range l.activeTxns {
		if ts < readTs {
			conflicts = append(conflicts, ts)
		}
	}
	return conflicts
}

func (l *List) inSnapshot(mpost *intern.Posting, readTs, deleteTs uint64) bool {
	l.AssertRLock()
	commitTs := atomic.LoadUint64(&mpost.CommitTs)
	if commitTs == 0 {
		commitTs = Oracle().CommitTs(mpost.StartTs)
		atomic.StoreUint64(&mpost.CommitTs, commitTs)
	}
	if commitTs == 0 {
		return mpost.StartTs == readTs
	}
	return commitTs <= readTs && commitTs >= deleteTs
}

func (l *List) iterate(readTs uint64, afterUid uint64, f func(obj *intern.Posting) bool) error {
	l.AssertRLock()
	midx := 0
	var deleteTs uint64
	if l.markdeleteAll == 0 {
	} else if l.markdeleteAll == readTs {
		// Check if there is uncommitted sp* at current readTs.
		deleteTs = readTs
	} else if l.markdeleteAll < readTs {
		// Ignore all reads before this.
		// Fixing the pl is difficult with locks.
		deleteTs = Oracle().CommitTs(l.markdeleteAll)
	}
	if readTs < l.minTs {
		return x.Errorf("readTs: %d less than minTs: %d for key: %q", readTs, l.minTs, l.key)
	}
	mlayerLen := len(l.mlayer)
	if afterUid > 0 {
		midx = sort.Search(mlayerLen, func(idx int) bool {
			mp := l.mlayer[idx]
			return afterUid < mp.Uid
		})
	}

	var mp, pp *intern.Posting
	cont := true
	var pitr PIterator
	pitr.Init(l.plist, afterUid)
	prevUid := uint64(0)
	for cont {
		if midx < mlayerLen {
			mp = l.mlayer[midx]
			if !l.inSnapshot(mp, readTs, deleteTs) {
				midx++
				continue
			}
		} else {
			mp = emptyPosting
		}
		if l.minTs > deleteTs && pitr.Valid() {
			pp = pitr.Posting()
			atomic.StoreUint64(&pp.CommitTs, l.minTs)
		} else {
			pp = emptyPosting
		}

		switch {
		case prevUid != 0 && mp.Uid == prevUid:
			midx++
		case pp.Uid == 0 && mp.Uid == 0:
			cont = false
		case mp.Uid == 0 || (pp.Uid > 0 && pp.Uid < mp.Uid):
			cont = f(pp)
			pitr.Next()
		case pp.Uid == 0 || (mp.Uid > 0 && mp.Uid < pp.Uid):
			if mp.Op != Del {
				cont = f(mp)
			}
			prevUid = mp.Uid
			midx++
		case pp.Uid == mp.Uid:
			if mp.Op != Del {
				cont = f(mp)
			}
			prevUid = mp.Uid
			pitr.Next()
			midx++
		default:
			log.Fatalf("Unhandled case during iteration of posting list.")
		}
	}
	return nil
}

func (l *List) CommitTs() uint64 {
	l.RLock()
	defer l.RUnlock()
	return l.commitTs
}

func (l *List) length(readTs, afterUid uint64) int {
	l.AssertRLock()
	count := 0
	err := l.iterate(readTs, afterUid, func(p *intern.Posting) bool {
		count++
		return true
	})
	if err != nil {
		return -1
	}
	return count
}

// Length iterates over the mutation layer and counts number of elements.
func (l *List) Length(readTs, afterUid uint64) int {
	l.RLock()
	defer l.RUnlock()
	return l.length(readTs, afterUid)
}

func doAsyncWrite(commitTs uint64, key []byte, data []byte, meta byte, f func(error)) {
	txn := pstore.NewTransactionAt(commitTs, true)
	defer txn.Discard()
	if err := txn.SetWithMeta(key, data, meta); err != nil {
		f(err)
	}
	if err := txn.CommitAt(commitTs, f); err != nil {
		f(err)
	}
}

func (l *List) SyncIfDirty(delFromCache bool) (committed bool, err error) {
	l.Lock()
	defer l.Unlock()
	return l.syncIfDirty(delFromCache)
}

func (l *List) MarshalToKv() (*intern.KV, error) {
	l.Lock()
	defer l.Unlock()
	x.AssertTrue(len(l.activeTxns) == 0)
	if err := l.rollup(); err != nil {
		return nil, err
	}

	kv := &intern.KV{}
	kv.Version = l.minTs
	kv.Key = l.key
	val, meta := marshalPostingList(l.plist)
	kv.UserMeta = []byte{meta}
	kv.Val = val
	return kv, nil
}

func marshalPostingList(plist *intern.PostingList) (data []byte, meta byte) {
	if len(plist.Uids) == 0 {
		data = nil
	} else if len(plist.Postings) > 0 {
		var err error
		data, err = plist.Marshal()
		x.Checkf(err, "Unable to marshal posting list")
	} else {
		data = plist.Uids
		meta = BitUidPosting
	}
	meta = meta | BitCompletePosting
	return
}

func (l *List) rollup() error {
	l.AssertLock()
	final := new(intern.PostingList)
	var bp bp128.BPackEncoder
	buf := make([]uint64, 0, bp128.BlockSize)

	// Pick all committed entries
	x.AssertTrue(l.minTs <= l.commitTs)
	err := l.iterate(l.commitTs, 0, func(p *intern.Posting) bool {
		commitTs := atomic.LoadUint64(&p.CommitTs)
		if commitTs == 0 || commitTs > l.commitTs {
			return true
		}
		buf = append(buf, p.Uid)
		if len(buf) == bp128.BlockSize {
			bp.PackAppend(buf)
			buf = buf[:0]
		}

		if p.Facets != nil || p.Value != nil || len(p.LangTag) != 0 || len(p.Label) != 0 {
			// I think it's okay to take the pointer from the iterator, because we have a lock
			// over List; which won't be released until final has been marshalled. Thus, the
			// underlying data wouldn't be changed.
			final.Postings = append(final.Postings, p)
		}
		return true
	})
	x.Check(err)
	if len(buf) > 0 {
		bp.PackAppend(buf)
	}
	sz := bp.Size()
	if sz > 0 {
		final.Uids = make([]byte, sz)
		// TODO: Add bytes method
		bp.WriteTo(final.Uids)
	}
	midx := 0
	for _, mpost := range l.mlayer {
		commitTs := atomic.LoadUint64(&mpost.CommitTs)
		if commitTs == 0 || commitTs > l.commitTs {
			l.mlayer[midx] = mpost
			midx++
		}
	}
	l.mlayer = l.mlayer[:midx]
	l.minTs = l.commitTs
	if sz > 0 {
		// Don't overwrite plist if nothing new is merged.
		// We set plist to emptyList when we do sp*
		l.plist = final
	}
	l.numCommits = 0
	return nil
}

// Merge mutation layer and immutable layer.
func (l *List) syncIfDirty(delFromCache bool) (committed bool, err error) {
	// emptyList is used to differentiate when we don't have any updates, v/s
	// when we have explicitly deleted everything.
	if len(l.mlayer) == 0 && l.plist != emptyList {
		return false, nil
	}
	if delFromCache {
		// Don't evict if there is pending transaction.
		x.AssertTrue(len(l.activeTxns) == 0)
	}

	lmlayer := len(l.mlayer)
	if err := l.rollup(); err != nil {
		return false, err
	}
	// Check if length of mlayer has changed after rollup, else skip writing to disk
	// minTs can remain same after rollup during schema mutations of large index, so
	// don't check minTs.
	if len(l.mlayer) == lmlayer && l.plist != emptyList { // Would be emptyList for s p *
		// There was no change in immutable layer.
		return false, nil
	}
	x.AssertTrue(l.minTs > 0)
	data, meta := marshalPostingList(l.plist)
	atomic.StoreInt32(&l.estimatedSize, l.calculateSize())

	for {
		pLen := atomic.LoadInt64(&x.MaxPlSz)
		if int64(len(data)) <= pLen {
			break
		}
		if atomic.CompareAndSwapInt64(&x.MaxPlSz, pLen, int64(len(data))) {
			x.MaxPlSize.Set(int64(len(data)))
			x.MaxPlLength.Set(int64(bp128.NumIntegers(l.plist.Uids)))
			break
		}
	}

	// Copy this over because minTs can change by the time callback returns.
	minTs := l.minTs
	retries := 0
	var f func(error)
	f = func(err error) {
		if err != nil {
			x.Printf("Got err in while doing async writes in SyncIfDirty: %+v", err)
			if retries > 5 {
				x.Fatalf("Max retries exceeded while doing async write for key: %s, err: %+v",
					l.key, err)
			}
			// Error from badger should be temporary, so we can retry.
			retries += 1
			doAsyncWrite(minTs, l.key, data, meta, f)
			return
		}
		if atomic.LoadInt32(&l.onDisk) == 0 {
			btree.Delete(l.key)
			atomic.StoreInt32(&l.onDisk, 1)
		}
		x.BytesWrite.Add(int64(len(data)))
		x.PostingWrites.Add(1)
		if delFromCache {
			x.AssertTrue(atomic.LoadInt32(&l.deleteMe) == 1)
			lcache.delete(l.key)
		}
	}

	doAsyncWrite(minTs, l.key, data, meta, f)
	return true, nil
}

// Copies the val if it's uid only posting, be careful
func UnmarshalOrCopy(val []byte, metadata byte, pl *intern.PostingList) {
	if metadata == BitUidPosting {
		buf := make([]byte, len(val))
		copy(buf, val)
		pl.Uids = buf
	} else if val != nil {
		x.Checkf(pl.Unmarshal(val), "Unable to Unmarshal PostingList from store")
	}
}

// Uids returns the UIDs given some query params.
// We have to apply the filtering before applying (offset, count).
// WARNING: Calling this function just to get Uids is expensive
func (l *List) Uids(opt ListOptions) (*intern.List, error) {
	// Pre-assign length to make it faster.
	l.RLock()
	// Use approximate length for initial capacity.
	res := make([]uint64, 0, len(l.mlayer)+bp128.NumIntegers(l.plist.Uids))
	out := &intern.List{}
	if len(l.mlayer) == 0 && opt.Intersect != nil {
		if opt.ReadTs < l.minTs {
			l.RUnlock()
			return out, ErrTsTooOld
		}
		algo.IntersectCompressedWith(l.plist.Uids, opt.AfterUID, opt.Intersect, out)
		l.RUnlock()
		return out, nil
	}

	err := l.iterate(opt.ReadTs, opt.AfterUID, func(p *intern.Posting) bool {
		if p.PostingType == intern.Posting_REF {
			res = append(res, p.Uid)
		}
		return true
	})
	l.RUnlock()
	if err != nil {
		return out, err
	}

	// Do The intersection here as it's optimized.
	out.Uids = res
	if opt.Intersect != nil {
		algo.IntersectWith(out, opt.Intersect, out)
	}
	return out, nil
}

// Postings calls postFn with the postings that are common with
// uids in the opt ListOptions.
func (l *List) Postings(opt ListOptions, postFn func(*intern.Posting) bool) error {
	l.RLock()
	defer l.RUnlock()

	return l.iterate(opt.ReadTs, opt.AfterUID, func(p *intern.Posting) bool {
		if p.PostingType != intern.Posting_REF {
			return true
		}
		return postFn(p)
	})
}

func (l *List) AllUntaggedValues(readTs uint64) ([]types.Val, error) {
	l.RLock()
	defer l.RUnlock()

	var vals []types.Val
	err := l.iterate(readTs, 0, func(p *intern.Posting) bool {
		if len(p.LangTag) == 0 {
			vals = append(vals, types.Val{
				Tid:   types.TypeID(p.ValType),
				Value: p.Value,
			})
		}
		return true
	})
	return vals, err
}

func (l *List) AllValues(readTs uint64) ([]types.Val, error) {
	l.RLock()
	defer l.RUnlock()

	var vals []types.Val
	err := l.iterate(readTs, 0, func(p *intern.Posting) bool {
		vals = append(vals, types.Val{
			Tid:   types.TypeID(p.ValType),
			Value: p.Value,
		})
		return true
	})
	return vals, err
}

// GetLangTags finds the language tags of each posting in the list.
func (l *List) GetLangTags(readTs uint64) ([]string, error) {
	l.RLock()
	defer l.RUnlock()

	var tags []string
	err := l.iterate(readTs, 0, func(p *intern.Posting) bool {
		tags = append(tags, string(p.LangTag))
		return true
	})
	return tags, err
}

// Returns Value from posting list.
// This function looks only for "default" value (one without language).
func (l *List) Value(readTs uint64) (rval types.Val, rerr error) {
	l.RLock()
	defer l.RUnlock()
	val, found, err := l.findValue(readTs, math.MaxUint64)
	if err != nil {
		return val, err
	}
	if !found {
		return val, ErrNoValue
	}
	return val, nil
}

// Returns Value from posting list, according to preferred language list (langs).
// If list is empty, value without language is returned; if such value is not available, value with
// smallest Uid is returned.
// If list consists of one or more languages, first available value is returned; if no language
// from list match the values, processing is the same as for empty list.
func (l *List) ValueFor(readTs uint64, langs []string) (rval types.Val, rerr error) {
	p, err := l.postingFor(readTs, langs)
	if err != nil {
		return rval, err
	}
	return valueToTypesVal(p), nil
}

func (l *List) postingFor(readTs uint64, langs []string) (p *intern.Posting, rerr error) {
	l.RLock()
	defer l.RUnlock()
	return l.postingForLangs(readTs, langs)
}

func (l *List) ValueForTag(readTs uint64, tag string) (rval types.Val, rerr error) {
	l.RLock()
	defer l.RUnlock()
	p, err := l.postingForTag(readTs, tag)
	if err != nil {
		return rval, err
	}
	return valueToTypesVal(p), nil
}

func valueToTypesVal(p *intern.Posting) (rval types.Val) {
	// This is ok because we dont modify the value of a Posting. We create a newPosting
	// and add it to the PostingList to do a set.
	rval.Value = p.Value
	rval.Tid = types.TypeID(p.ValType)
	return
}

func (l *List) postingForLangs(readTs uint64, langs []string) (pos *intern.Posting, rerr error) {
	l.AssertRLock()

	any := false
	// look for language in preferred order
	for _, lang := range langs {
		if lang == "." {
			any = true
			break
		}
		pos, rerr = l.postingForTag(readTs, lang)
		if rerr == nil {
			return pos, nil
		}
	}

	// look for value without language
	if any || len(langs) == 0 {
		if found, pos, err := l.findPosting(readTs, math.MaxUint64); err != nil {
			return nil, err
		} else if found {
			return pos, nil
		}
	}

	var found bool
	// last resort - return value with smallest lang Uid
	if any {
		err := l.iterate(readTs, 0, func(p *intern.Posting) bool {
			if p.PostingType == intern.Posting_VALUE_LANG {
				pos = p
				found = true
				return false
			}
			return true
		})
		if err != nil {
			return nil, err
		}
	}

	if found {
		return pos, nil
	}

	return pos, ErrNoValue
}

func (l *List) postingForTag(readTs uint64, tag string) (p *intern.Posting, rerr error) {
	l.AssertRLock()
	uid := farm.Fingerprint64([]byte(tag))
	found, p, err := l.findPosting(readTs, uid)
	if err != nil {
		return p, err
	}
	if !found {
		return p, ErrNoValue
	}

	return p, nil
}

func (l *List) findValue(readTs, uid uint64) (rval types.Val, found bool, err error) {
	l.AssertRLock()
	found, p, err := l.findPosting(readTs, uid)
	if !found {
		return rval, found, err
	}

	return valueToTypesVal(p), true, nil
}

func (l *List) findPosting(readTs uint64, uid uint64) (found bool, pos *intern.Posting, err error) {
	// Iterate starts iterating after the given argument, so we pass uid - 1
	err = l.iterate(readTs, uid-1, func(p *intern.Posting) bool {
		if p.Uid == uid {
			pos = p
			found = true
		}
		return false
	})

	return found, pos, err
}

// Facets gives facets for the posting representing value.
func (l *List) Facets(readTs uint64, param *intern.FacetParams, langs []string) (fs []*api.Facet,
	ferr error) {
	l.RLock()
	defer l.RUnlock()
	p, err := l.postingFor(readTs, langs)
	if err != nil {
		return nil, err
	}
	return facets.CopyFacets(p.Facets, param), nil
}
