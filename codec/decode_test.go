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
	"reflect"
	"testing"
)

var byteArray32 = [32]byte{}
var byteArray64 = [64]byte{}

type reverseByteTest struct {
	val    []byte
	output []byte
}

type decodeBigIntTest struct {
	val    []byte
	output *big.Int
}

type decodeByteArrayTest struct {
	val    []byte
	output []byte
}

type decodeBoolTest struct {
	val    byte
	output bool
}

type decodeTupleTest struct {
	val    []byte
	t      interface{}
	output interface{}
}

type decodeArrayTest struct {
	val    []byte
	t      interface{}
	output interface{}
}

var decodeFixedWidthIntTestsInt8 = []struct {
	val    []byte
	output int8
}{
	{val: []byte{0x00}, output: int8(0)},
	{val: []byte{0x01}, output: int8(1)},
	{val: []byte{0x2a}, output: int8(42)},
	{val: []byte{0x40}, output: int8(64)},
	{val: []byte{0x45}, output: int8(69)},
}

var decodeFixedWidthIntTestsUint8 = []struct {
	val    []byte
	output uint8
}{
	{val: []byte{0x00}, output: uint8(0)},
	{val: []byte{0x01}, output: uint8(1)},
	{val: []byte{0x2a}, output: uint8(42)},
	{val: []byte{0x40}, output: uint8(64)},
	{val: []byte{0x45}, output: uint8(69)},
}

var decodeFixedWidthIntTestsInt16 = []struct {
	val    []byte
	output int16
}{
	{val: []byte{0x00}, output: int16(0)},
	{val: []byte{0x01}, output: int16(1)},
	{val: []byte{0x2a}, output: int16(42)},
	{val: []byte{0x40}, output: int16(64)},
	{val: []byte{0x45}, output: int16(69)},
	{val: []byte{0xff, 0x3f}, output: int16(16383)},
	{val: []byte{0x00, 0x40}, output: int16(16384)},
}

var decodeFixedWidthIntTestsUint16 = []struct {
	val    []byte
	output uint16
}{
	{val: []byte{0x00}, output: uint16(0)},
	{val: []byte{0x01}, output: uint16(1)},
	{val: []byte{0x2a}, output: uint16(42)},
	{val: []byte{0x40}, output: uint16(64)},
	{val: []byte{0x45}, output: uint16(69)},
	{val: []byte{0xff, 0x3f}, output: uint16(16383)},
	{val: []byte{0x00, 0x40}, output: uint16(16384)},
}

var decodeFixedWidthIntTestsInt32 = []struct {
	val    []byte
	output int32
}{
	{val: []byte{0x00}, output: int32(0)},
	{val: []byte{0x01}, output: int32(1)},
	{val: []byte{0x2a}, output: int32(42)},
	{val: []byte{0x40}, output: int32(64)},
	{val: []byte{0x45}, output: int32(69)},
	{val: []byte{0xff, 0x3f}, output: int32(16383)},
	{val: []byte{0x00, 0x40}, output: int32(16384)},
	{val: []byte{0xff, 0xff, 0xff, 0x3f}, output: int32(1073741823)},
	{val: []byte{0x00, 0x00, 0x00, 0x40}, output: int32(1073741824)},
}

var decodeFixedWidthIntTestsUint32 = []struct {
	val    []byte
	output uint32
}{
	{val: []byte{0x00}, output: uint32(0)},
	{val: []byte{0x01}, output: uint32(1)},
	{val: []byte{0x2a}, output: uint32(42)},
	{val: []byte{0x40}, output: uint32(64)},
	{val: []byte{0x45}, output: uint32(69)},
	{val: []byte{0xff, 0x3f}, output: uint32(16383)},
	{val: []byte{0x00, 0x40}, output: uint32(16384)},
	{val: []byte{0xff, 0xff, 0xff, 0x3f}, output: uint32(1073741823)},
	{val: []byte{0x00, 0x00, 0x00, 0x40}, output: uint32(1073741824)},
}

var decodeFixedWidthIntTestsInt64 = []struct {
	val    []byte
	output int64
}{
	// compact integers
	{val: []byte{0x00}, output: int64(0)},
	{val: []byte{0x04}, output: int64(4)},
	{val: []byte{0xa8}, output: int64(168)},
	{val: []byte{0x01, 0x01}, output: int64(257)},
	{val: []byte{0x15, 0x01}, output: int64(277)},
	{val: []byte{0xfd, 0xff}, output: int64(65533)},
	{val: []byte{0x02, 0x00, 0x01, 0x00}, output: int64(65538)},
	{val: []byte{0xfe, 0xff, 0xff, 0xff}, output: int64(4294967294)},
	{val: []byte{0x03, 0x00, 0x00, 0x00, 0x40}, output: int64(274877906947)},
	{val: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, output: int64(-1)},
}

