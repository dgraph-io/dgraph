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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNode_GetLeaves(t *testing.T) {
	var bt *BlockTree
	var branches []testBranch

	for {
		bt, branches = createTestBlockTree(testHeader, 5, nil)
		if len(branches) > 0 && len(bt.getNode(branches[0].hash).children) > 0 {
			break
		}
	}

	testNode := bt.getNode(branches[0].hash).children[0]
	leaves := testNode.getLeaves(nil)

	expected := []*node{}
	for _, lf := range bt.leaves.toMap() {
		if lf.isDescendantOf(testNode) {
			expected = append(expected, lf)
		}
	}

	require.ElementsMatch(t, expected, leaves)
}

func TestNode_GetAllDescendantsExcluding(t *testing.T) {
	var bt *BlockTree
	var branches []testBranch

	for {
		bt, branches = createTestBlockTree(testHeader, 5, nil)
		if len(branches) > 0 && len(bt.getNode(branches[0].hash).children) > 0 {
			break
		}
	}

	excl := bt.getNode(branches[0].hash).children[0].hash
	res := bt.head.getAllDescendantsExcluding(nil, excl)

	// get all nodes of excluded sub-tree
	exclSubTree := bt.getNode(branches[0].hash).children[0].getAllDescendants(nil)

	// assert that there are no nodes in res that are in excluded sub-tree
	for _, d := range res {
		for _, e := range exclSubTree {
			require.NotEqual(t, d, e)
		}
	}
}
