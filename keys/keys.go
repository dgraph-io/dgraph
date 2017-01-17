package keys

import (
	"encoding/binary"
	"math"

	"github.com/dgraph-io/dgraph/plugin"
	"github.com/dgraph-io/dgraph/x"
)

const (
	byteData    = byte(0x00)
	byteIndex   = byte(0x01)
	byteReverse = byte(0x02)
)

func writeAttr(buf []byte, attr string) []byte {
	x.AssertTrue(len(attr) < math.MaxUint16)
	binary.BigEndian.PutUint16(buf[:2], uint16(len(attr)))

	rest := buf[2:]
	x.AssertTrue(len(attr) == copy(rest, attr[:]))

	return rest[len(attr):]
}

// DataKey returns the data key (not index, not reversed edge keys).
func DataKey(attr string, uid uint64, pluginContexts []string) []byte {
	// It might be better for DataKey to use a bytes.Buffer instead of []byte and
	// just ask each plugin to write there.
	pluginPrefix := plugin.DataKeyPrefix(attr, uid, pluginContexts)
	buf := make([]byte, len(pluginPrefix)+2+len(attr)+1+8)
	x.AssertTrue(len(pluginPrefix) == copy(buf, pluginPrefix[:]))
	buf = buf[len(pluginPrefix):]

	rest := writeAttr(buf, attr)
	rest[0] = byteData

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

// ReverseKey returns the key for reverse edges.
func ReverseKey(attr string, uid uint64, pluginContexts []string) []byte {
	pluginPrefix := plugin.ReverseKeyPrefix(attr, uid, pluginContexts)
	buf := make([]byte, len(pluginPrefix)+2+len(attr)+1+8)
	x.AssertTrue(len(pluginPrefix) == copy(buf, pluginPrefix[:]))
	buf = buf[len(pluginPrefix):]

	rest := writeAttr(buf, attr)
	rest[0] = byteReverse

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

// IndexKey returns the key for index edges.
func IndexKey(attr, term string, pluginContexts []string) []byte {
	pluginPrefix := plugin.IndexKeyPrefix(attr, term, pluginContexts)
	buf := make([]byte, len(pluginPrefix)+2+len(attr)+1+len(term))
	x.AssertTrue(len(pluginPrefix) == copy(buf, pluginPrefix[:]))
	buf = buf[len(pluginPrefix):]

	rest := writeAttr(buf, attr)
	rest[0] = byteIndex

	rest = rest[1:]
	x.AssertTrue(len(term) == copy(rest, term[:]))
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
	x.AssertTrue(len(k) == 1)
	k[0] = 0xFF
	return buf
}

func (p ParsedKey) SkipRangeOfSameType() []byte {
	buf := make([]byte, 2+len(p.Attr)+1)
	k := writeAttr(buf, p.Attr)
	x.AssertTrue(len(k) == 1)
	k[0] = p.byteType + 1
	return buf
}

// DataPrefix returns the prefix for data keys.
func (p ParsedKey) DataPrefix() []byte {
	buf := make([]byte, 2+len(p.Attr)+1)
	k := writeAttr(buf, p.Attr)
	x.AssertTrue(len(k) == 1)
	k[0] = byteData
	return buf
}

// IndexPrefix returns the prefix for index keys.
func (p ParsedKey) IndexPrefix() []byte {
	buf := make([]byte, 2+len(p.Attr)+1)
	k := writeAttr(buf, p.Attr)
	x.AssertTrue(len(k) == 1)
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
