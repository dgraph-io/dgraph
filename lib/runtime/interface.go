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

package runtime

import (
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"
)

// Storage interface
type Storage interface {
	Set(key []byte, value []byte) error
	Get(key []byte) ([]byte, error)
	Root() (common.Hash, error)
	SetChild(keyToChild []byte, child *trie.Trie) error
	SetChildStorage(keyToChild, key, value []byte) error
	GetChildStorage(keyToChild, key []byte) ([]byte, error)
	Delete(key []byte) error
	Entries() map[string][]byte
	SetBalance(key [32]byte, balance uint64) error
	GetBalance(key [32]byte) (uint64, error)
}

// BasicStorage interface for functions used by runtime offchain workers
type BasicStorage interface {
	Put(key []byte, value []byte) error
	Get(key []byte) ([]byte, error)
}
