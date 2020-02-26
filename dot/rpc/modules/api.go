package modules

import (
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/transaction"
)

// StorageAPI is the interface for the storage state
type StorageAPI interface{}

// BlockAPI is the interface for the block state
type BlockAPI interface{}

// NetworkAPI interface for network state methods
type NetworkAPI interface {
	Health() *common.Health
	NetworkState() *common.NetworkState
	Peers() []common.PeerInfo
}

// TransactionQueueAPI ...
type TransactionQueueAPI interface {
	Push(*transaction.ValidTransaction)
	Pop() *transaction.ValidTransaction
	Peek() *transaction.ValidTransaction
	Pending() []*transaction.ValidTransaction
}

// CoreAPI is the interface for the core methods
type CoreAPI interface{}
