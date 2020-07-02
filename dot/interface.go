package dot

import (
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
	Authorities() []*types.BABEAuthorityData
	SetAuthorities(a []*types.BABEAuthorityData)
}
