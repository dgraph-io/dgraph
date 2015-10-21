// automatically generated, do not modify

package result

import (
	flatbuffers "github.com/google/flatbuffers/go"
)
type Uids struct {
	_tab flatbuffers.Table
}

func GetRootAsUids(buf []byte, offset flatbuffers.UOffsetT) *Uids {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &Uids{}
	x.Init(buf, n + offset)
	return x
}

func (rcv *Uids) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Uids) Uid(j int) uint64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.GetUint64(a + flatbuffers.UOffsetT(j * 8))
	}
	return 0
}

func (rcv *Uids) UidLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func UidsStart(builder *flatbuffers.Builder) { builder.StartObject(1) }
func UidsAddUid(builder *flatbuffers.Builder, uid flatbuffers.UOffsetT) { builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(uid), 0) }
func UidsStartUidVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT { return builder.StartVector(8, numElems, 8)
}
func UidsEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
