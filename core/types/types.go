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
	"math/big"

	scale "github.com/ChainSafe/gossamer/codec"
	"github.com/ChainSafe/gossamer/common"
)

// Extrinsic is a generic transaction whose format is verified in the runtime
type Extrinsic []byte

// Block defines a state block
type Block struct {
	Header      *BlockHeader
	Body        *BlockBody
	arrivalTime uint64 // arrival time of this block
}

// GetBlockArrivalTime returns the arrival time for a block
func (b *Block) GetBlockArrivalTime() uint64 {
	return b.arrivalTime
}

// SetBlockArrivalTime sets the arrival time for a block
func (b *Block) SetBlockArrivalTime(t uint64) {
	b.arrivalTime = t
}

// BlockHeader is a state block header
type BlockHeader struct {
	ParentHash     common.Hash `json:"parentHash"`
	Number         *big.Int    `json:"number"`
	StateRoot      common.Hash `json:"stateRoot"`
	ExtrinsicsRoot common.Hash `json:"extrinsicsRoot"`
	Digest         []byte      `json:"digest"` // any additional block info eg. logs, seal
	hash           common.Hash
}

// NewBlockHeader creates a new block header and sets its hash field
func NewBlockHeader(parentHash common.Hash, number *big.Int, stateRoot common.Hash, extrinsicsRoot common.Hash, digest []byte) (*BlockHeader, error) {
	if number == nil {
		// Hash() will panic if number is nil
		return nil, errors.New("cannot have nil block number")
	}

	bh := &BlockHeader{
		ParentHash:     parentHash,
		Number:         number,
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         digest,
	}

	bh.Hash()
	return bh, nil
}

// Hash returns the hash of the block header
// If the internal hash field is nil, it hashes the block and sets the hash field.
// If hashing the header errors, this will panic.
func (bh *BlockHeader) Hash() common.Hash {
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

// BlockBody is the extrinsics inside a state block
type BlockBody []byte

/// BlockData is stored within the BlockDB
type BlockData struct {
	Hash   common.Hash
	Header *BlockHeader
	Body   *BlockBody
	// Receipt
	// MessageQueue
	// Justification
}
