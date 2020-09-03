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
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/scale"
)

// Header is a state block header
type Header struct {
	ParentHash     common.Hash `json:"parentHash"`
	Number         *big.Int    `json:"number"`
	StateRoot      common.Hash `json:"stateRoot"`
	ExtrinsicsRoot common.Hash `json:"extrinsicsRoot"`
	Digest         [][]byte    `json:"digest"`
	hash           common.Hash
}

// NewHeader creates a new block header and sets its hash field
func NewHeader(parentHash common.Hash, number *big.Int, stateRoot common.Hash, extrinsicsRoot common.Hash, digest [][]byte) (*Header, error) {
	if number == nil {
		// Hash() will panic if number is nil
		return nil, errors.New("cannot have nil block number")
	}

	bh := &Header{
		ParentHash:     parentHash,
		Number:         number,
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         digest,
	}

	bh.Hash()
	return bh, nil
}

// DeepCopy returns a deep copy of the header to prevent side effects down the road
func (bh *Header) DeepCopy() *Header {
	//copy everything but pointers / array
	safeCopyHeader := *bh
	//copy number ptr
	if bh.Number != nil {
		safeCopyHeader.Number = new(big.Int).Set(bh.Number)
	}
	//copy digest byte array
	if len(bh.Digest) > 0 {
		safeCopyHeader.Digest = make([][]byte, len(bh.Digest))
		copy(safeCopyHeader.Digest, bh.Digest)
	}

	return &safeCopyHeader
}

// String returns the formatted header as a string
func (bh *Header) String() string {
	return fmt.Sprintf("ParentHash=%s Number=%d StateRoot=%s ExtrinsicsRoot=%s Digest=%v Hash=%s",
		bh.ParentHash, bh.Number, bh.StateRoot, bh.ExtrinsicsRoot, bh.Digest, bh.Hash())
}

// Hash returns the hash of the block header
// If the internal hash field is nil, it hashes the block and sets the hash field.
// If hashing the header errors, this will panic.
func (bh *Header) Hash() common.Hash {
	if bh.hash == [32]byte{} {
		enc, err := scale.Encode(bh)
		if err != nil {
			panic(err)
		}

		hash, err := common.Blake2bHash(enc)
		if err != nil {
			panic(err)
		}

		bh.hash = hash
	}

	return bh.hash
}

// Encode returns the SCALE encoding of a header
func (bh *Header) Encode() ([]byte, error) {
	return scale.Encode(bh)
}

// MustEncode returns the SCALE encoded header and panics if it fails to encode
func (bh *Header) MustEncode() []byte {
	enc, err := bh.Encode()
	if err != nil {
		panic(err)
	}
	return enc
}

// Decode decodes the SCALE encoded input into this header
func (bh *Header) Decode(r io.Reader) (*Header, error) {
	sd := scale.Decoder{Reader: r}
	_, err := sd.Decode(bh)
	return bh, err
}

// AsOptional returns the Header as an optional.Header
func (bh *Header) AsOptional() *optional.Header {
	return optional.NewHeader(true, &optional.CoreHeader{
		ParentHash:     bh.ParentHash,
		Number:         bh.Number,
		StateRoot:      bh.StateRoot,
		ExtrinsicsRoot: bh.ExtrinsicsRoot,
		Digest:         bh.Digest,
	})
}

// NewHeaderFromOptional returns a Header given an optional.Header. If the optional.Header is None, an error is returned.
func NewHeaderFromOptional(oh *optional.Header) (*Header, error) {
	if !oh.Exists() {
		return nil, errors.New("header is None")
	}

	h := oh.Value()

	if h.Number == nil {
		// Hash() will panic if number is nil
		return nil, errors.New("cannot have nil block number")
	}

	bh := &Header{
		ParentHash:     h.ParentHash,
		Number:         h.Number,
		StateRoot:      h.StateRoot,
		ExtrinsicsRoot: h.ExtrinsicsRoot,
		Digest:         h.Digest,
	}

	bh.Hash()
	return bh, nil
}

// decodeOptionalHeader decodes a SCALE encoded optional Header into an *optional.Header
func decodeOptionalHeader(r io.Reader) (*optional.Header, error) {
	sd := scale.Decoder{Reader: r}

	exists, err := common.ReadByte(r)
	if err != nil {
		return nil, err
	}

	if exists == 1 {
		header := &Header{
			ParentHash:     common.Hash{},
			Number:         big.NewInt(0),
			StateRoot:      common.Hash{},
			ExtrinsicsRoot: common.Hash{},
			Digest:         [][]byte{},
		}
		_, err = sd.Decode(header)
		if err != nil {
			return nil, err
		}

		header.Hash()
		return header.AsOptional(), nil
	}

	return optional.NewHeader(false, nil), nil
}
