/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

import (
	"encoding/binary"
	"math"
	"strings"

	"github.com/golang/glog"
)

const (
	// TODO(pawan) - Make this 2 bytes long. Right now ParsedKey has byteType and
	// bytePrefix. Change it so that it just has one field which has all the information.

	// ByteData indicates the key stores data.
	ByteData = byte(0x00)
	// ByteIndex indicates the key stores an index.
	ByteIndex = byte(0x02)
	// ByteReverse indicates the key stores a reverse index.
	ByteReverse = byte(0x04)
	// ByteCount indicates the key stores a count index.
	ByteCount = byte(0x08)
	// ByteCountRev indicates the key stores a reverse count index.
	ByteCountRev = ByteCount | ByteReverse
	// DefaultPrefix is the prefix used for data, index and reverse keys so that relative
	// order of data doesn't change keys of same attributes are located together.
	DefaultPrefix = byte(0x00)
	byteSchema    = byte(0x01)
	byteType      = byte(0x02)
	// ByteSplit is a constant to specify a given key corresponds to a posting list split
	// into multiple parts.
	ByteSplit = byte(0x01)
	// ByteUnused is a constant to specify keys which need to be discarded.
	ByteUnused = byte(0xff)
)

func writeAttr(buf []byte, attr string) []byte {
	AssertTrue(len(attr) < math.MaxUint16)
	binary.BigEndian.PutUint16(buf[:2], uint16(len(attr)))

	rest := buf[2:]
	AssertTrue(len(attr) == copy(rest, attr))

	return rest[len(attr):]
}

// genKey creates the key and writes the initial bytes (type byte, length of attribute,
// and the attribute itself). It leaves the rest of the key empty for further processing
// if necessary.
func generateKey(typeByte byte, attr string, totalLen int) []byte {
	AssertTrue(totalLen >= 1+2+len(attr))

	buf := make([]byte, totalLen)
	buf[0] = typeByte
	rest := buf[1:]

	writeAttr(rest, attr)
	return buf
}

// SchemaKey returns schema key for given attribute. Schema keys are stored
// separately with unique prefix, since we need to iterate over all schema keys.
// The structure of a schema key is as follows:
//
// byte 0: key type prefix (set to byteSchema)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
func SchemaKey(attr string) []byte {
	return generateKey(byteSchema, attr, 1+2+len(attr))
}

// TypeKey returns type key for given type name. Type keys are stored separately
// with a unique prefix, since we need to iterate over all type keys.
// The structure of a type key is as follows:
//
// byte 0: key type prefix (set to byteType)
// byte 1-2: length of typeName
// next len(attr) bytes: value of typeName
func TypeKey(typeName string) []byte {
	return generateKey(byteType, typeName, 1+2+len(typeName))
}

