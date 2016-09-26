// automatically generated, do not modify

package task

import (
	flatbuffers "github.com/google/flatbuffers/go"
)
type KT struct {
	_tab flatbuffers.Table
}

func (rcv *KT) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *KT) Key(j int) byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.GetByte(a + flatbuffers.UOffsetT(j * 1))
	}
	return 0
}

func (rcv *KT) KeyLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *KT) KeyBytes() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *KT) Ts() int64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.GetInt64(o + rcv._tab.Pos)
	}
	return 0
}

func KTStart(builder *flatbuffers.Builder) { builder.StartObject(2) }
func KTAddKey(builder *flatbuffers.Builder, key flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(key), 0) }
func KTStartKeyVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT { return builder.StartVector(1, numElems, 1)
}
func KTAddTs(builder *flatbuffers.Builder, ts int64) { builder.PrependInt64Slot(1, ts, 0) }
func KTEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
