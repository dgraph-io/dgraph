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
	"encoding/hex"
	"math"

	"github.com/dgryski/go-farm"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/protos/pb"
)

const (
	// TODO(pawan) - Make this 2 bytes long. Right now ParsedKey has ByteType and
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
	ByteSchema    = byte(0x01)
	ByteType      = byte(0x02)
	// ByteSplit signals that the key stores an individual part of a multi-part list.
	ByteSplit = byte(0x04)
	// ByteUnused is a constant to specify keys which need to be discarded.
	ByteUnused = byte(0xff)
	// GalaxyNamespace is the default namespace name.
	GalaxyNamespace = uint32(0)
	// IgnoreBytes is the byte range which will be ignored while prefix match in subscription.
	IgnoreBytes = "1-8"
	// NamespaceOffset is the offset in badger key from which the next 8 bytes contain namespace.
	NamespaceOffset = 1
)

func Uint64ToBytes(val uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, val)
	return buf
}

func AttrEquals(attrId uint64, attr string) bool {
	_, id := ParseNsAttr(attrId)
	otherId := ToAttrId(attr)
	return id == otherId
}

var bitInternal uint32 = 0x80000000
var bitReverse uint32 = 0x40000000
var bitMask uint32 = 0x3FFFFFFF

// We use 2 most-significant bits in uint32 for Dgraph, Reverse respectively. So, clear those out.
func ToAttrId(attr string) uint32         { return farm.Fingerprint32([]byte(attr)) & bitMask }
func ToAttrIdReverse(attr string) uint32  { return ToAttrId(attr) | bitReverse }
func ToAttrIdInternal(attr string) uint32 { return ToAttrId(attr) | bitInternal }
func IsAttrIdReverse(attr uint32) bool    { return attr&bitReverse > 0 }
func IsAttrIdInternal(attr uint32) bool   { return attr&bitInternal > 0 }

// NamespaceAttr is used to generate attr from namespace.
func NamespaceAttr(ns uint32, attr string) uint64 {
	val := uint64(ns) << 32
	val |= uint64(ToAttrId(attr))
	return val
}

func NamespaceAttrList(ns uint32, preds []string) []uint64 {
	var resp []uint64
	for _, pred := range preds {
		resp = append(resp, NamespaceAttr(ns, pred))
	}
	return resp
}

func GalaxyAttr(attr string) uint64 {
	return NamespaceAttr(GalaxyNamespace, attr)
}

// ParseAttr returns the attr from the given value.
func ParseNsAttr(attr uint64) (uint32, uint32) {
	return uint32(attr >> 32), uint32(attr & 0xFFFFFFFF)
}

func ParseAttrList(attrs []uint64) []uint32 {
	var resp []uint32
	for _, attr := range attrs {
		_, id := ParseNsAttr(attr)
		resp = append(resp, id)
	}
	return resp
}

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
func generateKey(buf []byte, typeByte byte, attr uint64) []byte {
	AssertTrue(len(buf) >= 1+8)

	buf[0] = typeByte                         // 1 byte.
	binary.BigEndian.PutUint64(buf[1:], attr) // 8 bytes.

	return buf[9:]
}

// SchemaKey returns schema key for given attribute. Schema keys are stored
// separately with unique prefix, since we need to iterate over all schema keys.
// The structure of a schema key is as follows:
//
// byte 0: key type prefix (set to ByteSchema)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
func SchemaKey(attr uint64) []byte {
	buf := make([]byte, 9)
	generateKey(buf, ByteSchema, attr)
	return buf
}

// TypeKey returns type key for given type name. Type keys are stored separately
// with a unique prefix, since we need to iterate over all type keys.
// The structure of a type key is as follows:
//
// byte 0: key type prefix (set to ByteType)
// byte 1-2: length of typeName
// next len(attr) bytes: value of attr (the type name)
func TypeKey(attr uint64) []byte {
	buf := make([]byte, 9)
	generateKey(buf, ByteType, attr)
	return buf
}

