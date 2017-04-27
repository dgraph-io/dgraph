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
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos/facetsp"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

var (
	// ErrRetry can be triggered if the posting list got deleted from memory due to a hard commit.
	// In such a case, retry.
	ErrRetry = fmt.Errorf("Temporary Error. Please retry.")
	// ErrNoValue would be returned if no value was found in the posting list.
	ErrNoValue   = fmt.Errorf("No value found")
	emptyPosting = &typesp.Posting{}
	emptyList    = &typesp.PostingList{}
)

const (
	// Set means overwrite in mutation layer. It contributes 0 in Length.
	Set uint32 = 0x01
	// Del means delete in mutation layer. It contributes -1 in Length.
	Del uint32 = 0x02
	// Add means add new element in mutation layer. It contributes 1 in Length.
	Add uint32 = 0x03
)

type List struct {
	x.SafeMutex
	index       x.SafeMutex
	key         []byte
	ghash       uint64
	pbuffer     unsafe.Pointer
	mlayer      []*typesp.Posting // mutations
	pstore      *store.Store      // postinglist store
	lastCompact time.Time
	deleteMe    int32 // Using atomic for this, to avoid expensive SetForDeletion operation.
	refcount    int32
	deleteAll   int32

	water   *x.WaterMark
	pending []uint64
}

func (l *List) refCount() int32 { return atomic.LoadInt32(&l.refcount) }
func (l *List) incr() int32     { return atomic.AddInt32(&l.refcount, 1) }
func (l *List) decr() {
	val := atomic.AddInt32(&l.refcount, -1)
	x.AssertTruef(val >= 0, "List reference should never be less than zero: %v", val)
	if val > 0 {
		return
	}
	listPool.Put(l)
}

var listPool = sync.Pool{
	New: func() interface{} {
		return &List{}
	},
}

func getNew(key []byte, pstore *store.Store) *List {
	l := listPool.Get().(*List)
	*l = List{}
	l.key = key
	l.pstore = pstore
	l.ghash = farm.Fingerprint64(key)
	l.refcount = 1
	return l
}

// ListOptions is used in List.Uids (in posting) to customize our output list of
// UIDs, for each posting list. It should be internal to this package.
type ListOptions struct {
	AfterUID  uint64      // Any UID returned must be after this value.
	Intersect *taskp.List // Intersect results with this list of UIDs.
}

type ByUid []*typesp.Posting

func (pa ByUid) Len() int           { return len(pa) }
func (pa ByUid) Swap(i, j int)      { pa[i], pa[j] = pa[j], pa[i] }
func (pa ByUid) Less(i, j int) bool { return pa[i].Uid < pa[j].Uid }

