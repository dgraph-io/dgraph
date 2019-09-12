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
	"fmt"
	"math/big"

	"github.com/ChainSafe/gossamer/polkadb"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core"
	log "github.com/ChainSafe/log15"
	"github.com/disiqueira/gotree"
)

type Hash = common.Hash

// BlockTree represents the current state with all possible blocks
type BlockTree struct {
	head            *node
	leaves          leafMap
	finalizedBlocks []*node
	BlockDB         *polkadb.BadgerService
}

// NewBlockTreeFromGenesis initializes a blocktree with a genesis block.
func NewBlockTreeFromGenesis(genesis core.Block, db *polkadb.BadgerService) *BlockTree {
	head := &node{
		hash:     genesis.Header.Hash,
		number:   genesis.Header.Number,
		parent:   nil,
		children: []*node{},
		depth:    big.NewInt(0),
	}
	return &BlockTree{
		head:            head,
		finalizedBlocks: []*node{},
		leaves:          leafMap{head.hash: head},
		BlockDB:         db,
	}
}

// AddBlock inserts the block as child of its parent node
// Note: Assumes block has no children
func (bt *BlockTree) AddBlock(block core.Block) {
	parent := bt.GetNode(block.Header.ParentHash)
	// Check if it already exists
	// TODO: Can shortcut this by checking DB
	// TODO: Write blockData to db
	// TODO: Create getter functions to check if blockNum is greater than best block stored

	n := bt.GetNode(block.Header.Hash)
	if n != nil {
		log.Debug("Attempted to add block to tree that already exists", "hash", n.hash)
		return
	}

	depth := big.NewInt(0)
	depth.Add(parent.depth, big.NewInt(1))

	n = &node{
		hash:     block.Header.Hash,
		number:   block.Header.Number,
		parent:   parent,
		children: []*node{},
		depth:    depth,
	}
	parent.addChild(n)

	bt.leaves.Replace(parent, n)
}

// GetNode finds and returns a node based on its hash. Returns nil if not found.
func (bt *BlockTree) GetNode(h Hash) *node {
	if bt.head.hash == h {
		return bt.head
	}

	for _, child := range bt.head.children {
		if n := child.getNode(h); n != nil {
			return n
		}
	}

	return nil
}

// String utilizes github.com/disiqueira/gotree to create a printable tree
func (bt *BlockTree) String() string {
	// Construct tree
	tree := gotree.New(bt.head.String())
	for _, child := range bt.head.children {
		sub := tree.Add(child.String())
		child.createTree(sub)
	}

	// Format leaves
	var leaves string
	for k := range bt.leaves {
		leaves = leaves + fmt.Sprintf("0x%X ", k)
	}

	metadata := fmt.Sprintf("Leaves: %v", leaves)

	return fmt.Sprintf("%s\n%s\n", metadata, tree.Print())
}

// LongestPath returns the path from the root to leftmost deepest leaf in BlockTree BT
func (bt *BlockTree) LongestPath() []*node {
	dl := bt.DeepestLeaf()
	var path []*node
	for curr := dl; ; curr = curr.parent {
		path = append([]*node{curr}, path...)
		if curr.parent == nil {
			return path
		}
	}
}

// DeepestLeaf returns leftmost deepest leaf in BlockTree BT
func (bt *BlockTree) DeepestLeaf() *node {
	return bt.leaves.DeepestLeaf()
}
