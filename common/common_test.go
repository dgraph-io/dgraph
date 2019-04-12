package common

import (
	"bytes"
	"testing"
)

type concatTest struct {
	a, b   []byte
	output []byte
}

var concatTests = []concatTest{
	{a: []byte{}, b: []byte{}, output: []byte{}},
	{a: []byte{0x00}, b: []byte{}, output: []byte{0x00}},
	{a: []byte{0x00}, b: []byte{0x00}, output: []byte{0x00, 0x00}},
	{a: []byte{0x00}, b: []byte{0x00, 0x01}, output: []byte{0x00, 0x00, 0x01}},
	{a: []byte{0x01}, b: []byte{0x00, 0x01, 0x02}, output: []byte{0x01, 0x00, 0x01, 0x02}},
	{a: []byte{0x00, 0x01, 0x02, 0x00}, b: []byte{0x00, 0x01, 0x02}, output: []byte{0x000, 0x01, 0x02, 0x00, 0x00, 0x01, 0x02}},
}

func TestConcat(t *testing.T) {
	for _, test := range concatTests {
		output := Concat(test.a, test.b...)
		if !bytes.Equal(output, test.output) {
			t.Errorf("Fail: got %d expected %d", output, test.output)
		}
	}
}

func TestUint16ToBytes(t *testing.T) {
	tests := []struct {
		input    uint16
		expected []byte
	}{
		{uint16(0), []byte{0x0, 0x0}},
		{uint16(1), []byte{0x1, 0x0}},
		{uint16(255), []byte{0xff, 0x0}},
	}

	for _, test := range tests {
		res := Uint16ToBytes(test.input)
		if !bytes.Equal(res, test.expected) {
			t.Errorf("Output doesn't match expected. got=%v expected=%v\n", res, test.expected)
		}
	}
}