var decodeFixedWidthIntTestsUint64 = []struct {
	val    []byte
	output uint64
}{
	// compact unsigned integers
	{val: []byte{0x00}, output: uint64(0)},
	{val: []byte{0x04}, output: uint64(4)},
	{val: []byte{0xa8}, output: uint64(168)},
	{val: []byte{0x01, 0x01}, output: uint64(257)},
	{val: []byte{0x15, 0x01}, output: uint64(277)},
	{val: []byte{0xfd, 0xff}, output: uint64(65533)},
	{val: []byte{0x02, 0x00, 0x01, 0x00}, output: uint64(65538)},
	{val: []byte{0xfe, 0xff, 0xff, 0xff}, output: uint64(4294967294)},
	{val: []byte{0x03, 0x00, 0x00, 0x00, 0x40}, output: uint64(274877906947)},
	{val: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, output: uint64(1<<64 - 1)},
}

var decodeBigIntTests = []decodeBigIntTest{
	// compact integers
	{val: []byte{0x00}, output: big.NewInt(0)},
	{val: []byte{0x04}, output: big.NewInt(1)},
	{val: []byte{0xa8}, output: big.NewInt(42)},
	{val: []byte{0x01, 0x01}, output: big.NewInt(64)},
	{val: []byte{0x15, 0x01}, output: big.NewInt(69)},
	{val: []byte{0xfd, 0xff}, output: big.NewInt(16383)},
	{val: []byte{0x02, 0x00, 0x01, 0x00}, output: big.NewInt(16384)},
	{val: []byte{0xfe, 0xff, 0xff, 0xff}, output: big.NewInt(1073741823)},
	{val: []byte{0x03, 0x00, 0x00, 0x00, 0x40}, output: big.NewInt(1073741824)},
	{val: []byte{0x03, 0xff, 0xff, 0xff, 0xff}, output: big.NewInt(1<<32 - 1)},
	{val: []byte{0x07, 0x00, 0x00, 0x00, 0x00, 0x01}, output: big.NewInt(1 << 32)},
}

var decodeByteArrayTests = []decodeByteArrayTest{
	// byte arrays
	{val: []byte{0x04, 0x01}, output: []byte{0x01}},
	{val: []byte{0x04, 0xff}, output: []byte{0xff}},
	{val: []byte{0x08, 0x01, 0x01}, output: []byte{0x01, 0x01}},
	{val: append([]byte{0x01, 0x01}, byteArray(64)...), output: byteArray(64)},
	{val: append([]byte{0xfd, 0xff}, byteArray(16383)...), output: byteArray(16383)},
	{val: append([]byte{0x02, 0x00, 0x01, 0x00}, byteArray(16384)...), output: byteArray(16384)},
}

// Causes memory leaks with the CI's
var largeDecodeByteArrayTests = []decodeByteArrayTest{
	{val: append([]byte{0xfe, 0xff, 0xff, 0xff}, byteArray(1073741823)...), output: byteArray(1073741823)},
	{val: append([]byte{0x03, 0x00, 0x00, 0x00, 0x40}, byteArray(1073741824)...), output: byteArray(1073741824)},
}

var decodeBoolTests = []decodeBoolTest{
	{val: 0x01, output: true},
	{val: 0x00, output: false},
}

