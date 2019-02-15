package codec

import (
	"encoding/binary"
	"errors"
)

// Encode is the top-level function which performs SCALE encoding of b which may be of type []byte, int16, int32, int64,
// or bool
func Encode(b interface{}) ([]byte, error) {
	switch v := b.(type) {
	case []byte:
		return encodeByteArray(v)
	case int16:
		return encodeInteger(int64(v))
	case int32:
		return encodeInteger(int64(v))
	case int64:
		return encodeInteger(v)
	case bool:
		return encodeBool(v)
	default:
		return []byte{}, errors.New("Unsupported type!")
	}
	return []byte{}, nil
}

// encodeByteArray performs the following:
// b -> [encodeInteger(len(b)) b]
// it returns a byte array where the first byte is the length of b encoded with SCALE, followed by the byte array b itself
func encodeByteArray(b []byte) ([]byte, error) {
	encodedLen, err := encodeInteger(int64(len(b)))
	if err != nil {
		return []byte{}, err
	}
	return append(encodedLen, b...), nil
}

// encodeInteger performs the following on integer i:
// i  -> i^0...i^n where n is the length in bits of i
// note that the bit representation of i is in little endian; ie i^0 is the least significant bit of i,
// and i^n is the most significant bit
// if n < 2^6 return [00 i^2...i^8 ] [ 8 bits = 1 byte output ]
// if 2^6 <= n < 2^14 return [01 i^2...i^16] [ 16 bits = 2 byte output ]
// if 2^14 <= n < 2^30 return [10 i^2...i^32] [ 32 bits = 4 byte output ]
// if n >= 2^30 return [lower 2 bits of first byte = 11] [upper 6 bits of first byte = # of bytes following less 4]
// [append i as a byte array to the first byte]
func encodeInteger(i int64) ([]byte, error) {
	if i < 1<<6 {
		o := byte(i) << 2
		return []byte{o}, nil
	} else if i < 1<<14 {
		o := make([]byte, 2)
		binary.LittleEndian.PutUint16(o, uint16(i<<2)+1)
		return o, nil
	} else if i < 1<<30 {
		o := make([]byte, 4)
		binary.LittleEndian.PutUint32(o, uint32(i<<2)+2)
		return o, nil
	} else {
		o := make([]byte, 8)
		m := i
		var numBytes uint

		// calculate the number of bytes needed to store i
		// the most significant byte cannot be zero
		// each iteration, shift by 1 byte until the number is zero
		// then break and save the numBytes needed
		for numBytes = 1; numBytes < 256; numBytes++ {
			m = m >> 8
			if m == 0 {
				break
			}
		}

		topSixBits := uint8(numBytes - 4)
		lengthByte := topSixBits<<2 + 3

		l := make([]byte, 2)

		binary.LittleEndian.PutUint16(l, uint16(lengthByte))
		binary.LittleEndian.PutUint64(o, uint64(i))

		return append([]byte{l[0]}, o[0:numBytes]...), nil
	}
}

// encodeBool performs the following:
// l = true -> return [1]
// l = false -> return [0]
func encodeBool(l bool) ([]byte, error) {
	if l {
		return []byte{0x01}, nil
	} else {
		return []byte{0x00}, nil
	}
}
