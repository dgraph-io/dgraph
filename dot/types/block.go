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

package types

import (
	"io"

	"github.com/ChainSafe/gossamer/lib/scale"
)

// Block defines a state block
type Block struct {
	Header *Header
	Body   *Body
}

// NewBlock returns a new Block
func NewBlock(header *Header, body *Body) *Block {
	return &Block{
		Header: header,
		Body:   body,
	}
}

// NewEmptyBlock returns a new Block with an initialized but empty Header and Body
func NewEmptyBlock() *Block {
	return &Block{
		Header: new(Header),
		Body:   new(Body),
	}
}

// Encode returns the SCALE encoding of a block
func (b *Block) Encode() ([]byte, error) {
	enc, err := scale.Encode(b.Header)
	if err != nil {
		return nil, err
	}

	// the scale encoding of an empty array of arrays is 0
	if len(b.Header.Digest) == 0 {
		enc = append(enc, 0)
	}

	// block body is already SCALE encoded
	return append(enc, []byte(*b.Body)...), nil
}

// MustEncode returns the SCALE encoded block and panics if it fails to encode
func (b *Block) MustEncode() []byte {
	enc, err := b.Encode()
	if err != nil {
		panic(err)
	}
	return enc
}

// Decode decodes the SCALE encoded input into this block
func (b *Block) Decode(r io.Reader) error {
	sd := scale.Decoder{Reader: r}
	_, err := sd.Decode(b)
	return err
}

// DeepCopy returns a copy of the block
func (b *Block) DeepCopy() *Block {
	bc := make([]byte, len(*b.Body))
	copy(bc, *b.Body)
	return &Block{
		Header: b.Header.DeepCopy(),
		Body:   NewBody(bc),
	}
}
