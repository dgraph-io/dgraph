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

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/disiqueira/gotree"
)

// node is an element in the BlockTree
type node struct {
	hash        common.Hash // Block hash
	parent      *node       // Parent Node
	children    []*node     // Nodes of children blocks
	depth       *big.Int    // Depth within the tree
	arrivalTime uint64      // Arrival time of the block
}

// addChild appends Node to n's list of children
func (n *node) addChild(node *node) {
	n.children = append(n.children, node)
}

// string returns stringified hash and depth of node
func (n *node) string() string {
	return fmt.Sprintf("{hash: %s, depth: %s, arrivalTime: %d}", n.hash.String(), n.depth, n.arrivalTime)
}

// createTree adds all the nodes children to the existing printable tree.
// Note: this is strictly for BlockTree.String()
func (n *node) createTree(tree gotree.Tree) {
	for _, child := range n.children {
		sub := tree.Add(child.string())
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

// getNodesWithDepth returns all descendent nodes with the desired depth
func (n *node) getNodesWithDepth(depth *big.Int, hashes []common.Hash) []common.Hash {
	for _, child := range n.children {
		// depth matches
		if child.depth.Cmp(depth) == 0 {
			hashes = append(hashes, child.hash)
		}

		// are deeper than desired depth, return
		if child.depth.Cmp(depth) > 0 {
			return hashes
		}

		hashes = child.getNodesWithDepth(depth, hashes)
	}

	return hashes
}

// subChain searches for a chain with head n and descendant going from child -> parent
func (n *node) subChain(descendant *node) ([]*node, error) {
	if descendant == nil {
		return nil, ErrNilDescendant
	}

	var path []*node

	if n.hash == descendant.hash {
		path = append(path, n)
		return path, nil
	}

	for curr := descendant; curr != nil; curr = curr.parent {
		path = append([]*node{curr}, path...)
		if curr.hash == n.hash {
			return path, nil
		}
	}

	return nil, ErrDescendantNotFound
}

// isDescendantOf traverses the tree following all possible paths until it determines if n is a descendant of parent
func (n *node) isDescendantOf(parent *node) bool {
	if parent == nil || n == nil {
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

func (n *node) highestCommonAncestor(other *node) *node {
	for curr := n; curr != nil; curr = curr.parent {
		if curr.hash == other.hash {
			return curr
		}

		if other.isDescendantOf(curr) {
			return curr
		}
	}

	return nil
}

// getLeaves returns all nodes that are leaf nodes with the current node as its ancestor
func (n *node) getLeaves(leaves []*node) []*node {
	if n == nil {
		return leaves
	}

	if leaves == nil {
		leaves = []*node{}
	}

	if n.children == nil || len(n.children) == 0 {
		leaves = append(leaves, n)
	}

	for _, child := range n.children {
		leaves = child.getLeaves(leaves)
	}

	return leaves
}

// getAllDescendantsExcluding returns an array of the node's hash and all its descendants's hashes
// except for the excluded node and its subtree
func (n *node) getAllDescendantsExcluding(desc []Hash, excl Hash) []Hash {
	if n == nil || n.hash == excl {
		return desc
	}

	if desc == nil {
		desc = []Hash{}
	}

	desc = append(desc, n.hash)
	for _, child := range n.children {
		desc = child.getAllDescendantsExcluding(desc, excl)
	}

	return desc
}

// getAllDescendants returns an array of the node's hash and all its descendants's hashes
func (n *node) getAllDescendants(desc []Hash) []Hash {
	if n == nil {
		return desc
	}

	if desc == nil {
		desc = []Hash{}
	}

	desc = append(desc, n.hash)
	for _, child := range n.children {
		desc = child.getAllDescendants(desc)
	}

	return desc
}
