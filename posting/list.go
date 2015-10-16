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

type mutation struct {
	Set    types.Posting
	Delete types.Posting
}

type List struct {
	TList     *types.PostingList
	buffer    []byte
	mutations []mutation
}

func addTripleToPosting(b *flatbuffers.Builder,
	t x.Triple) flatbuffers.UOffsetT {

	so := b.CreateString(t.Source) // Do this before posting start.
	types.PostingStart(b)
	types.PostingAddUid(b, t.ValueId)
	types.PostingAddSource(b, so)
	types.PostingAddTs(b, t.Timestamp.UnixNano())
	return types.PostingEnd(b)
}

func addPosting(b *flatbuffers.Builder, p types.Posting) flatbuffers.UOffsetT {
	so := b.CreateByteString(p.Source()) // Do this before posting start.
	types.PostingStart(b)
	types.PostingAddUid(b, p.Uid())
	types.PostingAddSource(b, so)
	types.PostingAddTs(b, p.Ts())
	return types.PostingEnd(b)
}

func (l *List) Init() {
	b := flatbuffers.NewBuilder(0)
	types.PostingListStart(b)
	of := types.PostingListEnd(b)
	b.Finish(of)

	l.buffer = b.Bytes[b.Head():]
}

func (l *List) AddTriple(t x.Triple) {
	m := mutation{
		Add: t,
	}
}

func (l *List) Remove(t x.Triple) {

}

func addOrSet(ll *linked.List, m mutation) {
}

func remove(ll *linked.List, m mutation) {
	e := ll.Front()
	for e := ll.Front(); e != nil; e = e.Next() {
	}
}

func (l *List) GenerateLinkedList() *linked.List {
	plist := types.GetRootAsPostingList(l.Buffer, 0)
	ll := linked.New()

	for i := 0; i < plist.PostingsLength(); i++ {
		p := new(types.Posting)
		plist.Postings(p, i)

		ll.PushBack(p)
	}

	// Now go through mutations
	for i, m := range l.mutations {
		if m.Set.Ts > 0 {
			start := ll.Front
		} else if m.Delete.Ts > 0 {

		} else {
			log.Fatalf("Strange mutation: %+v", m)
		}
	}
}

func (l *List) Commit() {
	b := flatbuffers.NewBuilder(0)

	num := l.TList.PostingsLength()
	var offsets []flatbuffers.UOffsetT

	if num == 0 {
		offsets = append(offsets, addTripleToPosting(b, t))

	} else {
		added := false
		for i := 0; i < num; i++ {
			var p types.Posting
			l.TList.Postings(&p, i)

			// Put the triple just before the first posting which has a greater
			// uid than itself.
			if !added && p.Uid() > t.ValueId {
				offsets = append(offsets, addTripleToPosting(b, t))
				added = true
			}
			offsets = append(offsets, addPosting(b, p))
		}
		if !added {
			// t.ValueId is the largest. So, add at end.
			offsets = append(offsets, addTripleToPosting(b, t))
			added = true // useless, but consistent w/ behavior.
		}
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

	l.Buffer = b.Bytes[b.Head():]
	l.TList = types.GetRootAsPostingList(b.Bytes, b.Head())
}
