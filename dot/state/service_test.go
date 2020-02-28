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
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"
)

// helper method to create and start test state service
func newTestService(t *testing.T) (state *Service) {
	dir, err := ioutil.TempDir(os.TempDir(), "test_data")
	if err != nil {
		t.Fatal("failed to create temp dir: " + err.Error())
	}

	state = NewService(dir)

	return state
}

func TestService_Start(t *testing.T) {
	state := newTestService(t)

	genesisHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), trie.EmptyHash, trie.EmptyHash, [][]byte{})
	if err != nil {
		t.Fatal(err)
	}

	tr := trie.NewEmptyTrie(nil)

	err = state.Initialize(genesisHeader, tr)
	if err != nil {
		t.Fatal(err)
	}

	err = state.Start()
	if err != nil {
		t.Fatal(err)
	}

	state.Stop()
}

func addBlocksToState(blockState *BlockState, depth int) {
	previousHash := blockState.BestBlockHash()

	// branch tree randomly
	type testBranch struct {
		hash  common.Hash
		depth int
	}

	branches := []testBranch{}
	r := *rand.New(rand.NewSource(rand.Int63()))

	// create base tree
	for i := 1; i <= depth; i++ {
		block := &types.Block{
			Header: &types.Header{
				ParentHash: previousHash,
				Number:     big.NewInt(int64(i)),
				StateRoot:  trie.EmptyHash,
			},
			Body: &types.Body{},
		}

		hash := block.Header.Hash()
		blockState.AddBlock(block)
		previousHash = hash

		isBranch := r.Intn(2)
		if isBranch == 1 {
			branches = append(branches, testBranch{
				hash:  hash,
				depth: i,
			})
		}
	}

	// create tree branches
	for _, branch := range branches {
		for i := branch.depth; i <= depth; i++ {
			block := &types.Block{
				Header: &types.Header{
					ParentHash: previousHash,
					Number:     big.NewInt(int64(i)),
					StateRoot:  trie.EmptyHash,
				},
				Body: &types.Body{},
			}

			hash := block.Header.Hash()
			blockState.AddBlock(block)
			previousHash = hash
		}
	}
}

func TestService_BlockTree(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "test_data")
	if err != nil {
		t.Fatal("failed to create temp dir: " + err.Error())
	}

	state := NewService(dir)

	genesisHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), trie.EmptyHash, trie.EmptyHash, [][]byte{})
	if err != nil {
		t.Fatal(err)
	}

	tr := trie.NewEmptyTrie(nil)
	err = state.Initialize(genesisHeader, tr)
	if err != nil {
		t.Fatal(err)
	}

	err = state.Start()
	if err != nil {
		t.Fatal(err)
	}

	// add blocks to state
	addBlocksToState(state.Block, 10)

	state.Stop()

	state2 := NewService(dir)

	err = state2.Start()
	if err != nil {
		t.Fatal(err)
	}

	state2.Stop()

	if !reflect.DeepEqual(state.Block.BestBlockHash(), state2.Block.BestBlockHash()) {
		t.Fatalf("Fail: got %s expected %s", state.Block.BestBlockHash(), state2.Block.BestBlockHash())
	}
}
