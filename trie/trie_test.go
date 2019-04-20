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
	"bytes"
	"math/rand"
	"testing"

	"github.com/ChainSafe/gossamer/polkadb"
)

type commonPrefixTest struct {
	a, b   []byte
	output int
}

var commonPrefixTests = []commonPrefixTest{
	{a: []byte{}, b: []byte{}, output: 0},
	{a: []byte{0x00}, b: []byte{}, output: 0},
	{a: []byte{0x00}, b: []byte{0x00}, output: 1},
	{a: []byte{0x00}, b: []byte{0x00, 0x01}, output: 1},
	{a: []byte{0x01}, b: []byte{0x00, 0x01, 0x02}, output: 0},
	{a: []byte{0x00, 0x01, 0x02, 0x00}, b: []byte{0x00, 0x01, 0x02}, output: 3},
	{a: []byte{0x00, 0x01, 0x02, 0x00, 0xff}, b: []byte{0x00, 0x01, 0x02, 0x00}, output: 4},
	{a: []byte{0x00, 0x01, 0x02, 0x00, 0xff}, b: []byte{0x00, 0x01, 0x02, 0x00, 0xff, 0x00}, output: 5},
}

func TestCommonPrefix(t *testing.T) {
	for _, test := range commonPrefixTests {
		output := lenCommonPrefix(test.a, test.b)
		if output != test.output {
			t.Errorf("Fail: got %d expected %d", output, test.output)
		}
	}
}

func newEmpty() *Trie {
	db := &Database{
		db: polkadb.NewMemDatabase(),
	}
	t := NewEmptyTrie(db)
	return t
}

func TestNewEmptyTrie(t *testing.T) {
	trie := newEmpty()
	if trie == nil {
		t.Error("did not initialize trie")
	}
}

func TestNewTrie(t *testing.T) {
	db := &Database{
		db: polkadb.NewMemDatabase(),
	}
	trie := NewTrie(db, &leaf{key: []byte{0}, value: []byte{17}})
	if trie == nil {
		t.Error("did not initialize trie")
	}
}

type randTest struct {
	key   []byte
	value []byte
}

func generateRandTest(size int) []randTest {
	rt := make([]randTest, size)
	r := *rand.New(rand.NewSource(rand.Int63()))
	for i := range rt {
		rt[i] = randTest{}
		buf := make([]byte, r.Intn(379)+1)
		r.Read(buf)
		rt[i].key = buf

		buf = make([]byte, r.Intn(128))
		r.Read(buf)
		rt[i].value = buf
	}
	return rt
}

func TestBranch(t *testing.T) {
	trie := newEmpty()

	key1 := []byte{0x01, 0x35}
	value1 := []byte("spaghetti")
	key2 := []byte{0x01, 0x35, 0x79}
	value2 := []byte("gnocchi")
	key3 := []byte{0x07}
	value3 := []byte("ramen")
	key4 := []byte{0xf2}
	value4 := []byte("pho")

	err := trie.Put(key1, value1)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key1, value1, err.Error())
	}

	err = trie.Put(key2, value2)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key2, value2, err.Error())
	}

	err = trie.Put(key3, value3)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key3, value3, err.Error())
	}

	err = trie.Put(key4, value4)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key4, value4, err.Error())
	}

	val, err := trie.Get([]byte("noot"))
	if err != nil {
		t.Errorf("Fail to get key %x: %s", "noot", err.Error())
	} else if !bytes.Equal(val, nil) {
		t.Errorf("Fail to get key %x with nil value: got %x", "noot", val)
	}

	val, err = trie.Get([]byte{0})
	if err != nil {
		t.Errorf("Fail to get key %x: %s", []byte{0}, err.Error())
	} else if !bytes.Equal(val, nil) {
		t.Errorf("Fail to get key %x with nil value: got %x", []byte{0}, val)
	}

	val, err = trie.Get(key1)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key1, err.Error())
	} else if !bytes.Equal(val, value1) {
		t.Errorf("Fail to get key %x with value %x: got %x", key1, value1, val)
	}

	val, err = trie.Get(key2)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key2, err.Error())
	} else if !bytes.Equal(val, value2) {
		t.Errorf("Fail to get key %x with value %x: got %x", key2, value2, val)
	}

	val, err = trie.Get(key3)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key3, err.Error())
	} else if !bytes.Equal(val, value3) {
		t.Errorf("Fail to get key %x with value %x: got %x", key3, value3, val)
	}

	val, err = trie.Get(key4)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key4, err.Error())
	} else if !bytes.Equal(val, value4) {
		t.Errorf("Fail to get key %x with value %x: got %x", key4, value4, val)
	}
}

