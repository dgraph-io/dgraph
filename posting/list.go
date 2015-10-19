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
	dirty   bool

	// mlayer keeps only replace instructions for the posting list.
	// This works at the
	mlayer map[int]types.Posting
	mindex *linked.List
}

type ByUid []*types.Posting

func (pa ByUid) Len() int           { return len(pa) }
func (pa ByUid) Swap(i, j int)      { pa[i], pa[j] = pa[j], pa[i] }
func (pa ByUid) Less(i, j int) bool { return pa[i].Uid() < pa[j].Uid() }

func addTripleToPosting(b *flatbuffers.Builder,
	t x.Triple, op byte) flatbuffers.UOffsetT {

	so := b.CreateString(t.Source) // Do this before posting start.
	types.PostingStart(b)
	types.PostingAddUid(b, t.ValueId)
	types.PostingAddSource(b, so)
	types.PostingAddTs(b, t.Timestamp.UnixNano())
	types.PostingAddOp(b, op)
	return types.PostingEnd(b)
}

func addPosting(b *flatbuffers.Builder, p types.Posting) flatbuffers.UOffsetT {
	so := b.CreateByteString(p.Source()) // Do this before posting start.
	types.PostingStart(b)
	types.PostingAddUid(b, p.Uid())
	types.PostingAddSource(b, so)
	types.PostingAddTs(b, p.Ts())
	types.PostingAddOp(b, p.Op())
	return types.PostingEnd(b)
}

var empty []byte

// package level init
func init() {
	b := flatbuffers.NewBuilder(0)
	types.PostingListStart(b)
	of := types.PostingListEnd(b)
	b.Finish(of)
	empty = b.Bytes[b.Head():]
}

func (l *List) Init(key []byte, pstore, mstore *store.Store) {
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
		log.Errorf("While retrieving posting list from db: %v\n", err)
		// Error. Just set to empty.
		l.buffer = make([]byte, len(empty))
		copy(l.buffer, empty)
	}

	if l.mbuffer, err = mstore.Get(key); err != nil {
		log.Debugf("While retrieving mutation list from db: %v\n", err)
		// Error. Just set to empty.
		l.mbuffer = make([]byte, len(empty))
		copy(l.mbuffer, empty)
	}
}

func (l *List) Length() int {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	plist := types.GetRootAsPostingList(l.buffer, 0)
	mlist := types.GetRootAsPostingList(l.mbuffer, 0)
	return plist.PostingsLength() + mlist.PostingsLength()
}

func (l *List) Get(p *types.Posting, i int) bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	plist := types.GetRootAsPostingList(l.buffer, 0)
	if l.mindex == nil {
		return plist.Postings(p, i)
	}

	if i >= plist.PostingsLength()+l.mindex.Len() {
		return false
	}
	count := 0
	for e := l.mindex.Front(); e != nil; e = e.Next() {
		mlink := e.Value.(*MutationLink)
		if mlink.idx > i {
			break

		} else if mlink.idx == i {
			*p = mlink.posting
			return true
		}
		count += 1
	}
	return plist.Postings(p, i-count)
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
// mutationIndex would generate these:
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
func (l *List) mutationIndex() *linked.List {
	mlist := types.GetRootAsPostingList(l.mbuffer, 0)
	plist := types.GetRootAsPostingList(l.buffer, 0)
	if mlist.PostingsLength() == 0 {
		return nil
	}
	var muts []*types.Posting
	for i := 0; i < mlist.PostingsLength(); i++ {
		var mp types.Posting
		mlist.Postings(&mp, i)
		muts = append(muts, &mp)
	}
	sort.Sort(ByUid(muts))

	// TODO: Convert to binary search once this works.
	mchain := linked.New()
	pi := 0
	var pp types.Posting
	plist.Postings(&pp, pi)

	for mi, mp := range muts {
		for ; pi < plist.PostingsLength() && pp.Uid() < mp.Uid(); pi++ {
			plist.Postings(&pp, pi)
		}

		mlink := new(MutationLink)
		mlink.posting = *mp

		if pp.Uid() == mp.Uid() {
			if mp.Op() == 0x01 {
				// This is a replace, so don't move the main index forward.
				mlink.moveidx = 0
			} else if mp.Op() == 0x02 {
				// This is a delete, so move the main index next.
				mlink.moveidx = 1
			} else {
				log.Fatal("This operation isn't being handled.")
			}
		} else if mp.Uid() < pp.Uid() {
			mlink.moveidx = -1
		}

		mlink.idx = pi + mi
		mchain.PushBack(mlink)
	}
	return mchain
}

func (l *List) AddMutation(t x.Triple, op byte) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.dirty = true // Mark as dirty.

	b := flatbuffers.NewBuilder(0)
	muts := types.GetRootAsPostingList(l.mbuffer, 0)
	var offsets []flatbuffers.UOffsetT
	for i := 0; i < muts.PostingsLength(); i++ {
		var p types.Posting
		if ok := muts.Postings(&p, i); !ok {
			log.Errorf("While reading posting")
		} else {
			offsets = append(offsets, addPosting(b, p))
		}
	}
	offsets = append(offsets, addTripleToPosting(b, t, op))

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
	l.mindex = l.mutationIndex()
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
	return l.dirty
}

func (l *List) CommitIfDirty() error {
	if !l.isDirty() {
		return nil
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
	return nil
}
