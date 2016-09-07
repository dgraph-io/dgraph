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
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"context"

	"github.com/dgryski/go-farm"
	"github.com/google/flatbuffers/go"
	"github.com/zond/gotomic"

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

var E_TMP_ERROR = fmt.Errorf("Temporary Error. Please retry.")

const Set = 0x01
const Del = 0x02

type buffer struct {
	d []byte
}

type MutationLink struct {
	idx     int
	moveidx int
	posting *types.Posting
}

type List struct {
	sync.RWMutex
	key         []byte
	ghash       gotomic.Hashable
	hash        uint32
	pbuffer     unsafe.Pointer
	pstore      *store.Store // postinglist store
	clog        *commit.Logger
	lastCompact time.Time
	wg          sync.WaitGroup
	deleteMe    int32

	// Mutations
	mlayer        map[int]types.Posting // stores only replace instructions.
	mdelta        int                   // len(plist) + mdelta = final length.
	maxMutationTs int64                 // Track maximum mutation ts.
	mindex        []*MutationLink
	dirtyTs       int64 // Use atomics for this.
}

func NewList() *List {
	l := new(List)
	l.wg.Add(1)
	l.mlayer = make(map[int]types.Posting)
	return l
}

type ListOptions struct {
	Offset   int
	Count    int
	AfterUid uint64
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
	if !bytes.Equal(a.ValueBytes(), b.ValueBytes()) {
		return false
	}
	if !bytes.Equal(a.Source(), b.Source()) {
		return false
	}
	return true
}

// key = (entity uid, attribute)
func Key(uid uint64, attr string) []byte {
	buf := bytes.NewBufferString(attr)
	buf.WriteRune('|')
	if err := binary.Write(buf, binary.LittleEndian, uid); err != nil {
		log.Fatalf("Error while creating key with attr: %v uid: %v\n", attr, uid)
	}
	return buf.Bytes()
}

