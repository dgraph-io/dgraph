package state

import (
	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/config/genesis"
	"github.com/ChainSafe/gossamer/polkadb"
	"github.com/ChainSafe/gossamer/trie"
)

type StorageState struct {
	trie *trie.Trie
	Db   *polkadb.StateDB
}

func NewStorageState(dataDir string) (*StorageState, error) {
	stateDb, err := polkadb.NewStateDB(dataDir)
	if err != nil {
		return nil, err
	}
	db := trie.NewDatabase(stateDb.Db)
	return &StorageState{
		trie: trie.NewEmptyTrie(db),
		Db:   stateDb,
	}, nil
}

func (s *StorageState) ExistsStorage(key []byte) (bool, error) {
	val, err := s.trie.Get(key)
	return (val != nil), err
}

func (s *StorageState) GetStorage(key []byte) ([]byte, error) {
	return s.trie.Get(key)
}

func (s *StorageState) StorageRoot() (common.Hash, error) {
	return s.trie.Hash()
}

func (s *StorageState) EnumeratedTrieRoot(values [][]byte) {
	//TODO
	panic("not implemented")
}

func (s *StorageState) SetStorage(key []byte, value []byte) error {
	return s.trie.Put(key, value)
}

func (s *StorageState) ClearPrefix(prefix []byte) {
	// Implemented in ext_clear_prefix
	panic("not implemented")
}

func (s *StorageState) ClearStorage(key []byte) error {
	return s.trie.Delete(key)
}

func (s *StorageState) LoadHash() (common.Hash, error) {
	return s.trie.LoadHash()
}

func (s *StorageState) LoadFromDB(root common.Hash) error {
	return s.trie.LoadFromDB(root)
}

func (s *StorageState) Entries() map[string][]byte {
	return s.trie.Entries()
}

func (s *StorageState) LoadGenesisData() (*genesis.GenesisData, error) {
	return s.trie.Db().LoadGenesisData()
}

func (s *StorageState) SetStorageChild(keyToChild []byte, child *trie.Trie) error {
	return s.trie.PutChild(keyToChild, child)
}

func (s *StorageState) GetStorageChild(keyToChild []byte) (*trie.Trie, error) {
	return s.trie.GetChild(keyToChild)
}

func (s *StorageState) SetStorageIntoChild(keyToChild, key, value []byte) error {
	return s.trie.PutIntoChild(keyToChild, key, value)
}

func (s *StorageState) GetStorageFromChild(keyToChild, key []byte) ([]byte, error) {
	return s.trie.GetFromChild(keyToChild, key)
}