// DataKey generates a data key with the given attribute and UID.
// The structure of a data key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix or ByteSplit if part of a multi-part list)
// byte 1-8: Namespace (4 bytes) + Attribute (4-bytes)
// next byte: data type prefix (set to ByteData)
// next eight bytes: value of uid
// next eight bytes (optional): if the key corresponds to a split list, the startUid of
//   the split stored in this key and the first byte will be sets to ByteSplit.
func DataKey(attr uint64, uid uint64) []byte {
	totalLen := 2 * 9 // 9-bytes twice.
	src := make([]byte, totalLen)
	buf := generateKey(src, DefaultPrefix, attr)
	buf = generateKey(buf, ByteData, uid)
	return src
}

func reverseAttr(attrId uint64) uint64 {
	// Remove the MSB from attr uint32 for forward.
	// Add the MSB in attr uint32 for reverse.
	return attrId | uint64(0x80000000)
}

// ReverseKey generates a reverse key with the given attribute and UID.
// The structure of a reverse key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix or ByteSplit if part of a multi-part list)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
// next byte: data type prefix (set to ByteReverse)
// next eight bytes: value of uid
// next eight bytes (optional): if the key corresponds to a split list, the startUid of
//   the split stored in this key.
func ReverseKey(attr uint64, uid uint64) []byte {
	src := make([]byte, 2*9) // 9-bytes twice.
	buf := generateKey(src, DefaultPrefix, attr)
	buf = generateKey(buf, ByteReverse, uid)
	return src
}

// IndexKey generates a index key with the given attribute and term.
// The structure of an index key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix or ByteSplit if part of a multi-part list)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
// next byte: data type prefix (set to ByteIndex)
// next len(term) bytes: value of term
// next eight bytes (optional): if the key corresponds to a split list, the startUid of
//   the split stored in this key.
func IndexKey(attr uint64, term string) []byte {
	totalLen := 9 + 1 + len(term)
	src := make([]byte, totalLen)
	buf := generateKey(src, DefaultPrefix, attr)

	buf[0] = ByteIndex

	buf = buf[1:]
	AssertTrue(len(buf) == len(term))
	AssertTrue(len(term) == copy(buf, term))
	return src
}

// CountKey generates a count key with the given attribute and uid.
// The structure of a count key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
// next byte: data type prefix (set to ByteCount or ByteCountRev)
// next four bytes: value of count.
func CountKey(attr uint64, count uint32, reverse bool) []byte {
	totalLen := 9 + 1 + 4
	src := make([]byte, totalLen)
	buf := generateKey(src, DefaultPrefix, attr)

	if reverse {
		buf[0] = ByteCountRev
	} else {
		buf[0] = ByteCount
	}

	buf = buf[1:]
	binary.BigEndian.PutUint32(buf, count)
	return src
}

// ParsedKey represents a key that has been parsed into its multiple attributes.
type ParsedKey struct {
	Attr        uint64
	Uid         uint64
	StartUid    uint64
	Count       uint32
	Term        string
	bytePrefix  byte
	ByteType    byte
	HasStartUid bool
}

// IsData returns whether the key is a data key.
func (p ParsedKey) IsData() bool {
	return (p.bytePrefix == DefaultPrefix || p.bytePrefix == ByteSplit) && p.ByteType == ByteData
}

// IsReverse returns whether the key is a reverse key.
func (p ParsedKey) IsReverse() bool {
	return (p.bytePrefix == DefaultPrefix || p.bytePrefix == ByteSplit) && p.ByteType == ByteReverse
}

// IsCountOrCountRev returns whether the key is a count or a count rev key.
func (p ParsedKey) IsCountOrCountRev() bool {
	return p.IsCount() || p.IsCountRev()
}