// DataKey generates a data key with the given attribute and UID.
// The structure of a data key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
// next byte: data type prefix (set to ByteData)
// next byte: byte to determine if this key corresponds to a list that has been split
//   into multiple parts
// next eight bytes: value of uid
// next eight bytes (optional): if the key corresponds to a split list, the startUid of
//   the split stored in this key.
func DataKey(attr string, uid uint64) []byte {
	prefixLen := 1 + 2 + len(attr)
	totalLen := prefixLen + 1 + 1 + 8
	buf := generateKey(DefaultPrefix, attr, totalLen)

	rest := buf[prefixLen:]
	rest[0] = ByteData

	// By default, this key does not correspond to a part of a split key.
	rest = rest[1:]
	rest[0] = 0

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

// ReverseKey generates a reverse key with the given attribute and UID.
// The structure of a reverse key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
// next byte: data type prefix (set to ByteReverse)
// next byte: byte to determine if this key corresponds to a list that has been split
//   into multiple parts
// next eight bytes: value of uid
// next eight bytes (optional): if the key corresponds to a split list, the startUid of
//   the split stored in this key.
func ReverseKey(attr string, uid uint64) []byte {
	prefixLen := 1 + 2 + len(attr)
	totalLen := prefixLen + 1 + 1 + 8
	buf := generateKey(DefaultPrefix, attr, totalLen)

	rest := buf[prefixLen:]
	rest[0] = ByteReverse

	// By default, this key does not correspond to a part of a split key.
	rest = rest[1:]
	rest[0] = 0

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

// IndexKey generates a index key with the given attribute and term.
// The structure of an index key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
// next byte: data type prefix (set to ByteIndex)
// next byte: byte to determine if this key corresponds to a list that has been split
//   into multiple parts
// next len(term) bytes: value of term
// next eight bytes (optional): if the key corresponds to a split list, the startUid of
//   the split stored in this key.
func IndexKey(attr, term string) []byte {
	prefixLen := 1 + 2 + len(attr)
	totalLen := prefixLen + 1 + 1 + len(term)
	buf := generateKey(DefaultPrefix, attr, totalLen)

	rest := buf[prefixLen:]
	rest[0] = ByteIndex

	// By default, this key does not correspond to a part of a split key.
	rest = rest[1:]
	rest[0] = 0

	rest = rest[1:]
	AssertTrue(len(term) == copy(rest, term))
	return buf
}

// CountKey generates a count key with the given attribute and uid.
// The structure of a count key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
// next byte: data type prefix (set to ByteCount or ByteCountRev)
// next byte: byte to determine if this key corresponds to a list that has been split
//   into multiple parts. Since count indexes only store one number, this value will
//   always be zero.
// next four bytes: value of count.
func CountKey(attr string, count uint32, reverse bool) []byte {
	prefixLen := 1 + 2 + len(attr)
	totalLen := prefixLen + 1 + 1 + 4
	buf := generateKey(DefaultPrefix, attr, totalLen)

	rest := buf[prefixLen:]
	if reverse {
		rest[0] = ByteCountRev
	} else {
		rest[0] = ByteCount
	}

	// By default, this key does not correspond to a part of a split key.
	rest = rest[1:]
	rest[0] = 0

	rest = rest[1:]
	binary.BigEndian.PutUint32(rest, count)
	return buf
}

// ParsedKey represents a key that has been parsed into its multiple attributes.
type ParsedKey struct {
	byteType    byte
	Attr        string
	Uid         uint64
	HasStartUid bool
	StartUid    uint64
	Term        string
	Count       uint32
	bytePrefix  byte
}

// IsData returns whether the key is a data key.
func (p ParsedKey) IsData() bool {
	return p.bytePrefix == DefaultPrefix && p.byteType == ByteData
}

// IsReverse returns whether the key is a reverse key.
func (p ParsedKey) IsReverse() bool {
	return p.bytePrefix == DefaultPrefix && p.byteType == ByteReverse
}

// IsCount returns whether the key is a count key.
func (p ParsedKey) IsCount() bool {
	return p.bytePrefix == DefaultPrefix && (p.byteType == ByteCount ||
		p.byteType == ByteCountRev)
}

// IsIndex returns whether the key is an index key.
func (p ParsedKey) IsIndex() bool {
	return p.bytePrefix == DefaultPrefix && p.byteType == ByteIndex
}

// IsSchema returns whether the key is a schema key.
func (p ParsedKey) IsSchema() bool {
	return p.bytePrefix == byteSchema
}

// IsType returns whether the key is a type key.
func (p ParsedKey) IsType() bool {
	return p.bytePrefix == byteType
}

// IsOfType checks whether the key is of the given type.
func (p ParsedKey) IsOfType(typ byte) bool {
	switch typ {
	case ByteCount, ByteCountRev:
		return p.IsCount()
	case ByteReverse:
		return p.IsReverse()
	case ByteIndex:
		return p.IsIndex()
	case ByteData:
		return p.IsData()
	default:
	}
	return false
}

// SkipPredicate returns the first key after the keys corresponding to the predicate
// of this key. Useful when iterating in the reverse order.
func (p ParsedKey) SkipPredicate() []byte {
	buf := make([]byte, 1+2+len(p.Attr)+1)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = 0xFF
	return buf
}

// SkipSchema returns the first key after all the schema keys.
func (p ParsedKey) SkipSchema() []byte {
	var buf [1]byte
	buf[0] = byteSchema + 1
	return buf[:]
}

// SkipType returns the first key after all the type keys.
func (p ParsedKey) SkipType() []byte {
	var buf [1]byte
	buf[0] = byteType + 1
	return buf[:]
}

// DataPrefix returns the prefix for data keys.
func (p ParsedKey) DataPrefix() []byte {
	buf := make([]byte, 1+2+len(p.Attr)+1+1)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 2)
	k[0] = ByteData
	k[1] = 0
	return buf
}

// IndexPrefix returns the prefix for index keys.
func (p ParsedKey) IndexPrefix() []byte {
	buf := make([]byte, 1+2+len(p.Attr)+1+1)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 2)
	k[0] = ByteIndex
	k[1] = 0
	return buf
}

// ReversePrefix returns the prefix for index keys.
func (p ParsedKey) ReversePrefix() []byte {
	buf := make([]byte, 1+2+len(p.Attr)+1+1)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 2)
	k[0] = ByteReverse
	k[1] = 0
	return buf
}

// CountPrefix returns the prefix for count keys.
func (p ParsedKey) CountPrefix(reverse bool) []byte {
	buf := make([]byte, 1+2+len(p.Attr)+1+1)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 2)
	if reverse {
		k[0] = ByteCountRev
	} else {
		k[0] = ByteCount
	}
	k[1] = 0
	return buf
}

