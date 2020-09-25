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
	"errors"
	"fmt"
	"sync"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/ChainSafe/chaindb"
)

var storagePrefix = "storage"
var codeKey = common.CodeKey

// ErrTrieDoesNotExist is returned when attempting to interact with a trie that is not stored in the StorageState
var ErrTrieDoesNotExist = errors.New("trie with given root does not exist")

func errTrieDoesNotExist(hash common.Hash) error {
	return fmt.Errorf("%w: %s", ErrTrieDoesNotExist, hash)
}

// StorageState is the struct that holds the trie, db and lock
type StorageState struct {
	blockState *BlockState
	tries      map[common.Hash]*trie.Trie

	baseDB chaindb.Database
	db     chaindb.Database
	lock   sync.RWMutex

	// change notifiers
	changed     map[byte]chan<- *KeyValue
	changedLock sync.RWMutex
}

// NewStorageState creates a new StorageState backed by the given trie and database located at basePath.
func NewStorageState(db chaindb.Database, blockState *BlockState, t *trie.Trie) (*StorageState, error) {
	if db == nil {
		return nil, fmt.Errorf("cannot have nil database")
	}

	if t == nil {
		return nil, fmt.Errorf("cannot have nil trie")
	}

	tries := make(map[common.Hash]*trie.Trie)
	tries[t.MustHash()] = t

	return &StorageState{
		blockState: blockState,
		tries:      tries,
		baseDB:     db,
		db:         chaindb.NewTable(db, storagePrefix),
		changed:    make(map[byte]chan<- *KeyValue),
	}, nil
}

func (s *StorageState) pruneStorage() { //nolint
	// TODO: when a block is finalized, delete non-finalized tries from DB and mapping
	// as well as all states before finalized block
	// TODO: pruning options? eg archive, full, etc
}

// StoreTrie stores the given trie in the StorageState and writes it to the database
func (s *StorageState) StoreTrie(root common.Hash, ts *TrieState) error {
	s.lock.Lock()
	s.tries[root] = ts.t
	s.lock.Unlock()

	logger.Debug("stored trie in storage state", "root", root)
	return s.StoreInDB(root)
}

// TrieState returns the TrieState for a given state root.
// If no state root is provided, it returns the TrieState for the current chain head.
func (s *StorageState) TrieState(hash *common.Hash) (*TrieState, error) {
	if hash == nil {
		sr, err := s.blockState.BestBlockStateRoot()
		if err != nil {
			return nil, err
		}
		hash = &sr
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.tries[*hash] == nil {
		return nil, errTrieDoesNotExist(*hash)
	}

	return NewTrieState(s.tries[*hash]), nil
}

// StoreInDB encodes the entire trie and writes it to the DB
// The key to the DB entry is the root hash of the trie
func (s *StorageState) StoreInDB(root common.Hash) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.tries[root] == nil {
		return errTrieDoesNotExist(root)
	}

	return StoreTrie(s.baseDB, s.tries[root])
}

