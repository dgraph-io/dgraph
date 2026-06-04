/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
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
	// RootNamespace is the default namespace name.
	RootNamespace = uint64(0)
	// IgnoreBytes is the byte range which will be ignored while prefix match in subscription.
	IgnoreBytes = "1-8"
	// NamespaceOffset is the offset in badger key from which the next 8 bytes contain namespace.
	NamespaceOffset = 1
	// NsSeparator is the separator between the namespace and attribute.
	NsSeparator = "-"
)

// Invalid bytes are replaced with the Unicode replacement rune.
// See https://golang.org/pkg/encoding/json/#Marshal
const replacementRune = rune('\ufffd')

// AttrFrom2103 parses namespace that were stored in format used before 21.03 version.
// The first 8 bytes are the namespace, rest is the predicate. This format caused issues
// while marshalling, hence was removed. This function is there for backwards compatibility of
// restore. Now we store the predicate as a string of format "hex(namespace)-predicate"
func AttrFrom2103(attr string) (string, error) {
	if strings.ContainsRune(attr, replacementRune) {
		return "", errors.Errorf("replacement rune found while parsing attr: %s (%+v)",
			attr, []byte(attr))
	}
	ns, pred := binary.BigEndian.Uint64([]byte(attr[:8])), attr[8:]
	return NamespaceAttr(ns, pred), nil
}

func NamespaceToBytes(ns uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, ns)
	return buf
}

// NamespaceAttr is used to generate attr from namespace.
func NamespaceAttr(ns uint64, attr string) string {
	return uintToStr(ns) + NsSeparator + attr
}

func NamespaceAttrList(ns uint64, preds []string) []string {
	var resp []string
	for _, pred := range preds {
		resp = append(resp, NamespaceAttr(ns, pred))
	}
	return resp
}

func AttrInRootNamespace(attr string) string {
	return NamespaceAttr(RootNamespace, attr)
}

// ParseNamespaceAttr returns the namespace and attr from the given value.
func ParseNamespaceAttr(attr string) (uint64, string) {
	splits := strings.SplitN(attr, NsSeparator, 2)
	return strToUint(splits[0]), splits[1]
}

func ParseNamespaceBytes(attr string) ([]byte, string) {
	splits := strings.SplitN(attr, NsSeparator, 2)
	ns := make([]byte, 8)
	binary.BigEndian.PutUint64(ns, strToUint(splits[0]))
	return ns, splits[1]
}

// ParseAttr returns the attr from the given value.
func ParseAttr(attr string) string {
	return strings.SplitN(attr, NsSeparator, 2)[1]
}

// ParseNamespace returns the namespace from the given value.
func ParseNamespace(attr string) uint64 {
	return strToUint(strings.SplitN(attr, NsSeparator, 2)[0])
}

func ParseAttrList(attrs []string) []string {
	var resp []string
	for _, attr := range attrs {
		resp = append(resp, ParseAttr(attr))
	}
	return resp
}

// For consistency, use base16 to encode/decode the namespace.
func strToUint(s string) uint64 {
	ns, err := strconv.ParseUint(s, 16, 64)
	Check(err)
	return ns
}
func uintToStr(ns uint64) string {
	return strconv.FormatUint(ns, 16)
}

