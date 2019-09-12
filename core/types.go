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

package core

import (
	"math/big"

	"github.com/ChainSafe/gossamer/common"
)

// Block defines a state block
type Block struct {
	Header BlockHeader
	Body   BlockBody
}

// BlockHeader is a state block header
type BlockHeader struct {
	ParentHash     common.Hash
	Number         *big.Int
	StateRoot      common.Hash
	ExtrinsicsRoot common.Hash
	Digest         []byte
	// TODO: Not part of spec, can potentially remove
	Hash common.Hash
}

// BlockBody is the extrinsics inside a state block
type BlockBody []byte

/// BlockData is stored within the BlockDB
type BlockData struct {
	Hash   common.Hash
	Header BlockHeader
	Body   BlockBody
	// Receipt
	// MessageQueue
	// Justification
}
