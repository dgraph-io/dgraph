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
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/core/types"

	"github.com/stretchr/testify/require"
)

// test handleBlockDigest
func TestHandleBlockDigest(t *testing.T) {
	s := newTestServiceWithFirstBlock(t)

	number, err := s.blockState.BestBlockNumber()
	require.Nil(t, err)

	header, err := s.blockState.BestBlockHeader()
	require.Nil(t, err)

	err = s.handleBlockDigest(header)
	require.Nil(t, err)

	require.Equal(t, number, s.firstBlock)

	// test two blocks claiming to be first block
	s.firstBlock = big.NewInt(1)

	err = s.handleBlockDigest(header)
	require.NotNil(t, err) // expect error: "first block already set for current epoch"

	// expect first block not to be updated
	require.Equal(t, s.firstBlock, big.NewInt(1))

	// test two blocks claiming to be first block
	s.firstBlock = big.NewInt(99)

	err = s.handleBlockDigest(header)
	require.Nil(t, err)

	// expect first block to be updated
	require.Equal(t, s.firstBlock, number)
}

// test handleConsensusDigest
func TestHandleConsensusDigest(t *testing.T) {
	s := newTestServiceWithFirstBlock(t)

	number, err := s.blockState.BestBlockNumber()
	require.Nil(t, err)

	header, err := s.blockState.BestBlockHeader()
	require.Nil(t, err)

	var item types.DigestItem

	for _, digest := range header.Digest {
		item, err = types.DecodeDigestItem(digest)
		require.Nil(t, err)
	}

	// check if digest item is consensus digest type
	if item.Type() == types.ConsensusDigestType {
		digest := item.(*types.ConsensusDigest)

		err = s.handleConsensusDigest(header, digest)
		require.Nil(t, err)
	}

	require.Equal(t, number, s.firstBlock)
}

// test setNextEpochDescriptor
func TestSetNextEpochDescriptor(t *testing.T) {
	s := newTestServiceWithFirstBlock(t)

	header, err := s.blockState.BestBlockHeader()
	require.Nil(t, err)

	var item types.DigestItem

	for _, digest := range header.Digest {
		item, err = types.DecodeDigestItem(digest)
		require.Nil(t, err)
	}

	// check if digest item is consensus digest type
	if item.Type() == types.ConsensusDigestType {
		digest := item.(*types.ConsensusDigest)

		err = s.setNextEpochDescriptor(digest.Data)
		require.Nil(t, err)
	}
}
