/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
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
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	// ErrRetry can be triggered if the posting list got deleted from memory due to a hard commit.
	// In such a case, retry.
	ErrRetry = fmt.Errorf("Temporary Error. Please retry.")
	// ErrNoValue would be returned if no value was found in the posting list.
	ErrNoValue   = fmt.Errorf("No value found")
	emptyPosting = &types.Posting{}
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
	mlayer      []*types.Posting // mutations
	pstore      *store.Store     // postinglist store
	lastCompact time.Time
	deleteMe    int32 // Using atomic for this, to avoid expensive SetForDeletion operation.
	refcount    int32

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
	AfterUID  uint64     // Any UID returned must be after this value.
	Intersect *task.List // Intersect results with this list of UIDs.
}

type ByUid []*types.Posting

func (pa ByUid) Len() int           { return len(pa) }
func (pa ByUid) Swap(i, j int)      { pa[i], pa[j] = pa[j], pa[i] }
func (pa ByUid) Less(i, j int) bool { return pa[i].Uid < pa[j].Uid }

func samePosting(a *types.Posting, b *types.Posting) bool {
	if a.Uid != b.Uid {
		return false
	}
	if a.ValType != b.ValType {
		return false
	}
	if !bytes.Equal(a.Value, b.Value) {
		return false
	}
	// Checking source might not be necessary.
	if a.Label != b.Label {
		return false
	}
	return true
}

func newPosting(t *task.DirectedEdge) *types.Posting {
	x.AssertTruef(edgeType(t) != valueEmpty,
		"This should have been set by the caller.")

	var op uint32
	if t.Op == task.DirectedEdge_SET {
		op = Set
	} else if t.Op == task.DirectedEdge_DEL {
		op = Del
	} else {
		x.Fatalf("Unhandled operation: %+v", t)
	}

	return &types.Posting{
		Uid:     t.ValueId,
		Value:   t.Value,
		ValType: types.Posting_ValType(t.ValueType),
		Label:   t.Label,
		Lang:    t.Lang,
		Op:      op,
	}
}

func (l *List) WaitForCommit() {
	l.RLock()
	defer l.RUnlock()
	l.Wait()
}

func (l *List) PostingList() *types.PostingList {
	l.RLock()
	defer l.RUnlock()
	return l.getPostingList(0)
}

