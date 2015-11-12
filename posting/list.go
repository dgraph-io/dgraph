/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
	"github.com/google/flatbuffers/go"

	linked "container/list"
)

var glog = x.Log("posting")

const Set = 0x01
const Del = 0x02

type MutationLink struct {
	idx     int
	moveidx int
	posting types.Posting
}

type List struct {
	sync.RWMutex
	key           []byte
	hash          uint32
	buffer        []byte
	mutations     []*types.Posting
	pstore        *store.Store // postinglist store
	clog          *commit.Logger
	maxMutationTs int64 // Track maximum mutation ts

	// mlayer keeps only replace instructions for the posting list.
	// This works at the
	mlayer map[int]types.Posting
	mdelta int // Delta based on number of elements in posting list.
	mindex *linked.List
}

type ByUid []*types.Posting

func (pa ByUid) Len() int           { return len(pa) }
func (pa ByUid) Swap(i, j int)      { pa[i], pa[j] = pa[j], pa[i] }
func (pa ByUid) Less(i, j int) bool { return pa[i].Uid() < pa[j].Uid() }

// key = (entity uid, attribute)
func Key(uid uint64, attr string) []byte {
	buf := new(bytes.Buffer)
	buf.WriteString(attr)
	if err := binary.Write(buf, binary.LittleEndian, uid); err != nil {
		glog.Fatalf("Error while creating key with attr: %v uid: %v\n", attr, uid)
	}
	return buf.Bytes()
}

