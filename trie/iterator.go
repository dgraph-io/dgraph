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

package trie

import (
	"fmt"
)

// Print prints the trie through pre-order traversal
func (t *Trie) Print() {
	fmt.Println("printing trie...")
	t.print(t.root)
}

func (t *Trie) print(current node) {
	switch c := current.(type) {
	case *branch:
		fmt.Printf("branch pk %x children %b value %s\n", c.key, c.childrenBitmap(), c.value)
		for _, child := range c.children {
			t.print(child)
		}
	case *leaf:
		fmt.Printf("leaf pk %x val %s\n", c.key, c.value)
	default:
		// do nothing
	}
}
