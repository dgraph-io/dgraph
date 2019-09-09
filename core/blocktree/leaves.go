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

	"github.com/ChainSafe/gossamer/common"
)

// leafMap provides quick lookup for existing leaves
type leafMap map[common.Hash]*node

// Replace deletes the old node from the map and inserts the new one
func (ls leafMap) Replace(old, new *node) {
	delete(ls, old.hash)
	ls[new.hash] = new
}

// DeepestLeaf searches the stored leaves to the find the one with the greatest depth.
// TODO: Select left-most
func (ls leafMap) DeepestLeaf() *node {
	max := big.NewInt(-1)
	var dLeaf *node
	for _, n := range ls {
		if max.Cmp(n.depth) < 0 {
			max = n.depth
			dLeaf = n
		}
	}
	return dLeaf
}
