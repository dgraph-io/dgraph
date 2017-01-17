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

// attrPrefix returns the prefix with only the attribute and pluginPrefix and given
// number of extra bytes. Returns the full buffer and the buffer with "extra" bytes.
func attrPrefix(attr, pluginPrefix string, extra int) ([]byte, []byte) {
	buf := make([]byte, len(pluginPrefix)+len(attr)+2+extra)
	x.AssertTrue(len(pluginPrefix) == copy(buf, pluginPrefix[:]))
	k := writeAttr(buf[len(pluginPrefix):], attr)
	x.AssertTrue(len(k) == extra)
	return buf, k
}

// DataKey returns the data key (not index, not reversed edge keys).
func DataKey(attr string, uid uint64, pluginContexts []string) []byte {
	pluginPrefix := plugin.Prefix(pluginContexts)
	buf, rest := attrPrefix(attr, pluginPrefix, 1+8)
	rest[0] = byteData
	binary.BigEndian.PutUint64(rest[1:], uid)
	return buf
}

// ReverseKey returns the key for reverse edges.
func ReverseKey(attr string, uid uint64, pluginContexts []string) []byte {
	pluginPrefix := plugin.Prefix(pluginContexts)
	buf, rest := attrPrefix(attr, pluginPrefix, 1+8)
	rest[0] = byteReverse
	binary.BigEndian.PutUint64(rest[1:], uid)
	return buf
}

// IndexKey returns the key for index edges.
func IndexKey(attr, term string, pluginContexts []string) []byte {
	pluginPrefix := plugin.Prefix(pluginContexts)
	buf, rest := attrPrefix(attr, pluginPrefix, 1+len(term))
	rest[0] = byteIndex
	x.AssertTrue(len(term) == copy(rest[1:], term[:]))
	return buf
}

// ParsedKey is the key parsed into different components.
type ParsedKey struct {
	byteType     byte
	PluginPrefix string
	Attr         string
	Uid          uint64
	Term         string
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
	buf, k := attrPrefix(p.Attr, p.PluginPrefix, 1)
	k[0] = 0xFF
	return buf
}

func (p ParsedKey) SkipRangeOfSameType() []byte {
	buf, k := attrPrefix(p.Attr, p.PluginPrefix, 1)
	k[0] = p.byteType + 1
	return buf
}

// DataPrefix returns the prefix for data keys.
func (p ParsedKey) DataPrefix() []byte {
	buf, k := attrPrefix(p.Attr, p.PluginPrefix, 1)
	k[0] = byteData
	return buf
}

// IndexPrefix returns the prefix for index keys.
func (p ParsedKey) IndexPrefix() []byte {
	buf, k := attrPrefix(p.Attr, p.PluginPrefix, 1)
	k[0] = byteIndex
	return buf
}

// Parse parses the key in data store.
func Parse(key []byte) *ParsedKey {
	prefixLen := plugin.PrefixLen()
	p := &ParsedKey{
		PluginPrefix: string(key[:prefixLen]),
	}
	key = key[prefixLen:]
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