// SchemaPrefix returns the prefix for Schema keys.
func SchemaPrefix() []byte {
	var buf [1]byte
	buf[0] = byteSchema
	return buf[:]
}

// TypePrefix returns the prefix for Schema keys.
func TypePrefix() []byte {
	var buf [1]byte
	buf[0] = byteType
	return buf[:]
}

// PredicatePrefix returns the prefix for all keys belonging to this predicate except schema key.
func PredicatePrefix(predicate string) []byte {
	buf := make([]byte, 1+2+len(predicate))
	buf[0] = DefaultPrefix
	k := writeAttr(buf[1:], predicate)
	AssertTrue(len(k) == 0)
	return buf
}

// GetSplitKey takes a key baseKey and generates the key of the list split that starts at startUid.
func GetSplitKey(baseKey []byte, startUid uint64) []byte {
	keyCopy := make([]byte, len(baseKey)+8)
	copy(keyCopy, baseKey)

	p := Parse(baseKey)
	index := 1 + 2 + len(p.Attr) + 1
	if index >= len(keyCopy) {
		panic("Cannot write to key. Key is too small")
	}
	keyCopy[index] = ByteSplit
	binary.BigEndian.PutUint64(keyCopy[len(baseKey):], startUid)

	return keyCopy
}

// Parse would parse the key. ParsedKey does not reuse the key slice, so the key slice can change
// without affecting the contents of ParsedKey.
func Parse(key []byte) *ParsedKey {
	p := &ParsedKey{}

	p.bytePrefix = key[0]
	if p.bytePrefix == ByteUnused {
		return p
	}

	sz := int(binary.BigEndian.Uint16(key[1:3]))
	k := key[3:]

	p.Attr = string(k[:sz])
	k = k[sz:]

	switch p.bytePrefix {
	case byteSchema, byteType:
		return p
	default:
	}

	p.byteType = k[0]
	k = k[1:]

	p.HasStartUid = k[0] == ByteSplit
	k = k[1:]

	switch p.byteType {
	case ByteData, ByteReverse:
		if len(k) < 8 {
			glog.Errorf("Error: Uid length < 8 for key: %q, parsed key: %+v\n", key, p)
			return nil
		}
		p.Uid = binary.BigEndian.Uint64(k)

		if !p.HasStartUid {
			break
		}

		if len(k) < 16 {
			glog.Errorf("Error: StartUid length < 8 for key: %q, parsed key: %+v\n", key, p)
			return nil
		}

		k = k[8:]
		p.StartUid = binary.BigEndian.Uint64(k)
	case ByteIndex:
		if !p.HasStartUid {
			p.Term = string(k)
			break
		}

		if len(k) < 8 {
			glog.Errorf("Error: StartUid length < 8 for key: %q, parsed key: %+v\n", key, p)
			return nil
		}

		term := k[:len(k)-8]
		startUid := k[len(k)-8:]
		p.Term = string(term)
		p.StartUid = binary.BigEndian.Uint64(startUid)
	case ByteCount, ByteCountRev:
		if len(k) < 4 {
			glog.Errorf("Error: Count length < 4 for key: %q, parsed key: %+v\n", key, p)
			return nil
		}
		p.Count = binary.BigEndian.Uint32(k)

		if !p.HasStartUid {
			break
		}

		if len(k) < 12 {
			glog.Errorf("Error: StartUid length < 8 for key: %q, parsed key: %+v\n", key, p)
			return nil
		}

		k = k[4:]
		p.StartUid = binary.BigEndian.Uint64(k)
	default:
		// Some other data type.
		return nil
	}
	return p
}

var reservedPredicateMap = map[string]struct{}{
	"dgraph.type": {},
}

var aclPredicateMap = map[string]struct{}{
	"dgraph.xid":        {},
	"dgraph.password":   {},
	"dgraph.user.group": {},
	"dgraph.group.acl":  {},
}

// IsReservedPredicate returns true if the predicate is in the reserved predicate list.
func IsReservedPredicate(pred string) bool {
	_, ok := reservedPredicateMap[strings.ToLower(pred)]
	return ok || IsAclPredicate(pred)
}

// IsAclPredicate returns true if the predicate is in the list of reserved
// predicates for the ACL feature.
func IsAclPredicate(pred string) bool {
	_, ok := aclPredicateMap[strings.ToLower(pred)]
	return ok
}

// ReservedPredicates returns the complete list of reserved predicates.
func ReservedPredicates() []string {
	var preds []string
	for pred := range reservedPredicateMap {
		preds = append(preds, pred)
	}
	for pred := range aclPredicateMap {
		preds = append(preds, pred)
	}
	return preds
}
