// automatically generated, do not modify

package task

import (
	flatbuffers "github.com/google/flatbuffers/go"
)
type Group struct {
	_tab flatbuffers.Table
}

func (rcv *Group) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Group) Groupid() uint64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.GetUint64(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Group) Keys(obj *KT, j int) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		x := rcv._tab.Vector(o)
		x += flatbuffers.UOffsetT(j) * 4
		x = rcv._tab.Indirect(x)
	if obj == nil {
		obj = new(KT)
	}
		obj.Init(rcv._tab.Bytes, x)
		return true
	}
	return false
}

func (rcv *Group) KeysLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func GroupStart(builder *flatbuffers.Builder) { builder.StartObject(2) }
func GroupAddGroupid(builder *flatbuffers.Builder, groupid uint64) { builder.PrependUint64Slot(0, groupid, 0) }
func GroupAddKeys(builder *flatbuffers.Builder, keys flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(keys), 0) }
func GroupStartKeysVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT { return builder.StartVector(4, numElems, 4)
}
func GroupEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
