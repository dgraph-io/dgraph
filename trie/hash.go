package trie

import (
	"encoding/hex"
	"hash"

	"github.com/ChainSafe/gossamer/common"
	"golang.org/x/crypto/blake2s"
)

type Hasher struct {
	hash hash.Hash
}

func newHasher() (*Hasher, error) {
	key, err := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		return nil, err
	}

	h, err := blake2s.New256(key)
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
