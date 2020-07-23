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
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"github.com/ChainSafe/gossamer/lib/common"
)

// DecodeCustom check if interface has method Decode, if so use that, otherwise use regular scale decoding
func DecodeCustom(in []byte, t interface{}) error {
	someType := reflect.TypeOf(t)
	_, ok := someType.MethodByName("Decode")
	if ok {
		method := reflect.ValueOf(t).MethodByName("Decode")
		inVal := []reflect.Value{reflect.ValueOf(in)}
		res := method.Call(inVal)
		err := res[0].Interface()
		if err != nil {
			return err.(error)
		}
		return nil
	}
	return DecodePtr(in, t)
}

// DecodePtr a byte array into a interface pointer
func DecodePtr(in []byte, t interface{}) error {
	buf := &bytes.Buffer{}
	sd := Decoder{Reader: buf}
	_, err := buf.Write(in)
	if err != nil {
		return err
	}

	err = sd.DecodePtr(t)
	return err
}

// DecodeCustom check if interface has method Decode(io.Reader), if so use that, otherwise use regular scale decoding
func (sd *Decoder) DecodeCustom(t interface{}) (interface{}, error) {
	someType := reflect.TypeOf(t)
	val := reflect.ValueOf(t)
	if val.IsNil() {
		n := reflect.New(someType.Elem())
		t = n.Interface()
	}
	_, ok := someType.MethodByName("Decode")
	if ok {
		meth := reflect.ValueOf(t).MethodByName("Decode")
		inVal := []reflect.Value{reflect.ValueOf(sd.Reader)}
		res := meth.Call(inVal)
		if len(res) > 1 {
			err := res[1].Interface()
			if err != nil {
				return nil, err.(error)
			}
		}
		t = res[0].Interface()
		return t, nil
	}

	return nil, fmt.Errorf("cannot decode custom type %s", someType.String())
}

// DecodePtr is the high level function wrapping the specific type decoding functions
// The results of decode are written to t interface by reference (instead of returning
//  value as Decode does)
func (sd *Decoder) DecodePtr(t interface{}) (err error) {
	switch t := t.(type) {
	case *big.Int:
		err = sd.DecodePtrBigInt(t)
	case *int8, *uint8, *int16, *uint16, *int32, *uint32, *int64, *uint64, *int, *uint:
		err = sd.DecodePtrFixedWidthInt(t)
	case []byte, string:
		err = sd.DecodePtrByteArray(t)
	case *bool:
		err = sd.DecodePtrBool(t)
	case []int:
		err = sd.DecodePtrIntArray(t)
	case []bool:
		err = sd.DecodePtrBoolArray(t)
	case []*big.Int:
		err = sd.DecodePtrBigIntArray(t)
	case *common.Hash:
		b := make([]byte, 32)
		_, err = sd.Reader.Read(b)
		*t = common.NewHash(b)
	case [][32]byte, [][]byte:
		_, err = sd.DecodeSlice(t)
	case interface{}:
		_, err = sd.DecodeInterface(t)
	default:
		return errors.New("decode error: unsupported type")
	}
	return err
}

// DecodePtrFixedWidthInt decodes integers < 2**32 by reading the bytes in little endian
//  and writes results by reference t
func (sd *Decoder) DecodePtrFixedWidthInt(t interface{}) (err error) {
	switch t := t.(type) {
	case *int8:
		var b byte
		b, err = sd.ReadByte()
		*t = int8(b)
	case *uint8:
		var b byte
		b, err = sd.ReadByte()
		*t = b
	case *int16:
		buf := make([]byte, 2)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			*t = int16(binary.LittleEndian.Uint16(buf))
		}
	case *uint16:
		buf := make([]byte, 2)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			*t = binary.LittleEndian.Uint16(buf)
		}
	case *int32:
		buf := make([]byte, 4)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			*t = int32(binary.LittleEndian.Uint32(buf))
		}
	case *uint32:
		buf := make([]byte, 4)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			*t = binary.LittleEndian.Uint32(buf)
		}
	case *int64:
		buf := make([]byte, 8)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			*t = int64(binary.LittleEndian.Uint64(buf))
		}
	case *uint64:
		buf := make([]byte, 8)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			*t = binary.LittleEndian.Uint64(buf)
		}
	case *int:
		buf := make([]byte, 8)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			*t = int(binary.LittleEndian.Uint64(buf))
		}
	case *uint:
		buf := make([]byte, 8)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			*t = uint(binary.LittleEndian.Uint64(buf))
		}
	default:
		return fmt.Errorf("unexpected type: %s", reflect.TypeOf(t))
	}

	return err
}

