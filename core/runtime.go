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

package core

import (
	"errors"

	"github.com/ChainSafe/gossamer/runtime"

	scale "github.com/ChainSafe/gossamer/codec"
	tx "github.com/ChainSafe/gossamer/common/transaction"
	"github.com/ChainSafe/gossamer/core/types"
)

// runs the extrinsic through runtime function TaggedTransactionQueue_validate_transaction
// and returns *Validity
func (s *Service) validateTransaction(e types.Extrinsic) (*tx.Validity, error) {
	var loc int32 = 1000

	ret, err := s.rt.Exec(runtime.TaggedTransactionQueueValidateTransaction, loc, e)
	if err != nil {
		return nil, err
	}

	if ret[0] != 0 {
		return nil, errors.New("could not validate transaction")
	}

	v := tx.NewValidity(0, [][]byte{{}}, [][]byte{{}}, 0, false)
	_, err = scale.Decode(ret[1:], v)

	return v, err
}

// runs the block through runtime function Core_execute_block
// doesn't return data, but will error if the call isn't successful
func (s *Service) validateBlock(b []byte) error {
	var loc int32 = 1000

	_, err := s.rt.Exec(runtime.CoreExecuteBlock, loc, b)
	if err != nil {
		return err
	}

	return nil
}
