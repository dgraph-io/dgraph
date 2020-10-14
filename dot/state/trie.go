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
	"sync"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/ChainSafe/chaindb"
)

var triePrefix = []byte("tmp")

// TrieState is a wrapper around a transient trie that is used during the course of executing some runtime call.
// If the execution of the call is successful, the trie will be saved in the StorageState.
type TrieState struct {
	db   chaindb.Database
	t    *trie.Trie
	lock sync.RWMutex
}

// NewTrieState returns a new TrieState with the given trie
func NewTrieState(db chaindb.Database, t *trie.Trie) (*TrieState, error) {
	tdb := chaindb.NewTable(db, string(triePrefix))

	entries := t.Entries()
	for k, v := range entries {
		err := tdb.Put([]byte(k), v)
		if err != nil {
			return nil, err
		}
	}

	ts := &TrieState{
		db: tdb,
		t:  t,
	}

	return ts, nil
}

//nolint
// Commit ensures that the TrieState's trie and database match
// The database is the source of truth due to the runtime interpreter's undefined behaviour regarding the trie
func (s *TrieState) Commit() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.t = trie.NewEmptyTrie()
	iter := s.db.NewIterator()

	for iter.Next() {
		key := iter.Key()
		err := s.t.Put(key, iter.Value())
		if err != nil {
			return err
		}
	}

	iter.Release()
	return nil
}

// WriteTrieToDB writes the trie to the database
func (s *TrieState) WriteTrieToDB() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	for k, v := range s.t.Entries() {
		err := s.db.Put([]byte(k), v)
		if err != nil {
			return err
		}
	}

	return nil
}

// Free should be called once this trie state is no longer needed
func (s *TrieState) Free() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	iter := s.db.NewIterator()

	for iter.Next() {
		err := s.db.Del(iter.Key())
		if err != nil {
			return err
		}

		err = s.t.Delete(iter.Key())
		if err != nil {
			return err
		}
	}

	iter.Release()
	return nil
}

// Set sets a key-value pair in the trie
func (s *TrieState) Set(key []byte, value []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	err := s.db.Put(key, value)
	if err != nil {
		return err
	}

	return s.t.Put(key, value)
}

// Get gets a value from the trie
func (s *TrieState) Get(key []byte) ([]byte, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if has, _ := s.db.Has(key); has {
		return s.db.Get(key)
	}

	return nil, nil
}

// MustRoot returns the trie's root hash. It panics if it fails to compute the root.
func (s *TrieState) MustRoot() common.Hash {
	root, err := s.Root()
	if err != nil {
		panic(err)
	}

	return root
}

// Root returns the trie's root hash
func (s *TrieState) Root() (common.Hash, error) {
	err := s.Commit()
	if err != nil {
		return common.Hash{}, err
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	return s.t.Hash()
}

// Has returns whether or not a key exists
func (s *TrieState) Has(key []byte) (bool, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.db.Has(key)
}

// Delete deletes a key from the trie
func (s *TrieState) Delete(key []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	err := s.db.Del(key)
	if err != nil {
		return err
	}

	return s.t.Delete(key)
}

// SetChild sets the child trie at the given key
func (s *TrieState) SetChild(keyToChild []byte, child *trie.Trie) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.t.PutChild(keyToChild, child)
}

// SetChildStorage sets a key-value pair in a child trie
func (s *TrieState) SetChildStorage(keyToChild, key, value []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.t.PutIntoChild(keyToChild, key, value)
}

// GetChild returns the child trie at the given key
func (s *TrieState) GetChild(keyToChild []byte) (*trie.Trie, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.t.GetChild(keyToChild)
}

// GetChildStorage returns a value from a child trie
func (s *TrieState) GetChildStorage(keyToChild, key []byte) ([]byte, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.t.GetFromChild(keyToChild, key)
}

// Entries returns every key-value pair in the trie
func (s *TrieState) Entries() map[string][]byte {
	iter := s.db.NewIterator()

	entries := make(map[string][]byte)
	for iter.Next() {
		entries[string(iter.Key())] = iter.Value()
	}

	iter.Release()
	return entries
}

// SetBalance sets the balance for a given public key
func (s *TrieState) SetBalance(key [32]byte, balance uint64) error {
	skey, err := common.BalanceKey(key)
	if err != nil {
		return err
	}

	bb := make([]byte, 8)
	binary.LittleEndian.PutUint64(bb, balance)

	return s.Set(skey, bb)
}

// GetBalance returns the balance for a given public key
func (s *TrieState) GetBalance(key [32]byte) (uint64, error) {
	skey, err := common.BalanceKey(key)
	if err != nil {
		return 0, err
	}

	bal, err := s.Get(skey)
	if err != nil {
		return 0, err
	}

	if len(bal) != 8 {
		return 0, nil
	}

	return binary.LittleEndian.Uint64(bal), nil
}

// DeleteChildStorage deletes child storage from the trie
func (s *TrieState) DeleteChildStorage(key []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.t.DeleteFromChild(key)
}

// ClearChildStorage removes the child storage entry from the trie
func (s *TrieState) ClearChildStorage(keyToChild, key []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.t.ClearFromChild(keyToChild, key)
}
