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

type concatTest struct {
	a, b   []byte
	output []byte
}

var concatTests = []concatTest{
	{a: []byte{}, b: []byte{}, output: []byte{}},
	{a: []byte{0x00}, b: []byte{}, output: []byte{0x00}},
	{a: []byte{0x00}, b: []byte{0x00}, output: []byte{0x00, 0x00}},
	{a: []byte{0x00}, b: []byte{0x00, 0x01}, output: []byte{0x00, 0x00, 0x01}},
	{a: []byte{0x01}, b: []byte{0x00, 0x01, 0x02}, output: []byte{0x01, 0x00, 0x01, 0x02}},
	{a: []byte{0x00, 0x01, 0x02, 0x00}, b: []byte{0x00, 0x01, 0x02}, output: []byte{0x000, 0x01, 0x02, 0x00, 0x00, 0x01, 0x02}},
}

func TestConcat(t *testing.T) {
	for _, test := range concatTests {
		output := Concat(test.a, test.b...)
		if !bytes.Equal(output, test.output) {
			t.Errorf("Fail: got %d expected %d", output, test.output)
		}
	}
}

func TestUint16ToBytes(t *testing.T) {
	tests := []struct {
		input    uint16
		expected []byte
	}{
		{uint16(0), []byte{0x0, 0x0}},
		{uint16(1), []byte{0x1, 0x0}},
		{uint16(255), []byte{0xff, 0x0}},
	}

	for _, test := range tests {
		res := Uint16ToBytes(test.input)
		if !bytes.Equal(res, test.expected) {
			t.Errorf("Output doesn't match expected. got=%v expected=%v\n", res, test.expected)
		}
	}
}

func TestSwapByteNibbles(t *testing.T) {
	tests := []struct {
		input    byte
		expected byte
	}{
		{byte(0xA0), byte(0x0A)},
		{byte(0), byte(0)},
		{byte(0x24), byte(0x42)},
	}

	for _, test := range tests {
		res := SwapByteNibbles(test.input)
		if res != test.expected {
			t.Fatalf("got: %x; expected: %x", res, test.expected)
		}
	}
}

func TestSwapNibbles(t *testing.T) {
	tests := []struct {
		key        []byte
		encodedKey []byte
	}{
		{[]byte{0x01, 0x02, 0x03, 0x04, 0x05}, []byte{0x10, 0x20, 0x30, 0x40, 0x50}},
		{[]byte{0xff, 0x0, 0xAA, 0x81}, []byte{0xff, 0x00, 0xAA, 0x18}},
		{[]byte{0xAC, 0x19, 0x15}, []byte{0xCA, 0x91, 0x51}},
	}

	for _, test := range tests {
		res := SwapNibbles(test.key)
		if !bytes.Equal(res, test.encodedKey) {
			t.Fatalf("got: %x, expected: %x", res, test.encodedKey)
		}

		res = SwapNibbles(res)
		if !bytes.Equal(res, test.key) {
			t.Fatalf("Re-encoding failed. got: %x expected: %x", res, test.key)
		}
	}

}