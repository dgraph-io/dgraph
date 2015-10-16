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
	"github.com/google/flatbuffers/go"
	"github.com/manishrjain/dgraph/posting/types"
	"github.com/manishrjain/dgraph/x"

	linked "container/list"
)

var log = x.Log("posting")

const Set = 0x01
const Del = 0x02

type List struct {
	buffer    []byte
	mutations []byte
}

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

func (l *List) Init() {
	if len(empty) == 0 {
		b := flatbuffers.NewBuilder(0)
		types.PostingListStart(b)
		of := types.PostingListEnd(b)
		b.Finish(of)
		empty = b.Bytes[b.Head():]
	}
	l.buffer = make([]byte, len(empty))
	l.mutations = make([]byte, len(empty))
	copy(l.buffer, empty)
	copy(l.mutations, empty)
}

func (l *List) Root() *types.PostingList {
	return types.GetRootAsPostingList(l.buffer, 0)
}

func (l *List) AddMutation(t x.Triple, op byte) {
	b := flatbuffers.NewBuilder(0)
	muts := types.GetRootAsPostingList(l.mutations, 0)
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

	l.mutations = b.Bytes[b.Head():]
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

func (l *List) GenerateLinkedList() *linked.List {
	plist := types.GetRootAsPostingList(l.buffer, 0)
	ll := linked.New()

	for i := 0; i < plist.PostingsLength(); i++ {
		p := new(types.Posting)
		plist.Postings(p, i)

		ll.PushBack(p)
	}

	mlist := types.GetRootAsPostingList(l.mutations, 0)
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

func (l *List) Commit() {
	ll := l.GenerateLinkedList()
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
	l.mutations = make([]byte, len(empty))
	copy(l.mutations, empty)
}
