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

package scale

import (
	"bytes"
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"

	"github.com/stretchr/testify/require"
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

	{val: int64(1), output: []byte{0x01, 0, 0, 0, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: int64(16383), output: []byte{0xff, 0x3f, 0, 0, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: int64(1073741823), output: []byte{0xff, 0xff, 0xff, 0x3f, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: int64(9223372036854775807), output: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}, bytesEncoded: 8},

	{val: uint64(1), output: []byte{0x01, 0, 0, 0, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: uint64(16383), output: []byte{0xff, 0x3f, 0, 0, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: uint64(1073741823), output: []byte{0xff, 0xff, 0xff, 0x3f, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: uint64(9223372036854775807), output: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}, bytesEncoded: 8},

	{val: int(1), output: []byte{0x01, 0, 0, 0, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: int(16383), output: []byte{0xff, 0x3f, 0, 0, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: int(1073741823), output: []byte{0xff, 0xff, 0xff, 0x3f, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: int(9223372036854775807), output: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}, bytesEncoded: 8},

	{val: uint(1), output: []byte{0x01, 0, 0, 0, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: uint(16383), output: []byte{0xff, 0x3f, 0, 0, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: uint(1073741823), output: []byte{0xff, 0xff, 0xff, 0x3f, 0, 0, 0, 0}, bytesEncoded: 8},
	{val: uint(9223372036854775807), output: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}, bytesEncoded: 8},

	// compact integers
	{val: big.NewInt(0), output: []byte{0x00}, bytesEncoded: 1},
	{val: big.NewInt(1), output: []byte{0x04}, bytesEncoded: 1},
	{val: big.NewInt(42), output: []byte{0xa8}, bytesEncoded: 1},
	{val: big.NewInt(69), output: []byte{0x15, 0x01}, bytesEncoded: 2},
	{val: big.NewInt(1000), output: []byte{0xa1, 0x0f}, bytesEncoded: 2},
	{val: big.NewInt(16383), output: []byte{0xfd, 0xff}, bytesEncoded: 2},
	{val: big.NewInt(16384), output: []byte{0x02, 0x00, 0x01, 0x00}, bytesEncoded: 4},
	{val: big.NewInt(1073741823), output: []byte{0xfe, 0xff, 0xff, 0xff}, bytesEncoded: 4},
	{val: big.NewInt(1073741824), output: []byte{3, 0, 0, 0, 64}, bytesEncoded: 5},
	{val: big.NewInt(1<<32 - 1), output: []byte{0x03, 0xff, 0xff, 0xff, 0xff}, bytesEncoded: 5},

	// byte arrays
	{val: []byte{0x01}, output: []byte{0x04, 0x01}, bytesEncoded: 2},
	{val: []byte{0xff}, output: []byte{0x04, 0xff}, bytesEncoded: 2},
	{val: []byte{0x01, 0x01}, output: []byte{0x08, 0x01, 0x01}, bytesEncoded: 3},
	{val: []byte{0x01, 0x01}, output: []byte{0x08, 0x01, 0x01}, bytesEncoded: 3},
	{val: byteArray(32), output: append([]byte{0x80}, byteArray(32)...), bytesEncoded: 33},
	{val: byteArray(64), output: append([]byte{0x01, 0x01}, byteArray(64)...), bytesEncoded: 66},
	{val: byteArray(16384), output: append([]byte{0x02, 0x00, 0x01, 0x00}, byteArray(16384)...), bytesEncoded: 16388},

	// common.Hash
	{val: common.BytesToHash([]byte{0xff}), output: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff}, bytesEncoded: 32},
	{val: common.Hash{0xff}, output: []byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, bytesEncoded: 32},

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
	{val: [][]byte{{0x00, 0x01}, {0x01, 0x00}}, output: []byte{0x08, 0x08, 0x00, 0x01, 0x08, 0x01, 0x00}, bytesEncoded: 7},
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
		se := Encoder{Writer: &buffer}
		bytesEncoded, err := se.Encode(test.val)
		output := buffer.Bytes()

		if err != nil {
			t.Error(err)
		} else if !bytes.Equal(output, test.output) {
			if len(test.output) < 1<<15 {
				t.Errorf("Fail: input %v got %v expected %v", test.val, output, test.output)
			} else {
				//Only prints first 10 bytes of a failed test if output is > 2^15 bytes
				t.Errorf("Failed test with large output. First 10 bytes: got %x... expected %x...", output[0:10], test.output[0:10])
			}
		} else if bytesEncoded != test.bytesEncoded {
			t.Errorf("Fail: input %x  got %d bytes encoded expected %d", test.val, bytesEncoded, test.bytesEncoded)
		}
	}
}

func TestEncodeAndDecodeStringInStruct(t *testing.T) {
	test := &struct {
		A string
	}{
		A: "noot",
	}

	enc, err := Encode(test)
	if err != nil {
		t.Fatal(err)
	}

	dec, err := Decode(enc, &struct{ A string }{A: ""})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(test, dec) {
		t.Fatalf("Fail: got %v expected %v", dec, test)
	}
}

func TestEncodeAndDecodeStringArrayInStruct(t *testing.T) {
	test := &struct {
		A []string
	}{
		A: []string{"noot", "noot2"},
	}

	enc, err := Encode(test)
	require.Nil(t, err)
	require.NotEqual(t, 0, len(enc), "Failed to encode StringArrayInStruct")

	var result = &struct{ A []string }{}

	err = DecodePtr(enc, result)
	require.Nil(t, err)
	require.Equal(t, test, result, "Decoding failed")
}

// test type for encoding
type MockTypeA struct {
	A string
}

// Encode func for TypeReal that uses actual Scale Encode
func (tr *MockTypeA) Encode() ([]byte, error) {
	return Encode(tr)
}

// test to confirm EncodeCustom is return Scale Encoded result
func TestEncodeCustomMockTypeA(t *testing.T) {
	test := &MockTypeA{A: "hello"}

	encCust, err := EncodeCustom(test)
	require.Nil(t, err)

	encScale, err := Encode(test)
	require.Nil(t, err)

	require.Equal(t, encScale, encCust)
}

// test type for encoding, this type does not have Encode func
type MockTypeB struct {
	A string
}

// test to confirm EncodeCustom is return Scale Encoded result
func TestEncodeCustomMockTypeB(t *testing.T) {
	test := &MockTypeB{A: "hello"}

	encCust, err := EncodeCustom(test)
	require.Nil(t, err)

	encScale, err := Encode(test)
	require.Nil(t, err)

	require.Equal(t, encScale, encCust)
}

// test types for encoding
type MockTypeC struct {
	A string
}

// Encode func for MockTypeC that return fake byte array [1, 2, 3]
func (tr *MockTypeC) Encode() ([]byte, error) {
	return []byte{1, 2, 3}, nil
}

// test to confirm EncodeCustom is using type's Encode function
func TestEncodeCustomMockTypeC(t *testing.T) {
	test := &MockTypeC{A: "hello"}
	expected := []byte{1, 2, 3}

	encCust, err := EncodeCustom(test)
	require.Nil(t, err)

	require.Equal(t, expected, encCust)
}

// test types for encoding
type MockTypeD struct {
	A string
}

// Encode func for MockTypeD that return an error
func (tr *MockTypeD) Encode() ([]byte, error) {
	return nil, errors.New("error encoding")
}

// test to confirm EncodeCustom is handling errors
func TestEncodeCustomMockTypeD(t *testing.T) {
	test := &MockTypeD{A: "hello"}

	_, err := EncodeCustom(test)
	require.EqualError(t, err, "error encoding")
}
