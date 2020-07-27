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
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/OneOfOne/xxhash"
)

// ErrNoPrefix is returned when trying to convert a hex-encoded string with no 0x prefix
var ErrNoPrefix = errors.New("could not byteify non 0x prefixed string")

// StringToInts turns a string consisting of ints separated by commas into an int array
func StringToInts(in string) ([]int, error) {
	intstrs := strings.Split(in, ",")
	res := []int{}
	for _, intstr := range intstrs {
		i, err := strconv.Atoi(intstr)
		if err != nil {
			return res, err
		}
		res = append(res, i)
	}
	return res, nil
}

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
	if len(in) < 2 {
		return nil, errors.New("invalid string")
	}

	if strings.Compare(in[:2], "0x") != 0 {
		return nil, ErrNoPrefix
	}
	// Ensure we have an even length, otherwise hex.DecodeString will fail and return zero hash
	if len(in)%2 != 0 {
		return nil, errors.New("cannot decode a odd length string")
	}
	in = in[2:]
	out, err := hex.DecodeString(in)
	return out, err
}

// MustHexToBytes turns a 0x prefixed hex string into a byte slice
// it panic if it cannot decode the string
func MustHexToBytes(in string) []byte {
	if len(in) < 2 {
		panic("invalid string")
	}

	if strings.Compare(in[:2], "0x") != 0 {
		panic(ErrNoPrefix)
	}

	// Ensure we have an even length, otherwise hex.DecodeString will fail and return zero hash
	if len(in)%2 != 0 {
		panic("cannot decode a odd length string")
	}

	in = in[2:]
	out, err := hex.DecodeString(in)
	if err != nil {
		panic(err)
	}

	return out
}

// BytesToHex turns a byte slice into a 0x prefixed hex string
func BytesToHex(in []byte) string {
	s := hex.EncodeToString(in)
	return "0x" + s
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

// ReadByte reads a byte from the reader and returns it
func ReadByte(r io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := r.Read(buf)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

// ReadUint32 reads a 4-byte uint32 from the reader and returns it
func ReadUint32(r io.Reader) (uint32, error) {
	buf := make([]byte, 4)
	_, err := r.Read(buf)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf), nil
}

// ReadUint64 reads an 8-byte uint32 from the reader and returns it
func ReadUint64(r io.Reader) (uint64, error) {
	buf := make([]byte, 8)
	_, err := r.Read(buf)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf), nil
}

// Read32Bytes reads 32 bytes from the reader and returns it
func Read32Bytes(r io.Reader) ([32]byte, error) {
	buf := make([]byte, 32)
	_, err := r.Read(buf)
	if err != nil {
		return [32]byte{}, err
	}
	h := [32]byte{}
	copy(h[:], buf)
	return h, nil
}

// Twox128Hash computes xxHash64 twice with seeds 0 and 1 applied on given byte array
func Twox128Hash(msg []byte) ([]byte, error) {
	// compute xxHash64 twice with seeds 0 and 1 applied on given byte array
	h0 := xxhash.NewS64(0) // create xxHash with 0 seed
	_, err := h0.Write(msg[0:])
	if err != nil {
		return nil, err
	}
	res0 := h0.Sum64()
	hash0 := make([]byte, 8)
	binary.LittleEndian.PutUint64(hash0, res0)

	h1 := xxhash.NewS64(1) // create xxHash with 1 seed
	_, err = h1.Write(msg[0:])
	if err != nil {
		return nil, err
	}
	res1 := h1.Sum64()
	hash1 := make([]byte, 8)
	binary.LittleEndian.PutUint64(hash1, res1)

	//concatenated result
	both := append(hash0, hash1...)
	return both, nil
}
