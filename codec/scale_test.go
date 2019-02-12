package codec

import (
	"bytes"
	"testing"
)

type encodeVec struct {
	val 	interface{}
	output  []byte
	error 	string
}

var encodeTests = []encodeVec{
	// compact integers
	{val: int64(0), output: []byte{0x00}},
	{val: int64(1), output: []byte{0x04}},
	{val: int64(42), output: []byte{0xa8}},
	{val: int64(69), output: []byte{0x15, 0x01}},
	{val: int64(16383), output: []byte{0xfd, 0xff}},

	// byte arrays
	{val: []byte{0x01}, output: []byte{0x04, 0x01}},
	{val: []byte{0xff}, output: []byte{0x04, 0xff}},	
	{val: []byte{0x01, 0x01}, output: []byte{0x08, 0x01, 0x01}},	
}

func TestEncode(t *testing.T) {
	for _, test := range encodeTests {
		output, err := Encode(test.val)
		if err != nil {
			t.Error(err)
		} else if !bytes.Equal(output, test.output) {
			t.Errorf("Fail: got %x expected %x", output, test.output)
		}
	}
}

func TestDecode(t *testing.T) {

}