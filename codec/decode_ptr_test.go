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
package codec

import (
	"bytes"
	"math/big"
	"reflect"
	"testing"
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
