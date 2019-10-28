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

// Modified Merkle-Patricia Trie
// See https://github.com/w3f/polkadot-spec/blob/master/runtime-environment-spec/polkadot_re_spec.pdf for the full specification.
//
// Note that for the following definitions, `|` denotes concatentation
//
// Branch encoding:
// NodeHeader | Extra partial key length | Partial Key | Value
// `NodeHeader` is a byte such that:
// most significant two bits of `NodeHeader`: 10 if branch w/o value, 11 if branch w/ value
// least significant six bits of `NodeHeader`: if len(key) > 62, 0x3f, otherwise len(key)
// `Extra partial key length` is included if len(key) > 63 and consists of the remaining key length
// `Partial Key` is the branch's key
// `Value` is: Children Bitmap | SCALE Branch Node Value | Hash(Enc(Child[i_1])) | Hash(Enc(Child[i_2])) | ... | Hash(Enc(Child[i_n]))
//
// Leaf encoding:
// NodeHeader | Extra partial key length | Partial Key | Value
// `NodeHeader` is a byte such that:
// most significant two bits of `NodeHeader`: 01
// least signficant six bits of `NodeHeader`: if len(key) > 62, 0x3f, otherwise len(key)
// `Extra partial key length` is included if len(key) > 63 and consists of the remaining key length
// `Partial Key` is the leaf's key
// `Value` is the leaf's SCALE encoded value
package trie

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	scale "github.com/ChainSafe/gossamer/codec"
	"github.com/ChainSafe/gossamer/common"
)

type node interface {
	Encode() ([]byte, error)
	Decode(r io.Reader, h byte) error
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

// Encode encodes a branch with the encoding specified at the top of this package
func (b *branch) Encode() ([]byte, error) {
	encoding, err := b.header()
	if err != nil {
		return nil, err
	}

	encoding = append(encoding, nibblesToKeyLE(b.key)...)
	encoding = append(encoding, common.Uint16ToBytes(b.childrenBitmap())...)

	if b.value != nil {
		buffer := bytes.Buffer{}
		se := scale.Encoder{Writer: &buffer}
		_, err = se.Encode(b.value)
		if err != nil {
			return encoding, err
		}
		encoding = append(encoding, buffer.Bytes()...)
	}

	for _, child := range b.children {
		if child != nil {
			hasher, err := NewHasher()
			if err != nil {
				return nil, err
			}

			encChild, err := hasher.Hash(child)
			if err != nil {
				return encoding, err
			}
			scEncChild, err := scale.Encode(encChild)
			if err != nil {
				return encoding, err
			}
			encoding = append(encoding, scEncChild[:]...)
		}
	}

	return encoding, nil
}

// Encode encodes a leaf with the encoding specified at the top of this package
func (l *leaf) Encode() ([]byte, error) {
	encoding, err := l.header()
	if err != nil {
		return nil, err
	}

	encoding = append(encoding, nibblesToKeyLE(l.key)...)

	buffer := bytes.Buffer{}
	se := scale.Encoder{Writer: &buffer}
	_, err = se.Encode(l.value)
	if err != nil {
		return encoding, err
	}
	encoding = append(encoding, buffer.Bytes()...)

	return encoding, nil
}

// Decode wraps the decoding of different node types back into a node
func Decode(r io.Reader) (node, error) {
	header, err := readByte(r)
	if err != nil {
		return nil, err
	}

	nodeType := header >> 6
	if nodeType == 1 {
		l := new(leaf)
		err := l.Decode(r, header)
		return l, err
	} else if nodeType == 2 || nodeType == 3 {
		b := new(branch)
		err := b.Decode(r, header)
		return b, err
	}

	return nil, errors.New("cannot decode invalid encoding into node")
}

// Decode decodes a byte array with the encoding specified at the top of this package into a branch node
// Note that since the encoded branch stores the hash of the children nodes, we aren't able to reconstruct the child
// nodes from the encoding. This function instead stubs where the children are known to be with an empty leaf.
func (b *branch) Decode(r io.Reader, header byte) (err error) {
	if header == 0 {
		header, err = readByte(r)
		if err != nil {
			return err
		}
	}

	nodeType := header >> 6
	if nodeType != 2 && nodeType != 3 {
		return fmt.Errorf("cannot decode node to branch")
	}

	keyLen := header & 0x3f
	b.key, err = decodeKey(r, keyLen)
	if err != nil {
		return err
	}

	childrenBitmap := make([]byte, 2)
	_, err = r.Read(childrenBitmap)
	if err != nil {
		return err
	}

	if nodeType == 3 {
		// branch w/ value
		sd := &scale.Decoder{Reader: r}
		value, err := sd.Decode([]byte{})
		if err != nil {
			return err
		}
		b.value = value.([]byte)
	}

	for i := 0; i < 16; i++ {
		if (childrenBitmap[i/8]>>(i%8))&1 == 1 {
			b.children[i] = &leaf{}
		}
	}

	b.dirty = true

	return nil
}

// Decode decodes a byte array with the encoding specified at the top of this package into a leaf node
func (l *leaf) Decode(r io.Reader, header byte) (err error) {
	if header == 0 {
		header, err = readByte(r)
		if err != nil {
			return err
		}
	}

	nodeType := header >> 6
	if nodeType != 1 {
		return fmt.Errorf("cannot decode node to leaf")
	}

	keyLen := header & 0x3f
	l.key, err = decodeKey(r, keyLen)
	if err != nil {
		return err
	}

	sd := &scale.Decoder{Reader: r}
	value, err := sd.Decode([]byte{})
	if err != nil {
		return err
	}

	if len(value.([]byte)) > 0 {
		l.value = value.([]byte)
	}

	l.dirty = true

	return nil
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

func decodeKey(r io.Reader, keyLen byte) ([]byte, error) {
	var totalKeyLen int = int(keyLen)

	if keyLen == 0x3f {
		// partial key longer than 63, read next bytes for rest of pk len
		for {
			nextKeyLen, err := readByte(r)
			if err != nil {
				return nil, err
			}
			totalKeyLen += int(nextKeyLen)

			if nextKeyLen < 0xff {
				break
			}

			if totalKeyLen >= 1<<16 {
				return nil, errors.New("partial key length greater than or equal to 2^16")
			}
		}
	}

	if totalKeyLen != 0 {
		key := make([]byte, totalKeyLen/2+totalKeyLen%2)
		_, err := r.Read(key)
		if err != nil {
			return key, err
		}

		return keyToNibbles(key)[totalKeyLen%2:], nil
	}

	return []byte{}, nil
}

func readByte(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}
