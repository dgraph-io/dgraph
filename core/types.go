package core

import (
	"math/big"

	"github.com/ChainSafe/gossamer/common"
)

// Block defines a state block
type Block struct {
	Header BlockHeader
	Body   BlockBody
}

// BlockHeader is a state block header
type BlockHeader struct {
	ParentHash     common.Hash
	Number         *big.Int
	StateRoot      common.Hash
	ExtrinsicsRoot common.Hash
	Digest         []byte
	// TODO: Not part of spec, can potentially remove
	Hash common.Hash
}

// BlockBody is the extrinsics inside a state block
type BlockBody []byte
