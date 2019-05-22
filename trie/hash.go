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

package trie

import (
	"hash"

	"github.com/ChainSafe/gossamer/common"
	"golang.org/x/crypto/blake2b"
)

// Hasher is a wrapper around a hash function
type Hasher struct {
	hash hash.Hash
}

func newHasher() (*Hasher, error) {
	h, err := blake2b.New256(nil)
	if err != nil {
		return nil, err
	}

	return &Hasher{
		hash: h,
	}, nil
}

// Hash encodes the node and then hashes it if its encoded length is > 32 bytes
func (h *Hasher) Hash(n node) (res []byte, err error) {
	encNode, err := n.Encode()
	if err != nil {
		return nil, err
	}

	// if length of encoded leaf is less than 32 bytes, do not hash
	if len(encNode) < 32 {
		return common.AppendZeroes(encNode, 32), nil
	}

	// otherwise, hash encoded node
	_, err = h.hash.Write(encNode)
	if err == nil {
		res = h.hash.Sum(nil)
	}

	return res, err
}
