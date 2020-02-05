package core

import (
	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core/types"
)

// BlockState holds block state methods
type BlockState interface {
	LatestHeader() *types.Header
	AddBlock(*types.Block) error
	SetBlock(*types.Block) error
}

// StorageState holds storage state methods
type StorageState interface {
	StorageRoot() (common.Hash, error)
}
