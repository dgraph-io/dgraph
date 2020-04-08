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

package genesis

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"path/filepath"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"
)

// NewGenesisFromJSON parses a JSON formatted genesis file
func NewGenesisFromJSON(file string) (*Genesis, error) {
	fp, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(filepath.Clean(fp))
	if err != nil {
		return nil, err
	}

	g := new(Genesis)
	err = json.Unmarshal(data, g)
	return g, err
}

// NewTrieFromGenesis creates a new trie from the raw genesis data
func NewTrieFromGenesis(g *Genesis) (*trie.Trie, error) {
	t := trie.NewEmptyTrie()

	r := g.GenesisFields().Raw[0]

	err := t.Load(r)
	if err != nil {
		return nil, fmt.Errorf("failed to create trie from genesis: %s", err)
	}

	return t, nil
}

// NewGenesisBlockFromTrie creates a genesis block from the provided trie
func NewGenesisBlockFromTrie(t *trie.Trie) (*types.Header, error) {

	// create state root from trie hash
	stateRoot, err := t.Hash()
	if err != nil {
		return nil, fmt.Errorf("failed to create state root from trie hash: %s", err)
	}

	// create genesis block header
	header, err := types.NewHeader(
		common.NewHash([]byte{0}), // parentHash
		big.NewInt(0),             // number
		stateRoot,                 // stateRoot
		trie.EmptyHash,            // extrinsicsRoot
		[][]byte{},                // digest
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create genesis block header: %s", err)
	}

	return header, nil
}
