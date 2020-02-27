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
	"github.com/ChainSafe/gossamer/lib/database"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
)

// Service is the struct that holds storage, block and network states
type Service struct {
	dbPath           string
	db               database.Database
	Storage          *StorageState
	Block            *BlockState
	Network          *NetworkState
	TransactionQueue *TransactionQueue
}

// NewService create a new instance of Service
func NewService(path string) *Service {
	return &Service{
		dbPath:  path,
		db:      nil,
		Storage: nil,
		Block:   nil,
		Network: nil,
	}
}

// Initialize initializes the genesis state of the DB using the given storage trie. The trie should be loaded with the genesis storage state.
// The trie does not need a backing DB, since the DB will be created during Service.Start().
// This only needs to be called during genesis initialization of the node; it doesn't need to be called during normal startup.
func (s *Service) Initialize(genesisHeader *types.Header, t *trie.Trie) error {
	datadir, err := filepath.Abs(s.dbPath)
	if err != nil {
		return err
	}

	// initialize database
	db, err := database.NewBadgerDB(datadir)
	if err != nil {
		return err
	}

	// load genesis storage state into db
	storageState, err := NewStorageState(db, t)
	if err != nil {
		return err
	}

	err = storageState.StoreInDB()
	if err != nil {
		return err
	}

	// load genesis hash into db
	hash := genesisHeader.Hash()
	err = db.Put(common.LatestHeaderHashKey, hash[:])
	if err != nil {
		return err
	}

	// load genesis block into db
	_, err = NewBlockStateFromGenesis(db, genesisHeader)
	if err != nil {
		return err
	}

	return db.Close()
}

// Start initializes the Storage database and the Block database.
func (s *Service) Start() error {
	if s.Storage != nil || s.Block != nil || s.Network != nil {
		return nil
	}

	datadir, err := filepath.Abs(s.dbPath)
	if err != nil {
		return err
	}

	// initialize database
	db, err := database.NewBadgerDB(datadir)
	if err != nil {
		return err
	}

	s.db = db

	// retrieve latest header
	latestHeaderHash, err := s.db.Get(common.LatestHeaderHashKey)
	if err != nil {
		return fmt.Errorf("cannot get latest hash: %s", err)
	}

	log.Trace("state service", "latestHeaderHash", latestHeaderHash)

	// create storage state
	s.Storage, err = NewStorageState(db, trie.NewEmptyTrie(nil))
	if err != nil {
		return fmt.Errorf("cannot make storage state: %s", err)
	}

	// create block state
	s.Block, err = NewBlockState(db, common.BytesToHash(latestHeaderHash))
	if err != nil {
		return fmt.Errorf("cannot make block state: %s", err)
	}

	// load current storage state
	err = s.Storage.LoadFromDB(s.Block.latestHeader.StateRoot)
	if err != nil {
		return fmt.Errorf("cannot load state from DB: %s", err)
	}

	// create network state
	s.Network, err = NewNetworkState(db)
	if err != nil {
		return fmt.Errorf("cannot make network state: %s", err)
	}

	// create transaction queue
	s.TransactionQueue = NewTransactionQueue()

	return nil
}

// Stop closes each state database
func (s *Service) Stop() error {
	err := s.Storage.StoreInDB()
	if err != nil {
		return err
	}

	return s.db.Close()
}
