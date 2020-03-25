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
	"reflect"
	"strings"
	"testing"
)

func TestEncodeAndDecode(t *testing.T) {
	trie := &Trie{}

	tests := []Test{
		{key: []byte{0x01, 0x35}, value: []byte("pen")},
		{key: []byte{0x01, 0x35, 0x79}, value: []byte("penguin")},
		{key: []byte{0x01, 0x35, 0x7}, value: []byte("g")},
		{key: []byte{0xf2}, value: []byte("feather")},
		{key: []byte{0xf2, 0x3}, value: []byte("f")},
		{key: []byte{0x09, 0xd3}, value: []byte("noot")},
		{key: []byte{0x07}, value: []byte("ramen")},
		{key: []byte{0}, value: nil},
	}

	for _, test := range tests {
		err := trie.Put(test.key, test.value)
		if err != nil {
			t.Fatal(err)
		}
	}

	enc, err := trie.Encode()
	if err != nil {
		t.Fatal(err)
	}

	testTrie := &Trie{}
	err = testTrie.Decode(enc)
	if err != nil {
		testTrie.Print()
		t.Fatal(err)
	}

	if strings.Compare(testTrie.String(), trie.String()) != 0 {
		t.Errorf("Fail: got\n %s expected\n %s", testTrie.String(), trie.String())
	}

	if !reflect.DeepEqual(testTrie.root, trie.root) {
		t.Errorf("Fail: got\n %s expected\n %s", testTrie.String(), trie.String())
	}
}