var decodeTupleTests = []decodeTupleTest{
	{val: []byte{0x04, 0x01, 0x02}, t: &struct {
		Foo []byte
		Bar int64
	}{}, output: &struct {
		Foo []byte
		Bar int64
	}{[]byte{0x01}, 2}},

	{val: []byte{0x04, 0x01, 0xff, 0xff, 0xff, 0xff}, t: &struct {
		Foo []byte
		Bar int64
	}{}, output: &struct {
		Foo []byte
		Bar int64
	}{[]byte{0x01}, int64(1<<32 - 1)}},

	{val: append([]byte{0x04, 0x01, 0x02, 0x00, 0x01, 0x00}, byteArray(16384)...), t: &struct {
		Foo []byte
		Bar []byte
	}{}, output: &struct {
		Foo []byte
		Bar []byte
	}{[]byte{0x01}, byteArray(16384)}},

	{val: []byte{0x04, 0x01, 0xfd, 0xff, 0x07, 0x00, 0x00, 0x00, 0x00, 0x01}, t: &struct {
		Foo  []byte
		Bar  *big.Int
		Noot *big.Int
	}{}, output: &struct {
		Foo  []byte
		Bar  *big.Int
		Noot *big.Int
	}{[]byte{0x01}, big.NewInt(16383), big.NewInt(int64(1 << 32))}},

	{val: []byte{0x04, 0x01, 0xfd, 0xff, 0x01, 0x07, 0x00, 0x00, 0x00, 0x00, 0x01}, t: &struct {
		Foo  []byte
		Bar  *big.Int
		Bo   bool
		Noot *big.Int
	}{}, output: &struct {
		Foo  []byte
		Bar  *big.Int
		Bo   bool
		Noot *big.Int
	}{[]byte{0x01}, big.NewInt(16383), true, big.NewInt(int64(1 << 32))}},

	{val: []byte{0x04, 0x01, 0x04, 0x00}, t: &struct {
		Foo []byte
		Bar int8
		Bo  bool
	}{}, output: &struct {
		Foo []byte
		Bar int8
		Bo  bool
	}{[]byte{0x01}, 4, false}},

	{val: []byte{0x04, 0x01, 0x05, 0x04, 0x00}, t: &struct {
		Foo []byte
		Bar int16
		Bo  bool
	}{}, output: &struct {
		Foo []byte
		Bar int16
		Bo  bool
	}{[]byte{0x01}, 1029, false}},

	{val: []byte{0x04, 0x01, 0x05, 0x04, 0x00}, t: &struct {
		Foo []byte
		Bar uint16
		Bo  bool
	}{}, output: &struct {
		Foo []byte
		Bar uint16
		Bo  bool
	}{[]byte{0x01}, 1029, false}},

	{val: []byte{0x04, 0x01, 0x02, 0xff, 0xff, 0x01, 0x00}, t: &struct {
		Foo []byte
		Bar int32
		Bo  bool
	}{}, output: &struct {
		Foo []byte
		Bar int32
		Bo  bool
	}{[]byte{0x01}, 33554178, false}},
}

var decodeArrayTests = []decodeArrayTest{
	{val: []byte{}, t: [][]byte{}, output: [][]byte{}},
	{val: []byte{0x00}, t: []int{}, output: []int{}},
	{val: []byte{0x04, 0x04}, t: make([]int, 1), output: []int{1}},
	{val: []byte{0x10, 0x04, 0x08, 0x0c, 0x10}, t: make([]int, 4), output: []int{1, 2, 3, 4}},
	{val: []byte{0x10, 0x02, 0x00, 0x01, 0x00, 0x08, 0x0c, 0x10}, t: make([]int, 4), output: []int{16384, 2, 3, 4}},
	{val: []byte{0x10, 0x03, 0x00, 0x00, 0x00, 0x40, 0x08, 0x0c, 0x10}, t: make([]int, 4), output: []int{1073741824, 2, 3, 4}},
	{val: []byte{0x10, 0x07, 0x00, 0x00, 0x00, 0x00, 0x01, 0x08, 0x0c, 0x07, 0x00, 0x00, 0x00, 0x00, 0x01}, t: make([]int, 4), output: []int{1 << 32, 2, 3, 1 << 32}},
	{val: []byte{0x00}, t: []bool{}, output: []bool{}},
	{val: []byte{0x0c, 0x01, 0x00, 0x01}, t: make([]bool, 3), output: []bool{true, false, true}},
	{val: []byte{0x08, 0x00, 0x04}, t: make([]*big.Int, 2), output: []*big.Int{big.NewInt(0), big.NewInt(1)}},
	{val: append([]byte{0x8}, byteArray64[:]...), t: [][32]byte{{}, {}}, output: [][32]byte{byteArray32, byteArray32}},
	{val: []byte{0x4, 0x04, 0x01}, t: [][]byte{{}}, output: [][]byte{{0x01}}},
}

var reverseByteTests = []reverseByteTest{
	{val: []byte{0x00, 0x01, 0x02}, output: []byte{0x02, 0x01, 0x00}},
	{val: []byte{0x04, 0x05, 0x06, 0x07}, output: []byte{0x07, 0x06, 0x05, 0x04}},
	{val: []byte{0xff}, output: []byte{0xff}},
}

