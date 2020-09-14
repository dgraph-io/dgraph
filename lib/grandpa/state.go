// Copyright 2020 ChainSafe Systems (ON) Corp.
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

package grandpa

import (
	"math/big"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
)

// BlockState is the interface required by GRANDPA into the block state
type BlockState interface {
	GenesisHash() common.Hash
	HasHeader(hash common.Hash) (bool, error)
	GetHeader(hash common.Hash) (*types.Header, error)
	GetHeaderByNumber(num *big.Int) (*types.Header, error)
	IsDescendantOf(parent, child common.Hash) (bool, error)
	HighestCommonAncestor(a, b common.Hash) (common.Hash, error)
	HasFinalizedBlock(round, setID uint64) (bool, error)
	GetFinalizedHeader(uint64, uint64) (*types.Header, error)
	SetFinalizedHash(common.Hash, uint64, uint64) error
	BestBlockHeader() (*types.Header, error)
	BestBlockHash() common.Hash
	Leaves() []common.Hash
	BlocktreeAsString() string
	RegisterImportedChannel(ch chan<- *types.Block) (byte, error)
	UnregisterImportedChannel(id byte)
	RegisterFinalizedChannel(ch chan<- *types.Header) (byte, error)
	UnregisterFinalizedChannel(id byte)
	SetJustification(hash common.Hash, data []byte) error
	HasJustification(hash common.Hash) (bool, error)
	GetJustification(hash common.Hash) ([]byte, error)
}

// DigestHandler is the interface required by GRANDPA for the digest handler
type DigestHandler interface {
	NextGrandpaAuthorityChange() uint64
}
