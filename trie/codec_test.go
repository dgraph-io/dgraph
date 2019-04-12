package trie

import (
	"bytes"
	"testing"
)

func TestKeyEncodeByte(t *testing.T) {
	tests := []struct {
		input    byte
		expected byte
	}{
		{byte(0xA0), byte(0x0A)},
		{byte(0), byte(0)},
		{byte(0x24), byte(0x42)},
	}

	for _, test := range tests {
		res := keyEncodeByte(test.input)
		if res != test.expected {
			t.Fatalf("got: %x; expected: %x", res, test.expected)
		}
	}
}

func TestKeyEncode(t *testing.T) {
	tests := []struct {
		key        []byte
		encodedKey []byte
	}{
		{[]byte{0x01, 0x02, 0x03, 0x04, 0x05}, []byte{0x10, 0x20, 0x30, 0x40, 0x50}},
		{[]byte{0xff, 0x0, 0xAA, 0x81}, []byte{0xff, 0x00, 0xAA, 0x18}},
		{[]byte{0xAC, 0x19, 0x15}, []byte{0xCA, 0x91, 0x51}},
	}

	for _, test := range tests {
		res := KeyEncode(test.key)
		if !bytes.Equal(res, test.encodedKey) {
			t.Fatalf("got: %x, expected: %x", res, test.encodedKey)
		}

		res = KeyEncode(res)
		if !bytes.Equal(res, test.key) {
			t.Fatalf("Re-encoding failed. got: %x expected: %x", res, test.key)
		}
	}

}

func TestKeyToHex(t *testing.T) {
	tests := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte{0xFF}, []byte{0xF, 0xF}},
		{[]byte{0x3a, 0x05}, []byte{0x3, 0xa, 0x0, 0x5}},
		{[]byte{0xAA, 0xFF, 0x01}, []byte{0xa, 0xa, 0xf, 0xf, 0x0, 0x1}},
		{[]byte{0xAA, 0xFF, 0x01, 0xc2}, []byte{0xa, 0xa, 0xf, 0xf, 0x0, 0x1, 0xc, 0x2}},
	}

	for _, test := range tests {
		res := keyToHex(test.input)
		for i := 0; i < len(res)-1; i++ {
			if res[i] != test.expected[i] {
				t.Errorf("Output doesn't match expected. got=%v expected=%v\n", res, test.expected)
			}
		}
	}
}