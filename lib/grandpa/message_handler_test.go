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
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/scale"

	"github.com/stretchr/testify/require"
)

func TestDecodeMessage_VoteMessage(t *testing.T) {
	cm := &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              common.MustHexToBytes("0x004d000000000000006300000000000000017db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a777700000000000036e6eca85489bebbb0f687ca5404748d5aa2ffabee34e3ed272cc7b2f6d0a82c65b99bc7cd90dbc21bb528289ebf96705dbd7d96918d34d815509b4e0e2a030f34602b88f60513f1c805d87ef52896934baf6a662bc37414dbdbf69356b1a691"),
	}

	msg, err := decodeMessage(cm)
	require.NoError(t, err)

	sigb := common.MustHexToBytes("0x36e6eca85489bebbb0f687ca5404748d5aa2ffabee34e3ed272cc7b2f6d0a82c65b99bc7cd90dbc21bb528289ebf96705dbd7d96918d34d815509b4e0e2a030f")
	sig := [64]byte{}
	copy(sig[:], sigb)

	expected := &VoteMessage{
		Round: 77,
		SetID: 99,
		Stage: precommit,
		Message: &SignedMessage{
			Hash:        common.MustHexToHash("0x7db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a"),
			Number:      0x7777,
			Signature:   sig,
			AuthorityID: ed25519.PublicKeyBytes(common.MustHexToHash("0x34602b88f60513f1c805d87ef52896934baf6a662bc37414dbdbf69356b1a691")),
		},
	}

	require.Equal(t, expected, msg)
}

func TestDecodeMessage_FinalizationMessage(t *testing.T) {
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cm := &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              common.MustHexToBytes("0x024d000000000000007db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a0000000000000000040a0b0c0d00000000000000000000000000000000000000000000000000000000e7030000000000000102030400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000034602b88f60513f1c805d87ef52896934baf6a662bc37414dbdbf69356b1a691"),
	}

	msg, err := decodeMessage(cm)
	require.NoError(t, err)

	expected := &FinalizationMessage{
		Round: 77,
		Vote: &Vote{
			hash:   common.MustHexToHash("0x7db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a"),
			number: 0,
		},
		Justification: []*Justification{
			{
				Vote:        testVote,
				Signature:   testSignature,
				AuthorityID: kr.Alice.Public().(*ed25519.PublicKey).AsBytes(),
			},
		},
	}

	require.Equal(t, expected, msg)
}

func TestDecodeMessage_CatchUpRequest(t *testing.T) {
	cm := &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              common.MustHexToBytes("0x0311000000000000002200000000000000"),
	}

	msg, err := decodeMessage(cm)
	require.NoError(t, err)

	expected := &catchUpRequest{
		Round: 0x11,
		SetID: 0x22,
	}

	require.Equal(t, expected, msg)
}

func TestMessageHandler_VoteMessage(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters(t)
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Alice,
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	v, err := NewVoteFromHash(st.Block.BestBlockHash(), st.Block)
	require.NoError(t, err)

	gs.state.setID = 99
	gs.state.round = 77
	v.number = 0x7777
	vm, err := gs.createVoteMessage(v, precommit, gs.keypair)
	require.NoError(t, err)

	cm, err := vm.ToConsensusMessage()
	require.NoError(t, err)

	h := NewMessageHandler(gs, st.Block)
	out, err := h.HandleMessage(cm)
	require.NoError(t, err)
	require.Nil(t, out)

	select {
	case vote := <-gs.in:
		require.Equal(t, vm, vote)
	case <-time.After(time.Second):
		t.Fatal("did not receive VoteMessage")
	}
}

