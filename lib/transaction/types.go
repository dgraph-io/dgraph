// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package transaction

import (
	"encoding/binary"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/scale"
)

// Validity struct see: https://github.com/paritytech/substrate/blob/5420de3face1349a97eb954ae71c5b0b940c31de/core/sr-primitives/src/transaction_validity.rs#L178
type Validity struct {
	Priority  uint64
	Requires  [][]byte
	Provides  [][]byte
	Longevity uint64
	Propagate bool
}

// NewValidity returns Validity
func NewValidity(priority uint64, requires, provides [][]byte, longevity uint64, propagate bool) *Validity {
	return &Validity{
		Priority:  priority,
		Requires:  requires,
		Provides:  provides,
		Longevity: longevity,
		Propagate: propagate,
	}
}

// ValidTransaction struct
type ValidTransaction struct {
	Extrinsic types.Extrinsic
	Validity  *Validity
}

// NewValidTransaction returns ValidTransaction
func NewValidTransaction(extrinsic types.Extrinsic, validity *Validity) *ValidTransaction {
	return &ValidTransaction{
		Extrinsic: extrinsic,
		Validity:  validity,
	}
}

// Encode SCALE encodes the transaction
func (vt *ValidTransaction) Encode() ([]byte, error) {
	enc := []byte(vt.Extrinsic)

	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, vt.Validity.Priority)
	enc = append(enc, buf...)

	d, err := scale.Encode(vt.Validity.Requires)
	if err != nil {
		return nil, err
	}
	enc = append(enc, d...)

	d, err = scale.Encode(vt.Validity.Provides)
	if err != nil {
		return nil, err
	}
	enc = append(enc, d...)

	binary.LittleEndian.PutUint64(buf, vt.Validity.Longevity)
	enc = append(enc, buf...)

	if vt.Validity.Propagate {
		enc = append(enc, 1)
	} else {
		enc = append(enc, 0)
	}

	return enc, nil
}
