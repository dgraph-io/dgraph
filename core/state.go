package core

import (
	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core/types"
)

type BlockState interface {
	LatestHeader() *types.Header
	AddBlock(types.Block) error
}

type StorageState interface {
	StorageRoot() (common.Hash, error)
}
