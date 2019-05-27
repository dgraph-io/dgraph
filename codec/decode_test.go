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

type reverseByteTest struct {
	val    []byte
	output []byte
}

type decodeFixedWidthIntTest struct {
	val    []byte
	output int32
}

type decodeIntTest struct {
	val    []byte
	output int64
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

var decodeFixedWidthIntTests = []decodeFixedWidthIntTest{
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

var decodeIntTests = []decodeIntTest{
	// compact integers
	{val: []byte{0x00}, output: int64(0)},
	{val: []byte{0x04}, output: int64(1)},
	{val: []byte{0xa8}, output: int64(42)},
	{val: []byte{0x01, 0x01}, output: int64(64)},
	{val: []byte{0x15, 0x01}, output: int64(69)},
	{val: []byte{0xfd, 0xff}, output: int64(16383)},
	{val: []byte{0x02, 0x00, 0x01, 0x00}, output: int64(16384)},
	{val: []byte{0xfe, 0xff, 0xff, 0xff}, output: int64(1073741823)},
	{val: []byte{0x03, 0x00, 0x00, 0x00, 0x40}, output: int64(1073741824)},
	{val: []byte{0x03, 0xff, 0xff, 0xff, 0xff}, output: int64(1<<32 - 1)},
	{val: []byte{0x07, 0x00, 0x00, 0x00, 0x00, 0x01}, output: int64(1 << 32)},
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
	{val: []byte{0x04, 0x01, 0x08}, t: &struct {
		Foo []byte
		Bar int64
	}{}, output: &struct {
		Foo []byte
		Bar int64
	}{[]byte{0x01}, 2}},

	{val: []byte{0x04, 0x01, 0x03, 0xff, 0xff, 0xff, 0xff}, t: &struct {
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
		Bar  int64
		Noot int64
	}{}, output: &struct {
		Foo  []byte
		Bar  int64
		Noot int64
	}{[]byte{0x01}, 16383, int64(1 << 32)}},

	{val: []byte{0x04, 0x01, 0xfd, 0xff, 0x01, 0x07, 0x00, 0x00, 0x00, 0x00, 0x01}, t: &struct {
		Foo  []byte
		Bar  int64
		Bo   bool
		Noot int64
	}{}, output: &struct {
		Foo  []byte
		Bar  int64
		Bo   bool
		Noot int64
	}{[]byte{0x01}, 16383, true, int64(1 << 32)}},

	{val: []byte{0x04, 0x01, 0x04, 0x00}, t: &struct {
		Foo  []byte
		Bar  int8
		Bo   bool
	}{}, output: &struct {
		Foo  []byte
		Bar  int8
		Bo   bool
	}{[]byte{0x01}, 4, false}},

	{val: []byte{0x04, 0x01, 0x05, 0x04, 0x00}, t: &struct {
		Foo  []byte
		Bar  int16
		Bo   bool
	}{}, output: &struct {
		Foo  []byte
		Bar  int16
		Bo   bool
	}{[]byte{0x01}, 1029, false}},

	{val: []byte{0x04, 0x01, 0x02, 0xff, 0xff, 0x01, 0x00}, t: &struct {
		Foo  []byte
		Bar  int32
		Bo   bool
	}{}, output: &struct {
		Foo  []byte
		Bar  int32
		Bo   bool
	}{[]byte{0x01}, 33554178, false}},
}

var decodeArrayTests = []decodeArrayTest{
	{val: []byte{0x00}, t: []int{}, output: []int{}},
	{val: []byte{0x04, 0x04}, t: []int{}, output: []int{1}},
	{val: []byte{0x10, 0x04, 0x08, 0x0c, 0x10}, t: []int{}, output: []int{1, 2, 3, 4}},
	{val: []byte{0x10, 0x02, 0x00, 0x01, 0x00, 0x08, 0x0c, 0x10}, t: []int{}, output: []int{16384, 2, 3, 4}},
	{val: []byte{0x10, 0x03, 0x00, 0x00, 0x00, 0x40, 0x08, 0x0c, 0x10}, t: []int{}, output: []int{1073741824, 2, 3, 4}},
	{val: []byte{0x10, 0x07, 0x00, 0x00, 0x00, 0x00, 0x01, 0x08, 0x0c, 0x07, 0x00, 0x00, 0x00, 0x00, 0x01}, t: []int{}, output: []int{1 << 32, 2, 3, 1 << 32}},
	{val: []byte{0x00}, t: []bool{}, output: []bool{}},
	{val: []byte{0x0c, 0x01, 0x00, 0x01}, t: []bool{}, output: []bool{true, false, true}},
	{val: []byte{0x08, 0x00, 0x04}, t: []*big.Int{}, output: []*big.Int{big.NewInt(0), big.NewInt(1)}},
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
	for _, test := range decodeFixedWidthIntTests {
		buf := bytes.Buffer{}
		sd := Decoder{&buf}
		buf.Write(test.val)
		var i int32
		output, err := sd.Decode(i)
		if err != nil {
			t.Error(err)
		} else if output.(int) != int(test.output) {
			t.Errorf("Fail: input %d got %d expected %d", test.val, output, test.output)
		}
	}
}

func TestDecodeInts(t *testing.T) {
	for _, test := range decodeIntTests {
		buf := bytes.Buffer{}
		sd := Decoder{&buf}
		buf.Write(test.val)
		var i int64
		output, err := sd.Decode(i)
		if err != nil {
			t.Error(err)
		} else if output != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, output, test.output)
		}
	}
}

