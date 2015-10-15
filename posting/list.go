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
)

type List struct {
	TList *types.PostingList
}

func addTripleToPosting(b *flatbuffers.Builder,
	t x.Triple) flatbuffers.UOffsetT {

	// Do this before posting start.
	so := b.CreateString(t.Source)
	types.PostingStart(b)
	types.PostingAddUid(b, t.ValueId)

	// so := b.CreateString(t.Source)
	types.PostingAddSource(b, so)

	types.PostingAddTs(b, t.Timestamp.UnixNano())
	return types.PostingEnd(b)
}

func addPosting(b *flatbuffers.Builder, p types.Posting) flatbuffers.UOffsetT {
	// Do this before posting start.
	so := b.CreateByteString(p.Source())

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

	l.TList = types.GetRootAsPostingList(b.Bytes, b.Head())
}

func (l *List) AddTriple(t x.Triple) {
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

	l.TList = types.GetRootAsPostingList(b.Bytes, b.Head())
}
