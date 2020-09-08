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
	"os"
	"path/filepath"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/blocktree"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/ChainSafe/chaindb"
	log "github.com/ChainSafe/log15"
)

var logger = log.New("pkg", "state")

// Service is the struct that holds storage, block and network states
type Service struct {
	dbPath           string
	db               chaindb.Database
	isMemDB          bool // set to true if using an in-memory database; only used for testing.
	Storage          *StorageState
	Block            *BlockState
	Network          *NetworkState
	TransactionQueue *TransactionQueue
	Epoch            *EpochState
}

// NewService create a new instance of Service
func NewService(path string, lvl log.Lvl) *Service {
	handler := log.StreamHandler(os.Stdout, log.TerminalFormat())
	handler = log.CallerFileHandler(handler)
	logger.SetHandler(log.LvlFilterHandler(lvl, handler))

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
func (s *Service) DB() chaindb.Database {
	return s.db
}

// Initialize initializes the genesis state of the DB using the given storage trie. The trie should be loaded with the genesis storage state.
// This only needs to be called during genesis initialization of the node; it doesn't need to be called during normal startup.
func (s *Service) Initialize(data *genesis.Data, header *types.Header, t *trie.Trie, epochInfo *types.EpochInfo) error {
	var db chaindb.Database

	// check database type
	if s.isMemDB {

		// create memory database
		db = chaindb.NewMemDatabase()

	} else {

		// get data directory from service
		basepath, err := filepath.Abs(s.dbPath)
		if err != nil {
			return fmt.Errorf("failed to read basepath: %s", err)
		}

		// initialize database using data directory
		db, err = chaindb.NewBadgerDB(basepath)
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

	// create block state from genesis block
	blockState, err := NewBlockStateFromGenesis(db, header)
	if err != nil {
		return fmt.Errorf("failed to create block state from genesis: %s", err)
	}

	// create storage state from genesis trie
	storageState, err := NewStorageState(db, blockState, t)
	if err != nil {
		return fmt.Errorf("failed to create storage state from trie: %s", err)
	}

	epochState, err := NewEpochStateFromGenesis(db, epochInfo)
	if err != nil {
		return fmt.Errorf("failed to create epoch state: %s", err)
	}

	// check database type
	if s.isMemDB {

		// append memory database to state service
		s.db = db

		// append storage state and block state to state service
		s.Storage = storageState
		s.Block = blockState
		s.Epoch = epochState

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
func (s *Service) storeInitialValues(db chaindb.Database, data *genesis.Data, header *types.Header, t *trie.Trie) error {

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
	if !s.isMemDB && (s.Storage != nil || s.Block != nil || s.Network != nil || s.Epoch != nil) {
		return nil
	}

	db := s.db
	if !s.isMemDB {
		basepath, err := filepath.Abs(s.dbPath)
		if err != nil {
			return err
		}

		// initialize database
		db, err = chaindb.NewBadgerDB(basepath)
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

	logger.Trace("start", "best block hash", fmt.Sprintf("0x%x", bestHash))

	// load blocktree
	bt := blocktree.NewEmptyBlockTree(db)
	err = bt.Load()
	if err != nil {
		return fmt.Errorf("failed to load blocktree: %s", err)
	}

	// create block state
	s.Block, err = NewBlockState(db, bt)
	if err != nil {
		return fmt.Errorf("failed to create block state: %s", err)
	}

	// create storage state
	s.Storage, err = NewStorageState(db, s.Block, trie.NewEmptyTrie())
	if err != nil {
		return fmt.Errorf("failed to create storage state: %s", err)
	}

	stateRoot, err := LoadLatestStorageHash(s.db)
	if err != nil {
		return fmt.Errorf("cannot load latest storage root: %s", err)
	}

	logger.Debug("start", "latest state root", stateRoot)

	// load current storage state
	_, err = s.Storage.LoadFromDB(stateRoot)
	if err != nil {
		return fmt.Errorf("failed to get state root from database: %s", err)
	}

	// create network state
	s.Network = NewNetworkState()

	// create transaction queue
	s.TransactionQueue = NewTransactionQueue()

	// create epoch state
	s.Epoch = NewEpochState(db)
	return nil
}

// Stop closes each state database
func (s *Service) Stop() error {
	head, err := s.Block.BestBlockStateRoot()
	if err != nil {
		return err
	}

	s.Storage.lock.RLock()
	t := s.Storage.tries[head]
	s.Storage.lock.RUnlock()

	if t == nil {
		return errTrieDoesNotExist(head)
	}

	err = StoreLatestStorageHash(s.db, t)
	if err != nil {
		return err
	}

	err = s.Storage.StoreInDB(head)
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

	thash, err := t.Hash()
	if err != nil {
		return err
	}

	logger.Debug("stop", "best block hash", hash, "latest state root", thash)
	return s.db.Close()
}