func TestMessageHandler_FinalizationMessage_NoCatchUpRequest(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters(t)
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Alice,
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	gs.state.round = 77

	gs.justification[77] = []*Justification{
		{
			Vote:        testVote,
			Signature:   testSignature,
			AuthorityID: gs.publicKeyBytes(),
		},
	}

	fm := gs.newFinalizationMessage(gs.head, 77)
	cm, err := fm.ToConsensusMessage()
	require.NoError(t, err)

	h := NewMessageHandler(gs, st.Block)
	out, err := h.HandleMessage(cm)
	require.NoError(t, err)
	require.Nil(t, out)

	hash, err := st.Block.GetFinalizedHash(0, 0)
	require.NoError(t, err)
	require.Equal(t, fm.Vote.hash, hash)

	hash, err = st.Block.GetFinalizedHash(fm.Round, gs.state.setID)
	require.NoError(t, err)
	require.Equal(t, fm.Vote.hash, hash)
}

func TestMessageHandler_FinalizationMessage_WithCatchUpRequest(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters(t)
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Alice,
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	gs.justification[77] = []*Justification{
		{
			Vote:        testVote,
			Signature:   testSignature,
			AuthorityID: gs.publicKeyBytes(),
		},
	}

	fm := gs.newFinalizationMessage(gs.head, 77)
	cm, err := fm.ToConsensusMessage()
	require.NoError(t, err)

	h := NewMessageHandler(gs, st.Block)
	out, err := h.HandleMessage(cm)
	require.NoError(t, err)
	require.NotNil(t, out)

	req := newCatchUpRequest(77, gs.state.setID)
	expected, err := req.ToConsensusMessage()
	require.NoError(t, err)
	require.Equal(t, expected, out)
}

func TestMessageHandler_CatchUpRequest_InvalidRound(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters(t)
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Alice,
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	req := newCatchUpRequest(77, 0)
	cm, err := req.ToConsensusMessage()
	require.NoError(t, err)

	h := NewMessageHandler(gs, st.Block)
	_, err = h.HandleMessage(cm)
	require.Equal(t, ErrInvalidCatchUpRound, err)
}

func TestMessageHandler_CatchUpRequest_InvalidSetID(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters(t)
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Alice,
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	req := newCatchUpRequest(1, 77)
	cm, err := req.ToConsensusMessage()
	require.NoError(t, err)

	h := NewMessageHandler(gs, st.Block)
	_, err = h.HandleMessage(cm)
	require.Equal(t, ErrSetIDMismatch, err)
}

func TestMessageHandler_CatchUpRequest_WithResponse(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters(t)
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kr.Alice,
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	// set up needed info for response
	round := uint64(1)
	setID := uint64(0)
	gs.state.round = round + 1

	testHeader := &types.Header{
		Number: big.NewInt(1),
	}

	v := &Vote{
		hash:   testHeader.Hash(),
		number: 1,
	}

	err = gs.blockState.SetFinalizedHash(testHeader.Hash(), round, setID)
	require.NoError(t, err)
	err = gs.blockState.(*state.BlockState).SetHeader(testHeader)
	require.NoError(t, err)

	pvj := []*Justification{
		{
			Vote:        testVote,
			Signature:   testSignature,
			AuthorityID: testAuthorityID,
		},
	}

	pvjEnc, err := scale.Encode(pvj)
	require.NoError(t, err)

	pcj := []*Justification{
		{
			Vote:        testVote2,
			Signature:   testSignature,
			AuthorityID: testAuthorityID,
		},
	}

	pcjEnc, err := scale.Encode(pcj)
	require.NoError(t, err)

	err = gs.blockState.SetJustification(v.hash, append(pvjEnc, pcjEnc...))
	require.NoError(t, err)

	resp, err := gs.newCatchUpResponse(round, setID)
	require.NoError(t, err)

	expected, err := resp.ToConsensusMessage()
	require.NoError(t, err)

	// create and handle request
	req := newCatchUpRequest(round, setID)
	cm, err := req.ToConsensusMessage()
	require.NoError(t, err)

	h := NewMessageHandler(gs, st.Block)
	out, err := h.HandleMessage(cm)
	require.NoError(t, err)
	require.Equal(t, expected, out)
}
