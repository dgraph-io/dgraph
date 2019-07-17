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
	t.print(t.root, nil, false)
}

func (t *Trie) PrintEncoding() {
	t.print(t.root, nil, true)
}

func (t *Trie) print(current node, prefix []byte, withEncoding bool) {
	h, err := NewHasher()
	if err != nil {
		fmt.Printf("new hasher err %s\n", err)
	}
	var encoding []byte
	var hash []byte
	if withEncoding && current != nil {
		encoding, err = current.Encode()
		if err != nil {
			fmt.Printf("encoding err %s\n", err)
		}
		hash, err = h.Hash(current)
		if err != nil {
			fmt.Printf("hashing err %s\n", err)
		}
	}

	switch c := current.(type) {
	case *branch:
		fmt.Printf("branch prefix %x key %x children %b value %s\n", nibblesToKeyLE(prefix), nibblesToKey(c.key), c.childrenBitmap(), c.value)
		if withEncoding {
			fmt.Printf("branch encoding ")
			printHexBytes(encoding)
			fmt.Printf("branch hash ")
			printHexBytes(hash)
		}
		for i, child := range c.children {
			t.print(child, append(append(prefix, byte(i)), c.key...), withEncoding)
		}
	case *leaf:
		fmt.Printf("leaf prefix %x key %x value %x\n", nibblesToKeyLE(prefix), nibblesToKeyLE(c.key), c.value)
		if withEncoding {
			fmt.Printf("leaf encoding ")
			printHexBytes(encoding)
			fmt.Printf("leaf hash ")
			printHexBytes(hash)
		}
	default:
		// do nothing
	}
}

func printHexBytes(in []byte) {
	fmt.Print("[")
	for i, b := range in {
		if i < len(in)-1 {
			fmt.Printf("%x, ", b)
		} else {
			fmt.Printf("%x", b)
		}
	}
	fmt.Println("]")
}
