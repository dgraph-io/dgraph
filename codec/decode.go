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
	"encoding/binary"
	"errors"
	"io"
	"math/big"
	"reflect"

	"github.com/ChainSafe/gossamer/common"
)

// Decoder is a wrapping around io.Reader
type Decoder struct {
	Reader io.Reader
}

func Decode(in []byte, t interface{}) (interface{}, error) {
	buf := &bytes.Buffer{}
	sd := Decoder{Reader: buf}
	_, err := buf.Write(in)
	if err != nil {
		return nil, err
	}

	output, err := sd.Decode(t)
	return output, err
}

// Decode is the high level function wrapping the specific type decoding functions
func (sd *Decoder) Decode(t interface{}) (out interface{}, err error) {
	switch t.(type) {
	case *big.Int:
		out, err = sd.DecodeBigInt()
	case int8, uint8, int16, uint16, int32, uint32, int64, uint64:
		out, err = sd.DecodeFixedWidthInt(t)
	case []byte:
		out, err = sd.DecodeByteArray()
	case bool:
		out, err = sd.DecodeBool()
	case []int:
		out, err = sd.DecodeIntArray()
	case []bool:
		out, err = sd.DecodeBoolArray()
	case []*big.Int:
		out, err = sd.DecodeBigIntArray()
	case common.Hash, [32]byte:
		b := make([]byte, 32)
		_, err = sd.Reader.Read(b)
		out = b
	case interface{}:
		out, err = sd.DecodeInterface(t)
	default:
		return nil, errors.New("decode error: unsupported type")
	}
	return out, err
}

// ReadByte reads the one byte from the buffer
func (sd *Decoder) ReadByte() (byte, error) {
	b := make([]byte, 1)        // make buffer
	_, err := sd.Reader.Read(b) // read what's in the Decoder's underlying buffer to our new buffer b
	return b[0], err
}

// decodeSmallInt is used in the DecodeInteger and DecodeBigInteger functions when the mode is <= 2
// need to pass in the first byte, since we assume it's already been read
func (sd *Decoder) decodeSmallInt(firstByte byte, mode byte) (o int64, err error) {
	if mode == 0 { // 1 byte mode
		o = int64(firstByte >> 2)
	} else if mode == 1 { // 2 byte mode
		var buf byte
		buf, err = sd.ReadByte()
		if err == nil {
			o = int64(binary.LittleEndian.Uint16([]byte{firstByte, buf}) >> 2)
		}
	} else if mode == 2 { // 4 byte mode
		buf := make([]byte, 3)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			o = int64(binary.LittleEndian.Uint32(append([]byte{firstByte}, buf...)) >> 2)
		}
	} else {
		err = errors.New("could not decode small int: mode not <= 2")
	}

	return o, err
}

// DecodeFixedWidthInt decodes integers < 2**32 by reading the bytes in little endian
func (sd *Decoder) DecodeFixedWidthInt(t interface{}) (o interface{}, err error) {
	switch t.(type) {
	case int8:
		var b byte
		b, err = sd.ReadByte()
		o = int8(b)
	case uint8:
		var b byte
		b, err = sd.ReadByte()
		o = b
	case int16:
		buf := make([]byte, 2)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			o = int16(binary.LittleEndian.Uint16(buf))
		}
	case uint16:
		buf := make([]byte, 2)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			o = binary.LittleEndian.Uint16(buf)
		}
	case int32:
		buf := make([]byte, 4)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			o = int32(binary.LittleEndian.Uint32(buf))
		}
	case uint32:
		buf := make([]byte, 4)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			o = binary.LittleEndian.Uint32(buf)
		}
	case int64:
		buf := make([]byte, 8)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			o = int64(binary.LittleEndian.Uint64(buf))
		}
	case uint64:
		buf := make([]byte, 8)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			o = binary.LittleEndian.Uint64(buf)
		}
	}
	return o, err
}

// DecodeInteger accepts a byte array representing a SCALE encoded integer and performs SCALE decoding of the int
// if the encoding is valid, it then returns (o, bytesDecoded, err) where o is the decoded integer, bytesDecoded is the
// number of input bytes decoded, and err is nil
// otherwise, it returns 0, 0, and error
func (sd *Decoder) DecodeInteger() (_ int64, err error) {
	o, err := sd.DecodeUnsignedInteger()

	return int64(o), err
}

