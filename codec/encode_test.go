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

package codec

import (
	"bytes"
	"math/big"
	"strings"
	"testing"
)

type encodeTest struct {
	val          interface{}
	output       []byte
	bytesEncoded int
}

var encodeTests = []encodeTest{

	// fixed width
	{val: int8(1), output: []byte{0x01}, bytesEncoded: 1},
	{val: uint8(1), output: []byte{0x01}, bytesEncoded: 1},

	{val: int16(1), output: []byte{0x01, 0}, bytesEncoded: 2},
	{val: int16(16383), output: []byte{0xff, 0x3f}, bytesEncoded: 2},

	{val: uint16(1), output: []byte{0x01, 0}, bytesEncoded: 2},
	{val: uint16(16383), output: []byte{0xff, 0x3f}, bytesEncoded: 2},

	{val: int32(1), output: []byte{0x01, 0, 0, 0}, bytesEncoded: 4},
	{val: int32(16383), output: []byte{0xff, 0x3f, 0, 0}, bytesEncoded: 4},
	{val: int32(1073741823), output: []byte{0xff, 0xff, 0xff, 0x3f}, bytesEncoded: 4},

	{val: uint32(1), output: []byte{0x01, 0, 0, 0}, bytesEncoded: 4},
	{val: uint32(16383), output: []byte{0xff, 0x3f, 0, 0}, bytesEncoded: 4},
	{val: uint32(1073741823), output: []byte{0xff, 0xff, 0xff, 0x3f}, bytesEncoded: 4},

	// compact integers
	{val: big.NewInt(0), output: []byte{0x00}, bytesEncoded: 1},
	{val: big.NewInt(1), output: []byte{0x04}, bytesEncoded: 1},
	{val: big.NewInt(42), output: []byte{0xa8}, bytesEncoded: 1},
	{val: big.NewInt(69), output: []byte{0x15, 0x01}, bytesEncoded: 2},
	{val: big.NewInt(16383), output: []byte{0xfd, 0xff}, bytesEncoded: 2},
	{val: big.NewInt(16384), output: []byte{0x02, 0x00, 0x01, 0x00}, bytesEncoded: 4},
	{val: big.NewInt(1073741823), output: []byte{0xfe, 0xff, 0xff, 0xff}, bytesEncoded: 4},
	{val: big.NewInt(1<<32 - 1), output: []byte{0x03, 0xff, 0xff, 0xff, 0xff}, bytesEncoded: 5},

	// byte arrays
	{val: []byte{0x01}, output: []byte{0x04, 0x01}, bytesEncoded: 2},
	{val: []byte{0xff}, output: []byte{0x04, 0xff}, bytesEncoded: 2},
	{val: []byte{0x01, 0x01}, output: []byte{0x08, 0x01, 0x01}, bytesEncoded: 3},
	{val: []byte{0x01, 0x01}, output: []byte{0x08, 0x01, 0x01}, bytesEncoded: 3},
	{val: byteArray(64), output: append([]byte{0x01, 0x01}, byteArray(64)...), bytesEncoded: 66},
	{val: byteArray(16384), output: append([]byte{0x02, 0x00, 0x01, 0x00}, byteArray(16384)...), bytesEncoded: 16388},

	// booleans
	{val: true, output: []byte{0x01}, bytesEncoded: 1},
	{val: false, output: []byte{0x00}, bytesEncoded: 1},

	// structs
	{val: &struct {
		Foo []byte
		Bar int32
	}{[]byte{0x01}, 2}, output: []byte{0x04, 0x01, 0x02, 0, 0, 0}, bytesEncoded: 6},
	{val: &struct {
		Foo []byte
		Bar int32
		Ok  bool
	}{[]byte{0x01}, 2, true}, output: []byte{0x04, 0x01, 0x02, 0, 0, 0, 0x01}, bytesEncoded: 7},
	{val: &struct {
		Foo int32
		Bar []byte
	}{16384, []byte{0xff}}, output: []byte{0, 0x40, 0, 0, 0x04, 0xff}, bytesEncoded: 6},
	{val: &struct {
		Foo int64
		Bar []byte
	}{int64(1073741824), byteArray(64)}, output: append([]byte{0, 0, 0, 0x40, 0, 0, 0, 0, 1, 1}, byteArray(64)...), bytesEncoded: 74},

	// Arrays
	{val: []int{1, 2, 3, 4}, output: []byte{0x10, 0x04, 0x08, 0x0c, 0x10}, bytesEncoded: 5},
	{val: []int{16384, 2, 3, 4}, output: []byte{0x10, 0x02, 0x00, 0x01, 0x00, 0x08, 0x0c, 0x10}, bytesEncoded: 8},
	{val: []int{1073741824, 2, 3, 4}, output: []byte{0x10, 0x03, 0x00, 0x00, 0x00, 0x40, 0x08, 0x0c, 0x10}, bytesEncoded: 9},
	{val: []int{1 << 32, 2, 3, 1 << 32}, output: []byte{0x10, 0x07, 0x00, 0x00, 0x00, 0x00, 0x01, 0x08, 0x0c, 0x07, 0x00, 0x00, 0x00, 0x00, 0x01}, bytesEncoded: 15},
	{val: []bool{true, false, true}, output: []byte{0x0c, 0x01, 0x00, 0x01}, bytesEncoded: 4},
	{val: [][]int{{0, 1}, {1, 0}}, output: []byte{0x08, 0x08, 0x00, 0x04, 0x08, 0x04, 0x00}, bytesEncoded: 7},
	{val: []*big.Int{big.NewInt(0), big.NewInt(1)}, output: []byte{0x08, 0x00, 0x04}, bytesEncoded: 3},
}

