package state

import (
	"sync"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/config/genesis"
	"github.com/ChainSafe/gossamer/db"
	"github.com/ChainSafe/gossamer/trie"
)

// StateDB stores trie structure in an underlying Database
type StateDB struct {
	Db db.Database
}

type StorageState struct {
	trie *trie.Trie
	Db   *StateDB
	lock sync.RWMutex
}

// NewStateDB instantiates badgerDB instance for storing trie structure
func NewStateDB(dataDir string) (*StateDB, error) {
	db, err := db.NewBadgerDB(dataDir)
	if err != nil {
		return nil, err
	}

	return &StateDB{
		db,
	}, nil
}

// NewStorageState creates a new StorageState backed by the given trie and database located at dataDir.
func NewStorageState(dataDir string, t *trie.Trie) (*StorageState, error) {
	stateDb, err := NewStateDB(dataDir)
	if err != nil {
		return nil, err
	}
	db := trie.NewDatabase(stateDb.Db)
	t.SetDb(db)
	return &StorageState{
		trie: t,
		Db:   stateDb,
	}, nil
}

func (s *StorageState) ExistsStorage(key []byte) (bool, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	val, err := s.trie.Get(key)
	return val != nil, err
}

func (s *StorageState) GetStorage(key []byte) ([]byte, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.Get(key)
}

func (s *StorageState) StorageRoot() (common.Hash, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.Hash()
}

func (s *StorageState) EnumeratedTrieRoot(values [][]byte) {
	//TODO
	panic("not implemented")
}

func (s *StorageState) SetStorage(key []byte, value []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trie.Put(key, value)
}

func (s *StorageState) ClearPrefix(prefix []byte) {
	// Implemented in ext_clear_prefix
	panic("not implemented")
}

func (s *StorageState) ClearStorage(key []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trie.Delete(key)
}

func (s *StorageState) LoadHash() (common.Hash, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.LoadHash()
}

// LoadFromDB loads the trie state with the given root from the database.
func (s *StorageState) LoadFromDB(root common.Hash) error {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.LoadFromDB(root)
}

// StoreInDB stores the current trie state in the database.
func (s *StorageState) StoreInDB() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trie.StoreInDB()
}

func (s *StorageState) Entries() map[string][]byte {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.Entries()
}

func (s *StorageState) LoadGenesisData() (*genesis.GenesisData, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.Db().LoadGenesisData()
}

func (s *StorageState) SetStorageChild(keyToChild []byte, child *trie.Trie) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trie.PutChild(keyToChild, child)
}

func (s *StorageState) GetStorageChild(keyToChild []byte) (*trie.Trie, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.GetChild(keyToChild)
}

func (s *StorageState) SetStorageIntoChild(keyToChild, key, value []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trie.PutIntoChild(keyToChild, key, value)
}

func (s *StorageState) GetStorageFromChild(keyToChild, key []byte) ([]byte, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.GetFromChild(keyToChild, key)
}
