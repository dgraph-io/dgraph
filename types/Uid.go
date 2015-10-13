// automatically generated, do not modify

package types

import (
	flatbuffers "github.com/google/flatbuffers/go"
)
type Uid struct {
	_tab flatbuffers.Table
}

func GetRootAsUid(buf []byte, offset flatbuffers.UOffsetT) *Uid {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &Uid{}
	x.Init(buf, n + offset)
	return x
}

func (rcv *Uid) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Uid) Id() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *Uid) Name() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func UidStart(builder *flatbuffers.Builder) { builder.StartObject(2) }
func UidAddId(builder *flatbuffers.Builder, id flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(id), 0) }
func UidAddName(builder *flatbuffers.Builder, name flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(name), 0) }
func UidEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
