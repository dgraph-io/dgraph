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

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"

	database "github.com/ChainSafe/chaindb"
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
// Currently passes in arrival time as a parameter instead of setting it as time of instantiation
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

func newBlockTreeFromNode(head *node, db database.Database) *BlockTree {
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
		return ErrParentNotFound
	}

	// Check if it already exists
	n := bt.getNode(block.Header.Hash())
	if n != nil {
		return ErrBlockExists
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

// GetAllBlocksAtDepth will return all blocks hashes with the depth of the given hash plus one.
// To find all blocks at a depth matching a certain block, pass in that block's parent hash
func (bt *BlockTree) GetAllBlocksAtDepth(hash common.Hash) []common.Hash {
	hashes := []common.Hash{}

	if bt.getNode(hash) == nil {
		return hashes
	}

	depth := big.NewInt(0).Add(bt.getNode(hash).depth, big.NewInt(1))

	if bt.head.depth.Cmp(depth) == 0 {
		hashes = append(hashes, bt.head.hash)
		return hashes
	}

	return bt.head.getNodesWithDepth(depth, hashes)
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

// Prune sets the given hash as the new blocktree root, removing all nodes that are not the new root node or its descendant
// It returns an array of hashes that have been pruned
func (bt *BlockTree) Prune(newRoot Hash) (pruned []Hash) {
	if newRoot == bt.head.hash {
		return pruned
	}

	n := bt.getNode(newRoot)
	if n == nil {
		return pruned
	}

	// get pruned nodes
	pruned = bt.head.getAllDescendantsExcluding(nil, newRoot)

	// set blocktree with new root node
	next := newBlockTreeFromNode(n, bt.db)
	*bt = *next

	return pruned
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

	metadata := fmt.Sprintf("Leaves:\n %s", leaves)

	return fmt.Sprintf("%s\n%s\n", metadata, tree.Print())
}

// longestPath returns the path from the root to leftmost deepest leaf in BlockTree BT
func (bt *BlockTree) longestPath() []*node { //nolint
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
		return nil, ErrStartNodeNotFound
	}
	en := bt.getNode(end)
	if en == nil {
		return nil, ErrEndNodeNotFound
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
func (bt *BlockTree) deepestLeaf() *node { //nolint
	return bt.leaves.deepestLeaf()
}

// DeepestBlockHash returns the hash of the deepest block in the blocktree
// If there is multiple deepest blocks, it returns the one with the earliest arrival time.
func (bt *BlockTree) DeepestBlockHash() Hash {
	if bt.leaves == nil {
		return Hash{}
	}

	if bt.leaves.deepestLeaf() == nil {
		return Hash{}
	}

	return bt.leaves.deepestLeaf().hash
}

// IsDescendantOf returns true if the child is a descendant of parent, false otherwise.
// it returns an error if either the child or parent are not in the blocktree.
func (bt *BlockTree) IsDescendantOf(parent, child Hash) (bool, error) {
	pn := bt.getNode(parent)
	if pn == nil {
		return false, ErrStartNodeNotFound
	}
	cn := bt.getNode(child)
	if cn == nil {
		return false, ErrEndNodeNotFound
	}
	return cn.isDescendantOf(pn), nil
}

// Leaves returns the leaves of the blocktree as an array
func (bt *BlockTree) Leaves() []Hash {
	lm := bt.leaves.toMap()
	la := make([]common.Hash, len(lm))
	i := 0

	for k := range lm {
		la[i] = k
		i++
	}

	return la
}

// HighestCommonAncestor returns the highest block that is a Ancestor to both a and b
func (bt *BlockTree) HighestCommonAncestor(a, b Hash) (Hash, error) {
	an := bt.getNode(a)
	if an == nil {
		return common.Hash{}, ErrNodeNotFound
	}
	bn := bt.getNode(b)
	if bn == nil {
		return common.Hash{}, ErrNodeNotFound
	}

	return an.highestCommonAncestor(bn).hash, nil
}

// GetAllBlocks returns all the blocks in the tree
func (bt *BlockTree) GetAllBlocks() []Hash {
	return bt.head.getAllDescendants(nil)
}
