/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
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
)

const (
	// TODO(pawan) - Make this 2 bytes long. Right now ParsedKey has byteType and
	// bytePrefix. Change it so that it just has one field which has all the information.
	byteData     = byte(0x00)
	byteSchema   = byte(0x01)
	byteIndex    = byte(0x02)
	byteReverse  = byte(0x04)
	byteCount    = byte(0x08)
	byteCountRev = byteCount | byteReverse
	// same prefix for data, index and reverse keys so that relative order of data doesn't change
	// keys of same attributes are located together
	defaultPrefix = byte(0x00)
)

func writeAttr(buf []byte, attr string) []byte {
	AssertTrue(len(attr) < math.MaxUint16)
	binary.BigEndian.PutUint16(buf[:2], uint16(len(attr)))

	rest := buf[2:]
	AssertTrue(len(attr) == copy(rest, attr[:]))

	return rest[len(attr):]
}

// SchemaKey returns schema key for given attribute,
// schema keys are stored separately with unique prefix,
// since we need to iterate over all schema keys
func SchemaKey(attr string) []byte {
	buf := make([]byte, 2+len(attr)+2)
	buf[0] = byteSchema
	rest := buf[1:]

	rest = writeAttr(rest, attr)
	rest[0] = byteSchema

	return buf
}

func DataKey(attr string, uid uint64) []byte {
	buf := make([]byte, 2+len(attr)+2+8)
	buf[0] = defaultPrefix
	rest := buf[1:]

	rest = writeAttr(rest, attr)
	rest[0] = byteData

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

func ReverseKey(attr string, uid uint64) []byte {
	buf := make([]byte, 2+len(attr)+2+8)
	buf[0] = defaultPrefix
	rest := buf[1:]

	rest = writeAttr(rest, attr)
	rest[0] = byteReverse

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

func IndexKey(attr, term string) []byte {
	buf := make([]byte, 2+len(attr)+2+len(term))
	buf[0] = defaultPrefix
	rest := buf[1:]

	rest = writeAttr(rest, attr)
	rest[0] = byteIndex

	rest = rest[1:]
	AssertTrue(len(term) == copy(rest, term[:]))
	return buf
}

func CountKey(attr string, count uint32, reverse bool) []byte {
	buf := make([]byte, 1+2+len(attr)+1+4)
	buf[0] = defaultPrefix
	rest := buf[1:]

	rest = writeAttr(rest, attr)
	if reverse {
		rest[0] = byteCountRev
	} else {
		rest[0] = byteCount
	}

	rest = rest[1:]
	binary.BigEndian.PutUint32(rest, count)
	return buf
}

type ParsedKey struct {
	byteType   byte
	Attr       string
	Uid        uint64
	Term       string
	Count      uint32
	bytePrefix byte
}

func (p ParsedKey) IsData() bool {
	return p.byteType == byteData
}

func (p ParsedKey) IsReverse() bool {
	return p.byteType == byteReverse
}

func (p ParsedKey) IsCount() bool {
	return p.byteType == byteCount ||
		p.byteType == byteCountRev
}

func (p ParsedKey) IsIndex() bool {
	return p.byteType == byteIndex
}

func (p ParsedKey) IsSchema() bool {
	return p.byteType == byteSchema
}

func (p ParsedKey) SkipPredicate() []byte {
	buf := make([]byte, 2+len(p.Attr)+2)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = 0xFF
	return buf
}

func (p ParsedKey) SkipRangeOfSameType() []byte {
	buf := make([]byte, 2+len(p.Attr)+2)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = p.byteType + 1
	return buf
}

func (p ParsedKey) SkipSchema() []byte {
	buf := make([]byte, 1)
	buf[0] = byteSchema + 1
	return buf
}

// DataPrefix returns the prefix for data keys.
func (p ParsedKey) DataPrefix() []byte {
	buf := make([]byte, 2+len(p.Attr)+2)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = byteData
	return buf
}

// IndexPrefix returns the prefix for index keys.
func (p ParsedKey) IndexPrefix() []byte {
	buf := make([]byte, 2+len(p.Attr)+2)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = byteIndex
	return buf
}

// ReversePrefix returns the prefix for index keys.
func (p ParsedKey) ReversePrefix() []byte {
	buf := make([]byte, 2+len(p.Attr)+2)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 1)
	k[0] = byteReverse
	return buf
}

// CountPrefix returns the prefix for count keys.
func (p ParsedKey) CountPrefix(reverse bool) []byte {
	buf := make([]byte, 1+2+len(p.Attr)+1)
	buf[0] = p.bytePrefix
	rest := buf[1:]
	k := writeAttr(rest, p.Attr)
	AssertTrue(len(k) == 1)
	if reverse {
		k[0] = byteCountRev
	} else {
		k[0] = byteCount
	}
	return buf
}

// SchemaPrefix returns the prefix for Schema keys.
func SchemaPrefix() []byte {
	buf := make([]byte, 1)
	buf[0] = byteSchema
	return buf
}

func ParseAttr(key []byte) string {
	sz := int(binary.BigEndian.Uint16(key[1:3]))
	k := key[3:]
	return string(k[:sz])
}

func Parse(key []byte) *ParsedKey {
	p := &ParsedKey{}

	p.bytePrefix = key[0]
	sz := int(binary.BigEndian.Uint16(key[1:3]))
	k := key[3:]

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
	case byteCount, byteCountRev:
		p.Count = binary.BigEndian.Uint32(k)
	case byteSchema:
		break
	default:
		// Some other data type.
		return nil
	}
	return p
}
