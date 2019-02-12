package codec

import (
    "encoding/binary"
	"errors"
)

func Encode(b interface{}) ([]byte, error) {
	switch v := b.(type) {
		case []byte:
			return encodeByteArray(v)
		case int64:
			return encodeInteger(v)
		default:
			return []byte{}, errors.New("Unsupported type!!")
	}
	return []byte{}, nil
}

func encodeByteArray(b []byte) ([]byte, error) {
	encodedLen, err := encodeInteger(int64(len(b)))
	if err != nil {
		return []byte{}, err
	}
	return append(encodedLen, b...), nil
}

// encodeInteger performs the following on integer i:
// i -> base2(i) -> i^n...i^0 where n is the length in bits of i
// note that the bit representation of i is in little endian; ie i^n is the least significant bit of i,
// and i^0 is the most significant bit
// if n < 2^6 return i^n...i^2 00 [ 8 bits = 1 byte output ]
// if 2^6 <= n < 2^14 return i^n...i^2 01 [ 16 bits = 2 byte output ]
// if 2^14 <= n < 2^30 return i^n...i^2 10 [ 32 bits = 4 byte output ]
// if n >= 2^30 return 
func encodeInteger(n int64) ([]byte, error) {
	if n < 1 << 6 { 
		o := byte(n) << 2
		return []byte{o}, nil
	} else if n < 1 << 14 {
		o := make([]byte, 2)
		binary.LittleEndian.PutUint16(o, uint16(n<<2)+1)
		return o, nil
	} else if n < 1 << 30 {
		o := make([]byte, 4)
		binary.LittleEndian.PutUint16(o, uint16(n<<2)+2)
		return o, nil
	} else {
		return []byte{}, nil
	}
}

func encodeLargeInteger(n int) ([]byte, error) {
	if n < 1 << 30 {
		return encodeInteger(int64(n))
	} else {
		return []byte{}, nil
	}
}

func Decode(b []byte) []byte {
	return []byte{}
}