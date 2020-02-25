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

package state

import (
	"fmt"
	"path/filepath"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
)

// Service is the struct that holds storage, block and network states
type Service struct {
	dbPath  string
	Storage *StorageState
	Block   *BlockState
	Network *NetworkState
}

// NewService create a new instance of Service
func NewService(path string) *Service {
	return &Service{
		dbPath:  path,
		Storage: nil,
		Block:   nil,
		Network: nil,
	}
}

// Initialize initializes the genesis state of the DB using the given storage trie. The trie should be loaded with the genesis storage state.
// The trie does not need a backing DB, since the DB will be created during Service.Start().
// This only needs to be called during genesis initialization of the node; it doesn't need to be called during normal startup.
func (s *Service) Initialize(genesisHeader *types.Header, t *trie.Trie) error {
	stateDataDir := filepath.Join(s.dbPath, "state")
	blockDataDir := filepath.Join(s.dbPath, "block")
	networkDataDir := filepath.Join(s.dbPath, "network")

	storageState, err := NewStorageState(stateDataDir, t)
	if err != nil {
		return err
	}

	err = storageState.StoreInDB()
	if err != nil {
		return err
	}

	hash := genesisHeader.Hash()
	err = storageState.DB.DB.Put(common.LatestHeaderHashKey, hash[:])
	if err != nil {
		return err
	}

	blockState, err := NewBlockStateFromGenesis(blockDataDir, genesisHeader)
	if err != nil {
		return err
	}

	networkState, err := NewNetworkState(networkDataDir)
	if err != nil {
		return err
	}

	err = blockState.db.Db.Close()
	if err != nil {
		return err
	}

	err = networkState.db.Db.Close()
	if err != nil {
		return err
	}

	return storageState.DB.DB.Close()
}

// Start initializes the Storage database and the Block database.
func (s *Service) Start() error {
	if s.Storage != nil || s.Block != nil || s.Network != nil {
		return nil
	}

	stateDataDir := filepath.Join(s.dbPath, "state")
	blockDataDir := filepath.Join(s.dbPath, "block")
	networkDataDir := filepath.Join(s.dbPath, "network")

	storageState, err := NewStorageState(stateDataDir, trie.NewEmptyTrie(nil))
	if err != nil {
		return fmt.Errorf("cannot make storage state: %s", err)
	}

	latestHeaderHash, err := storageState.DB.DB.Get(common.LatestHeaderHashKey)
	if err != nil {
		return fmt.Errorf("cannot get latest hash: %s", err)
	}

	log.Trace("state service", "latestHeaderHash", latestHeaderHash)

	blockState, err := NewBlockState(blockDataDir, common.BytesToHash(latestHeaderHash))
	if err != nil {
		return fmt.Errorf("cannot make block state: %s", err)
	}

	err = storageState.LoadFromDB(blockState.latestHeader.StateRoot)
	if err != nil {
		return fmt.Errorf("cannot load state from DB: %s", err)
	}

	networkState, err := NewNetworkState(networkDataDir)
	if err != nil {
		return fmt.Errorf("cannot make network state: %s", err)
	}

	s.Storage = storageState
	s.Block = blockState
	s.Network = networkState

	return nil
}

// Stop closes each state database
func (s *Service) Stop() error {
	err := s.Storage.StoreInDB()
	if err != nil {
		return err
	}

	err = s.Storage.DB.DB.Close()
	if err != nil {
		return err
	}

	err = s.Block.db.Db.Close()
	if err != nil {
		return err
	}

	err = s.Network.db.Db.Close()
	if err != nil {
		return err
	}

	return nil
}
