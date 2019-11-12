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

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/disiqueira/gotree"
)

// node is an element in the BlockTree
type node struct {
	hash        common.Hash // Block hash
	parent      *node       // Parent node
	number      *big.Int    // Block Number
	children    []*node     // Nodes of children blocks
	depth       *big.Int    // Depth within the tree
	arrivalTime uint64      // Arrival time of the block
}

// addChild appends node to n's list of children
func (n *node) addChild(node *node) {
	n.children = append(n.children, node)
}

// String returns stringified hash and depth of node
func (n *node) String() string {
	return fmt.Sprintf("{h: %s, d: %s}", n.hash.String(), n.depth)
}

// createTree adds all the nodes children to the existing printable tree.
// Note: this is strictly for BlockTree.String()
func (n *node) createTree(tree gotree.Tree) {
	for _, child := range n.children {
		sub := tree.Add(child.String())
		child.createTree(sub)
	}
}

// getNode recursively searches for a node with a given hash
func (n *node) getNode(h common.Hash) *node {
	if n.hash == h {
		return n
	} else if len(n.children) == 0 {
		return nil
	} else {
		for _, child := range n.children {
			if n := child.getNode(h); n != nil {
				return n
			}
		}
	}
	return nil
}

// getNodeFromBlockNumber recursively searches for a node with a given Number
func (n *node) getNodeFromBlockNumber(b *big.Int) *node {
	if b.Cmp(n.number) == 0 {
		return n
	} else if len(n.children) == 0 {
		return nil
	} else {
		for _, child := range n.children {
			if n := child.getNodeFromBlockNumber(b); n != nil {
				return n
			}
		}
	}
	return nil
}

func (n *node) getBlockFromNode() *types.Block {
	bh := types.BlockHeader{
		ParentHash: n.parent.hash,
		Number:     n.number,
		Hash:       n.hash,
	}

	b := &types.Block{
		Header: bh,
		Body:   types.BlockBody{},
	}
	b.SetBlockArrivalTime(n.arrivalTime)

	return b
}

// subChain recursively searches for a chain with head n and end descendant
func (n *node) subChain(descendant *node) []*node {
	if descendant == nil {
		return nil
	}
	var path []*node
	for curr := descendant; ; curr = curr.parent {
		path = append([]*node{curr}, path...)
		if curr == n {
			return path
		}
	}
}

// TODO: This would improved by using parent in node struct and searching child -> parent
// TODO: verify that parent and child exist in the DB
// isDescendantOf traverses the tree following all possible paths until it determines if n is a descendant of parent
func (n *node) isDescendantOf(parent *node) bool {
	if parent == nil {
		return false
	}

	// NOTE: here we assume the nodes exists in tree
	if n.hash == parent.hash {
		return true
	} else if len(parent.children) == 0 {
		return false
	} else {
		for _, child := range parent.children {
			if n.isDescendantOf(child) {
				return true
			}
		}
	}
	return false
}