// IsCount returns whether the key is a count key.
func (p ParsedKey) IsCount() bool {
	return (p.bytePrefix == DefaultPrefix || p.bytePrefix == ByteSplit) && p.ByteType == ByteCount
}

// IsCountRev returns whether the key is a count rev key.
func (p ParsedKey) IsCountRev() bool {
	return (p.bytePrefix == DefaultPrefix || p.bytePrefix == ByteSplit) && p.ByteType == ByteCountRev
}

// IsIndex returns whether the key is an index key.
func (p ParsedKey) IsIndex() bool {
	return (p.bytePrefix == DefaultPrefix || p.bytePrefix == ByteSplit) && p.ByteType == ByteIndex
}

// IsSchema returns whether the key is a schema key.
func (p ParsedKey) IsSchema() bool {
	return p.bytePrefix == ByteSchema
}

// IsType returns whether the key is a type key.
func (p ParsedKey) IsType() bool {
	return p.bytePrefix == ByteType
}

// IsOfType checks whether the key is of the given type.
func (p ParsedKey) IsOfType(typ byte) bool {
	switch typ {
	case ByteCount, ByteCountRev:
		return p.IsCountOrCountRev()
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
	buf := make([]byte, 9+1)
	buf[0] = p.bytePrefix
	binary.BigEndian.PutUint64(buf[1:], p.Attr)
	buf[9] = 0xFF
	return buf
}

// TODO(Naman): Remove these functions as they are unused.
// SkipSchema returns the first key after all the schema keys.
func (p ParsedKey) SkipSchema() []byte {
	var buf [1]byte
	buf[0] = ByteSchema + 1
	return buf[:]
}

// SkipType returns the first key after all the type keys.
func (p ParsedKey) SkipType() []byte {
	var buf [1]byte
	buf[0] = ByteType + 1
	return buf[:]
}

// DataPrefix returns the prefix for data keys.
func (p ParsedKey) DataPrefix() []byte {
	buf := make([]byte, 9+1)
	buf[0] = p.bytePrefix
	binary.BigEndian.PutUint64(buf[1:], p.Attr)
	buf[9] = ByteData
	return buf
}

// IndexPrefix returns the prefix for index keys.
func (p ParsedKey) IndexPrefix() []byte {
	buf := make([]byte, 9+1)
	buf[0] = DefaultPrefix
	binary.BigEndian.PutUint64(buf[1:], p.Attr)
	buf[9] = ByteIndex
	return buf
}

// ReversePrefix returns the prefix for index keys.
// TODO: Understand how this works with p.Attr already holding the reverse bit.
func (p ParsedKey) ReversePrefix() []byte {
	buf := make([]byte, 9+1)
	buf[0] = DefaultPrefix
	binary.BigEndian.PutUint64(buf[1:], p.Attr)
	buf[9] = ByteReverse
	return buf
}

// CountPrefix returns the prefix for count keys.
func (p ParsedKey) CountPrefix(reverse bool) []byte {
	buf := make([]byte, 9+1)
	buf[0] = p.bytePrefix
	binary.BigEndian.PutUint64(buf[1:], p.Attr)
	if reverse {
		buf[9] = ByteCountRev
	} else {
		buf[9] = ByteCount
	}
	return buf
}

// ToBackupKey returns the key in the format used for writing backups.
func (p ParsedKey) ToBackupKey() *pb.BackupKey {
	key := pb.BackupKey{}
	key.Namespace, key.AttrId = ParseNsAttr(p.Attr)
	key.Uid = p.Uid
	key.StartUid = p.StartUid
	key.Term = p.Term
	key.Count = p.Count
	// TODO: Fill out key.Attr later.

	switch {
	case p.IsData():
		key.Type = pb.BackupKey_DATA
	case p.IsIndex():
		key.Type = pb.BackupKey_INDEX
	case p.IsReverse():
		key.Type = pb.BackupKey_REVERSE
	case p.IsCount():
		key.Type = pb.BackupKey_COUNT
	case p.IsCountRev():
		key.Type = pb.BackupKey_COUNT_REV
	case p.IsSchema():
		key.Type = pb.BackupKey_SCHEMA
	case p.IsType():
		key.Type = pb.BackupKey_TYPE
	}

	return &key
}

// FromBackupKey takes a key in the format used for backups and converts it to a key.
func FromBackupKey(backupKey *pb.BackupKey) []byte {
	if backupKey == nil {
		return nil
	}

	attr := NamespaceAttr(backupKey.Namespace, backupKey.Attr)

	var key []byte
	switch backupKey.Type {
	case pb.BackupKey_DATA:
		key = DataKey(attr, backupKey.Uid)
	case pb.BackupKey_INDEX:
		key = IndexKey(attr, backupKey.Term)
	case pb.BackupKey_REVERSE:
		key = ReverseKey(attr, backupKey.Uid)
	case pb.BackupKey_COUNT:
		key = CountKey(attr, backupKey.Count, false)
	case pb.BackupKey_COUNT_REV:
		key = CountKey(attr, backupKey.Count, true)
	case pb.BackupKey_SCHEMA:
		key = SchemaKey(attr)
	case pb.BackupKey_TYPE:
		key = TypeKey(attr)
	}

	if backupKey.StartUid > 0 {
		var err error
		key, err = SplitKey(key, backupKey.StartUid)
		Check(err)
	}
	return key
}

// SchemaPrefix returns the prefix for Schema keys.
func SchemaPrefix() []byte {
	var buf [1]byte
	buf[0] = ByteSchema
	return buf[:]
}

// TypePrefix returns the prefix for Schema keys.
func TypePrefix() []byte {
	var buf [1]byte
	buf[0] = ByteType
	return buf[:]
}

// PredicatePrefix returns the prefix for all keys belonging to this predicate except schema key.
func PredicatePrefix(attr uint64) []byte {
	buf := make([]byte, 9)
	generateKey(buf, DefaultPrefix, attr)
	return buf
}

// DataPrefix returns the prefix for all data keys belonging to this namespace.
func DataPrefix(ns uint32) []byte {
	buf := make([]byte, 1+4)
	buf[0] = DefaultPrefix
	binary.BigEndian.PutUint32(buf[1:], ns)
	return buf
}

// SplitKey takes a key baseKey and generates the key of the list split that starts at startUid.
func SplitKey(baseKey []byte, startUid uint64) ([]byte, error) {
	keyCopy := make([]byte, len(baseKey)+8)
	copy(keyCopy, baseKey)

	if keyCopy[0] != DefaultPrefix {
		return nil, errors.Errorf("only keys with default prefix can have a split key")
	}
	// Change the first byte (i.e the key prefix) to ByteSplit to signal this is an
	// individual part of a single list key.
	keyCopy[0] = ByteSplit

	// Append the start uid at the end of the key.
	binary.BigEndian.PutUint64(keyCopy[len(baseKey):], startUid)
	return keyCopy, nil
}

// Parse would parse the key. ParsedKey does not reuse the key slice, so the key slice can change
// without affecting the contents of ParsedKey.
func Parse(key []byte) (ParsedKey, error) {
	var p ParsedKey

	if len(key) < 9 {
		return p, errors.New("Key length less than 9")
	}
	p.bytePrefix = key[0]
	p.Attr = binary.BigEndian.Uint64(key[1:9])
	key = key[9:]
	if p.bytePrefix == ByteUnused {
		return p, nil
	}

	p.HasStartUid = p.bytePrefix == ByteSplit
	switch p.bytePrefix {
	case ByteSchema, ByteType:
		return p, nil
	default:
	}

	p.ByteType = key[0]
	key = key[1:]

	switch p.ByteType {
	case ByteData, ByteReverse:
		if len(key) < 8 {
			return p, errors.Errorf("uid length < 8 for key: %q, parsed key: %+v", key, p)
		}
		p.Uid = binary.BigEndian.Uint64(key)
		if p.Uid == 0 {
			return p, errors.Errorf("Invalid UID with value 0 for key: %v", key)
		}
		if !p.HasStartUid {
			break
		}

		if len(key) != 16 {
			return p, errors.Errorf("StartUid length != 8 for key: %q, parsed key: %+v", key, p)
		}

		key = key[8:]
		p.StartUid = binary.BigEndian.Uint64(key)
	case ByteIndex:
		if !p.HasStartUid {
			p.Term = string(key)
			break
		}

		if len(key) < 8 {
			return p, errors.Errorf("StartUid length < 8 for key: %q, parsed key: %+v", key, p)
		}

		term := key[:len(key)-8]
		startUid := key[len(key)-8:]
		p.Term = string(term)
		p.StartUid = binary.BigEndian.Uint64(startUid)
	case ByteCount, ByteCountRev:
		if len(key) < 4 {
			return p, errors.Errorf("count length < 4 for key: %q, parsed key: %+v", key, p)
		}
		p.Count = binary.BigEndian.Uint32(key)

		if !p.HasStartUid {
			break
		}

		if len(key) != 12 {
			return p, errors.Errorf("StartUid length != 8 for key: %q, parsed key: %+v", key, p)
		}

		key = key[4:]
		p.StartUid = binary.BigEndian.Uint64(key)
	default:
		// Some other data type.
		return p, errors.Errorf("Invalid data type")
	}
	return p, nil
}

func IsDropOpKey(key []byte) (bool, error) {
	pk, err := Parse(key)
	if err != nil {
		return false, errors.Wrapf(err, "could not parse key %s", hex.Dump(key))
	}

	if pk.IsData() && AttrEquals(pk.Attr, "dgraph.drop.op") {
		return true, nil
	}
	return false, nil
}

// These predicates appear for queries that have * as predicate in them.
var starAllPredicateMap = map[string]struct{}{
	"dgraph.type": {},
}

var aclPredicateMap = map[string]struct{}{
	"dgraph.xid":             {},
	"dgraph.password":        {},
	"dgraph.user.group":      {},
	"dgraph.rule.predicate":  {},
	"dgraph.rule.permission": {},
	"dgraph.acl.rule":        {},
}

// TODO: rename this map to a better suited name as per its properties. It is not just for GraphQL
// predicates, but for all those which are PreDefined and whose value is not allowed to be mutated
// by users. When renaming this also rename the IsGraphql context key in edgraph/server.go.
var graphqlReservedPredicate = map[string]struct{}{
	"dgraph.graphql.xid":     {},
	"dgraph.graphql.schema":  {},
	"dgraph.drop.op":         {},
	"dgraph.graphql.p_query": {},
}

// internalPredicateMap stores a set of Dgraph's internal predicate. An internal
// predicate is a predicate that has a special meaning in Dgraph and its query
// language and should not be allowed either as a user-defined predicate or as a
// predicate in initial internal schema.
var internalPredicateMap = map[string]struct{}{
	"uid": {},
}

var preDefinedTypeMap = map[string]struct{}{
	"dgraph.graphql":                 {},
	"dgraph.type.User":               {},
	"dgraph.type.Group":              {},
	"dgraph.type.Rule":               {},
	"dgraph.graphql.persisted_query": {},
}

// IsGraphqlReservedPredicate returns true if it is the predicate is reserved by graphql.
// These are a subset of PreDefined predicates, so follow all their properties. In addition,
// the value for these predicates is also not allowed to be mutated directly by the users.
func IsGraphqlReservedPredicate(pred string) bool {
	_, ok := graphqlReservedPredicate[pred]
	return ok
}

/*

// IsReservedPredicate returns true if the predicate is reserved for internal usage, i.e., prefixed
// with `dgraph.`.
//
// We reserve `dgraph.` as the namespace for the types/predicates we may create in future.
// So, users are not allowed to create a predicate under this namespace.
// Hence, we should always define internal predicates under `dgraph.` namespace.
//
// Reserved predicates are a superset of pre-defined predicates.
//
// When critical, use IsPreDefinedPredicate(pred string) to find out whether the predicate was
// actually defined internally or not.
//
// As an example, consider below predicates:
// 	1. dgraph.type (reserved = true,  pre_defined = true )
// 	2. dgraph.blah (reserved = true,  pre_defined = false)
// 	3. person.name (reserved = false, pre_defined = false)
func IsReservedPredicate(attr uint32) bool {
	return isReservedName(ParseAttr(pred))
}

// IsPreDefinedPredicate returns true only if the predicate has been defined by dgraph internally
// in the initial schema. These are not allowed to be dropped, as well as any schema update which
// is different than the initial internal schema is also not allowed for these.
// For example, `dgraph.type` or ACL predicates or GraphQL predicates are defined in the initial
// internal schema.
//
// We reserve `dgraph.` as the namespace for the types/predicates we may create in future.
// So, users are not allowed to create a predicate under this namespace.
// Hence, we should always define internal predicates under `dgraph.` namespace.
//
// Pre-defined predicates are subset of reserved predicates.
func IsPreDefinedPredicate(pred string) bool {
	pred = ParseAttr(pred)
	_, ok := starAllPredicateMap[strings.ToLower(pred)]
	return ok || IsAclPredicate(pred) || IsGraphqlReservedPredicate(pred)
}

// IsAclPredicate returns true if the predicate is in the list of reserved
// predicates for the ACL feature.
func IsAclPredicate(pred string) bool {
	_, ok := aclPredicateMap[strings.ToLower(pred)]
	return ok
}

// StarAllPredicates returns the complete list of pre-defined predicates that needs to
// be expanded when * is given as a predicate.
func StarAllPredicates(namespace uint64) []string {
	preds := make([]string, 0, len(starAllPredicateMap))
	for pred := range starAllPredicateMap {
		preds = append(preds, NamespaceAttr(namespace, pred))
	}
	return preds
}

func AllACLPredicates() []string {
	preds := make([]string, 0, len(aclPredicateMap))
	for pred := range aclPredicateMap {
		preds = append(preds, pred)
	}
	return preds
}

// IsInternalPredicate returns true if the predicate is in the internal predicate list.
// Currently, `uid` is the only such candidate.
func IsInternalPredicate(pred string) bool {
	_, ok := internalPredicateMap[strings.ToLower(ParseAttr(pred))]
	return ok
}

// IsReservedType returns true if the given typ is reserved for internal usage, i.e.,
// prefixed with `dgraph.`.
//
// We reserve `dgraph.` as the namespace for the types/predicates we may create in future.
// So, users are not allowed to create a type under this namespace.
// Hence, we should always define internal types under `dgraph.` namespace.
//
// Pre-defined types are subset of reserved types.
//
// When critical, use IsPreDefinedType(typ string) to find out whether the typ was
// actually defined internally or not.
func IsReservedType(typ string) bool {
	return isReservedName(ParseAttr(typ))
}

// IsPreDefinedType returns true only if the typ has been defined by dgraph internally.
// For example, `dgraph.graphql` or ACL types are defined in the initial internal types.
//
// We reserve `dgraph.` as the namespace for the types/predicates we may create in future.
// So, users are not allowed to create a predicate under this namespace.
// Hence, we should always define internal types under `dgraph.` namespace.
//
// Pre-defined types are subset of reserved types.
func IsPreDefinedType(typ string) bool {
	_, ok := preDefinedTypeMap[ParseAttr(typ)]
	return ok
}

// isReservedName returns true if the given name is prefixed with `dgraph.`
func isReservedName(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "dgraph.")
}
*/