func TestBranchMore(t *testing.T) {
	trie := newEmpty()

	key1 := []byte{0x01}
	value1 := []byte("spaghetti")
	key2 := []byte{0x02}
	value2 := []byte("gnocchi")
	key3 := []byte{0xf7}
	value3 := []byte("ramen")
	key4 := []byte{0x43}
	value4 := []byte("pho")

	err := trie.Put(key1, value1)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key1, value1, err.Error())
	}

	err = trie.Put(key2, value2)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key2, value2, err.Error())
	}

	err = trie.Put(key3, value3)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key3, value3, err.Error())
	}

	err = trie.Put(key4, value4)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key4, value4, err.Error())
	}

	val, err := trie.Get([]byte{0})
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key1, err.Error())
	} else if !bytes.Equal(val, nil) {
		t.Errorf("Fail to get key %x with nil value: got %x", "noot", val)
	}

	val, err = trie.Get(key1)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key1, err.Error())
	} else if !bytes.Equal(val, value1) {
		t.Errorf("Fail to get key %x with value %x: got %x", key1, value1, val)
	}

	val, err = trie.Get(key2)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key2, err.Error())
	} else if !bytes.Equal(val, value2) {
		t.Errorf("Fail to get key %x with value %x: got %x", key2, value2, val)
	}

	val, err = trie.Get(key3)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key3, err.Error())
	} else if !bytes.Equal(val, value3) {
		t.Errorf("Fail to get key %x with value %x: got %x", key3, value3, val)
	}

	val, err = trie.Get(key4)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key4, err.Error())
	} else if !bytes.Equal(val, value4) {
		t.Errorf("Fail to get key %x with value %x: got %x", key4, value4, val)
	}
}

func TestPutAndGet(t *testing.T) {
	for i := 0; i < 20; i++ {
		trie := newEmpty()
		rt := generateRandTest(20000)
		for _, test := range rt {
			err := trie.Put(test.key, test.value)
			if err != nil {
				t.Errorf("Fail to put with key %x and value %x: %s", test.key, test.value, err.Error())
			}

			val, err := trie.Get(test.key)
			if err != nil {
				t.Errorf("Fail to get key %x: %s", test.key, err.Error())
			} else if !bytes.Equal(val, test.value) {
				t.Errorf("Fail to get key %x with value %x: got %x", test.key, test.value, val)
			}
		}
	}
}

// To be used once trie.Delete is implemented
// func TestDelete(t *testing.T) {
// 	trie := newEmpty()

// 	rt := generateRandTest(1000)
// 	for _, test := range rt {
// 		err := trie.Put(test.key, test.value)
// 		if err != nil {
// 			t.Errorf("Fail to put with key %x and value %x: %s", test.key, test.value, err.Error())
// 		}
// 	}

// 	for _, test := range rt {
// 		r := rand.Int() % 2
// 		switch r {
// 		case 0:
// 			err := trie.Delete(test.key)
// 			if err != nil {
// 				t.Errorf("Fail to delete key %x: %s", test.key, err.Error())
// 			}

// 			val, err := trie.Get(test.key)
// 			if err != nil {
// 				t.Errorf("Error when attempting to get deleted key %x: %s", test.key, err.Error())
// 			} else if val != nil {
// 				t.Errorf("Fail to delete key %x with value %x: got %x", test.key, test.value, val)
// 			}
// 		case 1:
// 			val, err := trie.Get(test.key)
// 			if err != nil {
// 				t.Errorf("Error when attempting to get key %x: %s", test.key, err.Error())
// 			} else if !bytes.Equal(test.value, val) {
// 				t.Errorf("Fail to get key %x with value %x: got %x", test.key, test.value, val)
// 			}
// 		}
// 	}
// }