func IsReverseAttr(attr string) bool {
	pred := strings.SplitN(attr, NsSeparator, 2)[1]
	return pred[0] == '~'
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
// if necessary. It also returns next index from where further processing should be done.
func generateKey(typeByte byte, attr string, extra int) ([]byte, int) {
	// Separate namespace and attribute from attr and write namespace in the first 8 bytes of key.
	namespace, attr := ParseNamespaceBytes(attr)
	prefixLen := 1 + 8 + 2 + len(attr) // byteType + ns + len(pred) + pred
	buf := make([]byte, prefixLen+extra)
	buf[0] = typeByte
	AssertTrue(copy(buf[1:], namespace) == 8)
	rest := buf[9:]

	writeAttr(rest, attr)
	return buf, prefixLen
}

// SchemaKey returns schema key for given attribute. Schema keys are stored
// separately with unique prefix, since we need to iterate over all schema keys.
// The structure of a schema key is as follows:
//
// byte 0: key type prefix (set to ByteSchema)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
func SchemaKey(attr string) []byte {
	key, _ := generateKey(ByteSchema, attr, 0)
	return key
}

// TypeKey returns type key for given type name. Type keys are stored separately
// with a unique prefix, since we need to iterate over all type keys.
// The structure of a type key is as follows:
//
// byte 0: key type prefix (set to ByteType)
// byte 1-2: length of typeName
// next len(attr) bytes: value of attr (the type name)
func TypeKey(attr string) []byte {
	key, _ := generateKey(ByteType, attr, 0)
	return key
}

// DataKey generates a data key with the given attribute and UID.
// The structure of a data key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix or ByteSplit if part of a multi-part list)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
// next byte: data type prefix (set to ByteData)
// next eight bytes: value of uid
// next eight bytes (optional): if the key corresponds to a split list, the startUid of
// the split stored in this key and the first byte will be sets to ByteSplit.
func DataKey(attr string, uid uint64) []byte {
	extra := 1 + 8 // ByteData + UID
	buf, prefixLen := generateKey(DefaultPrefix, attr, extra)

	rest := buf[prefixLen:]
	rest[0] = ByteData

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

// ReverseKey generates a reverse key with the given attribute and UID.
// The structure of a reverse key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix or ByteSplit if part of a multi-part list)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
// next byte: data type prefix (set to ByteReverse)
// next eight bytes: value of uid
// next eight bytes (optional): if the key corresponds to a split list,
// the startUid of the split stored in this key.
func ReverseKey(attr string, uid uint64) []byte {
	extra := 1 + 8 // ByteReverse + UID
	buf, prefixLen := generateKey(DefaultPrefix, attr, extra)

	rest := buf[prefixLen:]
	rest[0] = ByteReverse

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

func IndexKeyAfterAllTerms(attr string) []byte {
	// Use a term with 0xFF to ensure lexicographically after any real term
	sentinelTerm := []byte{0xFF}
	extra := 1 + len(sentinelTerm) // ByteIndex + 0xFF
	buf, prefixLen := generateKey(DefaultPrefix, attr, extra)

	rest := buf[prefixLen:]
	rest[0] = ByteIndex
	copy(rest[1:], sentinelTerm)

	return buf
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
// the split stored in this key.
func IndexKey(attr, term string) []byte {
	extra := 1 + len(term) // ByteIndex + term
	buf, prefixLen := generateKey(DefaultPrefix, attr, extra)

	rest := buf[prefixLen:]
	rest[0] = ByteIndex

	rest = rest[1:]
	AssertTrue(len(rest) == len(term))
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
// next four bytes: value of count.
func CountKey(attr string, count uint32, reverse bool) []byte {
	extra := 1 + 4 // ByteCount + Count
	buf, prefixLen := generateKey(DefaultPrefix, attr, extra)

	rest := buf[prefixLen:]
	if reverse {
		rest[0] = ByteCountRev
	} else {
		rest[0] = ByteCount
	}

	rest = rest[1:]
	binary.BigEndian.PutUint32(rest, count)
	return buf
}

// BM25IndexKey generates the index key for a BM25 term posting list. The
// encodedToken already carries the BM25 tokenizer identifier byte, so BM25 term
// postings live at the same standard index key as every other tokenizer —
// IndexKey(attr, identifier || term) — and inherit rollup, splits, backup, and
// index-rebuild handling for free. This is a thin alias of IndexKey so the index
// write path and the query read path share one definition.
func BM25IndexKey(attr string, encodedToken string) []byte {
	return IndexKey(attr, encodedToken)
}

// bm25StatsPrefix namespaces the BM25 corpus-statistics keys. These hold the
// document count and total term count (used to derive the average document
// length); they are auxiliary metadata, not term postings, so they use a reserved
// token that cannot collide with any stemmed BM25 term.
const bm25StatsPrefix = "\x00_bm25stats_"

// BM25StatsKey generates the key for one bucket of BM25 corpus statistics. Stats
// are sharded across buckets (keyed by uid%numBuckets) to spread write contention.
func BM25StatsKey(attr string, bucket int) []byte {
	var buf [2]byte
	binary.BigEndian.PutUint16(buf[:], uint16(bucket))
	return IndexKey(attr, bm25StatsPrefix+string(buf[:]))
}

// ParsedKey represents a key that has been parsed into its multiple attributes.
type ParsedKey struct {
	Attr        string
	Uid         uint64
	StartUid    uint64
	Term        string
	Count       uint32
	ByteType    byte
	HasStartUid bool
	bytePrefix  byte
}

func (p ParsedKey) String() string {
	if p.IsIndex() {
		return fmt.Sprintf("UID: %v, Attr: %v, IsIndex: true, Term: %v", p.Uid, p.Attr, []byte(p.Term))
	} else if p.IsCountOrCountRev() {
		return fmt.Sprintf("UID: %v, Attr: %v, IsCount/Ref: true, Count: %v", p.Uid, p.Attr, p.Count)
	} else {
		return fmt.Sprintf("UID: %v, Attr: %v, Data key, prefix; %v, byte: %v", p.Uid, p.Attr, p.bytePrefix, p.ByteType)
	}
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
	buf, prefixLen := generateKey(p.bytePrefix, p.Attr, 1)
	AssertTrue(len(buf[prefixLen:]) == 1)
	buf[prefixLen] = 0xFF
	return buf
}

// TODO(Naman): Remove these functions as they are unused.

// DataPrefix returns the prefix for data keys.
func (p ParsedKey) DataPrefix() []byte {
	buf, prefixLen := generateKey(p.bytePrefix, p.Attr, 1)
	buf[prefixLen] = ByteData
	return buf
}

// IndexPrefix returns the prefix for index keys.
func (p ParsedKey) IndexPrefix() []byte {
	buf, prefixLen := generateKey(DefaultPrefix, p.Attr, 1)
	buf[prefixLen] = ByteIndex
	return buf
}

// ReversePrefix returns the prefix for index keys.
func (p ParsedKey) ReversePrefix() []byte {
	buf, prefixLen := generateKey(DefaultPrefix, p.Attr, 1)
	buf[prefixLen] = ByteReverse
	return buf
}

// CountPrefix returns the prefix for count keys.
func (p ParsedKey) CountPrefix(reverse bool) []byte {
	buf, prefixLen := generateKey(DefaultPrefix, p.Attr, 1)
	buf[prefixLen] = ByteReverse
	if reverse {
		buf[prefixLen] = ByteCountRev
	} else {
		buf[prefixLen] = ByteCount
	}
	return buf
}

// ToBackupKey returns the key in the format used for writing backups.
func (p ParsedKey) ToBackupKey() *pb.BackupKey {
	ns, attr := ParseNamespaceAttr(p.Attr)
	key := pb.BackupKey{}
	key.Namespace = ns
	key.Attr = attr
	key.Uid = p.Uid
	key.StartUid = p.StartUid
	key.Term = []byte(p.Term)
	key.Count = p.Count

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
		key = IndexKey(attr, string(backupKey.Term))
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
func PredicatePrefix(predicate string) []byte {
	buf, prefixLen := generateKey(DefaultPrefix, predicate, 0)
	AssertTrue(len(buf) == prefixLen)
	return buf
}

// DataPrefix returns the prefix for all data keys belonging to this namespace.
func DataPrefix(ns uint64) []byte {
	buf := make([]byte, 1+8)
	buf[0] = DefaultPrefix
	binary.BigEndian.PutUint64(buf[1:], ns)
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
	namespace := key[1:9]
	key = key[9:]
	if p.bytePrefix == ByteUnused {
		return p, nil
	}

	p.HasStartUid = p.bytePrefix == ByteSplit

	if len(key) < 3 {
		return p, errors.Errorf("Invalid format for key %v", key)
	}
	sz := int(binary.BigEndian.Uint16(key[:2]))
	k := key[2:]

	if len(k) < sz {
		return p, errors.Errorf("Invalid size %v for key %v", sz, key)
	}
	p.Attr = NamespaceAttr(binary.BigEndian.Uint64(namespace), string(k[:sz]))
	k = k[sz:]

	switch p.bytePrefix {
	case ByteSchema, ByteType:
		return p, nil
	default:
	}

	p.ByteType = k[0]
	k = k[1:]

	switch p.ByteType {
	case ByteData, ByteReverse:
		if len(k) < 8 {
			return p, errors.Errorf("uid length < 8 for key: %q, parsed key: %+v", key, p)
		}
		p.Uid = binary.BigEndian.Uint64(k)
		if p.Uid == 0 {
			return p, errors.Errorf("Invalid UID with value 0 for key: %v", key)
		}
		if !p.HasStartUid {
			break
		}

		if len(k) != 16 {
			return p, errors.Errorf("StartUid length != 8 for key: %q, parsed key: %+v", key, p)
		}

		k = k[8:]
		p.StartUid = binary.BigEndian.Uint64(k)
	case ByteIndex:
		if !p.HasStartUid {
			p.Term = string(k)
			break
		}

		if len(k) < 8 {
			return p, errors.Errorf("StartUid length < 8 for key: %q, parsed key: %+v", key, p)
		}

		term := k[:len(k)-8]
		startUid := k[len(k)-8:]
		p.Term = string(term)
		p.StartUid = binary.BigEndian.Uint64(startUid)
	case ByteCount, ByteCountRev:
		if len(k) < 4 {
			return p, errors.Errorf("count length < 4 for key: %q, parsed key: %+v", key, p)
		}
		p.Count = binary.BigEndian.Uint32(k)

		if !p.HasStartUid {
			break
		}

		if len(k) != 12 {
			return p, errors.Errorf("StartUid length != 8 for key: %q, parsed key: %+v", key, p)
		}

		k = k[4:]
		p.StartUid = binary.BigEndian.Uint64(k)
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

	if pk.IsData() && ParseAttr(pk.Attr) == "dgraph.drop.op" {
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

// ReservedNamespace lets a plugin claim ownership of a sub-namespace under the
// reserved `dgraph.` prefix, so the predicates and types it owns can be created
// at runtime via Alter even though they are not part of the pre-defined initial
// schema. Register one from an init() with RegisterReservedNamespace.
//
// With no registration the registry is empty and every check below reports
// false, so a stock build keeps the pristine behavior: nothing under `dgraph.`
// may be created except the pre-defined names.
//
// Ownership is deliberately explicit — a dynamic prefix and/or an exact name
// allowlist — so registering a namespace cannot be used to admit arbitrary
// reserved predicates through /alter or /mutate.
type ReservedNamespace struct {
	// All names below are bare (no namespace prefix); queries strip the
	// namespace before matching, as the reserved-predicate checks already do.
	//
	// PredicatePrefix admits dynamically-named predicates by prefix (e.g.
	// "dgraph.acme.rel."). Empty means the namespace owns no dynamic predicates.
	PredicatePrefix string
	// Predicates is the exact set of fixed predicate names the namespace owns.
	Predicates []string
	// Types is the exact set of type names the namespace owns.
	Types []string
	// ValueLocked is the subset of owned predicates whose stored *value* may
	// only be written by a request whose context carries TrustMarker. Use it
	// for predicates whose value is authoritative state the owner must control.
	// Names are the bare predicate form (no namespace prefix), matched
	// case-insensitively like Predicates above.
	ValueLocked []string
	// TrustMarker is a context key the owner's trusted in-process caller sets,
	// via context.WithValue(ctx, TrustMarker, true), to authorize writing
	// ValueLocked predicates. Required (non-nil) when ValueLocked is non-empty;
	// RegisterReservedNamespace panics otherwise.
	TrustMarker any
}

var (
	reservedNsMu          sync.RWMutex
	reservedNsPrefixes    []string
	reservedNsPredicates  = map[string]struct{}{}
	reservedNsTypes       = map[string]struct{}{}
	reservedNsValueLocked = map[string]any{} // lowercased bare predicate -> TrustMarker
)

// RegisterReservedNamespace records a plugin's ownership of names under the
// reserved `dgraph.` prefix. Call it from an init(); it is safe for concurrent
// use but is expected to run before the server starts handling requests.
//
// All names must be bare (no namespace separator) and uniquely owned, and a
// non-empty ValueLocked needs a TrustMarker. It panics on any of these, so a
// misconfiguration trips at startup rather than silently at mutation time.
func RegisterReservedNamespace(ns ReservedNamespace) {
	if len(ns.ValueLocked) > 0 && ns.TrustMarker == nil {
		panic("x.RegisterReservedNamespace: ValueLocked is set but TrustMarker is nil; " +
			"a value-locked predicate with no TrustMarker is unwritable by everyone, including its owner")
	}
	// Names must be bare (no namespace separator). The membership lookups strip
	// the namespace from the query, and the value-lock guard matches the bare
	// predicate the mutation path passes, so a namespace-qualified registration
	// would never match — silently leaving a value-locked predicate publicly
	// writable. Reject it at startup instead.
	for _, group := range [][]string{{ns.PredicatePrefix}, ns.Predicates, ns.Types, ns.ValueLocked} {
		for _, name := range group {
			if strings.Contains(name, NsSeparator) {
				panic(fmt.Sprintf("x.RegisterReservedNamespace: name %q must be bare, "+
					"without the %q namespace separator", name, NsSeparator))
			}
		}
	}

	reservedNsMu.Lock()
	defer reservedNsMu.Unlock()
	if ns.PredicatePrefix != "" {
		reservedNsPrefixes = append(reservedNsPrefixes, strings.ToLower(ns.PredicatePrefix))
	}
	// A reserved name has a single owner. A duplicate registration (two
	// namespaces, or a re-init) is a programming error, and for value locks it
	// would silently change which TrustMarker wins by import order — so panic
	// rather than last-writer-wins.
	for _, p := range ns.Predicates {
		key := strings.ToLower(p)
		if _, dup := reservedNsPredicates[key]; dup {
			panic(fmt.Sprintf("x.RegisterReservedNamespace: predicate %q already registered", key))
		}
		reservedNsPredicates[key] = struct{}{}
	}
	for _, t := range ns.Types {
		key := strings.ToLower(t)
		if _, dup := reservedNsTypes[key]; dup {
			panic(fmt.Sprintf("x.RegisterReservedNamespace: type %q already registered", key))
		}
		reservedNsTypes[key] = struct{}{}
	}
	for _, p := range ns.ValueLocked {
		key := strings.ToLower(p)
		if _, dup := reservedNsValueLocked[key]; dup {
			panic(fmt.Sprintf("x.RegisterReservedNamespace: value-locked predicate %q already registered", key))
		}
		reservedNsValueLocked[key] = ns.TrustMarker
	}
}

// IsRegisteredReservedPredicate reports whether pred is owned by a registered
// ReservedNamespace (by dynamic prefix or exact name). The alter validator and
// the no-schema mutation guard allow such predicates through even though they
// are not pre-defined. With no registration it always reports false.
func IsRegisteredReservedPredicate(pred string) bool {
	p := strings.ToLower(ParseAttr(pred))
	reservedNsMu.RLock()
	defer reservedNsMu.RUnlock()
	for _, prefix := range reservedNsPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	_, ok := reservedNsPredicates[p]
	return ok
}

// IsRegisteredReservedType reports whether typ is a type owned by a registered
// ReservedNamespace. With no registration it always reports false.
func IsRegisteredReservedType(typ string) bool {
	t := strings.ToLower(ParseAttr(typ))
	reservedNsMu.RLock()
	defer reservedNsMu.RUnlock()
	_, ok := reservedNsTypes[t]
	return ok
}

// ReservedPredicateValueLock reports whether pred's value is locked to a
// trusted writer and, if so, returns the context key (TrustMarker) that
// authorizes the write. The mutation-value guard rejects a write of a locked
// predicate when the request context does not carry that marker.
//
// pred is matched case-insensitively (like the predicate/type lookups above) so
// a value lock cannot be bypassed by changing the case of an owned name. Unlike
// those lookups it does not ParseAttr: the guard passes the bare predicate (no
// namespace separator), matching how IsOtherReservedPredicate is consulted.
func ReservedPredicateValueLock(pred string) (marker any, locked bool) {
	reservedNsMu.RLock()
	defer reservedNsMu.RUnlock()
	marker, locked = reservedNsValueLocked[strings.ToLower(pred)]
	return marker, locked
}

// TODO: rename this map to a better suited name as per its properties. It is not just for GraphQL
// predicates, but for all those which are PreDefined and whose value is not allowed to be mutated
// by users. When renaming this also rename the IsGraphql context key in edgraph/server.go.
var otherReservedPredicate = map[string]struct{}{
	"dgraph.graphql.xid":     {},
	"dgraph.graphql.schema":  {},
	"dgraph.drop.op":         {},
	"dgraph.graphql.p_query": {},
	"dgraph.namespace.id":    {},
	"dgraph.namespace.name":  {},
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
	"dgraph.namespace":               {},
}

// IsOtherReservedPredicate returns true if it is the predicate is reserved by graphql.
// These are a subset of PreDefined predicates, so follow all their properties. In addition,
// the value for these predicates is also not allowed to be mutated directly by the users.
func IsOtherReservedPredicate(pred string) bool {
	_, ok := otherReservedPredicate[pred]
	return ok
}

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
//  1. dgraph.type (reserved = true,  pre_defined = true )
//  2. dgraph.blah (reserved = true,  pre_defined = false)
//  3. person.name (reserved = false, pre_defined = false)
func IsReservedPredicate(pred string) bool {
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
	return ok || IsAclPredicate(pred) || IsOtherReservedPredicate(pred)
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
