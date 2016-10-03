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

func (rcv *PostingList) CommitTs() int64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.GetInt64(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *PostingList) Checksum() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *PostingList) Postings(obj *Posting, j int) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
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
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func PostingListStart(builder *flatbuffers.Builder) { builder.StartObject(3) }
func PostingListAddCommitTs(builder *flatbuffers.Builder, commitTs int64) { builder.PrependInt64Slot(0, commitTs, 0) }
func PostingListAddChecksum(builder *flatbuffers.Builder, checksum flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(checksum), 0) }
func PostingListAddPostings(builder *flatbuffers.Builder, postings flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(2, flatbuffers.UOffsetT(postings), 0) }
func PostingListStartPostingsVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT { return builder.StartVector(4, numElems, 4)
}
func PostingListEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
