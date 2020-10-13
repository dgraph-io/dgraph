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

package runtime

import (
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/transaction"
	"github.com/ChainSafe/gossamer/lib/trie"
)

// Instance is the interface a runtime instance must implement
type Instance interface {
	Stop()
	NodeStorage() NodeStorage
	NetworkService() BasicNetwork

	Exec(function string, data []byte) ([]byte, error)
	SetContext(s Storage) // used to set the TrieState before a runtime call
	Version() (*VersionAPI, error)
	Metadata() ([]byte, error)
	BabeConfiguration() (*types.BabeConfiguration, error)
	GrandpaAuthorities() ([]*types.Authority, error)
	ValidateTransaction(e types.Extrinsic) (*transaction.Validity, error)
	InitializeBlock(header *types.Header) error
	InherentExtrinsics(data []byte) ([]byte, error)
	ApplyExtrinsic(data types.Extrinsic) ([]byte, error)
	FinalizeBlock() (*types.Header, error)
	ExecuteBlock(block *types.Block) ([]byte, error)
}

// Storage interface
type Storage interface {
	Set(key []byte, value []byte) error
	Get(key []byte) ([]byte, error)
	Root() (common.Hash, error)
	SetChild(keyToChild []byte, child *trie.Trie) error
	SetChildStorage(keyToChild, key, value []byte) error
	GetChildStorage(keyToChild, key []byte) ([]byte, error)
	Delete(key []byte) error
	Entries() map[string][]byte
	SetBalance(key [32]byte, balance uint64) error
	GetBalance(key [32]byte) (uint64, error)
	DeleteChildStorage(key []byte) error
	ClearChildStorage(keyToChild, key []byte) error
}

// BasicNetwork interface for functions used by runtime network state function
type BasicNetwork interface {
	NetworkState() common.NetworkState
}

// BasicStorage interface for functions used by runtime offchain workers
type BasicStorage interface {
	Put(key []byte, value []byte) error
	Get(key []byte) ([]byte, error)
}

// TransactionState interface for adding transactions to pool
type TransactionState interface {
	AddToPool(vt *transaction.ValidTransaction) common.Hash
}
