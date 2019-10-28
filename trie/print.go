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

// String returns the trie stringified through pre-order traversal
func (t *Trie) String() string {
	return t.string("", t.root, nil, false)
}

// StringWithEncoding returns the trie stringified as well as the encoding of each node
func (t *Trie) StringWithEncoding() string {
	return t.string("", t.root, nil, true)
}

func (t *Trie) string(str string, current node, prefix []byte, withEncoding bool) string {
	h, err := NewHasher()
	if err != nil {
		return ""
	}

	var encoding []byte
	var hash []byte
	if withEncoding && current != nil {
		encoding, err = current.Encode()
		if err != nil {
			return ""
		}
		hash, err = h.Hash(current)
		if err != nil {
			return ""
		}
	}

	switch c := current.(type) {
	case *branch:
		str += fmt.Sprintf("branch prefix %x key %x children %b value %x\n", nibblesToKeyLE(prefix), nibblesToKey(c.key), c.childrenBitmap(), c.value)
		if withEncoding {
			str += fmt.Sprintf("branch encoding %x branch hash %x", encoding, hash)
		}

		for i, child := range c.children {
			str = t.string(str, child, append(append(prefix, byte(i)), c.key...), withEncoding)
		}
	case *leaf:
		str += fmt.Sprintf("leaf prefix %x key %x value %x\n", nibblesToKeyLE(prefix), nibblesToKeyLE(c.key), c.value)
		if withEncoding {
			str += fmt.Sprintf("leaf encoding %x leaf hash %x", encoding, hash)
		}
	}

	return str
}

// Print prints the trie through pre-order traversal
func (t *Trie) Print() {
	fmt.Println(t.String())
}

// PrintEncoding prints the trie with node encodings through pre-order traversal
func (t *Trie) PrintEncoding() {
	fmt.Println(t.StringWithEncoding())
}
