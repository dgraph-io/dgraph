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

// see: https://github.com/paritytech/substrate/blob/5420de3face1349a97eb954ae71c5b0b940c31de/core/sr-primitives/src/transaction_validity.rs#L178
type Validity struct {
	Priority  uint64
	Requires  [][]byte
	Provides  [][]byte
	Longevity uint64
	Propagate bool
}

func NewValidity(priority uint64, requires, provides [][]byte, longevity uint64, propagate bool) *Validity {
	return &Validity{
		Priority:  priority,
		Requires:  requires,
		Provides:  provides,
		Longevity: longevity,
		Propagate: propagate,
	}
}

type ValidTransaction struct {
	Extrinsic *types.Extrinsic
	Validity  *Validity
}

func NewValidTransaction(extrinsic *types.Extrinsic, validity *Validity) *ValidTransaction {
	return &ValidTransaction{
		Extrinsic: extrinsic,
		Validity:  validity,
	}
}
