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
	"errors"

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
// most significant two bits of first byte: 10 if branch w/o value, 11 if branch w/ value
// least significant six bits of first byte: if len(key) > 62, 0x3f, otherwise len(key)
// where Extra partial key length is included if len(key) > 63:
// consists of the remaining key length
// Partial Key is the branch's key
// Value is:
// Children Bitmap | Enc(Child[i_1]) | Enc(Child[i_2]) | ... | Enc(Child[i_n]) | SCALE Branch Node Value
func (b *branch) Encode() ([]byte, error) {
	encoding, err := b.header()
	if err != nil {
		return nil, err
	}

	encoding = append(encoding, nibblesToKey(b.key)...)
	encoding = append(encoding, common.Uint16ToBytes(b.childrenBitmap())...)

	for _, child := range b.children {
		if child != nil {
			encChild, e := Encode(child)
			if e != nil {
				return encoding, err
			}
			encoding = append(encoding, encChild...)
		}
	}

	buffer := bytes.Buffer{}
	se := scale.Encoder{Writer: &buffer}
	_, err = se.Encode(b.value)
	if err != nil {
		return encoding, err
	}
	encoding = append(encoding, buffer.Bytes()...)

	return encoding, nil
}

// Encode encodes a leaf with the following format:
// NodeHeader | Extra partial key length | Partial Key | Value
// where NodeHeader is a byte:
// most significant two bits of first byte: 01
// least signficant six bits of first byte: if len(key) > 62, 0x3f, otherwise len(key)
// where Extra partial key length is included if len(key) > 63:
// consists of the remaining key length
// Partial Key is the leaf's key
// Value is the leaf's SCALE encoded value
func (l *leaf) Encode() ([]byte, error) {
	encoding, err := l.header()
	if err != nil {
		return nil, err
	}

	encoding = append(encoding, nibblesToKey(l.key)...)

	buffer := bytes.Buffer{}
	se := scale.Encoder{Writer: &buffer}
	_, err = se.Encode(l.value)
	if err != nil {
		return encoding, err
	}
	encoding = append(encoding, buffer.Bytes()...)

	return encoding, nil
}

func (b *branch) header() ([]byte, error) {
	var header byte
	if b.value == nil {
		header = 2 << 6
	} else {
		header = 3 << 6
	}
	var encodePkLen []byte
	var err error

	if len(b.key) >= 63 {
		header = header | 0x3f
		encodePkLen, err = encodeExtraPartialKeyLength(len(b.key))
		if err != nil {
			return nil, err
		}
	} else {
		header = header | byte(len(b.key))
	}

	fullHeader := append([]byte{header}, encodePkLen...)
	return fullHeader, nil
}

func (l *leaf) header() ([]byte, error) {
	var header byte = 1 << 6
	var encodePkLen []byte
	var err error

	if len(l.key) >= 63 {
		header = header | 0x3f
		encodePkLen, err = encodeExtraPartialKeyLength(len(l.key))
		if err != nil {
			return nil, err
		}
	} else {
		header = header | byte(len(l.key))
	}

	fullHeader := append([]byte{header}, encodePkLen...)
	return fullHeader, nil
}

func encodeExtraPartialKeyLength(pkLen int) ([]byte, error) {
	pkLen -= 63
	fullHeader := []byte{}

	if pkLen >= 1<<16 {
		return nil, errors.New("partial key length greater than or equal to 2^16")
	}

	for i := 0; i < 1<<16; i++ {
		if pkLen < 255 {
			fullHeader = append(fullHeader, byte(pkLen))
			break
		} else {
			fullHeader = append(fullHeader, byte(255))
			pkLen -= 255
		}
	}

	return fullHeader, nil
}
