package core

import (
	"errors"

	scale "github.com/ChainSafe/gossamer/codec"
	tx "github.com/ChainSafe/gossamer/common/transaction"
	"github.com/ChainSafe/gossamer/core/types"
)

// runs the extrinsic through runtime function TaggedTransactionQueue_validate_transaction
// and returns *Validity
func (s *Service) validateTransaction(e types.Extrinsic) (*tx.Validity, error) {
	var loc int32 = 1000
	s.rt.Store(e, loc)

	ret, err := s.rt.Exec("TaggedTransactionQueue_validate_transaction", loc, int32(len(e)))
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
	s.rt.Store(b, loc)

	_, err := s.rt.Exec("Core_execute_block", loc, int32(len(b)))
	if err != nil {
		return err
	}

	return nil
}
