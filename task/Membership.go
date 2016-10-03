// automatically generated by the FlatBuffers compiler, do not modify

package task

import (
	flatbuffers "github.com/google/flatbuffers/go"
)

type Membership struct {
	_tab flatbuffers.Table
}

func GetRootAsMembership(buf []byte, offset flatbuffers.UOffsetT) *Membership {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &Membership{}
	x.Init(buf, n+offset)
	return x
}

func (rcv *Membership) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Membership) Id() uint64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.GetUint64(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Membership) MutateId(n uint64) bool {
	return rcv._tab.MutateUint64Slot(4, n)
}

func (rcv *Membership) Group() uint32 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.GetUint32(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Membership) MutateGroup(n uint32) bool {
	return rcv._tab.MutateUint32Slot(6, n)
}

func (rcv *Membership) Addr() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *Membership) Leader() byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(10))
	if o != 0 {
		return rcv._tab.GetByte(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Membership) MutateLeader(n byte) bool {
	return rcv._tab.MutateByteSlot(10, n)
}

func (rcv *Membership) Amdead() byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(12))
	if o != 0 {
		return rcv._tab.GetByte(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Membership) MutateAmdead(n byte) bool {
	return rcv._tab.MutateByteSlot(12, n)
}

func MembershipStart(builder *flatbuffers.Builder) {
	builder.StartObject(5)
}
func MembershipAddId(builder *flatbuffers.Builder, id uint64) {
	builder.PrependUint64Slot(0, id, 0)
}
func MembershipAddGroup(builder *flatbuffers.Builder, group uint32) {
	builder.PrependUint32Slot(1, group, 0)
}
func MembershipAddAddr(builder *flatbuffers.Builder, addr flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(2, flatbuffers.UOffsetT(addr), 0)
}
func MembershipAddLeader(builder *flatbuffers.Builder, leader byte) {
	builder.PrependByteSlot(3, leader, 0)
}
func MembershipAddAmdead(builder *flatbuffers.Builder, amdead byte) {
	builder.PrependByteSlot(4, amdead, 0)
}
func MembershipEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT {
	return builder.EndObject()
}
