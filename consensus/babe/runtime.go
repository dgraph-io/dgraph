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

package babe

import (
	"errors"

	scale "github.com/ChainSafe/gossamer/codec"
	tx "github.com/ChainSafe/gossamer/common/transaction"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/runtime"
)

// gets the configuration data for Babe from the runtime
func (b *Session) configurationFromRuntime() error {
	ret, err := b.rt.Exec(runtime.BabeApiConfiguration, 1, []byte{})
	if err != nil {
		return err
	}

	bc := new(BabeConfiguration)
	_, err = scale.Decode(ret, bc)
	if err != nil {
		return err
	}

	// Directly set the babe session's config
	b.config = bc

	return err
}

// calls runtime API function Core_initialize_block
func (b *Session) initializeBlock(blockHeader []byte) error {
	// TODO: use allocator to store block header
	var loc int32 = 1
	b.rt.Store(blockHeader, loc)

	_, err := b.rt.Exec(runtime.CoreInitializeBlock, loc, blockHeader)
	if err != nil {
		return err
	}

	return nil
}

// calls runtime API function BlockBuilder_inherent_extrinsics
func (b *Session) inherentExtrinsics(blockInherentData []byte) ([]byte, error) { //nolint:unused
	// TODO: use allocator to store inherents data
	var loc int32 = 1
	b.rt.Store(blockInherentData, loc)

	return b.rt.Exec(runtime.BlockBuilderInherentExtrinsics, loc, blockInherentData)
}

// calls runtime API function BlockBuilder_apply_extrinsic
func (b *Session) applyExtrinsic(e types.Extrinsic) ([]byte, error) { //nolint:unused
	// TODO: use allocator to store extrinsic
	var loc int32 = 1
	b.rt.Store(e, loc)

	return b.rt.Exec(runtime.BlockBuilderApplyExtrinsic, loc, e)
}

// calls runtime API function BlockBuilder_finalize_block
func (b *Session) finalizeBlock() (*types.Block, error) {
	ret, err := b.rt.Exec(runtime.BlockBuilderFinalizeBlock, 0, []byte{})
	if err != nil {
		return nil, err
	}

	bh := &types.Block{
		Header: new(types.BlockHeader),
		Body:   new(types.BlockBody),
	}

	_, err = scale.Decode(ret, bh)
	return bh, err
}

// calls runtime API function TaggedTransactionQueue_validate_transaction
func (b *Session) validateTransaction(e types.Extrinsic) (*tx.Validity, error) {
	// TODO: use allocator to store extrinsic
	var loc int32 = 1000
	b.rt.Store(e, loc)

	ret, err := b.rt.Exec(runtime.TaggedTransactionQueueValidateTransaction, loc, e)
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
