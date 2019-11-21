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
	"math/big"

	"github.com/ChainSafe/gossamer/common"
)

// Extrinsic is a generic transaction whose format is verified in the runtime
type Extrinsic []byte

// Block defines a state block
type Block struct {
	Header      BlockHeaderWithHash
	Body        BlockBody
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

// BlockHeaderWithHash is a state block header
type BlockHeaderWithHash struct {
	ParentHash     common.Hash `json:"parentHash"`
	Number         *big.Int    `json:"number"`
	StateRoot      common.Hash `json:"stateRoot"`
	ExtrinsicsRoot common.Hash `json:"extrinsicsRoot"`
	Digest         []byte      `json:"digest"` // any additional block info eg. logs, seal
	// TODO: Not part of spec, can potentially remove
	Hash common.Hash `json:"hash"`
}

// BlockBody is the extrinsics inside a state block
type BlockBody []byte

/// BlockData is stored within the BlockDB
type BlockData struct {
	Hash   common.Hash
	Header *BlockHeaderWithHash
	Body   *BlockBody
	// Receipt
	// MessageQueue
	// Justification
}
