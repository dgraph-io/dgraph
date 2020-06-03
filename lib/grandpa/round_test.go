// Copyright 2020 ChainSafe Systems (ON) Corp.
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

package grandpa

import (
	"math/rand"
	"testing"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/keystore"

	"github.com/stretchr/testify/require"
)

func onSameChain(blockState BlockState, a, b common.Hash) bool {
	descendant, err := blockState.IsDescendantOf(a, b)
	if err != nil {
		return false
	}

	if !descendant {
		descendant, err = blockState.IsDescendantOf(b, a)
		if err != nil {
			return false
		}
	}

	return descendant
}

func setupGrandpa(t *testing.T, kp *ed25519.Keypair) *Service {
	st := newTestState(t)
	voters := newTestVoters(t)

	cfg := &Config{
		BlockState: st.Block,
		Voters:     voters,
		Keypair:    kp,
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	return gs
}

func TestGrandpa_BaseCase(t *testing.T) {
	// this is a base test case that asserts that all validators finalize the same block if they all see the
	// same pre-votes and pre-commits, even if their chains are different
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gss := make([]*Service, len(kr.Keys))
	prevotes := make(map[ed25519.PublicKeyBytes]*Vote)
	precommits := make(map[ed25519.PublicKeyBytes]*Vote)

	for i, gs := range gss {
		gs = setupGrandpa(t, kr.Keys[i])
		gss[i] = gs
		state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 15)
		prevotes[gs.publicKeyBytes()], err = gs.determinePreVote()
		require.NoError(t, err)
	}

	for _, gs := range gss {
		gs.prevotes = prevotes
	}

	for _, gs := range gss {
		precommits[gs.publicKeyBytes()], err = gs.determinePreCommit()
		require.NoError(t, err)
		err = gs.finalize()
		require.NoError(t, err)
	}

	finalized := gss[0].head.Hash()
	for _, gs := range gss {
		require.Equal(t, finalized, gs.head.Hash())
	}
}

func TestGrandpa_DifferentChains(t *testing.T) {
	// this asserts that all validators finalize the same block if they all see the
	// same pre-votes and pre-commits, even if their chains are different lengths
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gss := make([]*Service, len(kr.Keys))
	prevotes := make(map[ed25519.PublicKeyBytes]*Vote)
	precommits := make(map[ed25519.PublicKeyBytes]*Vote)

	for i, gs := range gss {
		gs = setupGrandpa(t, kr.Keys[i])
		gss[i] = gs

		r := rand.Intn(3)
		state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 4+r)
		prevotes[gs.publicKeyBytes()], err = gs.determinePreVote()
		require.NoError(t, err)
	}

	// only want to add prevotes for a node that has a block that exists on its chain
	for _, gs := range gss {
		for k, pv := range prevotes {
			err = gs.validateVote(pv)
			if err == nil {
				gs.prevotes[k] = pv
			}
		}
	}

	for _, gs := range gss {
		precommits[gs.publicKeyBytes()], err = gs.determinePreCommit()
		require.NoError(t, err)
		err = gs.finalize()
		require.NoError(t, err)
	}

	t.Log(gss[0].blockState.BlocktreeAsString())
	finalized := gss[0].head

	for i, gs := range gss {
		// TODO: this can be changed to equal once attemptToFinalizeRound is implemented (needs check for >=2/3 precommits)
		headOk := onSameChain(gss[0].blockState, finalized.Hash(), gs.head.Hash())
		finalizedOK := onSameChain(gs.blockState, finalized.Hash(), gs.head.Hash())
		require.True(t, headOk || finalizedOK, "node %d did not match: %s", i, gs.blockState.BlocktreeAsString())
	}
}
