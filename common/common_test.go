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
