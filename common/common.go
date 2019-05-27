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

package common

// Concat concatenates two byte arrays
// used instead of append to prevent modifying the original byte array
func Concat(s1 []byte, s2 ...byte) []byte {
	r := make([]byte, len(s1)+len(s2))
	copy(r, s1)
	copy(r[len(s1):], s2)
	return r
}

// Uint16ToBytes converts a uint16 into a 2-byte slice
func Uint16ToBytes(in uint16) (out []byte) {
	out = make([]byte, 2)
	out[0] = byte(in & 0x00ff)
	out[1] = byte(in >> 8 & 0x00ff)
	return out
}

// AppendZeroes appends zeroes to the input byte array up until it has length size
func AppendZeroes(input []byte, size int) []byte {
	for {
		if len(input) < 32 {
			input = append(input, 0)
		} else {
			return input
		}
	}
}

// SwapByteNibbles swaps the two nibbles of a byte
func SwapByteNibbles(b byte) byte {
	b1 := (uint(b) & 240) >> 4
	b2 := (uint(b) & 15) << 4

	return byte(b1 | b2)
}

// SwapNibbles swaps the nibbles for each byte in the byte array
func SwapNibbles(k []byte) []byte {
	result := make([]byte, len(k))
	for i, b := range k {
		result[i] = SwapByteNibbles(b)
	}
	return result
}
