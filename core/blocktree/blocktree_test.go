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

package blocktree

import (
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/core/types"

	"github.com/ChainSafe/gossamer/common"
	db "github.com/ChainSafe/gossamer/polkadb"
)

var zeroHash, _ = common.HexToHash("0x00")

func createGenesisBlock() types.Block {
	b := types.Block{
		Header: &types.BlockHeader{
			ParentHash: zeroHash,
			Number:     big.NewInt(0),
		},
		Body: &types.BlockBody{},
	}
	b.Header.Hash()
	b.SetBlockArrivalTime(uint64(0))
	return b
}

func createFlatTree(t *testing.T, depth int) (*BlockTree, []common.Hash) {
	d := &db.BlockDB{
		Db: db.NewMemDatabase(),
	}

	bt := NewBlockTreeFromGenesis(createGenesisBlock(), d)

	previousHash := bt.head.hash
	previousAT := bt.head.arrivalTime

	hashes := []common.Hash{bt.head.hash}
	for i := 1; i <= depth; i++ {
		block := types.Block{
			Header: &types.BlockHeader{
				ParentHash: previousHash,
				Number:     big.NewInt(int64(i)),
			},
			Body: &types.BlockBody{},
		}

		hash := block.Header.Hash()
		hashes = append(hashes, hash)
		block.SetBlockArrivalTime(previousAT + uint64(1000))

		bt.AddBlock(block)
		previousHash = hash
		previousAT = block.GetBlockArrivalTime()
	}

	return bt, hashes
}

func TestBlockTree_GetBlock(t *testing.T) {
	// Calls AddBlock
	bt, hashes := createFlatTree(t, 2)

	n := bt.GetNode(hashes[2])
	if n == nil {
		t.Fatal("node is nil")
	}

	if n.number.Cmp(big.NewInt(2)) != 0 {
		t.Errorf("got: %s expected: %s", n.number, big.NewInt(2))
	}

}

func TestBlockTree_AddBlock(t *testing.T) {
	bt, hashes := createFlatTree(t, 1)

	block := types.Block{
		Header: &types.BlockHeader{
			ParentHash: hashes[1],
			Number:     big.NewInt(1),
		},
		Body: &types.BlockBody{},
	}

	hash := block.Header.Hash()
	bt.AddBlock(block)

	n := bt.GetNode(hash)

	if bt.leaves[n.hash] == nil {
		t.Errorf("expected %x to be a leaf", n.hash)
	}

	oldHash := common.Hash{0x01}

	if bt.leaves[oldHash] != nil {
		t.Errorf("expected %x to no longer be a leaf", oldHash)
	}
}

func TestNode_isDecendantOf(t *testing.T) {
	// Create tree with depth 4 (with 4 nodes)
	bt, hashes := createFlatTree(t, 4)

	// Check leaf is decendant of root
	leaf := bt.GetNode(hashes[3])
	if !leaf.isDescendantOf(bt.head) {
		t.Error("failed to verify leaf is descendant of root")
	}

	// Verify the inverse relationship does not hold
	if bt.head.isDescendantOf(leaf) {
		t.Error("root should not be decendant of anything")
	}

}

func TestBlockTree_LongestPath(t *testing.T) {
	bt, hashes := createFlatTree(t, 3)

	// Insert a block to create a competing path
	extraBlock := types.Block{
		Header: &types.BlockHeader{
			ParentHash: hashes[0],
			Number:     big.NewInt(1),
		},
		Body: &types.BlockBody{},
	}

	extraBlock.Header.Hash()
	bt.AddBlock(extraBlock)

	longestPath := bt.LongestPath()

	for i, n := range longestPath {
		if n.hash != hashes[i] {
			t.Errorf("expected Hash: 0x%X got: 0x%X\n", hashes[i], n.hash)
		}
	}
}

func TestBlockTree_Subchain(t *testing.T) {
	bt, hashes := createFlatTree(t, 4)
	expectedPath := hashes[1:]

	// Insert a block to create a competing path
	extraBlock := types.Block{
		Header: &types.BlockHeader{
			ParentHash: hashes[0],
			Number:     big.NewInt(1),
		},
		Body: &types.BlockBody{},
	}

	extraBlock.Header.Hash()
	bt.AddBlock(extraBlock)

	subChain := bt.SubChain(hashes[1], hashes[3])

	for i, n := range subChain {
		if n.hash != expectedPath[i] {
			t.Errorf("expected Hash: 0x%X got: 0x%X\n", expectedPath[i], n.hash)
		}
	}
}

func TestBlockTree_ComputeSlotForBlock(t *testing.T) {
	bt, hashes := createFlatTree(t, 9)

	expectedSlotNumber := uint64(9)
	slotNumber := bt.ComputeSlotForBlock(bt.GetNode(hashes[9]).getBlockFromNode(), 1000)

	if slotNumber != expectedSlotNumber {
		t.Errorf("expected Slot Number: %d got: %d", expectedSlotNumber, slotNumber)
	}

}

// TODO: Need to define leftmost (see BlockTree.LongestPath)
//func TestBlockTree_LongestPath_LeftMost(t *testing.T) {
//	bt := createFlatTree(t, 1)
//
//	// Insert a block to create a competing path
//	extraBlock := types.Block{
//		SlotNumber:   nil,
//		ParentHash: zeroHash,
//		Number:  big.NewInt(1),
//		Hash:         common.Hash{0xAB},
//	}
//
//	bt.AddBlock(extraBlock)
//
//	expectedPath := []*node{
//		bt.GetNode(common.Hash{0x00}),
//		bt.GetNode(common.Hash{0xAB}),
//	}
//
//	longestPath := bt.LongestPath()
//
//	for i, n := range longestPath {
//		if n.hash != expectedPath[i].hash {
//			t.Errorf("expected Hash: 0x%X got: 0x%X\n", expectedPath[i].hash, n.hash)
//		}
//	}
//}
