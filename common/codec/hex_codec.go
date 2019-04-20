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

// Encode assumes its input is an array of nibbles (4bits), and produces Hex-Encoded output.
// HexEncoded: For PK = (k_1,...,k_n), Enc_hex(PK) :=
// (0, k_1 + k_2 * 16,...) for even length
// (k_1, k_2 + k_3 * 16,...) for odd length
func Encode(in []byte) []byte {
	res := make([]byte, (len(in)/2)+1)
	if len(in) == 1 { // Single byte
		res[0] = in[0]
	} else {
		resI := 1
		var i int

		if len(in)%2 == 1 { // Odd length
			res[0] = in[0]
			i = 1 // Skip first nibble
		} else { // Even length
			res[0] = 0x0
			i = 0 // Start loop with first nibble
		}

		for ; i < len(in); i += 2 {
			res[resI] = combineNibbles(in[i+1], in[i])
			resI++
		}
	}

	return res
}

// combineNibbles concatenates two nibble to make a byte.
// Assumes nibbles are the lower 4 bits of each of the inputs
func combineNibbles(ms byte, ls byte) byte {
	return byte(ms<<4 | (ls & 0xF))
}
