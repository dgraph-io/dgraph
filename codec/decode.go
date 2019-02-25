package codec

import (
	"encoding/binary"
	"errors"
	"math/big"
	"reflect"
)

// DecodeInteger accepts a byte array representing a SCALE encoded integer and performs SCALE decoding of the int
// if the encoding is valid, it then returns (o, bytesDecoded, err) where o is the decoded integer, bytesDecoded is the
// number of input bytes decoded, and err is nil
// otherwise, it returns 0, 0, and error
func DecodeInteger(b []byte) (o int64, bytesDecoded int64, err error) {
	// check mode of encoding, stored at 2 least significant bits
	mode := b[0] & 0x03
	if mode == 0 { // 1 byte mode
		return int64(b[0] >> 2), 1, nil
	} else if mode == 1 { // 2 byte mode
		o := binary.LittleEndian.Uint16(b) >> 2
		return int64(o), 2, nil
	} else if mode == 2 { // 4 byte mode
		o := binary.LittleEndian.Uint32(b) >> 2
		return int64(o), 4, nil
	}

	// >4 byte mode
	topSixBits := (binary.LittleEndian.Uint16(b) & 0xff) >> 2
	byteLen := topSixBits + 4

	if len(b) < int(byteLen)+1 {
		err = errors.New("could not decode invalid integer")
	}

	if err == nil {
		bytesDecoded = int64(byteLen) + 1
		if byteLen == 4 {
			o = int64(binary.LittleEndian.Uint32(b[1:bytesDecoded]))
		} else if byteLen > 4 && byteLen < 8 {
			// need to pad upper bytes with zeros so that we can perform binary decoding
			upperBytes := make([]byte, 8-byteLen)
			upperBytes = append(b[5:bytesDecoded], upperBytes...)
			upper := int64(binary.LittleEndian.Uint32(upperBytes)) << 32
			lower := int64(binary.LittleEndian.Uint32(b[1:5]))
			o = upper + lower
		}

		if o == 0 {
			err = errors.New("could not decode invalid integer")
		}
	}

	return o, bytesDecoded, err
}

// DecodeBigInt decodes a SCALE encoded byte array into a *big.Int
// Works for all integers, including ints > 2**64
func DecodeBigInt(b []byte) (output *big.Int, bytesDecoded int64, err error) {
	// check mode of encoding, stored at 2 least significant bits
	mode := b[0] & 0x03
	if mode <= 2 {
		tmp, bytesDecoded, err := DecodeInteger(b)
		return big.NewInt(tmp), bytesDecoded, err
	}

	// >4 byte mode
	topSixBits := (binary.LittleEndian.Uint16(b) & 0xff) >> 2
	byteLen := topSixBits + 4

	if len(b) < int(byteLen)+1 {
		err = errors.New("could not decode invalid integer")
	}

	// TODO: use io.Writer to return number of bytesDecoded
	if err == nil {
		o := reverseBytes(b[1 : byteLen+1])
		output = new(big.Int).SetBytes(o)
		bytesDecoded = int64(byteLen) + 1
	}

	return output, bytesDecoded, nil
}

// DecodeByteArray accepts a byte array representing a SCALE encoded byte array and performs SCALE decoding
// of the byte array
// if the encoding is valid, it then returns the decoded byte array, the total number of input bytes decoded, and nil
// otherwise, it returns nil, 0, and error
func DecodeByteArray(b []byte) (o []byte, bytesDecoded int64, err error) {
	var length int64

	// check mode of encoding, stored at 2 least significant bits
	mode := b[0] & 0x03
	if mode == 0 { // encoding of length: 1 byte mode
		length, _, err = DecodeInteger([]byte{b[0]})
		if err == nil {
			if length == 0 || length > 1<<6 || int64(len(b)) < length+1 {
				err = errors.New("could not decode invalid byte array")
			} else {
				o = b[1 : length+1]
				bytesDecoded = length + 1
			}
		}
	} else if mode == 1 { // encoding of length: 2 byte mode
		// pass first two bytes of byte array to decode length
		length, _, err = DecodeInteger(b[0:2])

		if err == nil {
			if length < 1<<6 || length > 1<<14 || int64(len(b)) < length+2 {
				err = errors.New("could not decode invalid byte array")
			} else {
				o = b[2 : length+2]
				bytesDecoded = length + 2
			}
		}
	} else if mode == 2 { // encoding of length: 4 byte mode
		// pass first four bytes of byte array to decode length
		length, _, err = DecodeInteger(b[0:4])

		if err == nil {
			if length < 1<<14 || length > 1<<30 || int64(len(b)) < length+4 {
				err = errors.New("could not decode invalid byte array")
			} else {
				o = b[4 : length+4]
				bytesDecoded = length + 4
			}
		}
	} else if mode == 3 { // encoding of length: big-integer mode
		length, _, err = DecodeInteger(b)

		if err == nil {
			// get the length of the encoded length
			topSixBits := (binary.LittleEndian.Uint16(b) & 0xff) >> 2
			byteLen := topSixBits + 4

			if length < 1<<30 || int64(len(b)) < (length+int64(byteLen))+1 {
				err = errors.New("could not decode invalid byte array")
			} else {
				o = b[int64(byteLen)+1 : length+int64(byteLen)+1]
				bytesDecoded = int64(byteLen) + length + 1
			}
		}
	}

	return o, bytesDecoded, err
}

// DecodeBool accepts a byte array representing a SCALE encoded bool and performs SCALE decoding
// of the bool then returns it. if invalid, return false and an error
func DecodeBool(b byte) (bool, error) {
	if b == 1 {
		return true, nil
	} else if b == 0 {
		return false, nil
	}

	return false, errors.New("cannot decode invalid boolean")
}

// DecodeTuple accepts a byte array representing the SCALE encoded tuple and an interface. This interface should be a pointer
// to a struct which the encoded tuple should be marshalled into. If it is a valid encoding for the struct, it returns the
// decoded struct, otherwise error,
// Note that we return the same interface that was passed to this function; this is because we are writing directly to the
// struct that is passed in, using reflect to get each of the fields.
func DecodeTuple(b []byte, t interface{}) (interface{}, error) {
	v := reflect.ValueOf(t).Elem()

	var bytesDecoded int64
	var byteLen int64
	var err error
	var o interface{}

	val := reflect.Indirect(reflect.ValueOf(t))

	// iterate through each field in the struct
	for i := 0; i < v.NumField(); i++ {

		// get the field value at i
		fieldValue := val.Field(i)

		switch v.Field(i).Interface().(type) {
		case []byte:
			o, byteLen, err = DecodeByteArray(b[bytesDecoded:])
			if err != nil {
				break
			}
			// get the pointer to the value and set the value
			ptr := fieldValue.Addr().Interface().(*[]byte)
			*ptr = o.([]byte)
		case int64:
			o, byteLen, err = DecodeInteger(b[bytesDecoded:])
			if err != nil {
				break
			}
			// get the pointer to the value and set the value
			ptr := fieldValue.Addr().Interface().(*int64)
			*ptr = o.(int64)
		case bool:
			o, err = DecodeBool(b[bytesDecoded])
			if err != nil {
				break
			}
			// get the pointer to the value and set the value
			ptr := fieldValue.Addr().Interface().(*bool)
			*ptr = o.(bool)
			byteLen = 1
		}

		if len(b) < int(bytesDecoded)+1 {
			err = errors.New("could not decode invalid byte array into tuple")
		}
		bytesDecoded = bytesDecoded + byteLen
		if err != nil {
			break
		}
	}

	return t, err
}
