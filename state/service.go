package state

import (
	"fmt"
	"path/filepath"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/trie"
)

type Service struct {
	dbPath  string
	Storage *StorageState
	Block   *blockState
	Network *networkState
}

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

	storageDb, err := NewStorageState(stateDataDir, t)
	if err != nil {
		return err
	}

	err = storageDb.StoreInDB()
	if err != nil {
		return err
	}

	hash := genesisHeader.Hash()
	err = storageDb.Db.Db.Put(common.LatestHeaderHashKey, hash[:])
	if err != nil {
		return err
	}

	blockDb, err := NewBlockStateFromGenesis(blockDataDir, genesisHeader)
	if err != nil {
		return err
	}

	err = blockDb.db.Db.Close()
	if err != nil {
		return err
	}

	return storageDb.Db.Db.Close()
}

// Start initializes the Storage database and the Block database.
func (s *Service) Start() error {
	if s.Storage != nil || s.Block != nil {
		return nil
	}

	stateDataDir := filepath.Join(s.dbPath, "state")
	blockDataDir := filepath.Join(s.dbPath, "block")

	storageDb, err := NewStorageState(stateDataDir, trie.NewEmptyTrie(nil))
	if err != nil {
		return fmt.Errorf("cannot make storage state: %s", err)
	}

	latestHeaderHash, err := storageDb.Db.Db.Get(common.LatestHeaderHashKey)
	if err != nil {
		return fmt.Errorf("cnanot get latest hash: %s", err)
	}

	blockDb, err := NewBlockState(blockDataDir, common.BytesToHash(latestHeaderHash))
	if err != nil {
		return fmt.Errorf("cannot make block state: %s", err)
	}

	err = storageDb.LoadFromDB(blockDb.latestHeader.StateRoot)
	if err != nil {
		return fmt.Errorf("cannot load state from DB: %s", err)
	}

	s.Storage = storageDb
	s.Block = blockDb
	s.Network = NewNetworkState()

	return nil
}

func (s *Service) Stop() error {
	// Closing Badger Databases
	err := s.Storage.Db.Db.Close()
	if err != nil {
		return err
	}

	err = s.Block.db.Db.Close()
	if err != nil {
		return err
	}
	return nil
}
