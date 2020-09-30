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

package sync

import (
	"math/big"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
)

// BlockState is the interface for the block state
type BlockState interface {
	BestBlockHash() common.Hash
	BestBlockNumber() (*big.Int, error)
	AddBlock(*types.Block) error
	CompareAndSetBlockData(bd *types.BlockData) error
	GetBlockByNumber(*big.Int) (*types.Block, error)
	GetBlockBody(common.Hash) (*types.Body, error)
	SetHeader(*types.Header) error
	GetHeader(common.Hash) (*types.Header, error)
	HasHeader(hash common.Hash) (bool, error)
	SubChain(start, end common.Hash) ([]common.Hash, error)
	GetReceipt(common.Hash) ([]byte, error)
	GetMessageQueue(common.Hash) ([]byte, error)
	GetJustification(common.Hash) ([]byte, error)
}

// StorageState is the interface for the storage state
type StorageState interface {
	TrieState(root *common.Hash) (*state.TrieState, error)
	StoreTrie(root common.Hash, ts *state.TrieState) error
}

// TransactionState is the interface for transaction queue methods
type TransactionState interface {
	RemoveExtrinsic(ext types.Extrinsic)
}

// BlockProducer is the interface that a block production service must implement
type BlockProducer interface {
	Pause() error
	Resume() error
}

// DigestHandler is the interface for the consensus digest handler
type DigestHandler interface {
	Start()
	Stop()
	HandleConsensusDigest(*types.ConsensusDigest) error
}

// Verifier deals with block verification
type Verifier interface {
	VerifyBlock(header *types.Header) (bool, error)
}
