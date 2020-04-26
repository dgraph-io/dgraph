package modules

import (
	"math/big"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/transaction"
)

// StorageAPI is the interface for the storage state
type StorageAPI interface{}

// BlockAPI is the interface for the block state
type BlockAPI interface {
	GetHeader(hash common.Hash) (*types.Header, error)
	HighestBlockHash() common.Hash
	GetBlockByHash(hash common.Hash) (*types.Block, error)
	GetBlockHash(blockNumber *big.Int) (*common.Hash, error)
}

// NetworkAPI interface for network state methods
type NetworkAPI interface {
	Health() common.Health
	NetworkState() common.NetworkState
	Peers() []common.PeerInfo
}

// TransactionQueueAPI ...
type TransactionQueueAPI interface {
	Push(*transaction.ValidTransaction) (common.Hash, error)
	Pop() *transaction.ValidTransaction
	Peek() *transaction.ValidTransaction
	Pending() []*transaction.ValidTransaction
}

// CoreAPI is the interface for the core methods
type CoreAPI interface {
	InsertKey(kp crypto.Keypair)
	GetRuntimeVersion() (*runtime.VersionAPI, error)
	ValidateTransaction(e types.Extrinsic) (*transaction.Validity, error)
	IsBabeAuthority() bool
}
