// automatically generated, do not modify

package task

import (
	flatbuffers "github.com/google/flatbuffers/go"
)
type GroupKeys struct {
	_tab flatbuffers.Table
}

func (rcv *GroupKeys) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *GroupKeys) Groupid() uint64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.GetUint64(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *GroupKeys) Keys(obj *KC, j int) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		x := rcv._tab.Vector(o)
		x += flatbuffers.UOffsetT(j) * 4
		x = rcv._tab.Indirect(x)
	if obj == nil {
		obj = new(KC)
	}
		obj.Init(rcv._tab.Bytes, x)
		return true
	}
	return false
}

func (rcv *GroupKeys) KeysLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func GroupKeysStart(builder *flatbuffers.Builder) { builder.StartObject(2) }
func GroupKeysAddGroupid(builder *flatbuffers.Builder, groupid uint64) { builder.PrependUint64Slot(0, groupid, 0) }
func GroupKeysAddKeys(builder *flatbuffers.Builder, keys flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(keys), 0) }
func GroupKeysStartKeysVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT { return builder.StartVector(4, numElems, 4)
}
func GroupKeysEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
