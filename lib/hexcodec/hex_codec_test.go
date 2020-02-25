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

package hexcodec

import (
	"testing"
)

func TestHexEncode(t *testing.T) {
	tests := []struct {
		input    []byte
		expected []byte
	}{
		// Even
		{[]byte{0xF, 0x0}, []byte{0x0, 0x0F}},
		{[]byte{0xA, 0x8, 0xF, 0x0}, []byte{0x0, 0x8A, 0x0F}},
		{[]byte{0xD, 0xE, 0xA, 0xD, 0xB, 0xE, 0xE, 0xF}, []byte{0x0, 0xED, 0xDA, 0xEB, 0xFE}},
		// Odd
		{[]byte{0xF}, []byte{0xF}},
		{[]byte{0xA, 0xF, 0x0}, []byte{0xA, 0x0F}},
		{[]byte{0xA, 0xC, 0xA, 0xB, 0x1, 0x2, 0x3}, []byte{0xA, 0xAC, 0x1B, 0x32}},
	}

	for _, test := range tests {
		res := Encode(test.input)
		for i := 0; i < len(res); i++ {
			if res[i] != test.expected[i] {
				t.Fatalf("Output doesn't match expected. got=%v expected=%v\n", res, test.expected)
			}
		}
	}
}
