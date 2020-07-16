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
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/ChainSafe/chaindb"
)

var storagePrefix = []byte("storage")
var codeKey = []byte(":code")

// StorageDB stores trie structure in an underlying database
type StorageDB struct {
	db chaindb.Database
}

// Put appends `storage` to the key and sets the key-value pair in the db
func (storageDB *StorageDB) Put(key, value []byte) error {
	key = append(storagePrefix, key...)
	return storageDB.db.Put(key, value)
}

// Get appends `storage` to the key and retrieves the value from the db
func (storageDB *StorageDB) Get(key []byte) ([]byte, error) {
	key = append(storagePrefix, key...)
	return storageDB.db.Get(key)
}

// StorageState is the struct that holds the trie, db and lock
type StorageState struct {
	trie *trie.Trie
	db   *StorageDB
	lock sync.RWMutex

	// change notifiers
	changed     map[byte]chan<- *KeyValue
	changedLock sync.RWMutex
}

// NewStorageDB instantiates badgerDB instance for storing trie structure
func NewStorageDB(db chaindb.Database) *StorageDB {
	return &StorageDB{
		db,
	}
}

// NewStorageState creates a new StorageState backed by the given trie and database located at basePath.
func NewStorageState(db chaindb.Database, t *trie.Trie) (*StorageState, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot have nil database")
	}

	if t == nil {
		return nil, fmt.Errorf("cannot have nil trie")
	}

	return &StorageState{
		trie:    t,
		db:      NewStorageDB(db),
		changed: make(map[byte]chan<- *KeyValue),
	}, nil
}

// StoreInDB encodes the entire trie and writes it to the DB
// The key to the DB entry is the root hash of the trie
func (s *StorageState) StoreInDB() error {
	return StoreTrie(s.db.db, s.trie)
}

// LoadFromDB loads an encoded trie from the DB where the key is `root`
func (s *StorageState) LoadFromDB(root common.Hash) error {
	return LoadTrie(s.db.db, s.trie, root)
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
	kv := &KeyValue{
		Key:   key,
		Value: value,
	}
	err := s.trie.Put(key, value)
	if err != nil {
		return err
	}
	s.notifyChanged(kv)
	return nil
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
	kv := &KeyValue{
		Key:   key,
		Value: nil,
	}
	err := s.trie.Delete(key)
	if err != nil {
		return err
	}
	s.notifyChanged(kv)
	return nil
}

// Entries returns Entries from the trie
func (s *StorageState) Entries() map[string][]byte {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.trie.Entries()
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

// LoadCode returns the runtime code (located at :code)
func (s *StorageState) LoadCode() ([]byte, error) {
	return s.GetStorage(codeKey)
}

// LoadCodeHash returns the hash of the runtime code (located at :code)
func (s *StorageState) LoadCodeHash() (common.Hash, error) {
	code, err := s.LoadCode()
	if err != nil {
		return common.NewHash([]byte{}), err
	}

	return common.Blake2bHash(code)
}

// SetBalance sets the balance for an account with the given public key
func (s *StorageState) SetBalance(key [32]byte, balance uint64) error {
	skey, err := common.BalanceKey(key)
	if err != nil {
		return err
	}

	bb := make([]byte, 8)
	binary.LittleEndian.PutUint64(bb, balance)

	return s.SetStorage(skey, bb)
}

// GetBalance gets the balance for an account with the given public key
func (s *StorageState) GetBalance(key [32]byte) (uint64, error) {
	skey, err := common.BalanceKey(key)
	if err != nil {
		return 0, err
	}

	bal, err := s.GetStorage(skey)
	if err != nil {
		return 0, err
	}

	if len(bal) != 8 {
		return 0, nil
	}

	return binary.LittleEndian.Uint64(bal), nil
}
