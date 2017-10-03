package x

import (
	"encoding/binary"
)

type ProtoMessage interface {
	Size() int
	MarshalTo([]byte) (int, error)
}

func AppendProtoMsg(p []byte, msg ProtoMessage) ([]byte, error) {
	sz := msg.Size()
	p = ReserveCap(p, len(p)+sz)
	buf := p[len(p) : len(p)+sz]
	n, err := msg.MarshalTo(buf)
	AssertTrue(sz == n)
	return p[:len(p)+sz], err
}

func AppendUvarint(p []byte, x uint64) []byte {
	p = ReserveCap(p, len(p)+binary.MaxVarintLen64)
	buf := p[len(p) : len(p)+binary.MaxVarintLen64]
	n := binary.PutUvarint(buf, x)
	return p[:len(p)+n]
}

func ReserveCap(p []byte, atLeast int) []byte {
	if cap(p) >= atLeast {
		return p
	}
	newCap := cap(p) * 2
	if newCap < atLeast {
		newCap = atLeast
	}
	newP := make([]byte, len(p), newCap)
	copy(newP, p)
	return newP
}