// DecodePtrByteArray accepts a byte array representing a SCALE encoded byte array and performs SCALE decoding
// of the byte array
func (sd *Decoder) DecodePtrByteArray(output interface{}) error {
	_, err := sd.DecodeInteger()
	if err != nil {
		return err
	}

	_, err = sd.Reader.Read(output.([]byte))
	if err != nil {
		return errors.New("could not decode invalid byte array: reached early EOF")
	}

	return nil
}

// DecodePtrBigInt decodes a SCALE encoded byte array into a *big.Int
//  Changes the value of output to decoded value
// Works for all integers, including ints > 2**64
func (sd *Decoder) DecodePtrBigInt(output *big.Int) (err error) {
	b, err := sd.ReadByte()
	if err != nil {
		return err
	}

	// check mode of encoding, stored at 2 least significant bits
	mode := b & 0x03
	if mode <= 2 {
		var tmp int64
		tmp, err = sd.decodeSmallInt(b, mode)
		output.SetInt64(tmp)
		if err != nil {
			return err
		}
		return nil
	}

	// >4 byte mode
	topSixBits := b >> 2
	byteLen := uint(topSixBits) + 4

	buf := make([]byte, byteLen)
	_, err = sd.Reader.Read(buf)
	if err == nil {
		o := reverseBytes(buf)
		output.SetBytes(o)
	} else {
		err = errors.New("could not decode invalid big.Int: reached early EOF")
	}

	return err
}

// DecodePtrBool accepts a byte array representing a SCALE encoded bool and performs SCALE decoding
// of the bool then writes the result to output via reference. if invalid, false and an error
func (sd *Decoder) DecodePtrBool(output interface{}) error {
	b, err := sd.ReadByte()
	if err != nil {
		return err
	}

	if b == 1 {
		*output.(*bool) = true
		return nil
	} else if b == 0 {
		*output.(*bool) = false
		return nil
	}

	// if we got here, something went wrong, so set result to false
	*output.(*bool) = false
	return errors.New("cannot decode invalid boolean")
}

// DecodePtrIntArray decodes a byte array to an array of ints
func (sd *Decoder) DecodePtrIntArray(t interface{}) error {
	length, err := sd.DecodeInteger()
	if err != nil {
		return err
	}

	sl := make([]int, length)
	for i := range sl {
		temp, err := sd.DecodeInteger()
		sl[i] = int(temp)
		if err != nil {
			break
		}
	}

	copy(t.([]int), sl)
	return nil
}

// DecodePtrBigIntArray decodes a byte array to an array of *big.Ints
//  writes value to output by reference
func (sd *Decoder) DecodePtrBigIntArray(output interface{}) error {
	_, err := sd.DecodeInteger()
	if err != nil {
		return err
	}

	for i := range output.([]*big.Int) {
		var t *big.Int
		t, err = sd.DecodeBigInt()
		output.([]*big.Int)[i] = t
		if err != nil {
			break
		}
	}
	return nil
}

// DecodePtrBoolArray decodes a byte array to an array of bools
// that is written to output by reference
func (sd *Decoder) DecodePtrBoolArray(output interface{}) error {
	_, err := sd.DecodeInteger()
	if err != nil {
		return err
	}

	for i := range output.([]bool) {
		var err error
		output.([]bool)[i], err = sd.DecodeBool()
		if err != nil {
			break
		}
	}
	return nil
}
