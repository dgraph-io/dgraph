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

	// fix since scale doesn't handle *types.Body types, but does handle []byte
	encBody, err := scale.Encode([]byte(*b.Body))
	if err != nil {
		return nil, err
	}

	return append(enc, encBody...), nil
}

// Decode decodes the SCALE encoded input into this block
func (b *Block) Decode(in []byte) error {
	_, err := scale.Decode(in, b)
	return err
}
