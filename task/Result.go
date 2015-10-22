// automatically generated, do not modify

package task

import (
	flatbuffers "github.com/google/flatbuffers/go"
)
type Result struct {
	_tab flatbuffers.Table
}

func (rcv *Result) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Result) Uids(j int) uint64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.GetUint64(a + flatbuffers.UOffsetT(j * 8))
	}
	return 0
}

func (rcv *Result) UidsLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *Result) Values(obj *Value, j int) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		x := rcv._tab.Vector(o)
		x += flatbuffers.UOffsetT(j) * 4
		x = rcv._tab.Indirect(x)
	if obj == nil {
		obj = new(Value)
	}
		obj.Init(rcv._tab.Bytes, x)
		return true
	}
	return false
}

func (rcv *Result) ValuesLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func ResultStart(builder *flatbuffers.Builder) { builder.StartObject(2) }
func ResultAddUids(builder *flatbuffers.Builder, uids flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(uids), 0) }
func ResultStartUidsVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT { return builder.StartVector(8, numElems, 8)
}
func ResultAddValues(builder *flatbuffers.Builder, values flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(values), 0) }
func ResultStartValuesVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT { return builder.StartVector(4, numElems, 4)
}
func ResultEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