func (sd *Decoder) DecodeUnsignedInteger() (o uint64, err error) {
	b, err := sd.ReadByte()
	if err != nil {
		return 0, err
	}

	// check mode of encoding, stored at 2 least significant bits
	mode := b & 3
	if mode <= 2 {
		val, e := sd.decodeSmallInt(b, mode)
		return uint64(val), e
	}

	// >4 byte mode
	topSixBits := b >> 2
	byteLen := uint(topSixBits) + 4

	buf := make([]byte, byteLen)
	_, err = sd.Reader.Read(buf)
	if err != nil {
		return 0, err
	}

	if byteLen == 4 {
		o = uint64(binary.LittleEndian.Uint32(buf))
	} else if byteLen > 4 && byteLen < 8 {
		tmp := make([]byte, 8)
		copy(tmp, buf)
		o = uint64(binary.LittleEndian.Uint64(tmp))
	} else {
		err = errors.New("could not decode invalid integer")
	}

	return o, err
}

// DecodeBigInt decodes a SCALE encoded byte array into a *big.Int
// Works for all integers, including ints > 2**64
func (sd *Decoder) DecodeBigInt() (output *big.Int, err error) {
	b, err := sd.ReadByte()
	if err != nil {
		return nil, err
	}

	// check mode of encoding, stored at 2 least significant bits
	mode := b & 0x03
	if mode <= 2 {
		var tmp int64
		tmp, err = sd.decodeSmallInt(b, mode)
		if err != nil {
			return nil, err
		}
		return new(big.Int).SetInt64(tmp), nil
	}

	// >4 byte mode
	topSixBits := b >> 2
	byteLen := uint(topSixBits) + 4

	buf := make([]byte, byteLen)
	_, err = sd.Reader.Read(buf)
	if err == nil {
		o := reverseBytes(buf)
		output = new(big.Int).SetBytes(o)
	} else {
		err = errors.New("could not decode invalid big.Int: reached early EOF")
	}

	return output, err
}

// DecodeByteArray accepts a byte array representing a SCALE encoded byte array and performs SCALE decoding
// of the byte array
// if the encoding is valid, it then returns the decoded byte array, the total number of input bytes decoded, and nil
// otherwise, it returns nil, 0, and error
func (sd *Decoder) DecodeByteArray() (o []byte, err error) {
	length, err := sd.DecodeInteger()
	if err != nil {
		return nil, err
	}

	b := make([]byte, length)
	_, err = sd.Reader.Read(b)
	if err != nil {
		return nil, errors.New("could not decode invalid byte array: reached early EOF")
	}

	return b, nil
}

// DecodeBool accepts a byte array representing a SCALE encoded bool and performs SCALE decoding
// of the bool then returns it. if invalid, return false and an error
func (sd *Decoder) DecodeBool() (bool, error) {
	b, err := sd.ReadByte()
	if err != nil {
		return false, err
	}

	if b == 1 {
		return true, nil
	} else if b == 0 {
		return false, nil
	}

	return false, errors.New("cannot decode invalid boolean")
}

func (sd *Decoder) DecodeInterface(t interface{}) (interface{}, error) {
	switch reflect.ValueOf(t).Kind() {
	case reflect.Ptr:
		switch reflect.ValueOf(t).Elem().Kind() {
		case reflect.Slice, reflect.Array:
			return sd.DecodeArray(t)
		default:
			return sd.DecodeTuple(t)
		}
	case reflect.Slice, reflect.Array:
		return sd.DecodeArray(t)
	default:
		return sd.DecodeTuple(t)
	}
}

func (sd *Decoder) DecodeArray(t interface{}) (interface{}, error) {
	var v reflect.Value
	switch reflect.ValueOf(t).Kind() {
	case reflect.Ptr:
		v = reflect.ValueOf(t).Elem()
	case reflect.Slice, reflect.Array:
		v = reflect.ValueOf(t)
	}

	var err error
	var o interface{}

	length, err := sd.DecodeInteger()
	if err != nil {
		return nil, err
	}

	for i := 0; i < int(length); i++ {
		arrayValue := v.Index(i)

		switch v.Index(i).Interface().(type) {
		case []byte:
			o, err = sd.DecodeByteArray()
			if err != nil {
				break
			}

			// get the pointer to the value and set the value
			ptr := arrayValue.Addr().Interface().(*[]byte)
			*ptr = o.([]byte)
		case [32]byte:
			buf := make([]byte, 32)
			ptr := arrayValue.Addr().Interface().(*[32]byte)
			_, err = sd.Reader.Read(buf)

			var arr = [32]byte{}
			copy(arr[:], buf)
			*ptr = arr
		default:
			err = errors.New("could not decode invalid slice or array")
		}

		if err != nil {
			break
		}
	}

	return t, err
}

