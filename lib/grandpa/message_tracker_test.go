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
	"time"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/keystore"

	"github.com/stretchr/testify/require"
)

func TestMessageTracker_ValidateMessage(t *testing.T) {
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gs, _, _, _ := setupGrandpa(t, kr.Bob().(*ed25519.Keypair))
	state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 3)
	gs.tracker, err = newTracker(gs.blockState, gs.in)
	require.NoError(t, err)
	gs.tracker.start()

	fake := &types.Header{
		Number: big.NewInt(77),
	}

	msg, err := gs.createVoteMessage(NewVoteFromHeader(fake), prevote, kr.Alice())
	require.NoError(t, err)

	_, err = gs.validateMessage(msg)
	require.Equal(t, err, ErrBlockDoesNotExist)
	require.Equal(t, []*VoteMessage{msg}, gs.tracker.messages[fake.Hash()])
}

func TestMessageTracker_SendMessage(t *testing.T) {
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gs, in, _, _ := setupGrandpa(t, kr.Bob().(*ed25519.Keypair))
	state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 3)
	gs.tracker, err = newTracker(gs.blockState, gs.in)
	require.NoError(t, err)
	gs.tracker.start()

	parent, err := gs.blockState.BestBlockHeader()
	require.NoError(t, err)

	next := &types.Header{
		ParentHash: parent.Hash(),
		Number:     big.NewInt(4),
	}

	msg, err := gs.createVoteMessage(NewVoteFromHeader(next), prevote, kr.Alice())
	require.NoError(t, err)

	_, err = gs.validateMessage(msg)
	require.Equal(t, err, ErrBlockDoesNotExist)
	require.Equal(t, []*VoteMessage{msg}, gs.tracker.messages[next.Hash()])

	err = gs.blockState.(*state.BlockState).AddBlock(&types.Block{
		Header: next,
		Body:   &types.Body{},
	})
	require.NoError(t, err)

	select {
	case v := <-in:
		require.Equal(t, msg, v)
	case <-time.After(testTimeout):
		t.Errorf("did not receive vote message")
	}
}

func TestMessageTracker_ProcessMessage(t *testing.T) {
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gs, _, _, _ := setupGrandpa(t, kr.Bob().(*ed25519.Keypair))
	state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 3)
	gs.Start()
	time.Sleep(time.Second) // wait for round to initiate

	parent, err := gs.blockState.BestBlockHeader()
	require.NoError(t, err)

	next := &types.Header{
		ParentHash: parent.Hash(),
		Number:     big.NewInt(4),
	}

	msg, err := gs.createVoteMessage(NewVoteFromHeader(next), prevote, kr.Alice())
	require.NoError(t, err)

	_, err = gs.validateMessage(msg)
	require.Equal(t, ErrBlockDoesNotExist, err)
	require.Equal(t, []*VoteMessage{msg}, gs.tracker.messages[next.Hash()])

	err = gs.blockState.(*state.BlockState).AddBlock(&types.Block{
		Header: next,
		Body:   &types.Body{},
	})
	require.NoError(t, err)

	time.Sleep(time.Second)
	expected := &Vote{
		hash:   msg.Message.Hash,
		number: msg.Message.Number,
	}
	require.Equal(t, expected, gs.prevotes[kr.Alice().Public().(*ed25519.PublicKey).AsBytes()], gs.tracker.messages)
}
