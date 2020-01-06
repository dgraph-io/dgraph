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
	scale "github.com/ChainSafe/gossamer/codec"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/runtime"
)

// gets the configuration data for Babe from the runtime
func (b *Session) configurationFromRuntime() error {
	data, err := b.rt.Exec(runtime.BabeApiConfiguration, []byte{})
	if err != nil {
		return err
	}

	bc := new(BabeConfiguration)
	_, err = scale.Decode(data, bc)
	if err != nil {
		return err
	}

	// Directly set the babe session's config
	b.config = bc

	return err
}

// calls runtime API function Core_initialize_block
func (b *Session) initializeBlock(blockHeader []byte) error {
	_, err := b.rt.Exec(runtime.CoreInitializeBlock, blockHeader)
	return err
}

// calls runtime API function BlockBuilder_inherent_extrinsics
func (b *Session) inherentExtrinsics(data []byte) ([]byte, error) {
	return b.rt.Exec(runtime.BlockBuilderInherentExtrinsics, data)
}

// calls runtime API function BlockBuilder_apply_extrinsic
func (b *Session) applyExtrinsic(data types.Extrinsic) ([]byte, error) {
	return b.rt.Exec(runtime.BlockBuilderApplyExtrinsic, data)
}

// calls runtime API function BlockBuilder_finalize_block
func (b *Session) finalizeBlock() (*types.Block, error) {
	data, err := b.rt.Exec(runtime.BlockBuilderFinalizeBlock, []byte{})
	if err != nil {
		return nil, err
	}

	bh := &types.Block{
		Header: new(types.BlockHeader),
		Body:   new(types.BlockBody),
	}

	_, err = scale.Decode(data, bh)
	return bh, err
}
