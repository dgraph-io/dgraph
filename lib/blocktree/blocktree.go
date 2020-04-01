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
	"time"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/database"
	"github.com/disiqueira/gotree"
)

// Hash common.Hash
type Hash = common.Hash

// BlockTree represents the current state with all possible blocks
type BlockTree struct {
	head   *node // genesis node
	leaves *leafMap
	db     database.Database
}

// NewEmptyBlockTree creates a BlockTree with a nil head
func NewEmptyBlockTree(db database.Database) *BlockTree {
	return &BlockTree{
		head:   nil,
		leaves: newEmptyLeafMap(),
		db:     db,
	}
}

// NewBlockTreeFromGenesis initializes a blocktree with a genesis block.
// Currently passes in arrival time as a parameter instead of setting it as time of instanciation
func NewBlockTreeFromGenesis(genesis *types.Header, db database.Database) *BlockTree {
	head := &node{
		hash:        genesis.Hash(),
		parent:      nil,
		children:    []*node{},
		depth:       big.NewInt(0),
		arrivalTime: uint64(time.Now().Unix()), // TODO: genesis block doesn't need an arrival time, it isn't used in median algo
	}

	return &BlockTree{
		head:   head,
		leaves: newLeafMap(head),
		db:     db,
	}
}

// GenesisHash returns the hash of the genesis block
func (bt *BlockTree) GenesisHash() Hash {
	return bt.head.hash
}

// AddBlock inserts the block as child of its parent node
// Note: Assumes block has no children
func (bt *BlockTree) AddBlock(block *types.Block, arrivalTime uint64) error {
	parent := bt.getNode(block.Header.ParentHash)
	if parent == nil {
		return fmt.Errorf("cannot find parent block in blocktree")
	}

	// Check if it already exists
	n := bt.getNode(block.Header.Hash())
	if n != nil {
		return fmt.Errorf("cannot add block to blocktree that already exists: hash=%s", n.hash)
	}

	depth := big.NewInt(0)
	depth.Add(parent.depth, big.NewInt(1))

	n = &node{
		hash:        block.Header.Hash(),
		parent:      parent,
		children:    []*node{},
		depth:       depth,
		arrivalTime: arrivalTime,
	}
	parent.addChild(n)
	bt.leaves.replace(parent, n)

	return nil
}

// getNode finds and returns a node based on its Hash. Returns nil if not found.
func (bt *BlockTree) getNode(h Hash) *node {
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
	tree := gotree.New(bt.head.string())

	for _, child := range bt.head.children {
		sub := tree.Add(child.string())
		child.createTree(sub)
	}

	// Format leaves
	var leaves string
	bt.leaves.smap.Range(func(hash, node interface{}) bool {
		leaves = leaves + fmt.Sprintf("%s\n", hash.(Hash))
		return true
	})

	metadata := fmt.Sprintf("Leaves: %s", leaves)

	return fmt.Sprintf("%s\n%s\n", metadata, tree.Print())
}

// longestPath returns the path from the root to leftmost deepest leaf in BlockTree BT
func (bt *BlockTree) longestPath() []*node {
	dl := bt.deepestLeaf()
	var path []*node
	for curr := dl; ; curr = curr.parent {
		path = append([]*node{curr}, path...)
		if curr.parent == nil {
			return path
		}
	}
}

// subChain returns the path from the node with Hash start to the node with Hash end
func (bt *BlockTree) subChain(start Hash, end Hash) ([]*node, error) {
	sn := bt.getNode(start)
	if sn == nil {
		return nil, fmt.Errorf("start node does not exist")
	}
	en := bt.getNode(end)
	if en == nil {
		return nil, fmt.Errorf("end node does not exist")
	}
	return sn.subChain(en)
}

// SubBlockchain returns the path from the node with Hash start to the node with Hash end
func (bt *BlockTree) SubBlockchain(start Hash, end Hash) ([]Hash, error) {
	sc, err := bt.subChain(start, end)
	if err != nil {
		return nil, err
	}
	var bc []Hash
	for _, node := range sc {
		bc = append(bc, node.hash)
	}
	return bc, nil

}

// DeepestLeaf returns leftmost deepest leaf in BlockTree BT
func (bt *BlockTree) deepestLeaf() *node {
	return bt.leaves.deepestLeaf()
}

// DeepestBlockHash returns the hash of the deepest block in the blocktree
// If there is multiple deepest blocks, it returns the one with the earliest arrival time.
func (bt *BlockTree) DeepestBlockHash() Hash {
	return bt.leaves.deepestLeaf().hash
}
