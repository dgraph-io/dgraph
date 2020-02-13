package state

import (
	"fmt"
	"path/filepath"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/trie"
	log "github.com/ChainSafe/log15"
)

type Service struct {
	dbPath  string
	Storage *StorageState
	Block   *BlockState
	Network *NetworkState
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
	err = storageState.Db.Db.Put(common.LatestHeaderHashKey, hash[:])
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

	return storageState.Db.Db.Close()
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

	latestHeaderHash, err := storageState.Db.Db.Get(common.LatestHeaderHashKey)
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

	err = s.Storage.Db.Db.Close()
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
