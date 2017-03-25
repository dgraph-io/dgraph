/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

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

type ParsedKey struct {
	byteType   byte
	Attr       string
	Uid        uint64
	Term       string
	bytePrefix byte
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

// SchemaPrefix returns the prefix for Schema keys.
func SchemaPrefix() []byte {
	buf := make([]byte, 1)
	buf[0] = byteSchema
	return buf
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
	case byteSchema:
		break
	default:
		// Some other data type.
		return nil
	}
	return p
}