// getPostingList tries to get posting list from l.pbuffer. If it is nil, then
// we query RocksDB. There is no need for lock acquisition here.
func (l *List) getPostingList(loop int) *types.PostingList {
	if loop >= 10 {
		x.Fatalf("This is over the 10th loop: %v", loop)
	}
	l.AssertRLock()
	// Wait for any previous commits to happen before retrieving posting list again.
	l.Wait()

	pb := atomic.LoadPointer(&l.pbuffer)
	plist := (*types.PostingList)(pb)

	if plist == nil {
		x.AssertTrue(l.pstore != nil)
		plist = new(types.PostingList)

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

func (l *List) updateMutationLayer(mpost *types.Posting) bool {
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
func (l *List) AddMutation(ctx context.Context, t *task.DirectedEdge) (bool, error) {
	l.Lock()
	defer l.Unlock()
	return l.addMutation(ctx, t)
}

// TODO(tzdybal) - this is almost the same as in rdf/parse.go - maybe some refactoring?
type valueTypeInfo int32

// Type of a data inside DirectedEdge or Posting
const (
	valueEmpty    valueTypeInfo = iota // no UID and no value
	valueUid                           // UID
	valueUntagged                      // value without defined language tag
	valueTagged                        // value with defined language tag
)

func edgeType(t *task.DirectedEdge) valueTypeInfo {
	hasVal := !bytes.Equal(t.Value, nil)
	hasId := t.ValueId != 0
	switch {
	case hasVal && hasId:
		return valueTagged
	case hasVal && !hasId:
		return valueUntagged
	case !hasVal && hasId:
		return valueUid
	default:
		return valueEmpty
	}
}

// TODO(tzdybal) - refactor
func postingType(p *types.Posting) valueTypeInfo {
	if !bytes.Equal(p.Value, nil) {
		if len(p.Lang) == 0 {
			return valueUntagged // value without lang tag
		} else {
			return valueTagged // value with lang tag
		}
	} else {
		if p.Uid == math.MaxUint64 {
			return valueEmpty // empty - no Uid and no Value
		} else {
			return valueUid // Uid
		}

	}
}

func (l *List) addMutation(ctx context.Context, t *task.DirectedEdge) (bool, error) {
	if atomic.LoadInt32(&l.deleteMe) == 1 {
		x.TraceError(ctx, x.Errorf("DELETEME set to true. Temporary error."))
		return false, ErrRetry
	}

	l.AssertLock()
	// All edges with a value without LANGTAG, have the same uid. In other words,
	// an (entity, attribute) can only have one untagged value.
	if !bytes.Equal(t.Value, nil) {
		t.ValueId = math.MaxUint64
		// TODO(tzdybal) - uncomment when entire code is ready for the change
		/*
			if len(t.Lang) > 0 {
				t.ValueId = farm.Fingerprint64([]byte(t.Lang))
			} else {
				t.ValueId = math.MaxUint64
			}
		*/
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
		if rv, ok := ctx.Value("raft").(x.RaftValue); ok {
			l.water.Ch <- x.Mark{Index: rv.Index}
			l.pending = append(l.pending, rv.Index)
		}
		if dirtyChan != nil {
			dirtyChan <- l.ghash
		}
	}
	return hasMutated, nil
}

// Iterate will allow you to iterate over this Posting List, while having acquired a read lock.
// So, please keep this iteration cheap, otherwise mutations would get stuck.
// The iteration will start after the provided UID. The results would not include this UID.
// The function will loop until either the Posting List is fully iterated, or you return a false
// in the provided function, which will indicate to the function to break out of the iteration.
//
// 	pl.Iterate(func(p *types.Posting) bool {
//    // Use posting p
//    return true  // to continue iteration.
//    return false // to break iteration.
//  })
func (l *List) Iterate(afterUid uint64, f func(obj *types.Posting) bool) {
	l.RLock()
	defer l.RUnlock()
	l.iterate(afterUid, f)
}

func (l *List) iterate(afterUid uint64, f func(obj *types.Posting) bool) {
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

	var mp, pp *types.Posting
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

	if len(l.mlayer) == 0 {
		l.water.Ch <- x.Mark{Indices: l.pending, Done: true}
		l.pending = make([]uint64, 0, 3)
		return false, nil
	}

	var final types.PostingList
	ubuf := make([]byte, 16)
	h := md5.New()
	count := 0
	l.iterate(0, func(p *types.Posting) bool {
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
	return true, nil
}

func (l *List) LastCompactionTs() time.Time {
	l.RLock()
	defer l.RUnlock()
	return l.lastCompact
}

// Uids returns the UIDs given some query params.
// We have to apply the filtering before applying (offset, count).
func (l *List) Uids(opt ListOptions) *task.List {
	l.RLock()
	defer l.RUnlock()

	res := new(task.List)
	var wit algo.WriteIterator
	wit.Init(res)
	l.iterate(opt.AfterUID, func(p *types.Posting) bool {
		if postingType(p) != valueUid {
			return false
		}
		uid := p.Uid
		if opt.Intersect != nil {
			var it algo.ListIterator
			it.Init(opt.Intersect)
			for ; it.Valid() && it.Val() < uid; it.Next() {
			}
			if !it.Valid() || it.Val() > uid {
				return true
			}
		}
		wit.Append(uid)
		return true
	})
	wit.End()
	return res
}

func (l *List) Value() (rval types.Val, rerr error) {
	l.RLock()
	defer l.RUnlock()
	return l.value()
}

func (l *List) value() (rval types.Val, rerr error) {
	l.AssertRLock()
	var found bool
	l.iterate(math.MaxUint64-1, func(p *types.Posting) bool {
		if p.Uid == math.MaxUint64 {
			val := make([]byte, len(p.Value))
			copy(val, p.Value)
			rval.Value = val
			rval.Tid = types.TypeID(p.ValType)
			found = true
		}
		return false
	})

	if !found {
		return rval, ErrNoValue
	}
	return rval, nil
}

// TODO(tzdybal) function for value in given language