func TestDecodeBigInts(t *testing.T) {
	for _, test := range decodeBigIntTests {
		buf := bytes.Buffer{}
		sd := Decoder{&buf}
		buf.Write(test.val)
		var tmp *big.Int
		output, err := sd.Decode(tmp)
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
			buf := bytes.Buffer{}
			sd := Decoder{&buf}
			buf.Write(test.val)
			var tmp []byte
			output, err := sd.Decode(tmp)
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
		buf := bytes.Buffer{}
		sd := Decoder{&buf}
		buf.Write(test.val)
		var tmp []byte
		output, err := sd.Decode(tmp)
		if err != nil {
			t.Error(err)
		} else if !bytes.Equal(output.([]byte), test.output) {
			t.Errorf("Fail: got %d expected %d", len(output.([]byte)), len(test.output))
		}
	}
}

func TestDecodeBool(t *testing.T) {
	for _, test := range decodeBoolTests {
		buf := bytes.Buffer{}
		sd := Decoder{&buf}
		buf.Write([]byte{test.val})
		var tmp bool
		output, err := sd.Decode(tmp)
		if err != nil {
			t.Error(err)
		} else if output != test.output {
			t.Errorf("Fail: got %t expected %t", output, test.output)
		}
	}

	buf := bytes.Buffer{}
	sd := Decoder{&buf}
	buf.Write([]byte{0xff})
	output, err := sd.DecodeBool()
	if err == nil {
		t.Error("did not error for invalid bool")
	} else if output {
		t.Errorf("Fail: got %t expected false", output)
	}
}

func TestDecodeTuples(t *testing.T) {
	for _, test := range decodeTupleTests {
		buf := bytes.Buffer{}
		buf.Write(test.val)
		sd := Decoder{&buf}
		output, err := sd.Decode(test.t)
		if err != nil {
			t.Error(err)
		} else if !reflect.DeepEqual(output, test.output) {
			t.Errorf("Fail: got %d expected %d", output, test.output)
		}
	}
}

func TestDecodeArrays(t *testing.T) {
	for _, test := range decodeArrayTests {
		buf := bytes.Buffer{}
		buf.Write(test.val)
		sd := Decoder{&buf}
		output, err := sd.Decode(test.t)
		if err != nil {
			t.Error(err)
		} else if !reflect.DeepEqual(output, test.output) {
			t.Errorf("Fail: got %d expected %d", output, test.output)
		}
	}
}
