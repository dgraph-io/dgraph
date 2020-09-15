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

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/services"
	"github.com/ChainSafe/gossamer/lib/transaction"
)

// BlockState interface for block state methods
type BlockState interface {
	BestBlockHash() common.Hash
	BestBlockHeader() (*types.Header, error)
	BestBlockNumber() (*big.Int, error)
	BestBlockStateRoot() (common.Hash, error)
	BestBlock() (*types.Block, error)
	AddBlock(*types.Block) error
	GetAllBlocksAtDepth(hash common.Hash) []common.Hash
	AddBlockWithArrivalTime(*types.Block, uint64) error
	GetBlockByHash(common.Hash) (*types.Block, error)
	GetArrivalTime(common.Hash) (uint64, error)
	GenesisHash() common.Hash
	GetSlotForBlock(common.Hash) (uint64, error)
	HighestBlockHash() common.Hash
	HighestBlockNumber() *big.Int
	GetFinalizedHeader(uint64, uint64) (*types.Header, error)
	GetFinalizedHash(uint64, uint64) (common.Hash, error)
	SetFinalizedHash(common.Hash, uint64, uint64) error
	RegisterImportedChannel(ch chan<- *types.Block) (byte, error)
	UnregisterImportedChannel(id byte)
	RegisterFinalizedChannel(ch chan<- *types.Header) (byte, error)
	UnregisterFinalizedChannel(id byte)
}

// StorageState interface for storage state methods
type StorageState interface {
	StoreTrie(common.Hash, *state.TrieState) error
	StoreInDB(root common.Hash) error
	LoadCode(root *common.Hash) ([]byte, error)
	LoadCodeHash(root *common.Hash) (common.Hash, error)
	TrieState(root *common.Hash) (*state.TrieState, error)
}

// TransactionQueue is the interface for transaction queue methods
type TransactionQueue interface {
	Push(vt *transaction.ValidTransaction) (common.Hash, error)
	Pop() *transaction.ValidTransaction
	Peek() *transaction.ValidTransaction
	RemoveExtrinsic(ext types.Extrinsic)
}

// FinalityGadget is the interface that a finality gadget must implement
type FinalityGadget interface {
	services.Service

	GetVoteOutChannel() <-chan FinalityMessage
	GetVoteInChannel() chan<- FinalityMessage
	GetFinalizedChannel() <-chan FinalityMessage
	UpdateAuthorities(ad []*types.Authority)
	Authorities() []*types.Authority
}

// FinalityMessage is the interface a finality message must implement
type FinalityMessage interface {
	ToConsensusMessage() (*network.ConsensusMessage, error)
	Type() byte
}

// ConsensusMessageHandler is the interface a consensus message handler must implement
type ConsensusMessageHandler interface {
	HandleMessage(*network.ConsensusMessage) (*network.ConsensusMessage, error)
}

// BlockProducer is the interface that a block production service must implement
type BlockProducer interface {
	GetBlockChannel() <-chan types.Block
	SetRuntime(*runtime.Runtime) error
	Authorities() []*types.Authority
	SetAuthorities([]*types.Authority) error
}

// Verifier is the interface for the block verifier
type Verifier interface {
	SetRuntimeChangeAtBlock(header *types.Header, rt *runtime.Runtime) error
	SetAuthorityChangeAtBlock(header *types.Header, authorities []*types.Authority)
}

// Network is the interface for the network service
type Network interface {
	SendMessage(network.Message)
}
