package state

import (
	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/polkadb"
	"github.com/ChainSafe/gossamer/trie"
)

type storageState struct {
	trie *trie.Trie
	Db   *polkadb.StateDB
}

func NewStorageState(dataDir string) (*storageState, error) {
	stateDb, err := polkadb.NewStateDB(dataDir)
	if err != nil {
		return nil, err
	}
	return &storageState{
		trie: &trie.Trie{},
		Db:   stateDb,
	}, nil
}

func (s *storageState) ExistsStorage(key []byte) (bool, error) {
	val, err := s.trie.Get(key)
	return (val != nil), err
}

func (s *storageState) GetStorage(key []byte) ([]byte, error) {
	return s.trie.Get(key)
}

func (s *storageState) StorageRoot() (common.Hash, error) {
	return s.trie.Hash()
}

func (s *storageState) EnumeratedTrieRoot(values [][]byte) {
	//TODO
}

func (s *storageState) SetStorage(key []byte, value []byte) error {
	return s.trie.Put(key, value)
}

func (s *storageState) ClearPrefix(prefix []byte) {
	// Implemented in ext_clear_prefix
}

func (s *storageState) ClearStorage(key []byte) error {
	return s.trie.Delete(key)
}

//TODO: add child storage funcs
