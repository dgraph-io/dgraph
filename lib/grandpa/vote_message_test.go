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
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/keystore"

	"github.com/stretchr/testify/require"
)

func TestCheckForEquivocation_NoEquivocation(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Bob().(*ed25519.Keypair),
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)
	state.AddBlocksToState(t, st.Block, 3)

	h, err := st.Block.BestBlockHeader()
	require.NoError(t, err)

	vote := NewVoteFromHeader(h)
	require.NoError(t, err)

	for _, v := range voters {
		equivocated := gs.checkForEquivocation(v, vote, prevote)
		require.False(t, equivocated)
	}
}

func TestCheckForEquivocation_WithEquivocation(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	st := newTestState(t)
	voters := newTestVoters()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Bob().(*ed25519.Keypair),
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	branches := make(map[int]int)
	branches[6] = 1
	state.AddBlocksToStateWithFixedBranches(t, st.Block, 8, branches, 0)
	leaves := gs.blockState.Leaves()

	h, err := st.Block.BestBlockHeader()
	require.NoError(t, err)

	vote := NewVoteFromHeader(h)
	require.NoError(t, err)

	voter := voters[0]

	gs.prevotes[voter.key.AsBytes()] = vote

	vote2, err := NewVoteFromHash(leaves[1], st.Block)
	require.NoError(t, err)

	equivocated := gs.checkForEquivocation(voter, vote2, prevote)
	require.True(t, equivocated)

	require.Equal(t, 0, len(gs.prevotes))
	require.Equal(t, 1, len(gs.pvEquivocations))
	require.Equal(t, 2, len(gs.pvEquivocations[voter.key.AsBytes()]))
}

func TestCheckForEquivocation_WithExistingEquivocation(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Bob().(*ed25519.Keypair),
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	var branches []*types.Header
	for {
		_, branches = state.AddBlocksToState(t, st.Block, 8)
		if len(branches) > 1 {
			break
		}
	}

	h, err := st.Block.BestBlockHeader()
	require.NoError(t, err)

	vote := NewVoteFromHeader(h)
	require.NoError(t, err)

	voter := voters[0]

	gs.prevotes[voter.key.AsBytes()] = vote

	vote2 := NewVoteFromHeader(branches[0])
	require.NoError(t, err)

	equivocated := gs.checkForEquivocation(voter, vote2, prevote)
	require.True(t, equivocated)

	require.Equal(t, 0, len(gs.prevotes))
	require.Equal(t, 1, len(gs.pvEquivocations))

	vote3 := NewVoteFromHeader(branches[1])
	require.NoError(t, err)

	equivocated = gs.checkForEquivocation(voter, vote3, prevote)
	require.True(t, equivocated)

	require.Equal(t, 0, len(gs.prevotes))
	require.Equal(t, 1, len(gs.pvEquivocations))
	require.Equal(t, 3, len(gs.pvEquivocations[voter.key.AsBytes()]))
}

func TestValidateMessage_Valid(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Bob().(*ed25519.Keypair),
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)
	state.AddBlocksToState(t, st.Block, 3)

	h, err := st.Block.BestBlockHeader()
	require.NoError(t, err)

	msg, err := gs.createVoteMessage(NewVoteFromHeader(h), prevote, kr.Alice())
	require.NoError(t, err)

	vote, err := gs.validateMessage(msg)
	require.NoError(t, err)
	require.Equal(t, h.Hash(), vote.hash)
}

func TestValidateMessage_InvalidSignature(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Bob().(*ed25519.Keypair),
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)
	state.AddBlocksToState(t, st.Block, 3)

	h, err := st.Block.BestBlockHeader()
	require.NoError(t, err)

	msg, err := gs.createVoteMessage(NewVoteFromHeader(h), prevote, kr.Alice())
	require.NoError(t, err)

	msg.Message.Signature[63] = 0

	_, err = gs.validateMessage(msg)
	require.Equal(t, err, ErrInvalidSignature)
}

func TestValidateMessage_SetIDMismatch(t *testing.T) {
	st := newTestState(t)

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Keypair:       kr.Bob().(*ed25519.Keypair),
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)
	state.AddBlocksToState(t, st.Block, 3)

	h, err := st.Block.BestBlockHeader()
	require.NoError(t, err)

	msg, err := gs.createVoteMessage(NewVoteFromHeader(h), prevote, kr.Alice())
	require.NoError(t, err)

	gs.state.setID = 1

	_, err = gs.validateMessage(msg)
	require.Equal(t, err, ErrSetIDMismatch)
}

func TestValidateMessage_Equivocation(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Bob().(*ed25519.Keypair),
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	var branches []*types.Header
	for {
		_, branches = state.AddBlocksToState(t, st.Block, 8)
		if len(branches) != 0 {
			break
		}
	}

	h, err := st.Block.BestBlockHeader()
	require.NoError(t, err)

	vote := NewVoteFromHeader(h)
	require.NoError(t, err)

	voter := voters[0]

	gs.prevotes[voter.key.AsBytes()] = vote

	msg, err := gs.createVoteMessage(NewVoteFromHeader(branches[0]), prevote, kr.Alice())
	require.NoError(t, err)

	_, err = gs.validateMessage(msg)
	require.Equal(t, ErrEquivocation, err, gs.prevotes)
}

func TestValidateMessage_BlockDoesNotExist(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Bob().(*ed25519.Keypair),
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)
	state.AddBlocksToState(t, st.Block, 3)
	gs.tracker, err = newTracker(st.Block, gs.in)
	require.NoError(t, err)

	fake := &types.Header{
		Number: big.NewInt(77),
	}

	msg, err := gs.createVoteMessage(NewVoteFromHeader(fake), prevote, kr.Alice())
	require.NoError(t, err)

	_, err = gs.validateMessage(msg)
	require.Equal(t, err, ErrBlockDoesNotExist)
}

func TestValidateMessage_IsNotDescendant(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Bob().(*ed25519.Keypair),
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	var branches []*types.Header
	for {
		_, branches = state.AddBlocksToState(t, st.Block, 8)
		if len(branches) != 0 {
			break
		}
	}

	h, err := st.Block.BestBlockHeader()
	require.NoError(t, err)
	gs.head = h

	msg, err := gs.createVoteMessage(NewVoteFromHeader(branches[0]), prevote, kr.Alice())
	require.NoError(t, err)

	_, err = gs.validateMessage(msg)
	require.Equal(t, ErrDescendantNotFound, err, gs.prevotes)
}
