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
	"testing"
)

func TestKeyEncodeByte(t *testing.T) {
	tests := []struct {
		input    byte
		expected byte
	}{
		{byte(0xA0), byte(0x0A)},
		{byte(0), byte(0)},
		{byte(0x24), byte(0x42)},
	}

	for _, test := range tests {
		res := keyEncodeByte(test.input)
		if res != test.expected {
			t.Fatalf("got: %x; expected: %x", res, test.expected)
		}
	}
}

func TestKeyEncode(t *testing.T) {
	tests := []struct {
		key        []byte
		encodedKey []byte
	}{
		{[]byte{0x01, 0x02, 0x03, 0x04, 0x05}, []byte{0x10, 0x20, 0x30, 0x40, 0x50}},
		{[]byte{0xff, 0x0, 0xAA, 0x81}, []byte{0xff, 0x00, 0xAA, 0x18}},
		{[]byte{0xAC, 0x19, 0x15}, []byte{0xCA, 0x91, 0x51}},
	}

	for _, test := range tests {
		res := KeyEncode(test.key)
		if !bytes.Equal(res, test.encodedKey) {
			t.Fatalf("got: %x, expected: %x", res, test.encodedKey)
		}

		res = KeyEncode(res)
		if !bytes.Equal(res, test.key) {
			t.Fatalf("Re-encoding failed. got: %x expected: %x", res, test.key)
		}
	}

}

func TestKeyToNibbles(t *testing.T) {
	tests := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte{0x0}, []byte{0, 0}},
		{[]byte{0xFF}, []byte{0xF, 0xF}},
		{[]byte{0x3a, 0x05}, []byte{0x3, 0xa, 0x5}},
		{[]byte{0xAA, 0xFF, 0x01}, []byte{0xa, 0xa, 0xf, 0xf, 0x1}},
		{[]byte{0xAA, 0xFF, 0x01, 0xc2}, []byte{0xa, 0xa, 0xf, 0xf, 0x0, 0x1, 0xc, 0x2}},
		{[]byte{0xAA, 0xFF, 0x01, 0xc0}, []byte{0xa, 0xa, 0xf, 0xf, 0x0, 0x1, 0xc, 0x0}},
	}

	for _, test := range tests {
		res := keyToNibbles(test.input)
		if !bytes.Equal(test.expected, res) {
			t.Errorf("Output doesn't match expected. got=%v expected=%v\n", res, test.expected)
		}
	}
}

func TestNibblesToKey(t *testing.T) {
	tests := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte{0xF, 0xF}, []byte{0xFF}},
		{[]byte{0x3, 0xa, 0x0, 0x5}, []byte{0xa3, 0x50}},
		{[]byte{0xa, 0xa, 0xf, 0xf, 0x0, 0x1}, []byte{0xaa, 0xff, 0x10}},
		{[]byte{0xa, 0xa, 0xf, 0xf, 0x0, 0x1, 0xc, 0x2}, []byte{0xaa, 0xff, 0x10, 0x2c}},
		{[]byte{0xa, 0xa, 0xf, 0xf, 0x0, 0x1, 0xc}, []byte{0xaa, 0xff, 0x10, 0x0c}},
	}

	for _, test := range tests {
		res := nibblesToKey(test.input)
		if !bytes.Equal(test.expected, res) {
			t.Errorf("Output doesn't match expected. got=%x expected=%x\n", res, test.expected)
		}
	}
}

func TestNibblesToKeyLE(t *testing.T) {
	tests := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte{0xF, 0xF}, []byte{0xFF}},
		{[]byte{0x3, 0xa, 0x0, 0x5}, []byte{0x3a, 0x05}},
		{[]byte{0xa, 0xa, 0xf, 0xf, 0x0, 0x1}, []byte{0xaa, 0xff, 0x01}},
		{[]byte{0xa, 0xa, 0xf, 0xf, 0x0, 0x1, 0xc, 0x2}, []byte{0xaa, 0xff, 0x01, 0xc2}},
		{[]byte{0xa, 0xa, 0xf, 0xf, 0x0, 0x1, 0xc}, []byte{0xaa, 0xff, 0x01, 0xc}},
	}

	for _, test := range tests {
		res := nibblesToKeyLE(test.input)
		if !bytes.Equal(test.expected, res) {
			t.Errorf("Output doesn't match expected. got=%x expected=%x\n", res, test.expected)
		}
	}
}
