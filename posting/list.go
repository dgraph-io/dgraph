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
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

var (
	// ErrRetry can be triggered if the posting list got deleted from memory due to a hard commit.
	// In such a case, retry.
	ErrRetry = fmt.Errorf("Temporary Error. Please retry.")
	// ErrNoValue would be returned if no value was found in the posting list.
	ErrNoValue     = fmt.Errorf("No value found")
	ErrInvalidTxn  = fmt.Errorf("Invalid transaction")
	errUncommitted = fmt.Errorf("Posting List has uncommitted data")
	emptyPosting   = &protos.Posting{}
	emptyList      = &protos.PostingList{}
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
	plist         *protos.PostingList
	mlayer        []*protos.Posting // committed mutations, sorted by uid,ts
	minTs         uint64            // commit timestamp of immutable layer, reject reads before this ts.
	commitTs      uint64            // commit timestamp of mutable layer.
	startTs       uint64            // start timestamp of ongoing transaction.
	primaryAttr   string
	deleteMe      int32 // Using atomic for this, to avoid expensive SetForDeletion operation.
	markdeleteAll int32
	deleteAll     int32
	estimatedSize uint32
}

// calculateSize would give you the size estimate. Does not consider elements in mutation layer.
// Expensive, so don't run it carefully.
func (l *List) calculateSize() uint32 {
	sz := int(unsafe.Sizeof(l))
	sz += l.plist.Size()
	sz += cap(l.key)
	sz += cap(l.mlayer) * 8
	return uint32(sz)
}

type PIterator struct {
	pl         *protos.PostingList
	uidPosting *protos.Posting
	pidx       int // index of postings
	plen       int
	valid      bool
	bi         bp128.BPackIterator
	uids       []uint64
	// Offset into the uids slice
	offset int
}