func newPosting(t x.DirectedEdge, op byte) []byte {
	b := flatbuffers.NewBuilder(0)
	var bo flatbuffers.UOffsetT
	if t.Value != nil {
		if t.ValueId != math.MaxUint64 {
			glog.Fatal("This should have already been set by the caller.")
		}
		bytes, err := json.Marshal(t.Value)
		if err != nil {
			glog.WithError(err).Fatal("Unable to marshal value")
			return []byte{}
		}
		bo = b.CreateByteVector(bytes)
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
	if t.Value != nil {
		if t.ValueId != math.MaxUint64 {
			glog.Fatal("This should have already been set by the caller.")
		}
		bytes, err := json.Marshal(t.Value)
		if err != nil {
			glog.WithError(err).Fatal("Unable to marshal value")
			return 0
		}
		bo = b.CreateByteVector(bytes)
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

	glog.Infof("Empty size: [%d] EmptyPosting size: [%d]",
		len(empty), len(emptyPosting))
}

func ParseValue(i *interface{}, value []byte) error {
	if len(value) == 0 {
		return errors.New("No value found in posting")
	}

	if len(value) == 1 && value[0] == 0x00 {
		i = nil
		return nil
	}

	return json.Unmarshal(value, i)
}

func (l *List) init(key []byte, pstore *store.Store, clog *commit.Logger) {
	l.Lock()
	defer l.Unlock()

	if len(empty) == 0 {
		glog.Fatal("empty should have some bytes.")
	}
	l.key = key
	l.pstore = pstore
	l.clog = clog

	var err error
	if l.buffer, err = pstore.Get(key); err != nil {
		// glog.Debugf("While retrieving posting list from db: %v\n", err)
		// Error. Just set to empty.
		l.buffer = make([]byte, len(empty))
		copy(l.buffer, empty)
	}

	posting := types.GetRootAsPostingList(l.buffer, 0)
	l.maxMutationTs = posting.CommitTs()
	l.hash = farm.Fingerprint32(key)

	ch := make(chan []byte, 100)
	done := make(chan error)
	glog.Debug("Starting stream entries...")
	go clog.StreamEntries(posting.CommitTs()+1, l.hash, ch, done)

	for buffer := range ch {
		uo := flatbuffers.GetUOffsetT(buffer)
		m := new(types.Posting)
		m.Init(buffer, uo)
		l.mergeWithList(m)
	}
	if err := <-done; err != nil {
		glog.WithError(err).Error("While streaming entries.")
	}
	glog.Debug("Done streaming entries.")
	if len(l.mutations) > 0 {
		// Commit Logs are always streamed in increasing ts order.
		l.maxMutationTs = l.mutations[len(l.mutations)-1].Ts()
	}

	l.regenerateIndex()
}

func findIndex(pl *types.PostingList, uid uint64, begin, end int) int {
	if begin > end {
		return -1
	}
	mid := (begin + end) / 2
	var pmid types.Posting
	if ok := pl.Postings(&pmid, mid); !ok {
		return -1
	}
	if uid < pmid.Uid() {
		return findIndex(pl, uid, begin, mid-1)
	}
	if uid > pmid.Uid() {
		return findIndex(pl, uid, mid+1, end)
	}
	return mid
}

// Caller must hold at least a read lock.
func (l *List) find(uid uint64) int {
	posting := types.GetRootAsPostingList(l.buffer, 0)
	return findIndex(posting, uid, 0, posting.PostingsLength())
}

// Caller must hold at least a read lock.
func (l *List) length() int {
	plist := types.GetRootAsPostingList(l.buffer, 0)
	return plist.PostingsLength() + l.mdelta
}

func (l *List) Length() int {
	l.RLock()
	defer l.RUnlock()
	return l.length()
}

func (l *List) Get(p *types.Posting, i int) bool {
	l.RLock()
	defer l.RUnlock()
	return l.get(p, i)
}

// Caller must hold at least a read lock.
func (l *List) get(p *types.Posting, i int) bool {
	plist := types.GetRootAsPostingList(l.buffer, 0)
	if l.mindex == nil {
		return plist.Postings(p, i)
	}

	// Iterate over mindex, and see if we have instructions
	// for the given index. Otherwise, sum up the move indexes
	// uptil the given index, so we know where to look in
	// mlayer and/or the main posting list.
	move := 0
	for e := l.mindex.Front(); e != nil; e = e.Next() {
		mlink := e.Value.(*MutationLink)
		if mlink.idx > i {
			break

		} else if mlink.idx == i {
			// Found an instruction. Check what is says.
			if mlink.posting.Op() == 0x01 {
				// ADD
				*p = mlink.posting
				return true

			} else if mlink.posting.Op() == 0x02 {
				// DELETE
				// The loop will break in the next iteration, after updating the move
				// variable.

			} else {
				glog.Fatal("Someone, I mean you, forgot to tackle" +
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

// mutationIndex is useful to avoid having to parse the entire postinglist
// upto idx, for every Get(*types.Posting, idx), which has a complexity
// of O(idx). Iteration over N size posting list would this push us into
// O(N^2) territory, without this technique.
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
func (l *List) regenerateIndex() {
	l.mindex = nil
	l.mdelta = 0
	l.mlayer = make(map[int]types.Posting)
	plist := types.GetRootAsPostingList(l.buffer, 0)
	if len(l.mutations) == 0 {
		return
	}
	sort.Sort(ByUid(l.mutations))

	mchain := linked.New()
	pi := 0
	pp := new(types.Posting)
	if ok := plist.Postings(pp, pi); !ok {
		// There's some weird padding before Posting starts. Get that padding.
		padding := flatbuffers.GetUOffsetT(emptyPosting)
		pp.Init(emptyPosting, padding)
		if pp.Uid() != 0 {
			glog.Fatal("Playing with bytes is like playing with fire." +
				" Someone got burnt today!")
		}
	}

	// The following algorithm is O(m + n), where m = number of mutations, and
	// n = number of immutable postings. This could be optimized
	// to O(1) with potentially more complexity. TODO: Look into that later.
	l.mdelta = 0
	for mi, mp := range l.mutations {
		// TODO: Consider converting to binary search later.
		for ; pi < plist.PostingsLength() && pp.Uid() < mp.Uid(); pi++ {
			plist.Postings(pp, pi)
		}

		mlink := new(MutationLink)
		mlink.posting = *mp

		if pp.Uid() == mp.Uid() {
			if mp.Op() == Set {
				// This is a replace, so store it in mlayer, instead of mindex.
				// Note that mlayer index is based right off the plist.
				l.mlayer[pi] = *mp

			} else if mp.Op() == Del {
				// This is a delete, so move the plist index forward.
				mlink.moveidx = 1
				l.mdelta -= 1

			} else {
				glog.Fatal("This operation isn't being handled.")
			}
		} else if mp.Uid() < pp.Uid() {
			// This is an add, so move the plist index backwards.
			mlink.moveidx = -1
			l.mdelta += 1

		} else {
			// mp.Uid() > pp.Uid()
			// We've crossed over from posting list. Posting List shouldn't be
			// consulted in this case, so moveidx wouldn't be used. Just set it
			// to zero anyways, to represent that.
			mlink.moveidx = 0
			l.mdelta += 1
		}

		mlink.idx = pi + mi
		mchain.PushBack(mlink)
	}
	l.mindex = mchain
}

func (l *List) addIfValid(b *flatbuffers.Builder,
	offsets *[]flatbuffers.UOffsetT, t x.DirectedEdge, op byte) {

	if op == Del {
		if fi := l.find(t.ValueId); fi >= 0 {
			// Delete. Only add it to the list if it exists in the posting list.
			*offsets = append(*offsets, addEdgeToPosting(b, t, op))
		}
	} else {
		*offsets = append(*offsets, addEdgeToPosting(b, t, op))
	}
}

// Assumes a lock has already been acquired.
func (l *List) mergeWithList(mpost *types.Posting) {
	// If this mutation is a deletion, then check if there's a valid uid entry
	// in the immutable postinglist. If not, we can ignore this mutation.
	ignore := false
	if mpost.Op() == Del {
		if fi := l.find(mpost.Uid()); fi < 0 {
			ignore = true
		}
	}

	// Check if we already have a mutation entry with the same uid.
	// If so, ignore (in case of del)/replace it. Otherwise, append it.
	handled := false
	final := l.mutations[:0]
	for _, mut := range l.mutations {
		// mut := &l.mutations[i]
		if mpost.Uid() != mut.Uid() {
			final = append(final, mut)
			continue
		}

		handled = true
		if ignore {
			// Don't add to final.
		} else {
			final = append(final, mpost) // replaced original.
		}
	}
	if handled {
		l.mutations = final
	} else {
		l.mutations = append(l.mutations, mpost)
	}
}

func (l *List) AddMutation(t x.DirectedEdge, op byte) error {
	l.Lock()
	defer l.Unlock()

	if t.Timestamp.UnixNano() < l.maxMutationTs {
		return fmt.Errorf("Mutation ts lower than committed ts.")
	}

	// Mutation arrives:
	// - Check if we had any(SET/DEL) before this, stored in the mutation list.
	//		- If yes, then replace that mutation. Jump to a)
	// a)		check if the entity exists in main posting list.
	// 				- If yes, store the mutation.
	// 				- If no, disregard this mutation.

	// All edges with a value set, have the same uid. In other words,
	// an (entity, attribute) can only have one interface{} value.
	if t.Value != nil {
		t.ValueId = math.MaxUint64
	}
	mbuf := newPosting(t, op)
	uo := flatbuffers.GetUOffsetT(mbuf)
	mpost := new(types.Posting)
	mpost.Init(mbuf, uo)

	l.mergeWithList(mpost)
	l.regenerateIndex()
	l.maxMutationTs = t.Timestamp.UnixNano()
	return l.clog.AddLog(t.Timestamp.UnixNano(), l.hash, mbuf)
}

func addOrSet(ll *linked.List, p *types.Posting) {
	added := false
	for e := ll.Front(); e != nil; e = e.Next() {
		pe := e.Value.(*types.Posting)
		if pe == nil {
			glog.Fatal("Posting shouldn't be nil!")
		}

		if !added && pe.Uid() > p.Uid() {
			ll.InsertBefore(p, e)
			added = true

		} else if pe.Uid() == p.Uid() {
			added = true
			e.Value = p
		}
	}
	if !added {
		ll.PushBack(p)
	}
}

func remove(ll *linked.List, p *types.Posting) {
	for e := ll.Front(); e != nil; e = e.Next() {
		pe := e.Value.(*types.Posting)
		if pe.Uid() == p.Uid() {
			ll.Remove(e)
		}
	}
}

func (l *List) generateLinkedList() (*linked.List, int64) {
	plist := types.GetRootAsPostingList(l.buffer, 0)
	ll := linked.New()

	var maxTs int64
	for i := 0; i < plist.PostingsLength(); i++ {
		p := new(types.Posting)
		plist.Postings(p, i)
		if maxTs < p.Ts() {
			maxTs = p.Ts()
		}

		ll.PushBack(p)
	}

	// Now go through mutations
	for _, p := range l.mutations {
		if maxTs < p.Ts() {
			maxTs = p.Ts()
		}

		if p.Op() == 0x01 {
			// Set/Add
			addOrSet(ll, p)

		} else if p.Op() == 0x02 {
			// Delete
			remove(ll, p)

		} else {
			glog.Fatalf("Strange mutation: %+v", p)
		}
	}

	return ll, maxTs
}

func (l *List) isDirty() bool {
	l.RLock()
	defer l.RUnlock()
	return l.mindex != nil
}

func (l *List) CommitIfDirty() error {
	if !l.isDirty() {
		glog.WithField("dirty", false).Debug("Not Committing")
		return nil
	} else {
		glog.WithField("dirty", true).Debug("Committing")
	}

	l.Lock()
	defer l.Unlock()

	ll, commitTs := l.generateLinkedList()

	b := flatbuffers.NewBuilder(0)

	var offsets []flatbuffers.UOffsetT
	for e := ll.Front(); e != nil; e = e.Next() {
		p := e.Value.(*types.Posting)
		off := addPosting(b, *p)
		offsets = append(offsets, off)
	}

	types.PostingListStartPostingsVector(b, ll.Len())
	for i := len(offsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(offsets[i])
	}
	vend := b.EndVector(ll.Len())

	types.PostingListStart(b)
	types.PostingListAddPostings(b, vend)
	types.PostingListAddCommitTs(b, commitTs)
	end := types.PostingListEnd(b)
	b.Finish(end)

	l.buffer = b.Bytes[b.Head():]
	if err := l.pstore.SetOne(l.key, l.buffer); err != nil {
		glog.WithField("error", err).Errorf("While storing posting list")
		return err
	}
	l.mutations = nil

	l.regenerateIndex()
	return nil
}

func (l *List) GetUids() []uint64 {
	l.RLock()
	defer l.RUnlock()

	result := make([]uint64, l.length())
	result = result[:0]
	var p types.Posting
	for i := 0; i < l.length(); i++ {
		if ok := l.get(&p, i); !ok || p.Uid() == math.MaxUint64 {
			break
		}
		result = append(result, p.Uid())
	}
	return result
}

func (l *List) Value() (result []byte, rerr error) {
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