// LoadFromDB loads an encoded trie from the DB where the key is `root`
func (s *StorageState) LoadFromDB(root common.Hash) (*trie.Trie, error) {
	t := trie.NewEmptyTrie()
	err := LoadTrie(s.baseDB, t, root)
	if err != nil {
		return nil, err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.tries[t.MustHash()] = t
	return t, nil
}

// ExistsStorage check if the key exists in the storage trie with the given storage hash
// If no hash is provided, the current chain head is used
func (s *StorageState) ExistsStorage(hash *common.Hash, key []byte) (bool, error) {
	if hash == nil {
		sr, err := s.blockState.BestBlockStateRoot()
		if err != nil {
			return false, err
		}
		hash = &sr
	}

	s.lock.RLock()
	defer s.lock.RUnlock()
	val, err := s.tries[*hash].Get(key)
	return val != nil, err
}

// GetStorage gets the object from the trie using the given key and storage hash
// If no hash is provided, the current chain head is used
func (s *StorageState) GetStorage(hash *common.Hash, key []byte) ([]byte, error) {
	if hash == nil {
		sr, err := s.blockState.BestBlockStateRoot()
		if err != nil {
			return nil, err
		}
		hash = &sr
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.tries[*hash] == nil {
		return nil, errTrieDoesNotExist(*hash)
	}

	return s.tries[*hash].Get(key)
}

// GetStorageByBlockHash returns the value at the given key at the given block hash
func (s *StorageState) GetStorageByBlockHash(bhash common.Hash, key []byte) ([]byte, error) {
	header, err := s.blockState.GetHeader(bhash)
	if err != nil {
		return nil, err
	}

	return s.GetStorage(&header.StateRoot, key)
}

// StorageRoot returns the root hash of the current storage trie
func (s *StorageState) StorageRoot() (common.Hash, error) {
	sr, err := s.blockState.BestBlockStateRoot()
	if err != nil {
		return common.Hash{}, err
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.tries[sr] == nil {
		return common.Hash{}, errTrieDoesNotExist(sr)
	}

	return s.tries[sr].Hash()
}

// EnumeratedTrieRoot not implemented
func (s *StorageState) EnumeratedTrieRoot(values [][]byte) {
	//TODO
	panic("not implemented")
}

// Entries returns Entries from the trie
func (s *StorageState) Entries(hash *common.Hash) (map[string][]byte, error) {
	if hash == nil {
		head, err := s.blockState.BestBlockStateRoot()
		if err != nil {
			return nil, err
		}
		hash = &head
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.tries[*hash] == nil {
		return nil, errTrieDoesNotExist(*hash)
	}

	return s.tries[*hash].Entries(), nil
}

// GetStorageChild return GetChild from the trie
func (s *StorageState) GetStorageChild(hash *common.Hash, keyToChild []byte) (*trie.Trie, error) {
	if hash == nil {
		sr, err := s.blockState.BestBlockStateRoot()
		if err != nil {
			return nil, err
		}
		hash = &sr
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.tries[*hash] == nil {
		return nil, errTrieDoesNotExist(*hash)
	}

	return s.tries[*hash].GetChild(keyToChild)
}

// GetStorageFromChild return GetFromChild from the trie
func (s *StorageState) GetStorageFromChild(hash *common.Hash, keyToChild, key []byte) ([]byte, error) {
	if hash == nil {
		sr, err := s.blockState.BestBlockStateRoot()
		if err != nil {
			return nil, err
		}
		hash = &sr
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	if s.tries[*hash] == nil {
		return nil, errTrieDoesNotExist(*hash)
	}
	return s.tries[*hash].GetFromChild(keyToChild, key)
}

// LoadCode returns the runtime code (located at :code)
func (s *StorageState) LoadCode(hash *common.Hash) ([]byte, error) {
	return s.GetStorage(hash, codeKey)
}

// LoadCodeHash returns the hash of the runtime code (located at :code)
func (s *StorageState) LoadCodeHash(hash *common.Hash) (common.Hash, error) {
	code, err := s.LoadCode(hash)
	if err != nil {
		return common.NewHash([]byte{}), err
	}

	return common.Blake2bHash(code)
}

// GetBalance gets the balance for an account with the given public key
func (s *StorageState) GetBalance(hash *common.Hash, key [32]byte) (uint64, error) {
	skey, err := common.BalanceKey(key)
	if err != nil {
		return 0, err
	}

	bal, err := s.GetStorage(hash, skey)
	if err != nil {
		return 0, err
	}

	if len(bal) != 8 {
		return 0, nil
	}

	return binary.LittleEndian.Uint64(bal), nil
}

// setStorage set the storage value for a given key in the trie. only for testing
func (s *StorageState) setStorage(hash *common.Hash, key []byte, value []byte) error {
	if hash == nil {
		sr, err := s.blockState.BestBlockStateRoot()
		if err != nil {
			return err
		}
		hash = &sr
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	kv := &KeyValue{
		Key:   key,
		Value: value,
	}

	if s.tries[*hash] == nil {
		return errTrieDoesNotExist(*hash)
	}

	err := s.tries[*hash].Put(key, value)
	if err != nil {
		return err
	}
	s.notifyChanged(kv) // TODO: what is this used for? needs to be updated to work with new StorageState/TrieState API
	return nil
}

// setBalance sets the balance for an account with the given public key. only for testing
func (s *StorageState) setBalance(hash *common.Hash, key [32]byte, balance uint64) error {
	skey, err := common.BalanceKey(key)
	if err != nil {
		return err
	}

	bb := make([]byte, 8)
	binary.LittleEndian.PutUint64(bb, balance)

	return s.setStorage(hash, skey, bb)
}