func (it *PIterator) Init(pl *protos.PostingList, afterUid uint64) {
	it.pl = pl
	it.uidPosting = &protos.Posting{}
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

func (it *PIterator) Posting() *protos.Posting {
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
// UIDs, for each posting list. It should be internal to this package.
type ListOptions struct {
	ReadTs    uint64
	AfterUID  uint64       // Any UID returned must be after this value.
	Intersect *protos.List // Intersect results with this list of UIDs.
}

// samePosting tells whether this is same posting depending upon operation of new posting.
// if operation is Del, we ignore facets and only care about uid and value.
// otherwise we match everything.
func samePosting(oldp *protos.Posting, newp *protos.Posting) bool {
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
	if bytes.Compare(oldp.Metadata, newp.Metadata) != 0 {
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

func NewPosting(t *protos.DirectedEdge) *protos.Posting {
	x.AssertTruef(edgeType(t) != x.ValueEmpty,
		"This should have been set by the caller.")

	var op uint32
	if t.Op == protos.DirectedEdge_SET {
		op = Set
	} else if t.Op == protos.DirectedEdge_DEL {
		op = Del
	} else {
		x.Fatalf("Unhandled operation: %+v", t)
	}

	var postingType protos.Posting_PostingType
	var metadata []byte
	if len(t.Lang) > 0 {
		postingType = protos.Posting_VALUE_LANG
		metadata = []byte(t.Lang)
	} else if len(t.Value) == 0 {
		postingType = protos.Posting_REF
	} else if len(t.Value) > 0 {
		postingType = protos.Posting_VALUE
	}

	return &protos.Posting{
		Uid:         t.ValueId,
		Value:       t.Value,
		ValType:     protos.Posting_ValType(t.ValueType),
		PostingType: postingType,
		Metadata:    metadata,
		Label:       t.Label,
		Op:          op,
		Facets:      t.Facets,
		Commit:      math.MaxUint64,
	}
}

func (l *List) EstimatedSize() uint32 {
	return atomic.LoadUint32(&l.estimatedSize)
}

// SetForDeletion will mark this List to be deleted, so no more mutations can be applied to this.
func (l *List) SetForDeletion() bool {
	l.Lock()
	defer l.Unlock()
	if l.startTs > 0 {
		return false
	}
	atomic.StoreInt32(&l.deleteMe, 1)
	return true
}

// Ensure that you either abort the uncomitted postings or commit them before calling me.
func (l *List) updateMutationLayer(startTs uint64, mpost *protos.Posting) bool {
	l.AssertLock()
	x.AssertTrue(mpost.Op == Set || mpost.Op == Del)
	// Conflict check should happend before calling this function.
	x.AssertTrue(l.startTs == 0 || l.startTs == startTs)
	if mpost.Op == Del && bytes.Equal(mpost.Value, []byte(x.Star)) {
		atomic.StoreInt32(&l.markdeleteAll, 1)
		return true
	}
	atomic.StoreInt32(&l.markdeleteAll, 0)

	// First check the mutable layer.
	midx := sort.Search(len(l.mlayer), func(idx int) bool {
		mp := l.mlayer[idx]
		return mpost.Uid <= mp.Uid
	})

	// This block handles the case where mpost.UID is found in mutation layer.
	if midx < len(l.mlayer) && l.mlayer[midx].Uid == mpost.Uid {
		// mp is the posting found in mlayer.
		oldPost := l.mlayer[midx]

		// Note that mpost.Op is either Set or Del, whereas oldPost.Op can be
		// either Set or Del or Add.
		msame := samePosting(oldPost, mpost)
		if msame && ((mpost.Op == Del) == (oldPost.Op == Del)) {
			// This posting has similar content as what is found in mlayer. If the
			// ops are similar, then we do nothing. Note that Add and Set are
			// considered similar, and the second clause is true also when
			// mpost.Op==Add and oldPost.Op==Set.
			return false
		}

		if !msame && mpost.Op == Del {
			// Invalid Del as contents do not match.
			return false
		}

		// add it where midx is pointing to.
		l.mlayer = append(l.mlayer, nil)
		copy(l.mlayer[midx+1:], l.mlayer[midx:])
		l.mlayer[midx] = mpost
		return true
	}

	// Didn't find it in mutable layer. Now check the immutable layer.
	var psame bool
	var pitr PIterator
	pitr.Init(l.plist, mpost.Uid-1)
	if pitr.Valid() {
		pp := pitr.Posting()
		psame = samePosting(pp, mpost)
	}

	if mpost.Op == Set {
		if psame {
			return false
		}
	} else if !psame { // mpost.Op==Del
		// Either we fail to find UID in immutable PL or contents don't match.
		return false
	}

	// Doesn't match what we already have in immutable layer. So, add to mutable layer.
	if midx >= len(l.mlayer) {
		// Add it at the end.
		l.mlayer = append(l.mlayer, mpost)
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
// changes in mutation layers to RocksDB. Returns whether any mutation happens.
func (l *List) AddMutation(ctx context.Context, txn *Txn, t *protos.DirectedEdge) (bool, error) {
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

func edgeType(t *protos.DirectedEdge) x.ValueTypeInfo {
	hasVal := !bytes.Equal(t.Value, nil)
	hasId := t.ValueId != 0
	switch {
	case hasVal && hasId:
		return x.ValueMulti
	case hasVal && !hasId:
		return x.ValuePlain
	case !hasVal && hasId:
		return x.ValueUid
	default:
		return x.ValueEmpty
	}
}

func postingType(p *protos.Posting) x.ValueTypeInfo {
	switch p.PostingType {
	case protos.Posting_REF:
		return x.ValueUid
	case protos.Posting_VALUE:
		return x.ValuePlain
	case protos.Posting_VALUE_LANG:
		return x.ValueMulti
	default:
		return x.ValueEmpty
	}
}

// TypeID returns the typeid of destination vertex
func TypeID(edge *protos.DirectedEdge) types.TypeID {
	if edge.ValueId != 0 {
		return types.UidID
	}
	return types.TypeID(edge.ValueType)
}

func (l *List) addMutation(ctx context.Context, txn *Txn, t *protos.DirectedEdge) (bool, error) {
	if atomic.LoadInt32(&l.deleteMe) == 1 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("DELETEME set to true. Temporary error.")
		}
		return false, ErrRetry
	}
	if txn.HasConflict() {
		return false, ErrConflict
	}

	if !l.canPreWrite(ctx, txn) {
		pk := x.Parse(l.key)
		fmt.Printf("Can't prewrite due to %+v. txnstart=%d\n", pk, txn.StartTs)
		txn.AddConflict(&protos.TxnContext{StartTs: l.startTs, Primary: l.primaryAttr})
		return false, ErrConflict
	}

	// All edges with a value without LANGTAG, have the same uid. In other words,
	// an (entity, attribute) can only have one untagged value.
	if !bytes.Equal(t.Value, nil) {
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
			// Plain value for non-list type and without a language.
			t.ValueId = math.MaxUint64
		}
	}
	if t.ValueId == 0 {
		err := x.Errorf("ValueId cannot be zero for edge: %+v", t)
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return false, err
	}

	mpost := NewPosting(t)
	mpost.Commit = txn.StartTs
	t1 := time.Now()
	hasMutated := l.updateMutationLayer(txn.StartTs, mpost)
	atomic.AddUint32(&l.estimatedSize, uint32(mpost.Size()+16 /* various overhead */))
	if dur := time.Since(t1); dur > time.Millisecond {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("updated mutation layer %v %v %v", dur, len(l.mlayer), len(l.plist.Uids))
		}
	}
	if hasMutated {
		txn.AddDelta(l.key, mpost)
	}
	return hasMutated, nil
}

func (l *List) canPreWrite(ctx context.Context, txn *Txn) bool {
	if txn.StartTs < l.commitTs {
		return false
	}
	if l.startTs == 0 || l.startTs == txn.StartTs {
		l.startTs = txn.StartTs
		l.primaryAttr = txn.PrimaryAttr
		return true
	}
	return false
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
	if l.startTs == 0 {
		return nil
	}
	if l.startTs != startTs {
		return ErrInvalidTxn
	}
	midx := 0
	for _, mpost := range l.mlayer {
		if mpost.Commit != l.startTs {
			l.mlayer[midx] = mpost
			midx++
		}
		// TODO: Estimate size
	}
	l.mlayer = l.mlayer[:midx]
	l.startTs = 0
	l.primaryAttr = ""
	return nil
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
	if l.startTs == 0 && l.commitTs == commitTs {
		return nil
	}
	if l.startTs != startTs {
		return ErrInvalidTxn
	}

	if atomic.LoadInt32(&l.markdeleteAll) == 1 {
		l.deleteHelper(ctx)
		l.startTs = 0
		l.primaryAttr = ""
		l.commitTs = commitTs
		l.minTs = commitTs
		atomic.StoreInt32(&l.markdeleteAll, 0)
		return nil
	}
	for _, mpost := range l.mlayer {
		if mpost.Commit != l.startTs {
			continue
		}
		mpost.Commit = commitTs
	}
	l.startTs = 0
	l.primaryAttr = ""
	l.commitTs = commitTs
	// Calculate 5% of immutable layer
	numUids := (bp128.NumIntegers(l.plist.Uids) * 5) / 100
	if numUids < 1000 {
		numUids = 1000
	}
	if len(l.mlayer) > numUids {
		l.syncIfDirty(false)
	}
	return nil
}

func (l *List) deleteHelper(ctx context.Context) error {
	l.AssertLock()
	l.plist = emptyList
	l.mlayer = l.mlayer[:0] // Clear the mutation layer.
	atomic.StoreInt32(&l.deleteAll, 1)
	atomic.StoreUint32(&l.estimatedSize, l.calculateSize())
	return nil
}

// Iterate will allow you to iterate over this Posting List, while having acquired a read lock.
// So, please keep this iteration cheap, otherwise mutations would get stuck.
// The iteration will start after the provided UID. The results would not include this UID.
// The function will loop until either the Posting List is fully iterated, or you return a false
// in the provided function, which will indicate to the function to break out of the iteration.
//
// 	pl.Iterate(func(p *protos.Posting) bool {
//    // Use posting p
//    return true  // to continue iteration.
//    return false // to break iteration.
//  })
func (l *List) Iterate(readTs uint64, afterUid uint64, f func(obj *protos.Posting) bool) error {
	l.RLock()
	defer l.RUnlock()
	return l.iterate(readTs, afterUid, f)
}

// canRead must hold a read lock.
func (l *List) canRead(commitTs, readTs uint64) bool {
	if l.startTs > 0 {
		// Pending transaction
		return commitTs == readTs
	}

	// No pending transaction
	return commitTs <= readTs
}

func (l *List) HasConflict(readTs uint64) bool {
	l.RLock()
	defer l.RUnlock()
	return l.startTs != 0 && readTs > l.startTs
}

func (l *List) Pending() (uint64, string) {
	l.RLock()
	defer l.RUnlock()
	return l.startTs, l.primaryAttr
}

func (l *List) iterate(readTs uint64, afterUid uint64, f func(obj *protos.Posting) bool) error {
	l.AssertRLock()
	if l.startTs != 0 && readTs > l.startTs {
		return ErrConflict
	}
	// current pending transaction deletes the pl
	if readTs == l.startTs && atomic.LoadInt32(&l.markdeleteAll) == 1 {
		return nil
	}
	midx := 0

	if readTs < l.minTs {
		return ErrTsTooOld
	}
	mlayerLen := len(l.mlayer)
	if afterUid > 0 {
		midx = sort.Search(mlayerLen, func(idx int) bool {
			mp := l.mlayer[idx]
			return afterUid < mp.Uid
		})
	}

	var mp, pp *protos.Posting
	cont := true
	var pitr PIterator
	pitr.Init(l.plist, afterUid)
	prevUid := uint64(0)
	for cont {
		if pitr.Valid() {
			pp = pitr.Posting()
			pp.Commit = l.minTs
		} else {
			pp = emptyPosting
		}

		if midx < mlayerLen {
			mp = l.mlayer[midx]
			if !l.canRead(mp.Commit, readTs) {
				midx++
				continue
			}
		} else {
			mp = emptyPosting
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

func (l *List) length(readTs, afterUid uint64) int {
	l.AssertRLock()
	count := 0
	err := l.iterate(readTs, afterUid, func(p *protos.Posting) bool {
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
	meta = meta | BitCompletePosting
	if err := txn.Set(key, data, meta); err != nil {
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

func (l *List) MarshalToKv() (*protos.KV, error) {
	l.Lock()
	defer l.Unlock()
	final, err := l.rollup()
	if err != nil {
		return nil, err
	}

	kv := &protos.KV{}
	kv.Version = l.commitTs
	kv.Key = l.key
	val, meta := marshalPostingList(final)
	kv.UserMeta = []byte{meta}
	kv.Val = val
	return kv, nil
}

func marshalPostingList(plist *protos.PostingList) (data []byte, meta byte) {
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
	return
}

func (l *List) rollup() (*protos.PostingList, error) {
	l.AssertLock()
	x.AssertTrue(l.startTs == 0)
	final := new(protos.PostingList)
	var bp bp128.BPackEncoder
	buf := make([]uint64, 0, bp128.BlockSize)

	err := l.iterate(math.MaxUint64, 0, func(p *protos.Posting) bool {
		if p.Commit > l.commitTs {
			l.commitTs = p.Commit
		}
		buf = append(buf, p.Uid)
		if len(buf) == bp128.BlockSize {
			bp.PackAppend(buf)
			buf = buf[:0]
		}

		if p.Facets != nil || p.Value != nil || len(p.Metadata) != 0 || len(p.Label) != 0 {
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
	return final, nil
}

// Merge mutation layer and immutable layer.
func (l *List) syncIfDirty(delFromCache bool) (committed bool, err error) {
	// deleteAll is used to differentiate when we don't have any updates, v/s
	// when we have explicitly deleted everything.
	if len(l.mlayer) == 0 && atomic.LoadInt32(&l.deleteAll) == 0 {
		return false, nil
	}
	if l.startTs > 0 {
		// Don't merge if there is pending transaction.
		return false, errUncommitted
	}

	final, err := l.rollup()
	if err != nil {
		return false, err
	}
	data, meta := marshalPostingList(final)
	l.plist = final
	atomic.StoreUint32(&l.estimatedSize, l.calculateSize())

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
			doAsyncWrite(l.commitTs, l.key, data, meta, f)
			return
		}
		x.BytesWrite.Add(int64(len(data)))
		x.PostingWrites.Add(1)
		if delFromCache {
			x.AssertTrue(atomic.LoadInt32(&l.deleteMe) == 1)
			lcache.delete(l.key)
		}
		pstore.PurgeVersionsBelow(l.key, l.commitTs)
	}

	doAsyncWrite(l.commitTs, l.key, data, meta, f)
	l.mlayer = l.mlayer[:0]
	l.minTs = l.commitTs
	if l.startTs == 0 {
		// We don't merge uncomitted entries.
		atomic.StoreInt32(&l.deleteAll, 0) // Unset deleteAll only after it's committed.
	}
	return true, nil
}

// Copies the val if it's uid only posting, be careful
func UnmarshalOrCopy(val []byte, metadata byte, pl *protos.PostingList) {
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
func (l *List) Uids(opt ListOptions) (*protos.List, error) {
	// Pre-assign length to make it faster.
	l.RLock()
	// Use approximate length for initial capacity.
	res := make([]uint64, 0, len(l.mlayer)+bp128.NumIntegers(l.plist.Uids))
	out := &protos.List{}
	if len(l.mlayer) == 0 && opt.Intersect != nil {
		if opt.ReadTs < l.minTs {
			return out, ErrTsTooOld
		}
		algo.IntersectCompressedWith(l.plist.Uids, opt.AfterUID, opt.Intersect, out)
		l.RUnlock()
		return out, nil
	}

	err := l.iterate(opt.ReadTs, opt.AfterUID, func(p *protos.Posting) bool {
		if postingType(p) == x.ValueUid {
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
func (l *List) Postings(opt ListOptions, postFn func(*protos.Posting) bool) error {
	l.RLock()
	defer l.RUnlock()

	return l.iterate(opt.ReadTs, opt.AfterUID, func(p *protos.Posting) bool {
		if postingType(p) != x.ValueUid {
			return true
		}
		return postFn(p)
	})
}

func (l *List) AllValues(readTs uint64) (vals []types.Val, rerr error) {
	l.RLock()
	defer l.RUnlock()

	rerr = l.iterate(readTs, 0, func(p *protos.Posting) bool {
		vals = append(vals, types.Val{
			Tid:   types.TypeID(p.ValType),
			Value: p.Value,
		})
		return true
	})
	return
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

func (l *List) postingFor(readTs uint64, langs []string) (p *protos.Posting, rerr error) {
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

func valueToTypesVal(p *protos.Posting) (rval types.Val) {
	// This is ok because we dont modify the value of a Posting. We create a newPosting
	// and add it to the PostingList to do a set.
	rval.Value = p.Value
	rval.Tid = types.TypeID(p.ValType)
	return
}

func (l *List) postingForLangs(readTs uint64, langs []string) (pos *protos.Posting, rerr error) {
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
		err := l.iterate(readTs, 0, func(p *protos.Posting) bool {
			if postingType(p) == x.ValueMulti {
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

func (l *List) postingForTag(readTs uint64, tag string) (p *protos.Posting, rerr error) {
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

func (l *List) findPosting(readTs uint64, uid uint64) (found bool, pos *protos.Posting, err error) {
	// Iterate starts iterating after the given argument, so we pass uid - 1
	err = l.iterate(readTs, uid-1, func(p *protos.Posting) bool {
		if p.Uid == uid {
			pos = p
			found = true
		}
		return false
	})

	return found, pos, err
}

// Facets gives facets for the posting representing value.
func (l *List) Facets(readTs uint64, param *protos.Param, langs []string) (fs []*protos.Facet,
	ferr error) {
	l.RLock()
	defer l.RUnlock()
	p, err := l.postingFor(readTs, langs)
	if err != nil {
		return nil, err
	}
	return facets.CopyFacets(p.Facets, param), nil
}
