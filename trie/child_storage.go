package trie

import (
	"fmt"

	"github.com/ChainSafe/gossamer/common"
)

// ChildStorageKeyPrefix is the prefix for all child storage keys
var ChildStorageKeyPrefix = []byte(":child_storage:")

// PutChild inserts a child trie into the main trie at key :child_storage:[keyToChild]
func (t *Trie) PutChild(keyToChild []byte, child *Trie) error {
	childHash, err := child.Hash()
	if err != nil {
		return err
	}

	key := append(ChildStorageKeyPrefix, keyToChild...)
	value := [32]byte(childHash)

	err = t.Put(key, value[:])
	if err != nil {
		return err
	}

	t.children[childHash] = child
	return nil
}

// GetChild returns the child trie at key :child_storage:[keyToChild]
func (t *Trie) GetChild(keyToChild []byte) (*Trie, error) {
	key := append(ChildStorageKeyPrefix, keyToChild...)
	childHash, err := t.Get(key)
	if err != nil {
		return nil, err
	}

	hash := [32]byte{}
	copy(hash[:], childHash)
	return t.children[common.Hash(hash)], nil
}

// PutIntoChild puts a key-value pair into the child trie located in the main trie at key :child_storage:[keyToChild]
func (t *Trie) PutIntoChild(keyToChild, key, value []byte) error {
	child, err := t.GetChild(keyToChild)
	if err != nil {
		return err
	}

	origChildHash, err := child.Hash()
	if err != nil {
		return err
	}

	err = child.Put(key, value)
	if err != nil {
		return err
	}

	childHash, err := child.Hash()
	if err != nil {
		return err
	}

	t.children[origChildHash] = nil
	t.children[childHash] = child

	return t.PutChild(keyToChild, child)
}

// GetFromChild retrieves a key-value pair from the child trie located in the main trie at key :child_storage:[keyToChild]
func (t *Trie) GetFromChild(keyToChild, key []byte) ([]byte, error) {
	child, err := t.GetChild(keyToChild)
	if err != nil {
		return nil, err
	}

	if child == nil {
		return nil, fmt.Errorf("child trie does not exist at key %s%s", ChildStorageKeyPrefix, keyToChild)
	}

	return child.Get(key)
}
