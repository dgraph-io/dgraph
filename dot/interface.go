package dot

import (
	"math/big"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/services"
)

// BlockProducer is the interface that a block production service must implement
type BlockProducer interface {
	services.Service

	GetBlockChannel() <-chan types.Block
	SetRuntime(*runtime.Runtime) error
	Pause() error
	Resume() error
	Authorities() []*types.Authority
	SetAuthorities([]*types.Authority) error
	SetRandomness([types.RandomnessLength]byte)
	SetEpochThreshold(*big.Int)
}
