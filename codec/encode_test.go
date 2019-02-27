package codec

import (
	"bytes"
	"math/big"
	"testing"
)

type encodeTest struct {
	val          interface{}
	output       []byte
	bytesEncoded int
}

var encodeTests = []encodeTest{
	// compact integers
	{val: int64(0), output: []byte{0x00}, bytesEncoded: 1},
	{val: int64(1), output: []byte{0x04}, bytesEncoded: 1},
	{val: int64(42), output: []byte{0xa8}, bytesEncoded: 1},
	{val: int64(69), output: []byte{0x15, 0x01}, bytesEncoded: 2},
	{val: int64(16383), output: []byte{0xfd, 0xff}, bytesEncoded: 2},
	{val: int64(16384), output: []byte{0x02, 0x00, 0x01, 0x00}, bytesEncoded: 4},
	{val: int64(1073741823), output: []byte{0xfe, 0xff, 0xff, 0xff}, bytesEncoded: 4},
	{val: int64(1073741824), output: []byte{0x03, 0x00, 0x00, 0x00, 0x40}, bytesEncoded: 5},
	{val: int64(1<<32 - 1), output: []byte{0x03, 0xff, 0xff, 0xff, 0xff}, bytesEncoded: 5},
	{val: int64(1 << 32), output: []byte{0x07, 0x00, 0x00, 0x00, 0x00, 0x01}, bytesEncoded: 6},

	// byte arrays
	{val: []byte{0x01}, output: []byte{0x04, 0x01}, bytesEncoded: 2},
	{val: []byte{0xff}, output: []byte{0x04, 0xff}, bytesEncoded: 2},
	{val: []byte{0x01, 0x01}, output: []byte{0x08, 0x01, 0x01}, bytesEncoded: 3},
	{val: []byte{0x01, 0x01}, output: []byte{0x08, 0x01, 0x01}, bytesEncoded: 3},
	{val: byteArray(64), output: append([]byte{0x01, 0x01}, byteArray(64)...), bytesEncoded: 66},
	{val: byteArray(16384), output: append([]byte{0x02, 0x00, 0x01, 0x00}, byteArray(16384)...), bytesEncoded: 16388},

	// booleans
	{val: true, output: []byte{0x01}, bytesEncoded: 1},
	{val: false, output: []byte{0x00}, bytesEncoded: 1},

	// big ints
	{val: big.NewInt(0), output: []byte{0x00}, bytesEncoded: 1},
	{val: big.NewInt(1), output: []byte{0x04}, bytesEncoded: 1},
	{val: big.NewInt(42), output: []byte{0xa8}, bytesEncoded: 1},
	{val: big.NewInt(69), output: []byte{0x15, 0x01}, bytesEncoded: 2},
	{val: big.NewInt(16383), output: []byte{0xfd, 0xff}, bytesEncoded: 2},
	{val: big.NewInt(16384), output: []byte{0x02, 0x00, 0x01, 0x00}, bytesEncoded: 4},

	// big ints
	{val: big.NewInt(0), output: []byte{0x00}, bytesEncoded: 1},
	{val: big.NewInt(1), output: []byte{0x04}, bytesEncoded: 1},
	{val: big.NewInt(42), output: []byte{0xa8}, bytesEncoded: 1},
	{val: big.NewInt(69), output: []byte{0x15, 0x01}, bytesEncoded: 2},
	{val: big.NewInt(16383), output: []byte{0xfd, 0xff}, bytesEncoded: 2},
	{val: big.NewInt(16384), output: []byte{0x02, 0x00, 0x01, 0x00}, bytesEncoded: 4},

	// structs
	{val: struct {
		Foo []byte
		Bar int64
	}{[]byte{0x01}, 2}, output: []byte{0x04, 0x01, 0x08}, bytesEncoded: 3},
	{val: struct {
		Foo []byte
		Bar int64
		Ok  bool
	}{[]byte{0x01}, 2, true}, output: []byte{0x04, 0x01, 0x08, 0x01}, bytesEncoded: 4},
	{val: struct {
		Foo int64
		Bar []byte
	}{int64(16384), []byte{0xff}}, output: []byte{0x02, 0x00, 0x01, 0x00, 0x04, 0xff}, bytesEncoded: 6},
	{val: struct {
		Foo int64
		Bar []byte
	}{int64(1073741824), byteArray(64)}, output: append([]byte{0x03, 0x00, 0x00, 0x00, 0x40, 0x01, 0x01}, byteArray(64)...), bytesEncoded: 71},
}

func TestEncode(t *testing.T) {
	for _, test := range encodeTests {
		buffer := bytes.Buffer{}
		se := Encoder{&buffer}
		bytesEncoded, err := se.Encode(test.val)
		output := buffer.Bytes()
		if err != nil {
			t.Error(err)
		} else if !bytes.Equal(output, test.output) {
			t.Errorf("Fail: input %x got %x expected %x", test.val, output, test.output)
		} else if bytesEncoded != test.bytesEncoded {
			t.Errorf("Fail: input %x  got %d bytes encoded expected %d", test.val, bytesEncoded, test.bytesEncoded)
		}
	}
}
