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

	"github.com/ChainSafe/gossamer/common"
)

func generateRandBytes(size int) []byte {
	r := *rand.New(rand.NewSource(rand.Int63()))
	buf := make([]byte, r.Intn(size)+1)
	r.Read(buf)
	return buf
}

func generateRand(size int) [][]byte {
	rt := make([][]byte, size)
	r := *rand.New(rand.NewSource(rand.Int63()))
	for i := range rt {
		buf := make([]byte, r.Intn(379)+1)
		r.Read(buf)
		rt[i] = buf
	}
	return rt
}

func TestNewHasher(t *testing.T) {
	hasher, err := NewHasher()
	if err != nil {
		t.Fatalf("error creating new hasher: %s", err)
	} else if hasher == nil {
		t.Fatal("did not create new hasher")
	}

	_, err = hasher.hash.Write([]byte("noot"))
	if err != nil {
		t.Error(err)
	}

	sum := hasher.hash.Sum(nil)
	if sum == nil {
		t.Error("did not sum hash")
	}

	hasher.hash.Reset()
}

func TestHashLeaf(t *testing.T) {
	hasher, err := NewHasher()
	if err != nil {
		t.Fatal(err)
	}

	n := &leaf{key: generateRandBytes(380), value: generateRandBytes(64)}
	h, err := hasher.Hash(n)
	if err != nil {
		t.Errorf("did not hash leaf node: %s", err)
	} else if h == nil {
		t.Errorf("did not hash leaf node: nil")
	}
}

func TestHashBranch(t *testing.T) {
	hasher, err := NewHasher()
	if err != nil {
		t.Fatal(err)
	}

	n := &branch{key: generateRandBytes(380), value: generateRandBytes(380)}
	n.children[3] = &leaf{key: generateRandBytes(380), value: generateRandBytes(380)}
	h, err := hasher.Hash(n)
	if err != nil {
		t.Errorf("did not hash branch node: %s", err)
	} else if h == nil {
		t.Errorf("did not hash branch node: nil")
	}
}

func TestHashShort(t *testing.T) {
	hasher, err := NewHasher()
	if err != nil {
		t.Fatal(err)
	}

	n := &leaf{key: generateRandBytes(2), value: generateRandBytes(3)}
	expected, err := n.Encode()
	if err != nil {
		t.Fatal(err)
	}

	expected = common.AppendZeroes(expected, 32)

	h, err := hasher.Hash(n)
	if err != nil {
		t.Errorf("did not hash leaf node: %s", err)
	} else if h == nil {
		t.Errorf("did not hash leaf node: nil")
	} else if !bytes.Equal(h, expected) {
		t.Errorf("did not return encoded node padded to 32 bytes: got %s", h)
	}
}
