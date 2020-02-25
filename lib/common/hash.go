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
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/sha3"
)

// Blake2b128 returns the 128-bit blake2b hash of the input data
func Blake2b128(in []byte) ([]byte, error) {
	hasher, err := blake2b.New(16, nil)
	if err != nil {
		return nil, err
	}

	return hasher.Sum(in)[:16], nil
}

// Blake2bHash returns the 256-bit blake2b hash of the input data
func Blake2bHash(in []byte) (Hash, error) {
	h, err := blake2b.New256(nil)
	if err != nil {
		return [32]byte{}, err
	}

	var res []byte
	_, err = h.Write(in)
	if err != nil {
		return [32]byte{}, err
	}

	res = h.Sum(nil)
	var buf = [32]byte{}
	copy(buf[:], res)
	return buf, err
}

// Keccak256 returns the keccak256 hash of the input data
func Keccak256(in []byte) Hash {
	h := sha3.NewLegacyKeccak256()
	hash := h.Sum(in)
	var buf = [32]byte{}
	copy(buf[:], hash)
	return buf
}
