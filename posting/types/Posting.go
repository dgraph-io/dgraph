// automatically generated, do not modify

package types

import (
	flatbuffers "github.com/google/flatbuffers/go"
)
type Posting struct {
	_tab flatbuffers.Table
}

func (rcv *Posting) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Posting) Uid() uint64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.GetUint64(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Posting) Source() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *Posting) Ts() int64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.GetInt64(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Posting) Op() byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(10))
	if o != 0 {
		return rcv._tab.GetByte(o + rcv._tab.Pos)
	}
	return 0
}

func PostingStart(builder *flatbuffers.Builder) { builder.StartObject(4) }
func PostingAddUid(builder *flatbuffers.Builder, uid uint64) { builder.PrependUint64Slot(0, uid, 0) }
func PostingAddSource(builder *flatbuffers.Builder, source flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(source), 0) }
func PostingAddTs(builder *flatbuffers.Builder, ts int64) { builder.PrependInt64Slot(2, ts, 0) }
func PostingAddOp(builder *flatbuffers.Builder, op byte) { builder.PrependByteSlot(3, op, 0) }
func PostingEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
