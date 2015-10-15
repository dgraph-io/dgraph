// automatically generated, do not modify

package types

import (
	flatbuffers "github.com/google/flatbuffers/go"
)
type PostingList struct {
	_tab flatbuffers.Table
}

func GetRootAsPostingList(buf []byte, offset flatbuffers.UOffsetT) *PostingList {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &PostingList{}
	x.Init(buf, n + offset)
	return x
}

func (rcv *PostingList) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *PostingList) Postings(obj *Posting, j int) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		x := rcv._tab.Vector(o)
		x += flatbuffers.UOffsetT(j) * 4
		x = rcv._tab.Indirect(x)
	if obj == nil {
		obj = new(Posting)
	}
		obj.Init(rcv._tab.Bytes, x)
		return true
	}
	return false
}

func (rcv *PostingList) PostingsLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func PostingListStart(builder *flatbuffers.Builder) { builder.StartObject(1) }
func PostingListAddPostings(builder *flatbuffers.Builder, postings flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(postings), 0) }
func PostingListStartPostingsVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT { return builder.StartVector(4, numElems, 4)
}
func PostingListEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
