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

package common

import (
	"bytes"
	"testing"
)

func TestBlake2b218_EmptyHash(t *testing.T) {
	// test case from https://github.com/noot/blake2b_test which uses the blake2-rfp rust crate
	// also see https://github.com/paritytech/substrate/blob/master/core/primitives/src/hashing.rs
	in := []byte{}
	h, err := Blake2b128(in)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := HexToBytes("0xcae66941d9efbd404e4d88758ea67670")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, h) {
		t.Fatalf("Fail: got %x expected %x", h, expected)
	}
}

func TestBlake2bHash_EmptyHash(t *testing.T) {
	// test case from https://github.com/noot/blake2b_test which uses the blake2-rfp rust crate
	// also see https://github.com/paritytech/substrate/blob/master/core/primitives/src/hashing.rs
	in := []byte{}
	h, err := Blake2bHash(in)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := HexToBytes("0x0e5751c026e543b2e8ab2eb06099daa1d1e5df47778f7787faab45cdf12fe3a8")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, h[:]) {
		t.Fatalf("Fail: got %x expected %x", h, expected)
	}
}

func TestKeccak256_EmptyHash(t *testing.T) {
	// test case from https://github.com/debris/tiny-keccak/blob/master/tests/keccak.rs#L4
	in := []byte{}
	h := Keccak256(in)
	expected, err := HexToHash("0xc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(expected[:], h[:]) {
		t.Fatalf("Fail: got %x expected %x", h, expected)
	}
}
