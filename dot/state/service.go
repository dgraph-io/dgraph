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
	"github.com/ChainSafe/gossamer/lib/blocktree"
	"github.com/ChainSafe/gossamer/lib/database"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
)

// Service is the struct that holds storage, block and network states
type Service struct {
	dbPath           string
	db               database.Database
	isMemDB          bool // set to true if using an in-memory database; only used for testing.
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
		isMemDB: false,
		Storage: nil,
		Block:   nil,
		Network: nil,
	}
}

// UseMemDB tells the service to use an in-memory key-value store instead of a persistent database.
// This should be called after NewService, and before Initialize.
// This should only be used for testing.
func (s *Service) UseMemDB() {
	s.isMemDB = true
}

// DB returns the Service's database
func (s *Service) DB() database.Database {
	return s.db
}

// Initialize initializes the genesis state of the DB using the given storage trie. The trie should be loaded with the genesis storage state.
// This only needs to be called during genesis initialization of the node; it doesn't need to be called during normal startup.
func (s *Service) Initialize(data *genesis.Data, header *types.Header, t *trie.Trie) error {
	var db database.Database

	// check database type
	if s.isMemDB {

		// create memory database
		db = database.NewMemDatabase()

	} else {

		// get data directory from service
		datadir, err := filepath.Abs(s.dbPath)
		if err != nil {
			return fmt.Errorf("failed to read datadir: %s", err)
		}

		// initialize database using data directory
		db, err = database.NewBadgerDB(datadir)
		if err != nil {
			return fmt.Errorf("failed to create database: %s", err)
		}
	}

	// write initial genesis values to database
	err := s.storeInitialValues(db, data, header, t)
	if err != nil {
		return fmt.Errorf("failed to write genesis values to database: %s", err)
	}

	// create and store blockree from genesis block
	bt := blocktree.NewBlockTreeFromGenesis(header, db)
	err = bt.Store()
	if err != nil {
		return fmt.Errorf("failed to write blocktree to database: %s", err)
	}

	// create storage state from genesis trie
	storageState, err := NewStorageState(db, t)
	if err != nil {
		return fmt.Errorf("failed to create storage state from trie: %s", err)
	}

	// create block state from genesis block
	blockState, err := NewBlockStateFromGenesis(db, header)
	if err != nil {
		return fmt.Errorf("failed to create block state from genesis: %s", err)
	}

	// check database type
	if s.isMemDB {

		// append memory database to state service
		s.db = db

		// append storage state and block state to state service
		s.Storage = storageState
		s.Block = blockState

	} else {

		// close database
		err = db.Close()
		if err != nil {
			return fmt.Errorf("failed to close database: %s", err)
		}
	}

	return nil
}

// storeInitialValues writes initial genesis values to the state database
func (s *Service) storeInitialValues(db database.Database, data *genesis.Data, header *types.Header, t *trie.Trie) error {

	// write genesis trie to database
	err := StoreTrie(db, t)
	if err != nil {
		return fmt.Errorf("failed to write trie to database: %s", err)
	}

	// write storage hash to database
	err = StoreLatestStorageHash(db, t)
	if err != nil {
		return fmt.Errorf("failed to write storage hash to database: %s", err)
	}

	// write best block hash to state database
	err = StoreBestBlockHash(db, header.Hash())
	if err != nil {
		return fmt.Errorf("failed to write best block hash to database: %s", err)
	}

	// write genesis data to state database
	err = StoreGenesisData(db, data)
	if err != nil {
		return fmt.Errorf("failed to write genesis data to database: %s", err)
	}

	return nil
}

// Start initializes the Storage database and the Block database.
func (s *Service) Start() error {
	if !s.isMemDB && (s.Storage != nil || s.Block != nil || s.Network != nil) {
		return nil
	}

	db := s.db
	if !s.isMemDB {
		datadir, err := filepath.Abs(s.dbPath)
		if err != nil {
			return err
		}

		// initialize database
		db, err = database.NewBadgerDB(datadir)
		if err != nil {
			return err
		}

		s.db = db
	}

	// retrieve latest header
	bestHash, err := LoadBestBlockHash(db)
	if err != nil {
		return fmt.Errorf("failed to get best block hash: %s", err)
	}

	log.Trace("[state] start", "best block hash", fmt.Sprintf("0x%x", bestHash))

	// create storage state
	s.Storage, err = NewStorageState(db, trie.NewEmptyTrie())
	if err != nil {
		return fmt.Errorf("failed to create storage state: %s", err)
	}

	// load blocktree
	bt := blocktree.NewEmptyBlockTree(db)
	err = bt.Load()
	if err != nil {
		return err
	}

	// create block state
	s.Block, err = NewBlockState(db, bt)
	if err != nil {
		return fmt.Errorf("failed to create block state: %s", err)
	}

	headBlock, err := s.Block.GetHeader(s.Block.BestBlockHash())
	if err != nil {
		return fmt.Errorf("failed to get chain head from database: %s", err)
	}

	log.Trace("[state] start", "best block state root", headBlock.StateRoot)

	// load current storage state
	err = s.Storage.LoadFromDB(headBlock.StateRoot)
	if err != nil {
		return fmt.Errorf("failed to get state root from database: %s", err)
	}

	// create network state
	s.Network = NewNetworkState()

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

	err = s.Block.bt.Store()
	if err != nil {
		return err
	}

	hash := s.Block.BestBlockHash()
	err = StoreBestBlockHash(s.db, hash)
	if err != nil {
		return err
	}

	err = s.storeHash()
	if err != nil {
		return err
	}

	log.Trace("[state] stop", "best block hash", hash)

	return s.db.Close()
}

// StoreHash stores the current root hash in the database at LatestStorageHashKey
func (s *Service) storeHash() error {
	return StoreLatestStorageHash(s.db, s.Storage.trie)
}
