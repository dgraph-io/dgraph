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
	"bytes"
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

var zeroHash, _ = common.HexToHash("0x00")

func createFlatTree(t *testing.T, depth int) (*BlockTree, []common.Hash) {
	header := &types.Header{
		ParentHash: zeroHash,
		Number:     big.NewInt(0),
	}

	bt := NewBlockTreeFromGenesis(header, nil)
	require.NotNil(t, bt)

	previousHash := bt.head.hash

	hashes := []common.Hash{bt.head.hash}
	for i := 1; i <= depth; i++ {
		block := &types.Block{
			Header: &types.Header{
				ParentHash: previousHash,
				Number:     big.NewInt(int64(i)),
			},
			Body: &types.Body{},
		}

		hash := block.Header.Hash()
		hashes = append(hashes, hash)

		bt.AddBlock(block)
		previousHash = hash
	}

	return bt, hashes
}

func TestBlockTree_GetBlock(t *testing.T) {
	// Calls AddBlock
	bt, hashes := createFlatTree(t, 2)

	n := bt.getNode(hashes[2])
	if n == nil {
		t.Fatal("node is nil")
	}

	if !bytes.Equal(hashes[2][:], n.hash[:]) {
		t.Fatalf("Fail: got %x expected %x", n.hash, hashes[2])
	}

}

func TestBlockTree_AddBlock(t *testing.T) {
	bt, hashes := createFlatTree(t, 1)

	block := &types.Block{
		Header: &types.Header{
			ParentHash: hashes[1],
			Number:     big.NewInt(1),
		},
		Body: &types.Body{},
	}

	hash := block.Header.Hash()
	bt.AddBlock(block)

	n := bt.getNode(hash)

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

	// Check leaf is descendant of root
	leaf := bt.getNode(hashes[3])
	if !leaf.isDescendantOf(bt.head) {
		t.Error("failed to verify leaf is descendant of root")
	}

	// Verify the inverse relationship does not hold
	if bt.head.isDescendantOf(leaf) {
		t.Error("root should not be descendant of anything")
	}

}

func TestBlockTree_LongestPath(t *testing.T) {
	bt, hashes := createFlatTree(t, 3)

	// Insert a block to create a competing path
	extraBlock := &types.Block{
		Header: &types.Header{
			ParentHash: hashes[0],
			Number:     big.NewInt(1),
		},
		Body: &types.Body{},
	}

	extraBlock.Header.Hash()
	bt.AddBlock(extraBlock)

	longestPath := bt.longestPath()

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
	extraBlock := &types.Block{
		Header: &types.Header{
			ParentHash: hashes[0],
			Number:     big.NewInt(1),
		},
		Body: &types.Body{},
	}

	extraBlock.Header.Hash()
	bt.AddBlock(extraBlock)

	subChain := bt.subChain(hashes[1], hashes[3])

	for i, n := range subChain {
		if n.hash != expectedPath[i] {
			t.Errorf("expected Hash: 0x%X got: 0x%X\n", expectedPath[i], n.hash)
		}
	}
}

// TODO: Need to define leftmost (see BlockTree.LongestPath)
//func TestBlockTree_LongestPath_LeftMost(t *testing.T) {
//}

func TestBlockTree_GetNode(t *testing.T) {
	header := &types.Header{
		ParentHash: zeroHash,
		Number:     big.NewInt(0),
	}

	bt, branches := createTestBlockTree(header, 16, nil)

	for _, branch := range branches {
		block := &types.Block{
			Header: &types.Header{
				ParentHash: branch.hash,
				Number:     branch.depth,
			},
			Body: &types.Body{},
		}

		err := bt.AddBlock(block)
		if err != nil {
			t.Fatal(err)
		}
	}
}
