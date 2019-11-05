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

import (
	"encoding/hex"
	"errors"
	"strings"
)

// Length of hashes in bytes.
const (
	// HashLength is the expected length of the hash
	HashLength = 32
)

// StringArrayToBytes turns an array of strings into an array of byte arrays
func StringArrayToBytes(in []string) [][]byte {
	b := [][]byte{}
	for _, str := range in {
		b = append(b, []byte(str))
	}
	return b
}

// BytesToStringArray turns an array of byte arrays into an array strings
func BytesToStringArray(in [][]byte) []string {
	strs := []string{}
	for _, b := range in {
		strs = append(strs, string(b))
	}
	return strs
}

// HexToBytes turns a 0x prefixed hex string into a byte slice
func HexToBytes(in string) ([]byte, error) {
	if strings.Compare(in[:2], "0x") != 0 {
		return nil, errors.New("could not byteify non 0x prefixed string")
	}
	// Ensure we have an even length, otherwise hex.DecodeString will fail and return zero hash
	if len(in)%2 != 0 {
		return nil, errors.New("cannot decode a odd length string")
	}
	in = in[2:]
	out, err := hex.DecodeString(in)
	return out, err
}

// HexToBytes turns a 0x prefixed hex string into type Hash
func HexToHash(in string) (Hash, error) {
	if strings.Compare(in[:2], "0x") != 0 {
		return [32]byte{}, errors.New("could not byteify non 0x prefixed string")
	}
	in = in[2:]
	out, err := hex.DecodeString(in)
	if err != nil {
		return [32]byte{}, err
	}
	var buf = [32]byte{}
	copy(buf[:], out)
	return buf, err
}

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

// AppendZeroes appends zeroes to the input byte array up until it has length l
func AppendZeroes(in []byte, l int) []byte {
	for {
		if len(in) >= l {
			return in
		}
		in = append(in, 0)
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

// BytesToHash sets b to hash.
// If b is larger than len(h), b will be cropped from the left.
func BytesToHash(b []byte) Hash {
	var h Hash
	h.SetBytes(b)
	return h
}

// SetBytes sets the hash to the value of b.
// If b is larger than len(h), b will be cropped from the left.
func (h *Hash) SetBytes(b []byte) {
	if len(b) > len(h) {
		b = b[len(b)-HashLength:]
	}

	copy(h[HashLength-len(b):], b)
}
