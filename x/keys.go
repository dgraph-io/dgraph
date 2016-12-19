package x

import (
	"encoding/binary"
	"math"
)

const (
	byteData    = byte(0x00)
	byteIndex   = byte(0x01)
	byteReverse = byte(0x02)
)

func writeAttr(buf []byte, attr string) []byte {
	AssertTrue(len(attr) < math.MaxUint16)
	binary.BigEndian.PutUint16(buf[:2], uint16(len(attr)))

	rest := buf[2:]
	AssertTrue(len(attr) == copy(rest, attr[:]))

	return rest[len(attr):]
}

func DataKey(attr string, uid uint64) []byte {
	buf := make([]byte, 2+len(attr)+1+8)

	rest := writeAttr(buf, attr)
	rest[0] = byteData

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

func IndexKey(attr, term string) []byte {
	buf := make([]byte, 2+len(attr)+1+len(term))

	rest := writeAttr(buf, attr)
	rest[0] = byteIndex

	rest = rest[1:]
	AssertTrue(len(term) == copy(rest, term[:]))
	return buf
}

func ReverseKey(attr string, uid uint64) []byte {
	buf := make([]byte, 2+len(attr)+1+8)

	rest := writeAttr(buf, attr)
	rest[0] = byteReverse

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

type ParsedKey struct {
	byteType byte
	Attr     string
	Uid      uint64
	Term     string
}

func (p ParsedKey) IsIndex() bool {
	return p.byteType == byteIndex
}

func (p ParsedKey) IsData() bool {
	return p.byteType == byteData
}

func (p ParsedKey) SkipPredicate() []byte {
	buf := make([]byte, 2+len(p.Attr)+1)
	k := writeAttr(buf, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = 0xFF
	return buf
}

func (p ParsedKey) SkipRangeOfSameType() []byte {
	buf := make([]byte, 2+len(p.Attr)+1+1)
	k := writeAttr(buf, p.Attr)
	AssertTrue(len(k) == 2)
	k[0] = p.byteType
	k[1] = 0xFF
	return buf
}

func (p ParsedKey) IndexPrefix() []byte {
	buf := make([]byte, 2+len(p.Attr)+1)
	k := writeAttr(buf, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = byteIndex
	return buf
}

func Parse(key []byte) *ParsedKey {
	p := &ParsedKey{}

	sz := int(binary.BigEndian.Uint16(key[0:2]))
	k := key[2:]

	p.Attr = string(k[:sz])
	k = k[sz:]

	p.byteType = k[0]
	k = k[1:]

	if p.byteType == byteData {
		p.Uid = binary.BigEndian.Uint64(k)

	} else if p.byteType == byteIndex {
		p.Term = string(k)

	} else {
		// Some other data type.
		return nil
	}
	return p
}