// Test strings for various values of n & mode. Also test strings with special characters
func setUpStringTests() {

	testString1 := "We love you! We believe in open source as wonderful form of giving."                           // n = 67
	testString2 := strings.Repeat("We need a longer string to test with. Let's multiple this several times.", 230) // n = 72 * 230 = 16560
	testString3 := "Let's test some special ASCII characters: ~  · © ÿ"                                           // n = 55 (UTF-8 encoding versus n = 51 with ASCII encoding)

	testStrings := []encodeTest{
		{val: string("a"),
			output: []byte{0x04, 0x61}, bytesEncoded: 2},
		{val: string("go-pre"), // n = 6, mode = 0
			output: append([]byte{0x18}, string("go-pre")...), bytesEncoded: 7}, // n|mode = 0x18
		{val: testString1, // n = 67, mode = 1
			output: append([]byte{0x0D, 0x01}, testString1...), bytesEncoded: 69}, // n|mode = 0x010D (BE) = 0x0D01 (LE)
		{val: testString2, // n = 16560, mode = 2
			output: append([]byte{0xC2, 0x02, 0x01, 0x00}, testString2...), bytesEncoded: 16564}, // n|mode = 0x102C2 (BE) = 0xC20201 (LE)
		{val: testString3, // n = 55, mode = 0
			output: append([]byte{0xDC}, testString3...), bytesEncoded: 56}, // n|mode = 0xDC (BE/LE)
	}

	//Append stringTests to all other tests
	encodeTests = append(encodeTests, testStrings...)
}

// Causes memory leaks with the CI's
func setUpLargeStringTests() {
	testString1 := strings.Repeat("We need a longer string to test with. Let's multiple this several times.", 14913081) // n = 72 * 14913081 = 1073741832 (> 2^30 = 1073741824)

	testStrings := []encodeTest{
		{val: testString1, // n = 1073741832, mode = 3, num_bytes_n = 4
			output: append([]byte{0x03, 0x08, 0x00, 0x00, 0x40}, testString1...), bytesEncoded: 1073741837}, // (num_bytes_n - 4)|mode|n = 0x40 00 00 08 03 (BE) = 0x03 08 00 00 40 (LE)
	}

	// Append stringTests to all other tests
	encodeTests = append(encodeTests, testStrings...)
}

func TestEncode(t *testing.T) {
	setUpStringTests()

	if testing.Short() {
		t.Logf("\033[33mSkipping memory intesive test for TestEncode in short mode\033[0m")
	} else {
		setUpLargeStringTests()
	}

	for _, test := range encodeTests {

		buffer := bytes.Buffer{}
		se := Encoder{&buffer}
		bytesEncoded, err := se.Encode(test.val)
		output := buffer.Bytes()

		if err != nil {
			t.Error(err)
		} else if !bytes.Equal(output, test.output) {
			if len(test.output) < 1<<15 {
				t.Errorf("Fail: input %x got %x expected %x", test.val, output, test.output)
			} else {
				//Only prints first 10 bytes of a failed test if output is > 2^15 bytes
				t.Errorf("Failed test with large output. First 10 bytes: got %x... expected %x...", output[0:10], test.output[0:10])
			}
		} else if bytesEncoded != test.bytesEncoded {
			t.Errorf("Fail: input %x  got %d bytes encoded expected %d", test.val, bytesEncoded, test.bytesEncoded)
		}
	}
}
