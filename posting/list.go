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
	"encoding/gob"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/google/flatbuffers/go"
	"github.com/manishrjain/dgraph/posting/types"
	"github.com/manishrjain/dgraph/store"
	"github.com/manishrjain/dgraph/x"

	linked "container/list"
)

var log = x.Log("posting")

const Set = 0x01
const Del = 0x02

type MutationLink struct {
	idx     int
	moveidx int
	posting types.Posting
}

type List struct {
	key     []byte
	mutex   sync.RWMutex
	buffer  []byte
	mbuffer []byte
	pstore  *store.Store // postinglist store
	mstore  *store.Store // mutation store

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
		log.Fatalf("Error while creating key with attr: %v uid: %v\n", attr, uid)
	}
	return buf.Bytes()
}

func addTripleToPosting(b *flatbuffers.Builder,
	t x.Triple, op byte) flatbuffers.UOffsetT {

	var bo flatbuffers.UOffsetT
	if t.Value != nil {
		if t.ValueId != math.MaxUint64 {
			log.Fatal("This should have already been set by the caller.")
		}
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		if err := enc.Encode(t.Value); err != nil {
			x.Err(log, err).Fatal("Unable to encode interface")
			return 0
		}
		bo = b.CreateByteVector(buf.Bytes())
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

	log.Infof("Empty size: [%d] EmptyPosting size: [%d]",
		len(empty), len(emptyPosting))
}

func ParseValue(i interface{}, value []byte) error {
	if len(value) == 0 {
		return errors.New("No value found in posting")
	}
	var buf bytes.Buffer
	buf.Write(value)
	dec := gob.NewDecoder(&buf)
	return dec.Decode(i)
}

func (l *List) init(key []byte, pstore, mstore *store.Store) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if len(empty) == 0 {
		log.Fatal("empty should have some bytes.")
	}
	l.key = key
	l.pstore = pstore
	l.mstore = mstore

	var err error
	if l.buffer, err = pstore.Get(key); err != nil {
		// log.Debugf("While retrieving posting list from db: %v\n", err)
		// Error. Just set to empty.
		l.buffer = make([]byte, len(empty))
		copy(l.buffer, empty)
	}

	if l.mbuffer, err = mstore.Get(key); err != nil {
		// log.Debugf("While retrieving mutation list from db: %v\n", err)
		// Error. Just set to empty.
		l.mbuffer = make([]byte, len(empty))
		copy(l.mbuffer, empty)
	}
	l.generateIndex()
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
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.length()
}

