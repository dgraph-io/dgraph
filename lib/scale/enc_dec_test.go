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
	"io"
	"math/big"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"

	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeComplexStruct(t *testing.T) {
	type SimpleStruct struct {
		A int64
		B bool
	}

	type ComplexStruct struct {
		B   bool
		I   int
		I8  int8
		I16 int16
		I32 int32
		I64 int64
		U   uint
		U8  uint8
		U16 uint16
		U32 uint32
		U64 uint64
		Str string
		Bz  []byte
		Sub *SimpleStruct
	}

	test := &ComplexStruct{
		B:   true,
		I:   1,
		I8:  2,
		I16: 3,
		I32: 4,
		I64: 5,
		U:   6,
		U8:  7,
		U16: 8,
		U32: 9,
		U64: 10,
		Str: "choansafe",
		Bz:  []byte{0xDE, 0xAD, 0xBE, 0xEF},
		Sub: &SimpleStruct{
			A: 99,
			B: true,
		},
	}

	enc, err := Encode(test)
	if err != nil {
		t.Fatal(err)
	}

	res := &ComplexStruct{
		Sub: &SimpleStruct{},
	}
	output, err := Decode(enc, res)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(output.(*ComplexStruct), test) {
		t.Errorf("Fail: got %+v expected %+v", output.(*ComplexStruct), test)
	}
}

var bigIntTests = []*big.Int{
	big.NewInt(0),
	big.NewInt(1),
	big.NewInt(42),
	big.NewInt(64),
	big.NewInt(69),
	big.NewInt(16383),
	big.NewInt(16384),
	big.NewInt(1073741823),
	big.NewInt(1073741824),
	big.NewInt(1<<32 - 1),
	big.NewInt(1 << 32),
}

func TestEncodeDecodeBigInt(t *testing.T) {
	for _, test := range bigIntTests {
		enc, err := Encode(test)
		require.NoError(t, err)

		dec, err := Decode(enc, big.NewInt(0))
		require.NoError(t, err)
		require.Equal(t, test, dec)
	}

}

type testType struct {
	Data [64]byte
}

func (t *testType) Encode() ([]byte, error) {
	return Encode(t)
}

func (t *testType) Decode(r io.Reader) (*testType, error) {
	sd := Decoder{Reader: r}
	ti, err := sd.Decode(t)
	return ti.(*testType), err
}

func TestEncodeDecodeCustom_NoRecursion(t *testing.T) {
	tt := new(testType)
	tt.Data = [64]byte{1, 2, 3, 4}
	enc, err := tt.Encode()
	require.NoError(t, err)

	rw := &bytes.Buffer{}
	rw.Write(enc)
	dec, err := new(testType).Decode(rw)
	require.NoError(t, err)
	require.Equal(t, tt, dec)
}

type MyType [32]byte

func (t *MyType) Encode() ([]byte, error) {
	return t[:], nil
}

func (t *MyType) Decode(r io.Reader) (*MyType, error) {
	m, err := common.ReadHash(r)
	mt := MyType(m)
	return &mt, err
}

type testType2 struct {
	Data *MyType
}

func TestEncodeDecodeCustom_InsideStruct(t *testing.T) {
	tt := new(testType2)
	tt.Data = &MyType{1, 2, 3, 4}
	enc, err := Encode(tt)
	require.NoError(t, err)

	dec, err := Decode(enc, new(testType2))
	require.NoError(t, err)
	require.Equal(t, tt, dec)
}

type myBytes [64]byte

func TestEncodeDecodeCustom_Array(t *testing.T) {
	b := myBytes([64]byte{1, 2, 3, 4})
	enc, err := Encode(b)
	require.NoError(t, err)

	_, _ = Decode(enc, new(myBytes))
}

func TestEncodeDecode_Array(t *testing.T) {
	withCustom = false
	b := [64]byte{1, 2, 3, 4}
	enc, err := Encode(b)
	require.NoError(t, err)

	dec, err := Decode(enc, [64]byte{})
	require.NoError(t, err)
	require.Equal(t, b, dec)
}
