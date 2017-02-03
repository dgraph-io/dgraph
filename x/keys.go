package x

import (
	"encoding/binary"
	"math"
)

const (
	byteData    = byte(0x00)
	byteIndex   = byte(0x01)
	byteReverse = byte(0x02)
	byteSchema  = byte(0x03)
)

func writeAttr(buf []byte, attr string) []byte {
	AssertTrue(len(attr) < math.MaxUint16)
	binary.BigEndian.PutUint16(buf[:2], uint16(len(attr)))

	rest := buf[2:]
	AssertTrue(len(attr) == copy(rest, attr[:]))

	return rest[len(attr):]
}

func SchemaKey(attr string) []byte {
	buf := make([]byte, 1+len(attr))

	buf[0] = byteSchema

	rest := buf[1:]
	AssertTrue(len(attr) == copy(rest, attr[:]))
	return buf
}

func DataKey(attr string, uid uint64) []byte {
	buf := make([]byte, 2+len(attr)+1+8)

	rest := writeAttr(buf, attr)
	rest[0] = byteData

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
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

func IndexKey(attr, term string) []byte {
	buf := make([]byte, 2+len(attr)+1+len(term))

	rest := writeAttr(buf, attr)
	rest[0] = byteIndex

	rest = rest[1:]
	AssertTrue(len(term) == copy(rest, term[:]))
	return buf
}

type ParsedKey struct {
	byteType byte
	Attr     string
	Uid      uint64
	Term     string
}

func (p ParsedKey) IsData() bool {
	return p.byteType == byteData
}

func (p ParsedKey) IsReverse() bool {
	return p.byteType == byteReverse
}

func (p ParsedKey) IsIndex() bool {
	return p.byteType == byteIndex
}

func (p ParsedKey) SkipPredicate() []byte {
	buf := make([]byte, 2+len(p.Attr)+1)
	k := writeAttr(buf, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = 0xFF
	return buf
}

func (p ParsedKey) SkipRangeOfSameType() []byte {
	buf := make([]byte, 2+len(p.Attr)+1)
	k := writeAttr(buf, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = p.byteType + 1
	return buf
}

// DataPrefix returns the prefix for data keys.
func (p ParsedKey) DataPrefix() []byte {
	buf := make([]byte, 2+len(p.Attr)+1)
	k := writeAttr(buf, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = byteData
	return buf
}

// IndexPrefix returns the prefix for index keys.
func (p ParsedKey) IndexPrefix() []byte {
	buf := make([]byte, 2+len(p.Attr)+1)
	k := writeAttr(buf, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = byteIndex
	return buf
}

// SchemaPrefix returns the prefix for Schema keys.
func SchemaPrefix() []byte {
	buf := make([]byte, 1)
	buf[0] = byteSchema
	return buf
}

func ParseSchemaKey(key []byte) string {
	k := key[1:]
	return string(k)
}

func Parse(key []byte) *ParsedKey {
	p := &ParsedKey{}

	sz := int(binary.BigEndian.Uint16(key[0:2]))
	k := key[2:]

	p.Attr = string(k[:sz])
	k = k[sz:]

	p.byteType = k[0]
	k = k[1:]

	switch p.byteType {
	case byteData:
		fallthrough
	case byteReverse:
		p.Uid = binary.BigEndian.Uint64(k)
	case byteIndex:
		p.Term = string(k)
	default:
		// Some other data type.
		return nil
	}
	return p
}