func newPosting(t x.DirectedEdge, op byte) []byte {
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
	vend := types.PostingEnd(b)
	b.Finish(vend)

	return b.Bytes[b.Head():]
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

func (l *List) init(key []byte, pstore *store.Store, clog *commit.Logger) {
	l.Lock()
	defer l.Unlock()
	defer l.wg.Done()

	if len(empty) == 0 {
		log.Fatal("empty should have some bytes.")
	}
	l.key = key
	l.pstore = pstore
	l.clog = clog

	posting := l.getPostingList()
	l.maxMutationTs = posting.CommitTs()
	l.hash = farm.Fingerprint32(key)
	l.ghash = gotomic.IntKey(farm.Fingerprint64(key))
	l.mlayer = make(map[int]types.Posting)

	if clog == nil {
		return
	}

	// TODO(pawan) - Decouple commit logs and posting lists. RAFT would supersede
	// this functionality.
	ch := make(chan []byte, 100)
	done := make(chan error)
	go clog.StreamEntries(posting.CommitTs()+1, l.hash, ch, done)

	for buffer := range ch {
		uo := flatbuffers.GetUOffsetT(buffer)
		m := new(types.Posting)
		m.Init(buffer, uo)
		if m.Ts() > l.maxMutationTs {
			l.maxMutationTs = m.Ts()
		}
		l.mergeMutation(m)
	}
	if err := <-done; err != nil {
		log.Fatalf("Error while streaming entries: %v", err)
	}
}

// There's no need for lock acquisition for this.
func (l *List) getPostingList() *types.PostingList {
	pb := atomic.LoadPointer(&l.pbuffer)
	buf := (*buffer)(pb)

	if buf == nil || len(buf.d) == 0 {
		nbuf := new(buffer)
		var err error
		if nbuf.d, err = l.pstore.Get(l.key); err != nil {
			// Error. Just set to empty.
			nbuf.d = make([]byte, len(empty))
			copy(nbuf.d, empty)
		}
		if atomic.CompareAndSwapPointer(&l.pbuffer, pb, unsafe.Pointer(nbuf)) {
			return types.GetRootAsPostingList(nbuf.d, 0)

		} else {
			// Someone else replaced the pointer in the meantime. Retry recursively.
			return l.getPostingList()
		}
	}
	return types.GetRootAsPostingList(buf.d, 0)
}

// Caller must hold at least a read lock.
func (l *List) lePostingIndex(maxUid uint64) (int, uint64) {
	posting := l.getPostingList()
	left, right := 0, posting.PostingsLength()-1
	sofar := -1
	p := new(types.Posting)

	for left <= right {
		pos := (left + right) / 2
		if ok := posting.Postings(p, pos); !ok {
			log.Fatalf("Unable to parse posting from list idx: %v", pos)
		}
		val := p.Uid()
		if val > maxUid {
			right = pos - 1
			continue
		}
		if val == maxUid {
			return pos, val
		}
		sofar = pos
		left = pos + 1
	}
	if sofar == -1 {
		return -1, 0
	}
	if ok := posting.Postings(p, sofar); !ok {
		log.Fatalf("Unable to parse posting from list idx: %v", sofar)
	}
	return sofar, p.Uid()
}

func (l *List) leMutationIndex(maxUid uint64) (int, uint64) {
	left, right := 0, len(l.mindex)-1
	sofar := -1
	for left <= right {
		pos := (left + right) / 2
		m := l.mindex[pos]
		val := m.posting.Uid()
		if val > maxUid {
			right = pos - 1
			continue
		}
		if val == maxUid {
			return pos, val
		}
		sofar = pos
		left = pos + 1
	}
	if sofar == -1 {
		return -1, 0
	}
	return sofar, l.mindex[sofar].posting.Uid()
}

func (l *List) mindexInsertAt(mlink *MutationLink, mi int) {
	l.mindex = append(l.mindex, nil)
	copy(l.mindex[mi+1:], l.mindex[mi:])
	l.mindex[mi] = mlink
	for i := mi + 1; i < len(l.mindex); i++ {
		l.mindex[i].idx += 1
	}
}

func (l *List) mindexDeleteAt(mi int) {
	l.mindex = append(l.mindex[:mi], l.mindex[mi+1:]...)
	for i := mi; i < len(l.mindex); i++ {
		l.mindex[i].idx -= 1
	}
}

// mutationIndex (mindex) is useful to avoid having to parse the entire
// postinglist upto idx, for every Get(*types.Posting, idx), which has a
// complexity of O(idx). Iteration over N size posting list would this push
// us into O(N^2) territory, without this technique.
//
// Using this technique,
// we can overlay mutation layers over immutable posting list, to allow for
// O(m) lookups, where m = size of mutation list. Obviously, the size of
// mutation list should be much smaller than the size of posting list, except
// in tiny posting lists, where performance wouldn't be such a concern anyways.
//
// Say we have this data:
// Posting List (plist, immutable):
// idx:   0  1  2  3  4  5
// value: 2  5  9 10 13 15
//
// Mutation List (mlist):
// idx:          0   1   2
// value:        7  10  13' // posting uid is 13 but other values vary.
// Op:         SET DEL SET
// Effective:  ADD DEL REP  (REP = replace)
//
// ----------------------------------------------------------------------------
// regenerateIndex would generate these:
// mlayer (layer just above posting list contains only replace instructions)
// idx:          4
// value:       13'
// Op:       	 SET
// Effective:  REP  (REP = replace)
//
// mindex:
// idx:          2   4
// value:        7  10
// moveidx:     -1  +1
// Effective:  ADD DEL
//
// Now, let's see how the access would work:
// idx: get --> calculation [idx, served from, value]
// idx: 0 --> 0   [0, plist, 2]
// idx: 1 --> 1   [1, plist, 5]
// idx: 2 --> ADD from mindex
//        -->     [2, mindex, 7] // also has moveidx = -1
// idx: 3 --> 3 + moveidx=-1 = 2 [2, plist, 9]
// idx: 4 --> DEL from mindex
//        --> 4 + moveidx=-1 + moveidx=+1 = 4 [4, mlayer, 13']
// idx: 5 --> 5 + moveidx=-1 + moveidx=+1 = 5 [5, plist, 15]
//
// Thus we can provide mutation layers over immutable posting list, while
// still ensuring fast lookup access.
//
// NOTE: This function expects the caller to hold a RW Lock.
// Update: With mergeMutation function, we're adding mutations with a cost
// of O(log M + log N), where M = number of previous mutations, and N =
// number of postings in the immutable posting list.
func (l *List) mergeMutation(mp *types.Posting) {
	curUid := mp.Uid()
	pi, puid := l.lePostingIndex(curUid)  // O(log N)
	mi, muid := l.leMutationIndex(curUid) // O(log M)
	inPlist := puid == curUid

	// O(1) follows, but any additions or deletions from mindex would
	// be O(M) due to element shifting. In terms of benchmarks, this performs
	// a LOT better than when I was running O(N + M), re-generating mutation
	// flatbuffers, linked lists etc.
	mlink := new(MutationLink)
	mlink.posting = mp

	if mp.Op() == Del {
		if muid == curUid { // curUid found in mindex.
			if inPlist { // In plist, so replace previous instruction in mindex.
				mlink.moveidx = 1
				mlink.idx = pi + mi
				l.mindex[mi] = mlink

			} else { // Not in plist, so delete previous instruction in mindex.
				l.mdelta -= 1
				l.mindexDeleteAt(mi)
			}

		} else { // curUid not found in mindex.
			if inPlist { // In plist, so insert in mindex.
				mlink.moveidx = 1
				l.mdelta -= 1
				mlink.idx = pi + mi + 1
				l.mindexInsertAt(mlink, mi+1)

			} else {
				// Not found in plist, and not found in mindex. So, ignore.
			}
		}

	} else if mp.Op() == Set {
		if muid == curUid { // curUid found in mindex.
			if inPlist { // In plist, so delete previous instruction, set in mlayer.
				l.mindexDeleteAt(mi)
				l.mlayer[pi] = *mp

			} else { // Not in plist, so replace previous set instruction in mindex.
				// NOTE: This prev instruction couldn't have been a Del instruction.
				mlink.idx = pi + 1 + mi
				mlink.moveidx = -1
				l.mindex[mi] = mlink
			}

		} else { // curUid not found in mindex.
			if inPlist { // In plist, so just set it in mlayer.
				// If this posting matches what we already have in posting list,
				// we don't need to `dirty` this by adding to mlayer.
				plist := l.getPostingList()
				var cp types.Posting
				if ok := plist.Postings(&cp, pi); ok {
					if samePosting(&cp, mp) {
						return // do nothing.
					}
				}
				l.mlayer[pi] = *mp

			} else { // not in plist, not in mindex, so insert in mindex.
				mlink.moveidx = -1
				l.mdelta += 1
				mlink.idx = pi + 1 + mi + 1 // right of pi, and right of mi.
				l.mindexInsertAt(mlink, mi+1)
			}
		}

	} else {
		log.Fatalf("Invalid operation: %v", mp.Op())
	}
}

// Caller must hold at least a read lock.
func (l *List) length() int {
	plist := l.getPostingList()
	return plist.PostingsLength() + l.mdelta
}

func (l *List) Length() int {
	l.wg.Wait()

	l.RLock()
	defer l.RUnlock()
	return l.length()
}

func (l *List) Get(p *types.Posting, i int) bool {
	l.wg.Wait()

	l.RLock()
	defer l.RUnlock()
	return l.get(p, i)
}

// Caller must hold at least a read lock.
func (l *List) get(p *types.Posting, i int) bool {
	plist := l.getPostingList()
	if len(l.mindex) == 0 {
		if val, ok := l.mlayer[i]; ok {
			*p = val
			return true
		}
		return plist.Postings(p, i)
	}

	// Iterate over mindex, and see if we have instructions
	// for the given index. Otherwise, sum up the move indexes
	// uptil the given index, so we know where to look in
	// mlayer and/or the main posting list.
	move := 0
	for _, mlink := range l.mindex {
		if mlink.idx > i {
			break

		} else if mlink.idx == i {
			// Found an instruction. Check what is says.
			if mlink.posting.Op() == Set {
				// ADD
				*p = *mlink.posting
				return true

			} else if mlink.posting.Op() == Del {
				// DELETE
				// The loop will break in the next iteration, after updating the move
				// variable.

			} else {
				log.Fatal("Someone, I mean you, forgot to tackle" +
					" this operation. Stop drinking.")
			}
		}
		move += mlink.moveidx
	}
	newidx := i + move

	// Check if we have any replace instruction in mlayer.
	if val, ok := l.mlayer[newidx]; ok {
		*p = val
		return true
	}
	// Hit the main posting list.
	if newidx >= plist.PostingsLength() {
		return false
	}
	return plist.Postings(p, newidx)
}

func (l *List) SetForDeletion() {
	l.wg.Wait()
	atomic.StoreInt32(&l.deleteMe, 1)
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
func (l *List) AddMutation(ctx context.Context, t x.DirectedEdge, op byte) error {
	l.wg.Wait()
	if atomic.LoadInt32(&l.deleteMe) == 1 {
		x.Trace(ctx, "DELETEME set to true. Temporary error.")
		return E_TMP_ERROR
	}

	// All edges with a value set, have the same uid. In other words,
	// an (entity, attribute) can only have one value.
	if !bytes.Equal(t.Value, nil) {
		t.ValueId = math.MaxUint64
	}
	if t.ValueId == 0 {
		x.Trace(ctx, "ValueId cannot be zero")
		return fmt.Errorf("ValueId cannot be zero.")
	}
	mbuf := newPosting(t, op)
	var err error
	var ts int64
	if l.clog != nil {
		ts, err = l.clog.AddLog(l.hash, mbuf)
		if err != nil {
			x.Trace(ctx, "Error while adding log: %v", err)
			return err
		}
	}
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

	uo := flatbuffers.GetUOffsetT(mbuf)
	mpost := new(types.Posting)
	mpost.Init(mbuf, uo)

	l.mergeMutation(mpost)
	if len(l.mindex)+len(l.mlayer) > 0 {
		atomic.StoreInt64(&l.dirtyTs, time.Now().UnixNano())
		if dirtymap != nil {
			dirtymap.Put(l.ghash, true)
		}
	}
	l.maxMutationTs = ts
	x.Trace(ctx, "Mutation done")
	return nil
}

func (l *List) MergeIfDirty(ctx context.Context) (merged bool, err error) {
	if atomic.LoadInt64(&l.dirtyTs) == 0 {
		x.Trace(ctx, "Not committing")
		return false, nil
	} else {
		x.Trace(ctx, "Committing")
	}
	return l.merge()
}

func (l *List) merge() (merged bool, rerr error) {
	l.wg.Wait()
	l.Lock()
	defer l.Unlock()

	if len(l.mindex)+len(l.mlayer) == 0 {
		atomic.StoreInt64(&l.dirtyTs, 0)
		return false, nil
	}

	var p types.Posting
	sz := l.length()
	b := flatbuffers.NewBuilder(0)
	offsets := make([]flatbuffers.UOffsetT, sz)
	for i := 0; i < sz; i++ {
		if ok := l.get(&p, i); !ok {
			log.Fatalf("Idx: %d. Unable to parse posting.", i)
		}
		offsets[i] = addPosting(b, p)
	}
	types.PostingListStartPostingsVector(b, sz)
	for i := len(offsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(offsets[i])
	}
	vend := b.EndVector(sz)

	types.PostingListStart(b)
	types.PostingListAddPostings(b, vend)
	types.PostingListAddCommitTs(b, l.maxMutationTs)
	end := types.PostingListEnd(b)
	b.Finish(end)

	if err := l.pstore.SetOne(l.key, b.Bytes[b.Head():]); err != nil {
		log.Fatalf("Error while storing posting list: %v", err)
		return true, err
	}

	// Now reset the mutation variables.
	atomic.StorePointer(&l.pbuffer, nil) // Make prev buffer eligible for GC.
	atomic.StoreInt64(&l.dirtyTs, 0)     // Set as clean.
	l.lastCompact = time.Now()
	l.mlayer = make(map[int]types.Posting)
	l.mdelta = 0
	l.mindex = nil
	return true, nil
}

func (l *List) LastCompactionTs() time.Time {
	l.RLock()
	defer l.RUnlock()
	return l.lastCompact
}

func (l *List) Uids(opt ListOptions) []uint64 {
	l.wg.Wait()
	l.RLock()
	defer l.RUnlock()

	if opt.Offset < 0 {
		log.Fatalf("Unexpected offset: %v", opt.Offset)
		return make([]uint64, 0)
	}

	var p types.Posting
	if opt.AfterUid > 0 {
		// sort.Search returns the index of the first element > AfterUid.
		opt.Offset = sort.Search(l.length(), func(i int) bool {
			l.get(&p, i)
			return p.Uid() > opt.AfterUid
		})
	}
	if opt.Count < 0 {
		opt.Count = 0 - opt.Count
		opt.Offset = l.length() - opt.Count
	}
	if opt.Offset < 0 {
		opt.Offset = 0
	}

	if opt.Count == 0 || opt.Count > l.length()-opt.Offset {
		opt.Count = l.length() - opt.Offset
	}
	if opt.Count < 0 {
		opt.Count = 0
	}

	result := make([]uint64, opt.Count)
	result = result[:0]
	for i := opt.Offset; i < opt.Count+opt.Offset && i < l.length(); i++ {
		if ok := l.get(&p, i); !ok || p.Uid() == math.MaxUint64 {
			break
		}
		result = append(result, p.Uid())
	}
	return result
}

func (l *List) Value() (result []byte, rerr error) {
	l.wg.Wait()
	l.RLock()
	defer l.RUnlock()

	if l.length() == 0 {
		return result, fmt.Errorf("No value found")
	}

	var p types.Posting
	if ok := l.get(&p, l.length()-1); !ok {
		return result, fmt.Errorf("Unable to get last posting")
	}
	if p.Uid() != math.MaxUint64 {
		return result, fmt.Errorf("No value found")
	}
	return p.ValueBytes(), nil
}
