// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package trie

import (
	"bytes"

	scale "github.com/ChainSafe/gossamer/codec"
	"github.com/ChainSafe/gossamer/common"
)

type node interface {
	Encode() ([]byte, error)
	isDirty() bool
	setDirty(dirty bool)
	setKey(key []byte)
}

type (
	branch struct {
		key      []byte // partial key
		children [16]node
		value    []byte
		dirty    bool
	}
	leaf struct {
		key   []byte // partial key
		value []byte
		dirty bool
	}
)

func (b *branch) childrenBitmap() uint16 {
	var bitmap uint16
	var i uint
	for i = 0; i < 16; i++ {
		if b.children[i] != nil {
			bitmap = bitmap | 1<<i
		}
	}
	return bitmap
}

func (b *branch) numChildren() int {
	var i, count int
	for i = 0; i < 16; i++ {
		if b.children[i] != nil {
			count++
		}
	}
	return count
}

func (l *leaf) isDirty() bool {
	return l.dirty
}

func (b *branch) isDirty() bool {
	return b.dirty
}

func (l *leaf) setDirty(dirty bool) {
	l.dirty = dirty
}

func (b *branch) setDirty(dirty bool) {
	b.dirty = dirty
}

func (l *leaf) setKey(key []byte) {
	l.key = key
}

func (b *branch) setKey(key []byte) {
	b.key = key
}

// Encode is the high-level function wrapping the encoding for different node types
// encoding has the following format:
// NodeHeader | Extra partial key length | Partial Key | Value
func Encode(n node) ([]byte, error) {
	switch n := n.(type) {
	case *branch:
		return n.Encode()
	case *leaf:
		return n.Encode()
	case nil:
		return []byte{0}, nil
	}

	return nil, nil
}

// Encode encodes a branch with the following format:
// NodeHeader | Extra partial key length | Partial Key | Value
// where NodeHeader is a byte:
// bottom two bits of first byte: 10 if branch w/o value, 11 if branch w/ value
// top six bits of first byte: if len(key) > 62, 0xff, otherwise len(key)
// where Extra partial key length is included if len(key) > 63:
// consists of the remaining key length
// Partial Key is the branch's key
// Value is:
// Children Bitmap | Enc(Child[i_1]) | Enc(Child[i_2]) | ... | Enc(Child[i_n]) | SCALE Branch Node Value
func (b *branch) Encode() ([]byte, error) {
	encoding := b.header()
	encoding = append(encoding, nibblesToKey(b.key)...)
	encoding = append(encoding, common.Uint16ToBytes(b.childrenBitmap())...)

	for _, child := range b.children {
		if child != nil {
			encChild, err := Encode(child)
			if err != nil {
				return encoding, err
			}
			encoding = append(encoding, encChild...)
		}
	}

	buffer := bytes.Buffer{}
	se := scale.Encoder{Writer: &buffer}
	_, err := se.Encode(b.value)
	if err != nil {
		return encoding, err
	}
	encoding = append(encoding, buffer.Bytes()...)

	return encoding, nil
}

// Encode encodes a leaf with the following format:
// NodeHeader | Extra partial key length | Partial Key | Value
// where NodeHeader is a byte:
// bottom two bits of first byte: 01
// top six bits of first byte: if len(key) > 62, 0xff, otherwise len(key)
// where Extra partial key length is included if len(key) > 63:
// consists of the remaining key length
// Partial Key is the leaf's key
// Value is the leaf's SCALE encoded value
func (l *leaf) Encode() ([]byte, error) {
	encoding := l.header()
	encoding = append(encoding, nibblesToKey(l.key)...)

	buffer := bytes.Buffer{}
	se := scale.Encoder{Writer: &buffer}
	_, err := se.Encode(l.value)
	if err != nil {
		return encoding, err
	}
	encoding = append(encoding, buffer.Bytes()...)

	return encoding, nil
}

func (b *branch) header() []byte {
	var header byte
	if b.value == nil {
		header = 2 << 6
	} else {
		header = 3 << 6
	}
	var encodePkLen []byte

	if len(b.key) >= 63 {
		header = header | 0x3f
		encodePkLen = encodeExtraPartialKeyLength(len(b.key))
	} else {
		header = header | byte(len(b.key))
	}

	fullHeader := append([]byte{header}, encodePkLen...)
	return fullHeader
}

func (l *leaf) header() []byte {
	var header byte = 1 << 6
	var encodePkLen []byte

	if len(l.key) >= 63 {
		header = header | 0x3f
		encodePkLen = encodeExtraPartialKeyLength(len(l.key))
	} else {
		header = header | byte(len(l.key))
	}

	fullHeader := append([]byte{header}, encodePkLen...)
	return fullHeader
}

func encodeExtraPartialKeyLength(pkLen int) []byte {
	pkLen -= 63
	fullHeader := []byte{}
	for i := 0; i < 317; i++ {
		if pkLen < 255 {
			fullHeader = append(fullHeader, byte(pkLen))
			break
		} else {
			fullHeader = append(fullHeader, byte(255))
			pkLen -= 255
		}
	}

	return fullHeader
}