func TestGetPartialKey(t *testing.T) {
	trie := newEmpty()

	key1 := []byte{0x01, 0x35}
	value1 := []byte("pen")
	key2 := []byte{0x01, 0x35, 0x79}
	value2 := []byte("penguin")
	key3 := []byte{0xf2}
	value3 := []byte("feather")
	key4 := []byte{0x09, 0xd3}
	value4 := []byte("noot")
	key5 := []byte{}
	value5 := []byte("floof")

	pk0 := []byte{0x1, 0x3, 0x5}
	pk1 := []byte{0x3, 0x5}
	pk2 := []byte{0x9}
	pk3 := []byte{0x2}
	pk4 := []byte{0x0d, 0x03}

	err := trie.Put(key1, value1)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key1, value1, err.Error())
	}

	err = trie.Put(key2, value2)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key2, value2, err.Error())
	}

	err = trie.Put(key5, value5)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key5, value5, err.Error())
	}

	var val []byte
	leaf, err := trie.getLeaf(key1)
	if leaf == nil {
		t.Errorf("Fail to get key %x: nil leaf", key1)
	} else if err != nil {
		t.Errorf("Fail to get key %x: %s", key1, err.Error())
	} else if !bytes.Equal(leaf.value, value1) {
		t.Errorf("Fail to get key %x with value %x: got %x", key1, value1, val)
	} else if !bytes.Equal(leaf.key, pk0) {
		t.Errorf("Fail to get correct partial key %x: got %x", pk0, leaf.key)
	}

	leaf, err = trie.getLeaf(key2)
	if leaf == nil {
		t.Errorf("Fail to get key %x: nil leaf", key2)
	} else if err != nil {
		t.Errorf("Fail to get key %x: %s", key2, err.Error())
	} else if !bytes.Equal(leaf.value, value2) {
		t.Errorf("Fail to get key %x with value %x: got %x", key2, value2, val)
	} else if !bytes.Equal(leaf.key, pk2) {
		t.Errorf("Fail to get correct partial key %x: got %x", pk2, leaf.key)
	}

	err = trie.Put(key3, value3)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key3, value3, err.Error())
	}

	err = trie.Put(key4, value4)
	if err != nil {
		t.Errorf("Fail to put with key %x and value %x: %s", key4, value4, err.Error())
	}

	trie.Print()

	val, err = trie.Get(key5)
	if err != nil {
		t.Errorf("Fail to get key %x: %s", key5, err.Error())
	} else if !bytes.Equal(val, value5) {
		t.Errorf("Fail to get key %x with value %x: got %x", key5, value5, val)
	}

	leaf, err = trie.getLeaf(key1)
	if leaf == nil {
		t.Errorf("Fail to get key %x: nil leaf", key1)
	} else if err != nil {
		t.Errorf("Fail to get key %x: %s", key1, err.Error())
	} else if !bytes.Equal(leaf.value, value1) {
		t.Errorf("Fail to get key %x with value %x: got %x", key1, value1, val)
	} else if !bytes.Equal(leaf.key, pk1) {
		t.Errorf("Fail to get correct partial key %x: got %x", pk1, leaf.key)
	}

	leaf, err = trie.getLeaf(key2)
	if leaf == nil {
		t.Errorf("Fail to get key %x: nil leaf", key2)
	} else if err != nil {
		t.Errorf("Fail to get key %x: %s", key2, err.Error())
	} else if !bytes.Equal(leaf.value, value2) {
		t.Errorf("Fail to get key %x with value %x: got %x", key2, value2, val)
	} else if !bytes.Equal(leaf.key, pk2) {
		t.Errorf("Fail to get correct partial key %x: got %x", pk2, leaf.key)
	}

	leaf, err = trie.getLeaf(key3)
	if leaf == nil {
		t.Errorf("Fail to get key %x: nil leaf", key3)
	} else if err != nil {
		t.Errorf("Fail to get key %x: %s", key3, err.Error())
	} else if !bytes.Equal(leaf.value, value3) {
		t.Errorf("Fail to get key %x with value %x: got %x", key3, value3, val)
	} else if !bytes.Equal(leaf.key, pk3) {
		t.Errorf("Fail to get correct partial key %x: got %x", pk3, leaf.key)
	}

	leaf, err = trie.getLeaf(key4)
	if leaf == nil {
		t.Errorf("Fail to get key %x: nil leaf", key4)
	} else if err != nil {
		t.Errorf("Fail to get key %x: %s", key4, err.Error())
	} else if !bytes.Equal(leaf.value, value4) {
		t.Errorf("Fail to get key %x with value %x: got %x", key4, value4, val)
	} else if !bytes.Equal(leaf.key, pk4) {
		t.Errorf("Fail to get correct partial key %x: got %x", pk4, leaf.key)
	}
}