func (l *List) Get(p *types.Posting, i int) bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
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
// generateIndex would generate these:
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
func (l *List) generateIndex() {
	l.mindex = nil
	l.mdelta = 0
	l.mlayer = make(map[int]types.Posting)
	mlist := types.GetRootAsPostingList(l.mbuffer, 0)
	plist := types.GetRootAsPostingList(l.buffer, 0)
	if mlist.PostingsLength() == 0 {
		return
	}
	var muts []*types.Posting
	for i := 0; i < mlist.PostingsLength(); i++ {
		var mp types.Posting
		mlist.Postings(&mp, i)
		muts = append(muts, &mp)
	}
	sort.Sort(ByUid(muts))

	mchain := linked.New()
	pi := 0
	pp := new(types.Posting)
	if ok := plist.Postings(pp, pi); !ok {
		// There's some weird padding before Posting starts. Get that padding.
		padding := flatbuffers.GetUOffsetT(emptyPosting)
		pp.Init(emptyPosting, padding)
		if pp.Uid() != 0 {
			log.Fatal("Playing with bytes is like playing with fire." +
				" Someone got burnt today!")
		}
	}

	l.mdelta = 0
	for mi, mp := range muts {
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
				log.Fatal("This operation isn't being handled.")
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
	offsets *[]flatbuffers.UOffsetT, t x.Triple, op byte) {

	if op == Del {
		if fi := l.find(t.ValueId); fi >= 0 {
			// Delete. Only add it to the list if it exists in the posting list.
			*offsets = append(*offsets, addTripleToPosting(b, t, op))
		}
	} else {
		*offsets = append(*offsets, addTripleToPosting(b, t, op))
	}
}

func (l *List) AddMutation(t x.Triple, op byte) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Mutation arrives:
	// - Check if we had any(SET/DEL) before this, stored in the mutation list.
	//		- If yes, then replace that mutation. Jump to a)
	// a)		check if the entity exists in main posting list.
	// 				- If yes, store the mutation.
	// 				- If no, disregard this mutation.

	// All triples with a value set, have the same uid. In other words,
	// an (entity, attribute) can only have one interface{} value.
	if t.Value != nil {
		t.ValueId = math.MaxUint64
	}

	b := flatbuffers.NewBuilder(0)
	muts := types.GetRootAsPostingList(l.mbuffer, 0)
	var offsets []flatbuffers.UOffsetT
	added := false
	for i := 0; i < muts.PostingsLength(); i++ {
		var p types.Posting
		if ok := muts.Postings(&p, i); !ok {
			log.Fatal("While reading posting")
			return errors.New("Error reading posting")
		}

		if p.Uid() != t.ValueId {
			offsets = append(offsets, addPosting(b, p))

		} else {
			// An operation on something we already have a mutation for.
			// Overwrite the previous one if it came earlier.
			if p.Ts() <= t.Timestamp.UnixNano() {
				l.addIfValid(b, &offsets, t, op)
			} // else keep the previous one.
			added = true
		}
	}
	if !added {
		l.addIfValid(b, &offsets, t, op)
	}

	types.PostingListStartPostingsVector(b, len(offsets))
	for i := len(offsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(offsets[i])
	}
	vend := b.EndVector(len(offsets))

	types.PostingListStart(b)
	types.PostingListAddPostings(b, vend)
	end := types.PostingListEnd(b)
	b.Finish(end)

	l.mbuffer = b.Bytes[b.Head():]
	l.generateIndex()
	return l.mstore.SetOne(l.key, l.mbuffer)
}

func addOrSet(ll *linked.List, p *types.Posting) {
	added := false
	for e := ll.Front(); e != nil; e = e.Next() {
		pe := e.Value.(*types.Posting)
		if pe == nil {
			log.Fatal("Posting shouldn't be nil!")
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

func (l *List) generateLinkedList() *linked.List {
	plist := types.GetRootAsPostingList(l.buffer, 0)
	ll := linked.New()

	for i := 0; i < plist.PostingsLength(); i++ {
		p := new(types.Posting)
		plist.Postings(p, i)

		ll.PushBack(p)
	}

	mlist := types.GetRootAsPostingList(l.mbuffer, 0)
	// Now go through mutations
	for i := 0; i < mlist.PostingsLength(); i++ {
		p := new(types.Posting)
		mlist.Postings(p, i)

		if p.Op() == 0x01 {
			// Set/Add
			addOrSet(ll, p)

		} else if p.Op() == 0x02 {
			// Delete
			remove(ll, p)

		} else {
			log.Fatalf("Strange mutation: %+v", p)
		}
	}

	return ll
}

func (l *List) isDirty() bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.mindex != nil
}

func (l *List) CommitIfDirty() error {
	if !l.isDirty() {
		log.WithField("dirty", false).Debug("Not Committing")
		return nil
	} else {
		log.WithField("dirty", true).Debug("Committing")
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	ll := l.generateLinkedList()
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
	end := types.PostingListEnd(b)
	b.Finish(end)

	l.buffer = b.Bytes[b.Head():]
	if err := l.pstore.SetOne(l.key, l.buffer); err != nil {
		log.WithField("error", err).Errorf("While storing posting list")
		return err
	}

	if err := l.mstore.Delete(l.key); err != nil {
		log.WithField("error", err).Errorf("While deleting mutation list")
		return err
	}
	l.mbuffer = make([]byte, len(empty))
	copy(l.mbuffer, empty)
	l.generateIndex()
	return nil
}

// This is a blocking function. It would block when the channel buffer capacity
// has been reached.
func (l *List) StreamUids(ch chan uint64) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	var p types.Posting
	for i := 0; i < l.length(); i++ {
		if ok := l.get(&p, i); !ok || p.Uid() == math.MaxUint64 {
			break
		}
		ch <- p.Uid()
	}
	close(ch)
}

func (l *List) Value() (result []byte, rerr error) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

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
