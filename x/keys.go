package x

import (
	"encoding/binary"
	"math"
)

const (
	byteData  = byte(0x00)
	byteIndex = byte(0x01)
)

func DataKey(attr string, uid uint64) []byte {
	buf := make([]byte, 3+len(attr)+8)
	buf[0] = byteData
	AssertTrue(len(attr) < math.MaxUint16)
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(attr)))

	rest := buf[3:]
	AssertTrue(len(attr) == copy(rest, attr[:]))
	rest = rest[len(attr):]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

func IndexKey(attr, term string) []byte {
	buf := make([]byte, 3+len(attr)+len(term))
	buf[0] = byteIndex
	AssertTrue(len(attr) < math.MaxUint16)
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(attr)))

	rest := buf[3:]
	AssertTrue(len(attr) == copy(rest, attr[:]))

	rest = rest[len(attr):]
	AssertTrue(len(term) == copy(rest, term[:]))
	return buf
}

func IsData(key []byte) bool {
	return key[0] == byteData
}

func IsIndex(key []byte) bool {
	return key[0] == byteIndex
}

func TermFromIndex(key []byte) string {
	AssertTruef(key[0] == byteIndex, "Expected an index key. Got key %q", key)
	sz := binary.BigEndian.Uint16(key[1:3])
	k := key[3+int(sz):]
	return string(k)
}

func PredicateFrom(key []byte) string {
	sz := binary.BigEndian.Uint16(key[1:3])
	k := key[3:]
	return string(k[:int(sz)])
}

func ParseData(key []byte) (string, uint64) {
	AssertTruef(key[0] == byteData, "Expected a data key. Got key %q", key)
	sz := binary.BigEndian.Uint16(key[1:3])
	AssertTruef(sz > 0, "Expected an attribute of size greater than zero. Key: %q", key)

	k := key[3:]
	attr := string(k[:int(sz)])
	k = k[int(sz):]
	uid := binary.BigEndian.Uint64(k)
	return attr, uid
}