// samePosting tells whether this is same posting depending upon operation of new posting.
// if operation is Del, we ignore facets and only care about uid and value.
// otherwise we match everything.
func samePosting(oldp *typesp.Posting, newp *typesp.Posting) bool {
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

func newPosting(t *taskp.DirectedEdge) *typesp.Posting {
	x.AssertTruef(edgeType(t) != x.ValueEmpty,
		"This should have been set by the caller.")

	var op uint32
	if t.Op == taskp.DirectedEdge_SET {
		op = Set
	} else if t.Op == taskp.DirectedEdge_DEL {
		op = Del
	} else {
		x.Fatalf("Unhandled operation: %+v", t)
	}

	var postingType typesp.Posting_PostingType
	var metadata []byte
	if len(t.Lang) > 0 {
		postingType = typesp.Posting_VALUE_LANG
		metadata = []byte(t.Lang)
	} else if len(t.Value) == 0 {
		postingType = typesp.Posting_REF
	} else if len(t.Value) > 0 {
		postingType = typesp.Posting_VALUE
	}

	return &typesp.Posting{
		Uid:         t.ValueId,
		Value:       t.Value,
		ValType:     typesp.Posting_ValType(t.ValueType),
		PostingType: postingType,
		Metadata:    metadata,
		Label:       t.Label,
		Op:          op,
		Facets:      t.Facets,
	}
}

func (l *List) WaitForCommit() {
	l.RLock()
	defer l.RUnlock()
	l.Wait()
}

func (l *List) PostingList() *typesp.PostingList {
	l.RLock()
	defer l.RUnlock()
	return l.getPostingList(0)
}

// getPostingList tries to get posting list from l.pbuffer. If it is nil, then
// we query RocksDB. There is no need for lock acquisition here.
func (l *List) getPostingList(loop int) *typesp.PostingList {
	if loop >= 10 {
		x.Fatalf("This is over the 10th loop: %v", loop)
	}
	l.AssertRLock()
	// Wait for any previous commits to happen before retrieving posting list again.
	l.Wait()

	pb := atomic.LoadPointer(&l.pbuffer)
	plist := (*typesp.PostingList)(pb)

	if plist == nil {
		x.AssertTrue(l.pstore != nil)
		plist = new(typesp.PostingList)

		if slice, err := l.pstore.Get(l.key); err == nil && slice != nil {
			x.Checkf(plist.Unmarshal(slice.Data()), "Unable to Unmarshal PostingList from store")
			slice.Free()
		}
		if atomic.CompareAndSwapPointer(&l.pbuffer, pb, unsafe.Pointer(plist)) {
			return plist
		}
		// Someone else replaced the pointer in the meantime. Retry recursively.
		return l.getPostingList(loop + 1)
	}
	return plist
}

// SetForDeletion will mark this List to be deleted, so no more mutations can be applied to this.
func (l *List) SetForDeletion() {
	atomic.StoreInt32(&l.deleteMe, 1)
}

func (l *List) updateMutationLayer(mpost *typesp.Posting) bool {
	l.AssertLock()
	x.AssertTrue(mpost.Op == Set || mpost.Op == Del)

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

		// Here are the remaining cases.
		// Del, Set: Replace with new post.
		// Del, Del: Replace with new post.
		// Set, Del: Replace with new post.
		// Set, Set: Replace with new post.
		// Add, Del: Undo by removing oldPost.
		// Add, Set: Replace with new post. Need to set mpost.Op to Add.
		if oldPost.Op == Add {
			if mpost.Op == Del {
				// Undo old post.
				copy(l.mlayer[midx:], l.mlayer[midx+1:])
				l.mlayer[len(l.mlayer)-1] = nil
				l.mlayer = l.mlayer[:len(l.mlayer)-1]
				return true
			}
			// Add followed by Set is considered an Add. Hence, mutate mpost.Op.
			mpost.Op = Add
		}
		l.mlayer[midx] = mpost
		return true
	}

	// Didn't find it in mutable layer. Now check the immutable layer.
	pl := l.getPostingList(0)
	pidx := sort.Search(len(pl.Postings), func(idx int) bool {
		p := pl.Postings[idx]
		return mpost.Uid <= p.Uid
	})

	var uidFound, psame bool
	if pidx < len(pl.Postings) {
		p := pl.Postings[pidx]
		uidFound = mpost.Uid == p.Uid
		if uidFound {
			psame = samePosting(p, mpost)
		}
	}

	if mpost.Op == Set {
		if psame {
			return false
		}
		if !uidFound {
			// Posting not found in PL. This is considered an Add operation.
			mpost.Op = Add
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
func (l *List) AddMutation(ctx context.Context, t *taskp.DirectedEdge) (bool, error) {
	l.Lock()
	defer l.Unlock()
	return l.addMutation(ctx, t)
}

func edgeType(t *taskp.DirectedEdge) x.ValueTypeInfo {
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

func postingType(p *typesp.Posting) x.ValueTypeInfo {
	switch p.PostingType {
	case typesp.Posting_REF:
		return x.ValueUid
	case typesp.Posting_VALUE:
		return x.ValuePlain
	case typesp.Posting_VALUE_LANG:
		return x.ValueMulti
	default:
		return x.ValueEmpty
	}
}

// TypeID returns the typeid of destination vertex
func TypeID(edge *taskp.DirectedEdge) types.TypeID {
	if edge.ValueId != 0 {
		return types.UidID
	}
	return types.TypeID(edge.ValueType)
}

func (l *List) addMutation(ctx context.Context, t *taskp.DirectedEdge) (bool, error) {
	if atomic.LoadInt32(&l.deleteMe) == 1 {
		x.TraceError(ctx, x.Errorf("DELETEME set to true. Temporary error."))
		return false, ErrRetry
	}

	l.AssertLock()
	// All edges with a value without LANGTAG, have the same uid. In other words,
	// an (entity, attribute) can only have one untagged value.
	if !bytes.Equal(t.Value, nil) {
		if len(t.Lang) > 0 {
			t.ValueId = farm.Fingerprint64([]byte(t.Lang))
		} else {
			t.ValueId = math.MaxUint64
		}

		if t.Attr == "_predicate_" {
			t.ValueId = farm.Fingerprint64(t.Value)
		}
	}
	if t.ValueId == 0 {
		err := x.Errorf("ValueId cannot be zero")
		x.TraceError(ctx, err)
		return false, err
	}
	mpost := newPosting(t)

	// Mutation arrives:
	// - Check if we had any(SET/DEL) before this, stored in the mutation list.
	//		- If yes, then replace that mutation. Jump to a)
	// a)		check if the entity exists in main posting list.
	// 				- If yes, store the mutation.
	// 				- If no, disregard this mutation.

	hasMutated := l.updateMutationLayer(mpost)
	if hasMutated {
		var gid uint32
		if rv, ok := ctx.Value("raft").(x.RaftValue); ok {
			l.water.Ch <- x.Mark{Index: rv.Index}
			l.pending = append(l.pending, rv.Index)
			gid = rv.Group
		}
		// if mutation doesn't come via raft
		if gid == 0 {
			gid = group.BelongsTo(t.Attr)
		}
		if dirtyChan != nil {
			dirtyChan <- fingerPrint{fp: l.ghash, gid: gid}
		}
	}
	return hasMutated, nil
}

func (l *List) delete(ctx context.Context, t *taskp.DirectedEdge) error {
	l.AssertLock()

	atomic.StorePointer(&l.pbuffer, unsafe.Pointer(emptyList)) // Make this an empty list
	l.mlayer = l.mlayer[:0]                                    // Clear the mutation layer.
	atomic.StoreInt32(&l.deleteAll, 1)

	var gid uint32
	if rv, ok := ctx.Value("raft").(x.RaftValue); ok {
		l.water.Ch <- x.Mark{Index: rv.Index}
		l.pending = append(l.pending, rv.Index)
		gid = rv.Group
	}
	// if mutation doesn't come via raft
	if gid == 0 {
		gid = group.BelongsTo(t.Attr)
	}
	if dirtyChan != nil {
		dirtyChan <- fingerPrint{fp: l.ghash, gid: gid}
	}
	return nil
}

// Iterate will allow you to iterate over this Posting List, while having acquired a read lock.
// So, please keep this iteration cheap, otherwise mutations would get stuck.
// The iteration will start after the provided UID. The results would not include this UID.
// The function will loop until either the Posting List is fully iterated, or you return a false
// in the provided function, which will indicate to the function to break out of the iteration.
//
// 	pl.Iterate(func(p *typesp.Posting) bool {
//    // Use posting p
//    return true  // to continue iteration.
//    return false // to break iteration.
//  })
func (l *List) Iterate(afterUid uint64, f func(obj *typesp.Posting) bool) {
	l.RLock()
	defer l.RUnlock()
	l.iterate(afterUid, f)
}

func (l *List) iterate(afterUid uint64, f func(obj *typesp.Posting) bool) {
	l.AssertRLock()
	pidx, midx := 0, 0
	pl := l.getPostingList(0)

	postingLen := len(pl.Postings)
	mlayerLen := len(l.mlayer)
	if afterUid > 0 {
		pidx = sort.Search(postingLen, func(idx int) bool {
			p := pl.Postings[idx]
			return afterUid < p.Uid
		})
		midx = sort.Search(mlayerLen, func(idx int) bool {
			mp := l.mlayer[idx]
			return afterUid < mp.Uid
		})
	}

	var mp, pp *typesp.Posting
	cont := true
	for cont {
		if pidx < postingLen {
			pp = pl.Postings[pidx]
		} else {
			pp = emptyPosting
		}
		if midx < mlayerLen {
			mp = l.mlayer[midx]
		} else {
			mp = emptyPosting
		}

		switch {
		case pp.Uid == 0 && mp.Uid == 0:
			cont = false
		case mp.Uid == 0 || (pp.Uid > 0 && pp.Uid < mp.Uid):
			cont = f(pp)
			pidx++
		case pp.Uid == 0 || (mp.Uid > 0 && mp.Uid < pp.Uid):
			if mp.Op != Del {
				cont = f(mp)
			}
			midx++
		case pp.Uid == mp.Uid:
			if mp.Op != Del {
				cont = f(mp)
			}
			pidx++
			midx++
		default:
			log.Fatalf("Unhandled case during iteration of posting list.")
		}
	}
}

// Length iterates over the mutation layer and counts number of elements.
func (l *List) Length(afterUid uint64) int {
	l.RLock()
	defer l.RUnlock()

	pidx, midx := 0, 0
	pl := l.getPostingList(0)

	if afterUid > 0 {
		pidx = sort.Search(len(pl.Postings), func(idx int) bool {
			p := pl.Postings[idx]
			return afterUid < p.Uid
		})
		midx = sort.Search(len(l.mlayer), func(idx int) bool {
			mp := l.mlayer[idx]
			return afterUid < mp.Uid
		})
	}

	count := len(pl.Postings) - pidx
	for _, p := range l.mlayer[midx:] {
		if p.Op == Add {
			count++
		} else if p.Op == Del {
			count--
		}
	}
	return count
}

func (l *List) SyncIfDirty(ctx context.Context) (committed bool, err error) {
	l.Lock()
	defer l.Unlock()

	if len(l.mlayer) == 0 && atomic.LoadInt32(&l.deleteAll) == 0 {
		l.water.Ch <- x.Mark{Indices: l.pending, Done: true}
		l.pending = make([]uint64, 0, 3)
		return false, nil
	}

	var final typesp.PostingList
	ubuf := make([]byte, 16)
	h := md5.New()
	count := 0
	l.iterate(0, func(p *typesp.Posting) bool {
		// Checksum code.
		n := binary.PutVarint(ubuf, int64(count))
		h.Write(ubuf[0:n])
		n = binary.PutUvarint(ubuf, p.Uid)
		h.Write(ubuf[0:n])
		h.Write(p.Value)
		h.Write([]byte(p.Label))
		count++

		// I think it's okay to take the pointer from the iterator, because we have a lock
		// over List; which won't be released until final has been marshalled. Thus, the
		// underlying data wouldn't be changed.
		final.Postings = append(final.Postings, p)
		return true
	})
	final.Checksum = h.Sum(nil)
	data, err := final.Marshal()
	x.Checkf(err, "Unable to marshal posting list")

	sw := l.StartWait() // Corresponding l.Wait() in getPostingList.
	ce := syncEntry{
		key:     l.key,
		val:     data,
		sw:      sw,
		water:   l.water,
		pending: l.pending,
	}
	syncCh <- ce

	// Now reset the mutation variables.
	l.pending = make([]uint64, 0, 3)
	atomic.StorePointer(&l.pbuffer, nil) // Make prev buffer eligible for GC.
	l.mlayer = l.mlayer[:0]
	l.lastCompact = time.Now()
	atomic.StoreInt32(&l.deleteAll, 0) // Unset deleteAll
	return true, nil
}

func (l *List) LastCompactionTs() time.Time {
	l.RLock()
	defer l.RUnlock()
	return l.lastCompact
}

// Uids returns the UIDs given some query params.
// We have to apply the filtering before applying (offset, count).
func (l *List) Uids(opt ListOptions) *taskp.List {
	// Pre-assign length to make it faster.
	res := make([]uint64, 0, l.Length(opt.AfterUID))

	l.Postings(opt, func(p *typesp.Posting) bool {
		res = append(res, p.Uid)
		return true
	})
	// Do The intersection here as it's optimized.
	out := &taskp.List{res}
	if opt.Intersect != nil {
		algo.IntersectWith(out, opt.Intersect, out)
	}
	return out
}

// Postings calls postFn with the postings that are common with
// uids in the opt ListOptions.
func (l *List) Postings(opt ListOptions, postFn func(*typesp.Posting) bool) {
	l.RLock()
	defer l.RUnlock()

	l.iterate(opt.AfterUID, func(p *typesp.Posting) bool {
		if postingType(p) != x.ValueUid {
			return true
		}
		return postFn(p)
	})
}

func (l *List) AllValues() (vals []types.Val, rerr error) {
	l.RLock()
	defer l.RUnlock()

	l.iterate(0, func(p *typesp.Posting) bool {
		// x.AssertTruef(postingType(p) == x.ValueMulti,
		//	"Expected a value posting.")
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
func (l *List) Value() (rval types.Val, rerr error) {
	l.RLock()
	defer l.RUnlock()
	val, found := l.findValue(math.MaxUint64)
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
func (l *List) ValueFor(langs []string) (rval types.Val, rerr error) {
	p, err := l.postingFor(langs)
	if err != nil {
		return rval, err
	}
	return copyValueToVal(p), nil
}

func (l *List) postingFor(langs []string) (p *typesp.Posting, rerr error) {
	l.RLock()
	defer l.RUnlock()
	return l.postingForLangs(langs)
}

func (l *List) ValueForTag(tag string) (rval types.Val, rerr error) {
	l.RLock()
	defer l.RUnlock()
	p, err := l.postingForTag(tag)
	if err != nil {
		return rval, err
	}
	return copyValueToVal(p), nil
}

func copyValueToVal(p *typesp.Posting) (rval types.Val) {
	val := make([]byte, len(p.Value))
	copy(val, p.Value)
	rval.Value = val
	rval.Tid = types.TypeID(p.ValType)
	return
}

func (l *List) postingForLangs(langs []string) (pos *typesp.Posting, rerr error) {
	l.AssertRLock()
	var found bool

	// look for language in preffered order
	for _, lang := range langs {
		pos, rerr = l.postingForTag(lang)
		if rerr == nil {
			return pos, nil
		}
	}

	// look for value without language
	if !found {
		found, pos = l.findPosting(math.MaxUint64)
	}

	// last resort - return value with smallest lang Uid
	if !found {
		l.iterate(0, func(p *typesp.Posting) bool {
			if postingType(p) == x.ValueMulti {
				pos = p
				return false
			}
			return true
		})
	}

	if !found {
		return pos, ErrNoValue
	}

	return pos, nil
}

func (l *List) postingForTag(tag string) (p *typesp.Posting, rerr error) {
	l.AssertRLock()
	uid := farm.Fingerprint64([]byte(tag))
	found, p := l.findPosting(uid)
	if !found {
		return p, ErrNoValue
	}

	return p, nil
}

func (l *List) findValue(uid uint64) (rval types.Val, found bool) {
	l.AssertRLock()
	found, p := l.findPosting(uid)
	if !found {
		return rval, found
	}

	return copyValueToVal(p), true
}

func (l *List) findPosting(uid uint64) (found bool, pos *typesp.Posting) {
	l.iterate(uid-1, func(p *typesp.Posting) bool {
		if p.Uid == uid {
			pos = p
			found = true
		}
		return false
	})

	return found, pos
}

// Facets gives facets for the posting representing value.
func (l *List) Facets(param *facetsp.Param, langs []string) (fs []*facetsp.Facet,
	ferr error) {
	l.RLock()
	defer l.RUnlock()
	p, err := l.postingFor(langs)
	if err != nil {
		return nil, err
	}
	return facets.CopyFacets(p.Facets, param), nil
}
