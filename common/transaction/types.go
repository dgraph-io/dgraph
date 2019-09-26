package transaction

import (
	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/core/types"
)

type Pool map[common.Hash]*ValidTransaction

type Queue interface {
	Pop() *ValidTransaction
	Insert(vt *ValidTransaction)
}

type TransactionTag []byte

// see: https://github.com/paritytech/substrate/blob/5420de3face1349a97eb954ae71c5b0b940c31de/core/sr-primitives/src/transaction_validity.rs#L178
type Validity struct {
	priority uint64
	// requires  []TransactionTag
	// provides  []TransactionTag
	// longevity uint64
	// propagate bool
}

type ValidTransaction struct {
	extrinsic *types.Extrinsic
	validity  *Validity
}

// NewValidTransaction creates a new ValidTransaction contains an extrinsic and Validity
func NewValidTransaction(extrinsic *types.Extrinsic, validity *Validity) *ValidTransaction {
	return &ValidTransaction{
		extrinsic: extrinsic,
		validity:  validity,
	}
}
