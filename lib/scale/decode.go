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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"runtime"
	"strings"

	"github.com/ChainSafe/gossamer/lib/common"
)

// withCustom is true is custom encoding is supported. set to true by default.
// only should be set to false for testing.
var withCustom = true

// Decoder is a wrapping around io.Reader
type Decoder struct {
	Reader io.Reader
}

// Decode a byte array into interface
func Decode(in []byte, t interface{}) (interface{}, error) {
	buf := &bytes.Buffer{}
	sd := Decoder{
		Reader: buf,
	}
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
	case int, uint, int8, uint8, int16, uint16, int32, uint32, int64, uint64:
		out, err = sd.DecodeFixedWidthInt(t)
	case []byte, string:
		out, err = sd.DecodeByteArray()
	case bool:
		out, err = sd.DecodeBool()
	case []int:
		out, err = sd.DecodeIntArray()
	case []bool:
		out, err = sd.DecodeBoolArray()
	case []*big.Int:
		out, err = sd.DecodeBigIntArray()
	case common.Hash:
		b := make([]byte, 32)
		_, err = sd.Reader.Read(b)
		out = common.NewHash(b)
	case [][32]byte, [][]byte:
		out, err = sd.DecodeSlice(t)
	case interface{}:
		// check if type has a custom Decode function defined
		// but first, make sure that the function that called this function wasn't the type's Decode function,
		// or else we will end up in an infinite recursive loop
		pc, _, _, ok := runtime.Caller(1)
		details := runtime.FuncForPC(pc)
		var caller string
		if ok && details != nil {
			caller = details.Name()
		}

		// allow the call to DecodeCustom to proceed if the call comes from inside scale, and isn't a test
		if !strings.Contains(caller, "Decode") && withCustom || (strings.Contains(caller, "scale") && !strings.Contains(caller, "test")) && withCustom {
			if out, err = sd.DecodeCustom(t); err == nil {
				return out, nil
			}
		}

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
	case int:
		buf := make([]byte, 8)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			o = int(binary.LittleEndian.Uint64(buf))
		}
	case uint:
		buf := make([]byte, 8)
		_, err = sd.Reader.Read(buf)
		if err == nil {
			o = uint(binary.LittleEndian.Uint64(buf))
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

// DecodeUnsignedInteger will decode unsigned integer
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
		o = binary.LittleEndian.Uint64(tmp)
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

// DecodeInterface will decode to interface
func (sd *Decoder) DecodeInterface(t interface{}) (interface{}, error) {
	switch reflect.ValueOf(t).Kind() {
	case reflect.Ptr:
		switch reflect.ValueOf(t).Elem().Kind() {
		case reflect.Slice:
			return sd.DecodeSlice(t)
		case reflect.Array:
			return sd.DecodeArray(t)
		default:
			return sd.DecodeTuple(t)
		}
	case reflect.Slice:
		return sd.DecodeSlice(t)
	case reflect.Struct:
		return sd.DecodeTuple(t)
	case reflect.Array:
		return sd.DecodeArray(t)
	default:
		return nil, fmt.Errorf("unexpected kind: %s", reflect.ValueOf(t).Kind())
	}
}

// DecodeArray decodes a fixed-length array
func (sd *Decoder) DecodeArray(t interface{}) (interface{}, error) {
	v := reflect.ValueOf(t)
	var err error

	// this means t is a custom type with an underlying array.
	// not handled, requires a custom decode function
	switch v.Kind() {
	case reflect.Ptr:
		return nil, errors.New("unsupported type")
	}

	length := v.Len()
	arr := reflect.New(reflect.TypeOf(t)).Elem()
	if length == 0 {
		return arr.Interface(), nil
	}

	for i := 0; i < length; i++ {
		elem := v.Index(i)
		switch elem.Interface().(type) {
		case byte:
			var b byte
			b, err = sd.ReadByte()
			arr.Index(i).Set(reflect.ValueOf(b))
		default:
			var res interface{}
			res, err = sd.DecodeCustom(elem.Interface())
			if err != nil {
				return nil, err
			}
			elem.Set(reflect.ValueOf(res))
		}

		if err != nil {
			break
		}
	}

	return arr.Interface(), nil
}

// DecodeSlice will decode a slice
func (sd *Decoder) DecodeSlice(t interface{}) (interface{}, error) {
	v := reflect.ValueOf(t)
	var err error
	var o interface{}

	length, err := sd.DecodeInteger()
	if err != nil {
		return nil, err
	}

	if length == 0 {
		return t, nil
	}

	sl := reflect.MakeSlice(v.Type(), int(length), int(length))

	for i := 0; i < int(length); i++ {
		arrayValue := sl.Index(i)

		switch ptr := arrayValue.Addr().Interface().(type) {
		case *[]byte:
			if o, err = sd.DecodeByteArray(); err == nil {
				// get the pointer to the value and set the value
				*ptr = o.([]byte)
			}
		case *[32]byte:
			buf := make([]byte, 32)
			if _, err = sd.Reader.Read(buf); err == nil {
				var arr = [32]byte{}
				copy(arr[:], buf)
				*ptr = arr
			}
		case *common.PeerInfo:
			_, err = sd.DecodeInterface(ptr)
			if err != nil {
				return nil, err
			}

		default:
			var res interface{}
			res, err = sd.DecodeCustom(sl.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			arrayValue.Set(reflect.ValueOf(res))
		}

		if err != nil {
			break
		}
	}

	switch t.(type) {
	case [][]byte:
		copy(t.([][]byte), sl.Interface().([][]byte))
	case [][32]byte:
		copy(t.([][32]byte), sl.Interface().([][32]byte))
	}

	return sl.Interface(), err
}

// DecodeTuple accepts a byte array representing the SCALE encoded tuple and an interface. This interface should be a pointer
// to a struct which the encoded tuple should be marshaled into. If it is a valid encoding for the struct, it returns the
// decoded struct, otherwise error,
// Note that we return the same interface that was passed to this function; this is because we are writing directly to the
// struct that is passed in, using reflect to get each of the fields.
func (sd *Decoder) DecodeTuple(t interface{}) (interface{}, error) { //nolint
	var v reflect.Value
	switch reflect.ValueOf(t).Kind() {
	case reflect.Ptr:
		v = reflect.ValueOf(t).Elem()
	default:
		v = reflect.ValueOf(t)
	}

	var err error
	var o interface{}

	// iterate through each field in the struct
	for i := 0; i < v.NumField(); i++ {
		// get the field value at i
		field := v.Field(i)

		if field.CanInterface() {
			fieldValue := field.Addr().Interface()

			switch ptr := fieldValue.(type) {
			case *byte:
				b := make([]byte, 1)
				if _, err = sd.Reader.Read(b); err == nil {
					*ptr = b[0]
				}
			case *[]byte:
				if o, err = sd.DecodeByteArray(); err == nil {
					// get the pointer to the value and set the value
					*ptr = o.([]byte)
				}
			case *[][]byte:
				if o, err = sd.DecodeSlice([][]byte{}); err == nil {
					*ptr = o.([][]byte)
				}
			case *int8:
				if o, err = sd.DecodeFixedWidthInt(int8(0)); err == nil {
					*ptr = o.(int8)
				}
			case *int16:
				if o, err = sd.DecodeFixedWidthInt(int16(0)); err == nil {
					*ptr = o.(int16)
				}
			case *int32:
				if o, err = sd.DecodeFixedWidthInt(int32(0)); err == nil {
					*ptr = o.(int32)
				}
			case *int64:
				if o, err = sd.DecodeFixedWidthInt(int64(0)); err == nil {
					*ptr = o.(int64)
				}
			case *uint16:
				if o, err = sd.DecodeFixedWidthInt(uint16(0)); err == nil {
					*ptr = o.(uint16)
				}
			case *uint32:
				if o, err = sd.DecodeFixedWidthInt(uint32(0)); err == nil {
					*ptr = o.(uint32)
				}
			case *uint64:
				if o, err = sd.DecodeFixedWidthInt(uint64(0)); err == nil {
					*ptr = o.(uint64)
				}
			case *int:
				if o, err = sd.DecodeFixedWidthInt(0); err == nil {
					*ptr = o.(int)
				}
			case *uint:
				if o, err = sd.DecodeFixedWidthInt(uint(0)); err == nil {
					*ptr = o.(uint)
				}
			case *bool:
				if o, err = sd.DecodeBool(); err == nil {
					*ptr = o.(bool)
				}
			case **big.Int:
				if o, err = sd.DecodeBigInt(); err == nil {
					*ptr = o.(*big.Int)
				}
			case *common.Hash:
				b := make([]byte, 32)
				if _, err = sd.Reader.Read(b); err == nil {
					*ptr = common.NewHash(b)
				}
			case *[32]byte:
				b := make([]byte, 32)
				if _, err = sd.Reader.Read(b); err == nil {
					copy((*ptr)[:], b)
				}
			case *[64]byte:
				b := make([]byte, 64)
				if _, err = sd.Reader.Read(b); err == nil {
					copy((*ptr)[:], b)
				}
			case *string:
				if o, err = sd.DecodeByteArray(); err == nil {
					// get the pointer to the value and set the value
					*ptr = string(o.([]byte))
				}
			case *[]string:
				if o, err = sd.DecodeStringArray(); err == nil {
					*ptr = o.([]string)
				}
			default:
				if o, err = sd.DecodeCustom(fieldValue); err == nil {
					field.Set(reflect.ValueOf(o))
					continue
				}

				// TODO: clean up this function, can use field.Set everywhere (remove switch case?)
				if o, err = sd.Decode(v.Field(i).Interface()); err == nil {
					field.Set(reflect.ValueOf(o))
				}
			}

			if err != nil {
				break
			}
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

// DecodeStringArray will decode to string array
func (sd *Decoder) DecodeStringArray() ([]string, error) {
	length, err := sd.DecodeInteger()
	if err != nil {
		return nil, err
	}
	s := make([]string, length)

	for i := 0; i < int(length); i++ {
		o, err := sd.DecodeByteArray()
		if err != nil {
			return nil, err
		}
		s[i] = string(o[:]) // cast []byte into string
	}
	return s, nil
}
