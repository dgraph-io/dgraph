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
	"testing"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common"

	"github.com/stretchr/testify/require"
)

func TestInitiateEpoch(t *testing.T) {
	bs := createTestService(t, nil)
	bs.config.EpochLength = testEpochLength

	// epoch 1
	err := bs.initiateEpoch(1, testEpochLength+1)
	require.NoError(t, err)

	// add blocks w/ babe header to state
	parent := genesisHeader
	for i := 1; i < int(testEpochLength*2+1); i++ {
		block, _ := createTestBlock(t, bs, parent, nil, uint64(i))
		err = bs.blockState.AddBlock(block)
		require.NoError(t, err)
		parent = block.Header
	}

	// epoch 2
	state.AddBlocksToState(t, bs.blockState.(*state.BlockState), int(testEpochLength*2))
	err = bs.initiateEpoch(2, testEpochLength*2+1)
	require.NoError(t, err)

	// assert epoch info was stored
	has, err := bs.epochState.HasEpochInfo(1)
	require.NoError(t, err)
	require.True(t, has)

	has, err = bs.epochState.HasEpochInfo(2)
	require.NoError(t, err)
	require.True(t, has)

	// assert slot lottery was run for epochs 0, 1 and 2
	require.Equal(t, int(testEpochLength*3), len(bs.slotToProof))
}

func TestEpochRandomness(t *testing.T) {
	bs := createTestService(t, nil)
	parent := genesisHeader

	epoch := 3
	buf := append(bs.randomness[:], []byte{byte(epoch), 0, 0, 0, 0, 0, 0, 0}...)

	for i := 1; i < int(testEpochLength*2+1); i++ {
		block, _ := createTestBlock(t, bs, parent, nil, uint64(i))
		err := bs.blockState.AddBlock(block)
		require.NoError(t, err)

		if uint64(i) <= testEpochLength {
			out, err := getVRFOutput(block.Header)
			require.NoError(t, err)
			buf = append(buf, out[:]...)
		}

		parent = block.Header
	}

	rand, err := bs.epochRandomness(uint64(epoch))
	require.NoError(t, err)
	expected, err := common.Blake2bHash(buf)
	require.NoError(t, err)
	require.Equal(t, expected[:], rand[:])
}

func TestGetVRFOutput(t *testing.T) {
	bs := createTestService(t, nil)
	block, _ := createTestBlock(t, bs, genesisHeader, nil, 1)
	out, err := getVRFOutput(block.Header)
	require.NoError(t, err)
	require.Equal(t, bs.slotToProof[1].output, out)
}

func TestIncrementEpoch(t *testing.T) {
	bs := createTestService(t, nil)
	next, err := bs.incrementEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(2), next)

	next, err = bs.incrementEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(3), next)

	epoch, err := bs.epochState.GetCurrentEpoch()
	require.NoError(t, err)
	require.Equal(t, uint64(3), epoch)
}