// DecodeTuple accepts a byte array representing the SCALE encoded tuple and an interface. This interface should be a pointer
// to a struct which the encoded tuple should be marshalled into. If it is a valid encoding for the struct, it returns the
// decoded struct, otherwise error,
// Note that we return the same interface that was passed to this function; this is because we are writing directly to the
// struct that is passed in, using reflect to get each of the fields.
func (sd *Decoder) DecodeTuple(t interface{}) (interface{}, error) {
	v := reflect.ValueOf(t).Elem()

	var err error
	var o interface{}

	val := reflect.Indirect(reflect.ValueOf(t))

	// iterate through each field in the struct
	for i := 0; i < v.NumField(); i++ {
		// get the field value at i
		fieldValue := val.Field(i)

		switch v.Field(i).Interface().(type) {
		case byte:
			b := make([]byte, 1)
			_, err = sd.Reader.Read(b)

			ptr := fieldValue.Addr().Interface().(*byte)
			*ptr = b[0]
		case []byte:
			o, err = sd.DecodeByteArray()
			if err != nil {
				break
			}

			// get the pointer to the value and set the value
			ptr := fieldValue.Addr().Interface().(*[]byte)
			*ptr = o.([]byte)
		case int8:
			o, err = sd.DecodeFixedWidthInt(int8(0))
			if err != nil {
				break
			}

			ptr := fieldValue.Addr().Interface().(*int8)
			*ptr = o.(int8)
		case int16:
			o, err = sd.DecodeFixedWidthInt(int16(0))
			if err != nil {
				break
			}

			ptr := fieldValue.Addr().Interface().(*int16)
			*ptr = o.(int16)
		case int32:
			o, err = sd.DecodeFixedWidthInt(int32(0))
			if err != nil {
				break
			}

			ptr := fieldValue.Addr().Interface().(*int32)
			*ptr = o.(int32)
		case int64:
			o, err = sd.DecodeFixedWidthInt(int64(0))
			if err != nil {
				break
			}

			ptr := fieldValue.Addr().Interface().(*int64)
			*ptr = o.(int64)
		case uint16:
			o, err = sd.DecodeFixedWidthInt(uint16(0))
			if err != nil {
				break
			}

			ptr := fieldValue.Addr().Interface().(*uint16)
			*ptr = o.(uint16)
		case uint32:
			o, err = sd.DecodeFixedWidthInt(uint32(0))
			if err != nil {
				break
			}

			ptr := fieldValue.Addr().Interface().(*uint32)
			*ptr = o.(uint32)
		case uint64:
			o, err = sd.DecodeFixedWidthInt(uint64(0))
			if err != nil {
				break
			}

			ptr := fieldValue.Addr().Interface().(*uint64)
			*ptr = o.(uint64)
		case bool:
			o, err = sd.DecodeBool()
			if err != nil {
				break
			}

			ptr := fieldValue.Addr().Interface().(*bool)
			*ptr = o.(bool)
		case *big.Int:
			o, err = sd.DecodeBigInt()
			if err != nil {
				break
			}

			ptr := fieldValue.Addr().Interface().(**big.Int)
			*ptr = o.(*big.Int)
		case common.Hash:
			b := make([]byte, 32)
			_, err = sd.Reader.Read(b)

			ptr := fieldValue.Addr().Interface().(*common.Hash)
			*ptr = common.NewHash(b)
		default:
			_, err = sd.Decode(v.Field(i).Interface())
			if err != nil {
				break
			}
		}

		if err != nil {
			break
		}
	}

	return t, err
}

// DecodeIntArray decodes a byte array to an array of ints
func (sd *Decoder) DecodeIntArray() ([]int, error) {
	length, err := sd.DecodeInteger()
	if err != nil {
		return nil, err
	}

	o := make([]int, length)
	for i := range o {
		var t int64
		t, err = sd.DecodeInteger()
		o[i] = int(t)
		if err != nil {
			break
		}
	}
	return o, nil
}

// DecodeBigIntArray decodes a byte array to an array of *big.Ints
func (sd *Decoder) DecodeBigIntArray() ([]*big.Int, error) {
	length, err := sd.DecodeInteger()
	if err != nil {
		return nil, err
	}

	o := make([]*big.Int, length)
	for i := range o {
		var t *big.Int
		t, err = sd.DecodeBigInt()
		o[i] = t
		if err != nil {
			break
		}
	}
	return o, nil
}

// DecodeBoolArray decodes a byte array to an array of bools
func (sd *Decoder) DecodeBoolArray() ([]bool, error) {
	length, err := sd.DecodeInteger()
	if err != nil {
		return nil, err
	}

	o := make([]bool, length)
	for i := range o {
		o[i], err = sd.DecodeBool()
		if err != nil {
			break
		}
	}
	return o, nil
}
