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

func (rcv *PostingList) Ids(j int) uint64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.GetUint64(a + flatbuffers.UOffsetT(j * 8))
	}
	return 0
}

func (rcv *PostingList) IdsLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func PostingListStart(builder *flatbuffers.Builder) { builder.StartObject(1) }
func PostingListAddIds(builder *flatbuffers.Builder, ids flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(ids), 0) }
func PostingListStartIdsVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT { return builder.StartVector(8, numElems, 8)
}
func PostingListEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
