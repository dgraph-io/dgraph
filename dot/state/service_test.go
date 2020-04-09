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
	"math/big"
	"math/rand"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/lib/utils"
)

// branch tree randomly
type testBranch struct {
	hash  common.Hash
	depth int
}

// helper method to create and start test state service
func newTestService(t *testing.T) (state *Service) {
	testDir := utils.NewTestDir(t)
	state = NewService(testDir)
	return state
}

func newTestMemDBService() *Service {
	state := NewService("")
	state.UseMemDB()
	return state
}

func TestService_Start(t *testing.T) {
	state := newTestService(t)
	defer utils.RemoveTestDir(t)

	genesisHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), trie.EmptyHash, trie.EmptyHash, [][]byte{})
	if err != nil {
		t.Fatal(err)
	}

	tr := trie.NewEmptyTrie()

	genesisData := new(genesis.Data)

	err = state.Initialize(genesisData, genesisHeader, tr)
	if err != nil {
		t.Fatal(err)
	}

	err = state.Start()
	if err != nil {
		t.Fatal(err)
	}

	state.Stop()
}

func TestMemDB_Start(t *testing.T) {
	state := newTestMemDBService()

	genesisHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), trie.EmptyHash, trie.EmptyHash, [][]byte{})
	if err != nil {
		t.Fatal(err)
	}

	tr := trie.NewEmptyTrie()

	genesisData := new(genesis.Data)

	err = state.Initialize(genesisData, genesisHeader, tr)
	if err != nil {
		t.Fatal(err)
	}

	err = state.Start()
	if err != nil {
		t.Fatal(err)
	}

	state.Stop()
}

func addBlocksToState(blockState *BlockState, depth int) ([]*types.Header, []*types.Header) {
	previousHash := blockState.BestBlockHash()

	branches := []testBranch{}
	r := *rand.New(rand.NewSource(rand.Int63()))

	arrivalTime := uint64(1)
	currentChain := []*types.Header{}
	branchChains := []*types.Header{}

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

		currentChain = append(currentChain, block.Header)

		hash := block.Header.Hash()
		blockState.AddBlockWithArrivalTime(block, arrivalTime)
		previousHash = hash

		isBranch := r.Intn(2)
		if isBranch == 1 {
			branches = append(branches, testBranch{
				hash:  hash,
				depth: i,
			})
		}

		arrivalTime++
	}

	// create tree branches
	for _, branch := range branches {
		previousHash = branch.hash

		for i := branch.depth; i < depth; i++ {
			block := &types.Block{
				Header: &types.Header{
					ParentHash: previousHash,
					Number:     big.NewInt(int64(i) + 1),
					StateRoot:  trie.EmptyHash,
					Digest:     [][]byte{{byte(i)}},
				},
				Body: &types.Body{},
			}

			branchChains = append(branchChains, block.Header)

			hash := block.Header.Hash()
			blockState.AddBlockWithArrivalTime(block, arrivalTime)
			previousHash = hash

			arrivalTime++
		}
	}

	return currentChain, branchChains
}

func TestService_BlockTree(t *testing.T) {
	testDir := utils.NewTestDir(t)

	// removes all data directories created within test directory
	defer utils.RemoveTestDir(t)

	stateA := NewService(testDir)

	genesisHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), trie.EmptyHash, trie.EmptyHash, [][]byte{})
	if err != nil {
		t.Fatal(err)
	}

	genesisData := new(genesis.Data)

	tr := trie.NewEmptyTrie()
	err = stateA.Initialize(genesisData, genesisHeader, tr)
	if err != nil {
		t.Fatal(err)
	}

	err = stateA.Start()
	if err != nil {
		t.Fatal(err)
	}

	// add blocks to state
	addBlocksToState(stateA.Block, 10)

	stateA.Stop()

	stateB := NewService(testDir)

	err = stateB.Start()
	if err != nil {
		t.Fatal(err)
	}

	stateB.Stop()

	if !reflect.DeepEqual(stateA.Block.BestBlockHash(), stateB.Block.BestBlockHash()) {
		t.Fatalf("Fail: got %s expected %s", stateA.Block.BestBlockHash(), stateB.Block.BestBlockHash())
	}
}