func TestReverseBytes(t *testing.T) {
	for _, test := range reverseByteTests {
		output := reverseBytes(test.val)
		if !bytes.Equal(output, test.output) {
			t.Errorf("Fail: got %d expected %d", output, test.output)
		}
	}
}

func TestReadByte(t *testing.T) {
	buf := bytes.Buffer{}
	sd := Decoder{&buf}
	buf.Write([]byte{0xff})
	output, err := sd.ReadByte()
	if err != nil {
		t.Error(err)
	} else if output != 0xff {
		t.Errorf("Fail: got %x expected %x", output, 0xff)
	}
}

func TestDecodeFixedWidthInts(t *testing.T) {
	for _, test := range decodeFixedWidthIntTestsInt8 {
		output, err := Decode(test.val, int8(0))
		if err != nil {
			t.Error(err)
		} else if output.(int8) != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, output, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsUint8 {
		output, err := Decode(test.val, uint8(0))
		if err != nil {
			t.Error(err)
		} else if output.(uint8) != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, output, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsInt16 {
		output, err := Decode(test.val, int16(0))
		if err != nil {
			t.Error(err)
		} else if output.(int16) != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, output, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsUint16 {
		output, err := Decode(test.val, uint16(0))
		if err != nil {
			t.Error(err)
		} else if output.(uint16) != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, output, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsInt32 {
		output, err := Decode(test.val, int32(0))
		if err != nil {
			t.Error(err)
		} else if output.(int32) != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, output, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsUint32 {
		output, err := Decode(test.val, uint32(0))
		if err != nil {
			t.Error(err)
		} else if output.(uint32) != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, output, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsInt64 {
		output, err := Decode(test.val, int64(0))
		if err != nil {
			t.Error(err)
		} else if output.(int64) != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, output, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsUint64 {
		output, err := Decode(test.val, uint64(0))
		if err != nil {
			t.Error(err)
		} else if output.(uint64) != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, output, test.output)
		}
	}
}

func TestDecodeBigInts(t *testing.T) {
	for _, test := range decodeBigIntTests {
		output, err := Decode(test.val, big.NewInt(0))
		if err != nil {
			t.Error(err)
		} else if output.(*big.Int).Cmp(test.output) != 0 {
			t.Errorf("Fail: got %s expected %s", output.(*big.Int).String(), test.output.String())
		}
	}
}

func TestLargeDecodeByteArrays(t *testing.T) {
	if testing.Short() {
		t.Skip("\033[33mSkipping memory intesive test for TestDecodeByteArrays in short mode\033[0m")
	} else {
		for _, test := range largeDecodeByteArrayTests {
			output, err := Decode(test.val, []byte{})
			if err != nil {
				t.Error(err)
			} else if !bytes.Equal(output.([]byte), test.output) {
				t.Errorf("Fail: got %d expected %d", len(output.([]byte)), len(test.output))
			}
		}
	}
}

func TestDecodeByteArrays(t *testing.T) {
	for _, test := range decodeByteArrayTests {
		output, err := Decode(test.val, []byte{})
		if err != nil {
			t.Error(err)
		} else if !bytes.Equal(output.([]byte), test.output) {
			t.Errorf("Fail: got %d expected %d", len(output.([]byte)), len(test.output))
		}
	}
}

func TestDecodeBool(t *testing.T) {
	for _, test := range decodeBoolTests {
		output, err := Decode([]byte{test.val}, true)
		if err != nil {
			t.Error(err)
		} else if output != test.output {
			t.Errorf("Fail: got %t expected %t", output, test.output)
		}
	}

	output, err := Decode([]byte{0xff}, true)
	if err == nil {
		t.Error("did not error for invalid bool")
	} else if output.(bool) {
		t.Errorf("Fail: got %t expected false", output)
	}
}

func TestDecodeTuples(t *testing.T) {
	for _, test := range decodeTupleTests {
		output, err := Decode(test.val, test.t)
		if err != nil {
			t.Error(err)
		} else if !reflect.DeepEqual(output, test.output) {
			t.Errorf("Fail: got %d expected %d", output, test.output)
		}
	}
}

func TestDecodeArrays(t *testing.T) {
	for _, test := range decodeArrayTests {
		output, err := Decode(test.val, test.t)
		if err != nil {
			t.Error(err)
		} else if !reflect.DeepEqual(output, test.output) {
			t.Errorf("Fail: got %d expected %d", output, test.output)
		}
	}
}
