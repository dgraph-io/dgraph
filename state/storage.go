package state

import (
	"fmt"
	"sync"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/config/genesis"
	"github.com/ChainSafe/gossamer/db"
	"github.com/ChainSafe/gossamer/trie"
)

// DB stores trie structure in an underlying Database
type DB struct {
	DB db.Database
}

// StorageState is the struct that holds the trie, db and lock
type StorageState struct {
	trie *trie.Trie
	DB   *DB
	lock sync.RWMutex
}

// NewStateDB instantiates badgerDB instance for storing trie structure
func NewStateDB(dataDir string) (*DB, error) {
	db, err := db.NewBadgerDB(dataDir)
	if err != nil {
		return nil, err
	}

	return &DB{
		db,
	}, nil
}

// NewStorageState creates a new StorageState backed by the given trie and database located at dataDir.
func NewStorageState(dataDir string, t *trie.Trie) (*StorageState, error) {
	stateDb, err := NewStateDB(dataDir)
	if err != nil {
		return nil, err
	}
	db := trie.NewDatabase(stateDb.DB)
	t.SetDb(db)
	return &StorageState{
		trie: t,
		DB:   stateDb,
	}, nil
}

// SetLatestHeaderHash sets the LatestHeaderHashKey in the DB to the given hash
func (s *StorageState) SetLatestHeaderHash(hash []byte) error {
	err := s.DB.DB.Put(common.LatestHeaderHashKey, hash)
	if err != nil {
		return fmt.Errorf("cannot get latest hash: %s", err)
	}

	return nil
}

// GetLatestHeaderHash retrieves the value stored in the DB at LatestHeaderHashKey
func (s *StorageState) GetLatestHeaderHash() ([]byte, error) {
	latestHeaderHash, err := s.DB.DB.Get(common.LatestHeaderHashKey)
	if err != nil {
		return nil, fmt.Errorf("cannot get latest hash: %s", err)
	}

	return latestHeaderHash, err
}

// ExistsStorage check if the key exists in the storage trie
func (s *StorageState) ExistsStorage(key []byte) (bool, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	val, err := s.trie.Get(key)
	return val != nil, err
}

// GetStorage gets the object from the trie using key
func (s *StorageState) GetStorage(key []byte) ([]byte, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.Get(key)
}

// StorageRoot returns the trie hash
func (s *StorageState) StorageRoot() (common.Hash, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.Hash()
}

// EnumeratedTrieRoot not implemented
func (s *StorageState) EnumeratedTrieRoot(values [][]byte) {
	//TODO
	panic("not implemented")
}

// SetStorage set the storage value for a given key in the trie
func (s *StorageState) SetStorage(key []byte, value []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trie.Put(key, value)
}

// ClearPrefix not implemented
func (s *StorageState) ClearPrefix(prefix []byte) {
	// Implemented in ext_clear_prefix
	panic("not implemented")
}

// ClearStorage will delete a key/value from the trie for a given @key
func (s *StorageState) ClearStorage(key []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trie.Delete(key)
}

// LoadHash returns the tire LoadHash and error
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

// Entries returns Entries from the trie
func (s *StorageState) Entries() map[string][]byte {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.Entries()
}

// LoadGenesisData returns LoadGenesisData from the trie
func (s *StorageState) LoadGenesisData() (*genesis.Data, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.Db().LoadGenesisData()
}

// SetStorageChild return PutChild from the trie
func (s *StorageState) SetStorageChild(keyToChild []byte, child *trie.Trie) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trie.PutChild(keyToChild, child)
}

// GetStorageChild return GetChild from the trie
func (s *StorageState) GetStorageChild(keyToChild []byte) (*trie.Trie, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.GetChild(keyToChild)
}

// SetStorageIntoChild return PutIntoChild from the trie
func (s *StorageState) SetStorageIntoChild(keyToChild, key, value []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.trie.PutIntoChild(keyToChild, key, value)
}

// GetStorageFromChild return GetFromChild from the trie
func (s *StorageState) GetStorageFromChild(keyToChild, key []byte) ([]byte, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.GetFromChild(keyToChild, key)
}
