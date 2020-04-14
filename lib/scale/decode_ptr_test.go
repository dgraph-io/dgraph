// Copyright 2020 ChainSafe Systems (ON) Corp.
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
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"

	"github.com/stretchr/testify/require"
)

func TestDecodePtrFixedWidthInts(t *testing.T) {
	for _, test := range decodeFixedWidthIntTestsInt8 {
		var res int8
		err := DecodePtr(test.val, &res)
		if err != nil {
			t.Error(err)
		} else if res != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, res, test.output)
		}

	}

	for _, test := range decodeFixedWidthIntTestsUint8 {
		var res uint8
		err := DecodePtr(test.val, &res)
		if err != nil {
			t.Error(err)
		} else if res != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, res, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsInt16 {
		var res int16
		err := DecodePtr(test.val, &res)
		if err != nil {
			t.Error(err)
		} else if res != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, res, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsUint16 {
		var res uint16
		err := DecodePtr(test.val, &res)
		if err != nil {
			t.Error(err)
		} else if res != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, res, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsInt32 {
		var res int32
		err := DecodePtr(test.val, &res)
		if err != nil {
			t.Error(err)
		} else if res != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, res, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsUint32 {
		var res uint32
		err := DecodePtr(test.val, &res)
		if err != nil {
			t.Error(err)
		} else if res != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, res, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsInt64 {
		var res int64
		err := DecodePtr(test.val, &res)
		if err != nil {
			t.Error(err)
		} else if res != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, res, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsUint64 {
		var res uint64
		err := DecodePtr(test.val, &res)
		if err != nil {
			t.Error(err)
		} else if res != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, res, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsInt {
		var res int
		err := DecodePtr(test.val, &res)
		if err != nil {
			t.Error(err)
		} else if res != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, res, test.output)
		}
	}

	for _, test := range decodeFixedWidthIntTestsUint {
		var res uint
		err := DecodePtr(test.val, &res)
		if err != nil {
			t.Error(err)
		} else if res != test.output {
			t.Errorf("Fail: input %d got %d expected %d", test.val, res, test.output)
		}
	}
}

func TestDecodePtrBigInts(t *testing.T) {
	for _, test := range decodeBigIntTests {
		res := big.NewInt(0)
		err := DecodePtr(test.val, res)
		if err != nil {
			t.Error(err)
		} else if res.Cmp(test.output) != 0 {
			t.Errorf("Fail: got %s expected %s", res.String(), test.output.String())
		}
	}
}

func TestLargeDecodePtrByteArrays(t *testing.T) {
	if testing.Short() {
		t.Skip("\033[33mSkipping memory intesive test for TestDecodePtrByteArrays in short mode\033[0m")
	} else {
		// Causes memory leaks with the CI's
		var largeDecodeByteArrayTests = []decodeByteArrayTest{
			{val: append([]byte{0xfe, 0xff, 0xff, 0xff}, byteArray(1073741823)...), output: byteArray(1073741823)},
			{val: append([]byte{0x03, 0x00, 0x00, 0x00, 0x40}, byteArray(1073741824)...), output: byteArray(1073741824)},
		}

		for _, test := range largeDecodeByteArrayTests {
			var result = make([]byte, len(test.output))
			err := DecodePtr(test.val, result)
			if err != nil {
				t.Error(err)
			} else if !bytes.Equal(result, test.output) {
				t.Errorf("Fail: got %d expected %d", len(result), len(test.output))
			}
		}
	}
}

func TestDecodePtrByteArrays(t *testing.T) {
	for _, test := range decodeByteArrayTests {
		var result = make([]byte, len(test.output))
		err := DecodePtr(test.val, result)
		if err != nil {
			t.Error(err)
		} else if !bytes.Equal(result, test.output) {
			t.Errorf("Fail: got %d expected %d", len(result), len(test.output))
		}
	}
}

func TestDecodePtrBool(t *testing.T) {
	for _, test := range decodeBoolTests {
		var result bool
		err := DecodePtr([]byte{test.val}, &result)
		if err != nil {
			t.Error(err)
		} else if result != test.output {
			t.Errorf("Fail: got %t expected %t", result, test.output)
		}
	}

	var result bool = true
	err := DecodePtr([]byte{0xff}, &result)
	if err == nil {
		t.Error("did not error for invalid bool")
	} else if result {
		t.Errorf("Fail: got %t expected false", result)
	}
}

func TestDecodePtrTuples(t *testing.T) {
	for _, test := range decodeTupleTests {
		err := DecodePtr(test.val, test.t)
		if err != nil {
			t.Error(err)
		} else if !reflect.DeepEqual(test.t, test.output) {
			t.Errorf("Fail: got %d expected %d", test.val, test.output)
		}
	}
}

func TestDecodePtrArrays(t *testing.T) {
	for _, test := range decodeArrayTests {
		err := DecodePtr(test.val, test.t)
		if err != nil {
			t.Error(err)
		} else if !reflect.DeepEqual(test.t, test.output) {
			t.Errorf("Fail: got %d expected %d", test.t, test.output)
		}
	}
}

func TestDecodePtr_DecodeCommonHash(t *testing.T) {
	in := []byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	decodedHash := common.NewHash([]byte{})
	expectedHash := common.NewHash([]byte{0xff})
	err := DecodePtr(in, &decodedHash)
	require.Nil(t, err)
	require.Equal(t, expectedHash, decodedHash)
}

// add Decode func to MockTypeA
func (tr *MockTypeA) Decode(in []byte) error {
	return DecodePtr(in, tr)
}

// test decoding for MockTypeA (which has Decode func)
func TestDecodeCustom_DecodeMockTypeA(t *testing.T) {
	expected := &MockTypeA{A: "hello"}
	encoded := []byte{20, 104, 101, 108, 108, 111}
	mockType := new(MockTypeA)

	err := DecodeCustom(encoded, mockType)
	require.Nil(t, err)
	require.Equal(t, expected, mockType)
}

// test decoding for MockTypeB (which does not have Decode func)
func TestDecodeCustom_DecodeMockTypeB(t *testing.T) {
	expected := &MockTypeB{A: "hello"}
	encoded := []byte{20, 104, 101, 108, 108, 111}
	mockType := new(MockTypeB)

	err := DecodeCustom(encoded, mockType)
	require.Nil(t, err)
	require.Equal(t, expected, mockType)
}

// add Decode func to MockTypeC which will return fake data (so we'll know when it was called)
func (tr *MockTypeC) Decode(in []byte) error {
	tr.A = "goodbye"
	return nil
}

// test decoding for MockTypeC (which has Decode func that returns fake data (A: "goodbye"))
func TestDecodeCustom_DecodeMockTypeC(t *testing.T) {
	expected := &MockTypeC{A: "goodbye"}
	encoded := []byte{20, 104, 101, 108, 108, 111}
	mockType := new(MockTypeC)

	err := DecodeCustom(encoded, mockType)
	require.Nil(t, err)
	require.Equal(t, expected, mockType)
}

// add Decode func to MockTypeD which will return an error
func (tr *MockTypeD) Decode(in []byte) error {
	return errors.New("error decoding")
}

// test decoding for MockTypeD (which has Decode func that returns error)
func TestDecodeCustom_DecodeMockTypeD(t *testing.T) {
	encoded := []byte{20, 104, 101, 108, 108, 111}
	mockType := new(MockTypeD)

	err := DecodeCustom(encoded, mockType)
	require.EqualError(t, err, "error decoding")
}
