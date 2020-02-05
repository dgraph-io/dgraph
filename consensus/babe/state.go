package babe

import (
	"github.com/ChainSafe/gossamer/core/types"
)

type BlockState interface {
	LatestHeader() *types.Header
	AddBlock(*types.Block) error
}
