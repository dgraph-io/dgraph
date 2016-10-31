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
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/dgryski/go-farm"
	"github.com/google/flatbuffers/go"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

var E_TMP_ERROR = fmt.Errorf("Temporary Error. Please retry.")
var ErrNoValue = fmt.Errorf("No value found")

const (
	Set byte = 0x01
	Del byte = 0x02
)

type buffer struct {
	d []byte
}

type List struct {
	sync.RWMutex
	key         []byte
	ghash       uint64
	hash        uint32
	pbuffer     unsafe.Pointer
	mlayer      []*types.Posting // mutations
	count       []int8           // How does posting contribute to overall count.
	pstore      *store.Store     // postinglist store
	lastCompact time.Time
	wg          sync.WaitGroup
	deleteMe    int32
	refcount    int32

	dirtyTs int64 // Use atomics for this.
}

func (l *List) refCount() int32 { return atomic.LoadInt32(&l.refcount) }
func (l *List) incr() int32     { return atomic.AddInt32(&l.refcount, 1) }
func (l *List) decr() {
	val := atomic.AddInt32(&l.refcount, -1)
	x.Assertf(val >= 0, "List reference should never be less than zero: %v", val)
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

func getNew() *List {
	l := listPool.Get().(*List)
	*l = List{}
	l.wg.Add(1)
	x.Assert(len(l.key) == 0)
	l.refcount = 1
	return l
}

// ListOptions is used in List.Uids (in posting) to customize our output list of
// UIDs, for each posting list. It should be internal to this package.
type ListOptions struct {
	AfterUID  uint64        // Any UID returned must be after this value.
	Intersect *algo.UIDList // Intersect results with this list of UIDs.
}

type ByUid []*types.Posting

func (pa ByUid) Len() int           { return len(pa) }
func (pa ByUid) Swap(i, j int)      { pa[i], pa[j] = pa[j], pa[i] }
func (pa ByUid) Less(i, j int) bool { return pa[i].Uid() < pa[j].Uid() }

func samePosting(a *types.Posting, b *types.Posting) bool {
	if a.Uid() != b.Uid() {
		return false
	}
	if a.ValueLength() != b.ValueLength() {
		return false
	}
	if a.ValType() != b.ValType() {
		return false
	}
	if !bytes.Equal(a.ValueBytes(), b.ValueBytes()) {
		return false
	}
	// Checking source might not be necessary.
	if !bytes.Equal(a.Source(), b.Source()) {
		return false
	}
	return true
}

// Key = attribute|uid
func Key(uid uint64, attr string) []byte {
	buf := make([]byte, len(attr)+9)
	for i, ch := range attr {
		buf[i] = byte(ch)
	}
	buf[len(attr)] = '|'
	binary.BigEndian.PutUint64(buf[len(attr)+1:], uid)
	return buf
}

func debugKey(key []byte) string {
	var b bytes.Buffer
	var rest []byte
	for i, ch := range key {
		if ch == '|' {
			b.WriteByte(':')
			rest = key[i+1:]
			break
		}
		b.WriteByte(ch)
	}
	uid := binary.BigEndian.Uint64(rest)
	b.WriteString(strconv.FormatUint(uid, 16))
	return b.String()
}

func newPosting(t x.DirectedEdge, op byte) *types.Posting {
	b := flatbuffers.NewBuilder(0)
	var bo flatbuffers.UOffsetT
	if !bytes.Equal(t.Value, nil) {
		if t.ValueId != math.MaxUint64 {
			log.Fatal("This should have already been set by the caller.")
		}
		bo = b.CreateByteVector(t.Value)
	}
	so := b.CreateString(t.Source)
	types.PostingStart(b)
	if bo > 0 {
		types.PostingAddValue(b, bo)
	}
	types.PostingAddUid(b, t.ValueId)
	types.PostingAddSource(b, so)
	types.PostingAddTs(b, t.Timestamp.UnixNano())
	types.PostingAddOp(b, op)
	if t.ValueType != 0 {
		types.PostingAddValType(b, t.ValueType)
	}
	vend := types.PostingEnd(b)
	b.Finish(vend)

	return types.GetRootAsPosting(b.Bytes[b.Head():], 0)
}

func addEdgeToPosting(b *flatbuffers.Builder,
	t x.DirectedEdge, op byte) flatbuffers.UOffsetT {

	var bo flatbuffers.UOffsetT
	if !bytes.Equal(t.Value, nil) {
		if t.ValueId != math.MaxUint64 {
			log.Fatal("This should have already been set by the caller.")
		}
		bo = b.CreateByteVector(t.Value)
	}
	so := b.CreateString(t.Source) // Do this before posting start.

	types.PostingStart(b)
	if bo > 0 {
		types.PostingAddValue(b, bo)
	}
	types.PostingAddUid(b, t.ValueId)
	types.PostingAddSource(b, so)
	types.PostingAddTs(b, t.Timestamp.UnixNano())
	types.PostingAddOp(b, op)
	if t.ValueType != 0 {
		types.PostingAddValType(b, t.ValueType)
	}
	return types.PostingEnd(b)
}

func addPosting(b *flatbuffers.Builder, p types.Posting) flatbuffers.UOffsetT {
	so := b.CreateByteString(p.Source()) // Do this before posting start.
	var bo flatbuffers.UOffsetT
	if p.ValueLength() > 0 {
		bo = b.CreateByteVector(p.ValueBytes())
	}

	types.PostingStart(b)
	types.PostingAddUid(b, p.Uid())
	if bo > 0 {
		types.PostingAddValue(b, bo)
	}
	types.PostingAddSource(b, so)
	types.PostingAddTs(b, p.Ts())
	types.PostingAddOp(b, p.Op())
	if p.ValType() != 0 {
		types.PostingAddValType(b, p.ValType())
	}
	return types.PostingEnd(b)
}

var empty []byte
var emptyPosting []byte

// package level init
func init() {
	{
		b := flatbuffers.NewBuilder(0)
		types.PostingListStart(b)
		of := types.PostingListEnd(b)
		b.Finish(of)
		empty = b.Bytes[b.Head():]
	}

	{
		b := flatbuffers.NewBuilder(0)
		types.PostingStart(b)
		types.PostingAddUid(b, 0)
		of := types.PostingEnd(b)
		b.Finish(of)
		emptyPosting = b.Bytes[b.Head():]
	}
}

func (l *List) init(key []byte, pstore *store.Store) {
	l.Lock()
	defer l.Unlock()
	defer l.wg.Done()

	if len(empty) == 0 {
		log.Fatal("empty should have some bytes.")
	}
	l.key = key
	l.pstore = pstore

	l.hash = farm.Fingerprint32(key)
	l.ghash = farm.Fingerprint64(key)
}

// getPostingList tries to get posting list from l.pbuffer. If it is nil, then
// we query RocksDB. There is no need for lock acquisition here.
func (l *List) getPostingList() *types.PostingList {
	pb := atomic.LoadPointer(&l.pbuffer)
	buf := (*buffer)(pb)

	if buf == nil || len(buf.d) == 0 {
		nbuf := new(buffer)
		var err error
		x.Assert(l.pstore != nil)
		if nbuf.d, err = l.pstore.Get(l.key); err != nil || nbuf.d == nil {
			// Error. Just set to empty.
			nbuf.d = make([]byte, len(empty))
			copy(nbuf.d, empty)
		}
		if atomic.CompareAndSwapPointer(&l.pbuffer, pb, unsafe.Pointer(nbuf)) {
			return types.GetRootAsPostingList(nbuf.d, 0)
		}
		// Someone else replaced the pointer in the meantime. Retry recursively.
		return l.getPostingList()
	}
	return types.GetRootAsPostingList(buf.d, 0)
}

// SetForDeletion will mark this List to be deleted, so no more mutations can be applied to this.
func (l *List) SetForDeletion() {
	l.wg.Wait()
	atomic.StoreInt32(&l.deleteMe, 1)
}

func (l *List) updateMutationLayer(mpost *types.Posting) bool {
	findUid := mpost.Uid()

	// First check the mutable layer.
	midx := sort.Search(len(l.mlayer), func(idx int) bool {
		mp := l.mlayer[idx]
		return findUid <= mp.Uid()
	})
	if midx < len(l.mlayer) {
		// Found this in mlayer.
		mp := l.mlayer[midx]
		msame := samePosting(mp, mpost)
		if msame && mp.Op() == mpost.Op() {
			// Everything is the same. No change.
			return false
		}
		if mp.Uid() == mpost.Uid() {
			if !msame && mpost.Op() == Del {
				return false
			}
			// Same UID. So, set the new version.
			if mp.Op() != mpost.Op() {
				// Only update counts when the ops are different.
				if mpost.Op() == Set {
					// Increment instead of setting to 1 because the final value can be
					// 0 or 1.
					l.count[midx]++
				} else if mpost.Op() == Del {
					// Increment instead of setting to 1 because the final value can be
					// 0 or -1.
					l.count[midx]--
				}
			}
			l.mlayer[midx] = mpost
			return true
		}
	}

	// Didn't find it in mutable layer. Now check the immutable layer.
	pl := l.getPostingList()
	pidx := sort.Search(pl.PostingsLength(), func(idx int) bool {
		p := new(types.Posting)
		x.Assert(pl.Postings(p, idx))
		return findUid <= p.Uid()
	})
	if pidx < pl.PostingsLength() {
		p := new(types.Posting)
		x.Assertf(pl.Postings(p, pidx), "Unable to parse Posting at index: %v", pidx)

		psame := samePosting(p, mpost)
		if psame && mpost.Op() == Set {
			// Found exact same posting in immutable layer. So, no need to Set.
			return false
		}
		if p.Uid() != mpost.Uid() && mpost.Op() == Del {
			// Couldn't find the posting in immutable layer. So, no need to Del.
			return false
		}
		if p.Uid() == mpost.Uid() && mpost.Op() == Del {
			// Found the index in the immutable layer, but
			if !psame {
				return false
			}
		}
	}
	// Doesn't match what we already have in immutable layer. So, add to mutable layer.
	var postCount int8
	if mpost.Op() == Set {
		postCount = 1
	} else if mpost.Op() == Del {
		postCount = -1
	} else {
		x.Fatalf("Invalid posting op: %v", mpost.Op())
	}

	if midx >= len(l.mlayer) {
		// Add it at the end.
		l.mlayer = append(l.mlayer, mpost)
		l.count = append(l.count, postCount)
		return true
	}

	// Otherwise, add it where midx is pointing to.
	l.mlayer = append(l.mlayer, nil)
	copy(l.mlayer[midx+1:], l.mlayer[midx:])
	l.mlayer[midx] = mpost

	// Update counts.
	l.count = append(l.count, 0)
	copy(l.count[midx+1:], l.count[midx:])
	l.count[midx] = postCount

	return true
}

// In benchmarks, the time taken per AddMutation before was
// plateauing at 2.5 ms with sync per 10 log entries, and increasing
// for sync per 100 log entries (to 3 ms per AddMutation), largely because
// of how index generation was being done.
//
// With this change, the benchmarks perform as good as benchmarks for
// commit.Logger, where the less frequently file sync happens, the faster
// AddMutations run.
//
// PASS
// BenchmarkAddMutations_SyncEveryLogEntry-6    	     100	  24712455 ns/op
// BenchmarkAddMutations_SyncEvery10LogEntry-6  	     500	   2485961 ns/op
// BenchmarkAddMutations_SyncEvery100LogEntry-6 	   10000	    298352 ns/op
// BenchmarkAddMutations_SyncEvery1000LogEntry-6	   30000	     63544 ns/op
// ok  	github.com/dgraph-io/dgraph/posting	10.291s
//
// Update: With the latest changes, we no longer use commit.Log, in fact, the
// commit package is not present anymore. With RAFT, everything goes into WAL
// before being applied to PL. So, AddMutation now is solely a memory based operation.
// This is the result with the latest changes, running on my i5 laptop.
//
// BenchmarkAddMutations-4    	  300000	     26737 ns/op

// AddMutation adds mutation to mutation layers. Note that it does not write
// anything to disk. Some other background routine will be responsible for merging
// changes in mutation layers to RocksDB. Returns whether any mutation happens.
func (l *List) AddMutation(ctx context.Context, t x.DirectedEdge, op byte) (bool, error) {
	l.wg.Wait()
	if atomic.LoadInt32(&l.deleteMe) == 1 {
		x.TraceError(ctx, x.Errorf("DELETEME set to true. Temporary error."))
		return false, E_TMP_ERROR
	}

	// All edges with a value set, have the same uid. In other words,
	// an (entity, attribute) can only have one value.
	if !bytes.Equal(t.Value, nil) {
		t.ValueId = math.MaxUint64
	}
	if t.ValueId == 0 {
		err := x.Errorf("ValueId cannot be zero")
		x.TraceError(ctx, err)
		return false, err
	}
	mpost := newPosting(t, op)

	// Mutation arrives:
	// - Check if we had any(SET/DEL) before this, stored in the mutation list.
	//		- If yes, then replace that mutation. Jump to a)
	// a)		check if the entity exists in main posting list.
	// 				- If yes, store the mutation.
	// 				- If no, disregard this mutation.
	x.Trace(ctx, "Acquiring lock")
	l.Lock()
	defer l.Unlock()
	x.Trace(ctx, "Lock acquired")

	hasMutated := l.updateMutationLayer(mpost)
	if len(l.mlayer) > 0 {
		atomic.StoreInt64(&l.dirtyTs, time.Now().UnixNano())
		if dirtyChan != nil {
			dirtyChan <- l.ghash
		}
	}
	x.Trace(ctx, "Mutation done")
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
	l.wg.Wait()
	l.RLock()
	defer l.RUnlock()
	l.iterate(afterUid, f)
}

func (l *List) iterate(afterUid uint64, f func(obj *types.Posting) bool) {
	pidx, midx := 0, 0
	pl := l.getPostingList()

	if afterUid > 0 {
		pidx = sort.Search(pl.PostingsLength(), func(idx int) bool {
			p := new(types.Posting)
			x.Assert(pl.Postings(p, idx))
			return afterUid < p.Uid()
		})
		midx = sort.Search(len(l.mlayer), func(idx int) bool {
			mp := l.mlayer[idx]
			return afterUid < mp.Uid()
		})
	}

	empty := types.GetRootAsPosting(emptyPosting, 0)
	mp, pp := new(types.Posting), new(types.Posting)
	cont := true
	for cont {
		if pidx < pl.PostingsLength() {
			x.Assert(pl.Postings(pp, pidx))
		} else {
			pp = empty
		}
		if midx < len(l.mlayer) {
			mp = l.mlayer[midx]
		} else {
			mp = empty
		}

		switch {
		case pp.Uid() == 0 && mp.Uid() == 0:
			cont = false
		case mp.Uid() == 0 || (pp.Uid() > 0 && pp.Uid() < mp.Uid()):
			cont = f(pp)
			pidx++
		case pp.Uid() == 0 || (mp.Uid() > 0 && mp.Uid() < pp.Uid()):
			if mp.Op() == Set {
				cont = f(mp)
			}
			midx++
		case pp.Uid() == mp.Uid():
			if mp.Op() == Set {
				cont = f(mp)
			}
			pidx++
			midx++
		default:
			log.Fatalf("Unhandled case during iteration of posting list.")
		}
	}
}

// Length iterates over the posting list and counts the number of elements. This is NOT CHEAP. So, use it sparingly.
func (l *List) Length(afterUid uint64) int {
	pidx, midx := 0, 0
	pl := l.getPostingList()

	if afterUid > 0 {
		pidx = sort.Search(pl.PostingsLength(), func(idx int) bool {
			p := new(types.Posting)
			x.Assert(pl.Postings(p, idx))
			return afterUid < p.Uid()
		})
		midx = sort.Search(len(l.mlayer), func(idx int) bool {
			mp := l.mlayer[idx]
			return afterUid < mp.Uid()
		})
	}

	count := pl.PostingsLength() - pidx
	for _, c := range l.count[midx:] {
		count += int(c)
	}
	return count
}

func (l *List) CommitIfDirty(ctx context.Context) (committed bool, err error) {
	if atomic.LoadInt64(&l.dirtyTs) == 0 {
		x.Trace(ctx, "Not committing")
		return false, nil
	}
	x.Trace(ctx, "Committing")
	return l.commit()
}

func (l *List) commit() (committed bool, rerr error) {
	l.wg.Wait()
	l.Lock()
	defer l.Unlock()

	if len(l.mlayer) == 0 {
		atomic.StoreInt64(&l.dirtyTs, 0)
		return false, nil
	}

	b := flatbuffers.NewBuilder(0)
	offsets := make([]flatbuffers.UOffsetT, 0, 10)

	ubuf := make([]byte, 16)
	h := md5.New()
	count := 0
	l.iterate(0, func(p *types.Posting) bool {
		n := binary.PutVarint(ubuf, int64(count))
		h.Write(ubuf[0:n])
		n = binary.PutUvarint(ubuf, p.Uid())
		h.Write(ubuf[0:n])
		h.Write(p.ValueBytes())
		h.Write(p.Source())
		count++

		offsets = append(offsets, addPosting(b, *p))
		return true
	})

	types.PostingListStartPostingsVector(b, count)
	for i := len(offsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(offsets[i])
	}
	vend := b.EndVector(count)

	co := b.CreateString(fmt.Sprintf("%x", h.Sum(nil)))
	types.PostingListStart(b)
	types.PostingListAddChecksum(b, co)
	types.PostingListAddPostings(b, vend)
	end := types.PostingListEnd(b)
	b.Finish(end)

	if err := l.pstore.SetOne(l.key, b.Bytes[b.Head():]); err != nil {
		log.Fatalf("Error while storing posting list: %v", err)
		return true, err
	}

	// Now reset the mutation variables.
	atomic.StorePointer(&l.pbuffer, nil) // Make prev buffer eligible for GC.
	atomic.StoreInt64(&l.dirtyTs, 0)     // Set as clean.
	l.mlayer = l.mlayer[:0]
	l.count = l.count[:0]
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
func (l *List) Uids(opt ListOptions) *algo.UIDList {
	l.wg.Wait()
	l.RLock()
	defer l.RUnlock()

	result := make([]uint64, 0, 10)
	var intersectIdx int // Indexes into opt.Intersect if it exists.
	l.iterate(opt.AfterUID, func(p *types.Posting) bool {
		if p.Uid() == math.MaxUint64 {
			return false
		}
		uid := p.Uid()
		if opt.Intersect != nil {
			for ; intersectIdx < opt.Intersect.Size() && opt.Intersect.Get(intersectIdx) < uid; intersectIdx++ {
			}
			if intersectIdx >= opt.Intersect.Size() || opt.Intersect.Get(intersectIdx) > uid {
				return true
			}
		}
		result = append(result, uid)
		return true
	})
	return algo.NewUIDList(result)
}

func (l *List) Value() (val []byte, vtype byte, rerr error) {
	l.wg.Wait()
	l.RLock()
	defer l.RUnlock()

	var found bool
	l.iterate(math.MaxUint64-1, func(p *types.Posting) bool {
		if p.Uid() == math.MaxUint64 {
			val = make([]byte, len(p.ValueBytes()))
			copy(val, p.ValueBytes())
			vtype = p.ValType()
			found = true
		}
		return false
	})

	if !found {
		return val, vtype, ErrNoValue
	}
	return val, vtype, nil
}
