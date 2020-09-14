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

var testHeader = &types.Header{
	ParentHash: testGenesisHeader.Hash(),
	Number:     big.NewInt(1),
}

var testHash = testHeader.Hash()

func buildTestJustifications(t *testing.T, qty int, round, setID uint64, kr *keystore.Ed25519Keyring, subround subround) []*Justification {
	just := []*Justification{}
	for i := 0; i < qty; i++ {
		j := &Justification{
			Vote:        NewVote(testHash, round),
			Signature:   createSignedVoteMsg(t, round, round, setID, kr.Keys[i%len(kr.Keys)], subround),
			AuthorityID: kr.Keys[i%len(kr.Keys)].Public().(*ed25519.PublicKey).AsBytes(),
		}
		just = append(just, j)
	}
	return just

}

func createSignedVoteMsg(t *testing.T, number, round, setID uint64, pk *ed25519.Keypair, subround subround) [64]byte {
	// create vote message
	msg, err := scale.Encode(&FullVote{
		Stage: subround,
		Vote:  NewVote(testHash, number),
		Round: round,
		SetID: setID,
	})
	require.NoError(t, err)

	var sMsgArray [64]byte
	sMsg, err := pk.Sign(msg)
	require.NoError(t, err)
	copy(sMsgArray[:], sMsg)
	return sMsgArray
}

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
				AuthorityID: kr.Alice().Public().(*ed25519.PublicKey).AsBytes(),
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
	gs, st := newTestService(t)

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

func TestMessageHandler_VerifyJustification_InvalidSig(t *testing.T) {
	gs, st := newTestService(t)
	gs.state.round = 77

	just := &Justification{
		Vote:        testVote,
		Signature:   [64]byte{0x1},
		AuthorityID: gs.publicKeyBytes(),
	}

	h := NewMessageHandler(gs, st.Block)
	err := h.verifyJustification(just, just.Vote, gs.state.round, gs.state.setID, precommit)
	require.Equal(t, err, ErrInvalidSignature)
}

func TestMessageHandler_FinalizationMessage_NoCatchUpRequest_ValidSig(t *testing.T) {
	gs, st := newTestService(t)

	round := uint64(77)
	gs.state.round = round
	gs.justification[round] = buildTestJustifications(t, int(gs.state.threshold()), round, gs.state.setID, kr, precommit)

	fm := gs.newFinalizationMessage(gs.head, round)
	fm.Vote = NewVote(testHash, round)
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

func TestMessageHandler_FinalizationMessage_NoCatchUpRequest_MinVoteError(t *testing.T) {
	gs, st := newTestService(t)

	round := uint64(77)
	gs.state.round = round

	gs.justification[round] = buildTestJustifications(t, int(gs.state.threshold()), round, gs.state.setID, kr, precommit)

	fm := gs.newFinalizationMessage(gs.head, round)
	cm, err := fm.ToConsensusMessage()
	require.NoError(t, err)

	h := NewMessageHandler(gs, st.Block)
	out, err := h.HandleMessage(cm)
	require.EqualError(t, err, ErrMinVotesNotMet.Error())
	require.Nil(t, out)
}

func TestMessageHandler_FinalizationMessage_WithCatchUpRequest(t *testing.T) {
	gs, st := newTestService(t)

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
	gs.state.voters = gs.state.voters[:1]

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
	gs, st := newTestService(t)

	req := newCatchUpRequest(77, 0)
	cm, err := req.ToConsensusMessage()
	require.NoError(t, err)

	h := NewMessageHandler(gs, st.Block)
	_, err = h.HandleMessage(cm)
	require.Equal(t, ErrInvalidCatchUpRound, err)
}

func TestMessageHandler_CatchUpRequest_InvalidSetID(t *testing.T) {
	gs, st := newTestService(t)

	req := newCatchUpRequest(1, 77)
	cm, err := req.ToConsensusMessage()
	require.NoError(t, err)

	h := NewMessageHandler(gs, st.Block)
	_, err = h.HandleMessage(cm)
	require.Equal(t, ErrSetIDMismatch, err)
}

func TestMessageHandler_CatchUpRequest_WithResponse(t *testing.T) {
	gs, st := newTestService(t)

	// set up needed info for response
	round := uint64(1)
	setID := uint64(0)
	gs.state.round = round + 1

	v := &Vote{
		hash:   testHeader.Hash(),
		number: 1,
	}

	err := gs.blockState.SetFinalizedHash(testHeader.Hash(), round, setID)
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

func TestVerifyJustification(t *testing.T) {
	gs, st := newTestService(t)
	h := NewMessageHandler(gs, st.Block)

	vote := NewVote(testHash, 123)
	just := &Justification{
		Vote:        vote,
		Signature:   createSignedVoteMsg(t, vote.number, 77, gs.state.setID, kr.Alice().(*ed25519.Keypair), precommit),
		AuthorityID: kr.Alice().Public().(*ed25519.PublicKey).AsBytes(),
	}

	err := h.verifyJustification(just, vote, 77, gs.state.setID, precommit)
	require.NoError(t, err)
}

func TestVerifyJustification_InvalidSignature(t *testing.T) {
	gs, st := newTestService(t)
	h := NewMessageHandler(gs, st.Block)

	vote := NewVote(testHash, 123)
	just := &Justification{
		Vote: vote,
		// create signed vote with mismatched vote number
		Signature:   createSignedVoteMsg(t, vote.number+1, 77, gs.state.setID, kr.Alice().(*ed25519.Keypair), precommit),
		AuthorityID: kr.Alice().Public().(*ed25519.PublicKey).AsBytes(),
	}

	err := h.verifyJustification(just, vote, 77, gs.state.setID, precommit)
	require.EqualError(t, err, ErrInvalidSignature.Error())
}

func TestVerifyJustification_InvalidAuthority(t *testing.T) {
	gs, st := newTestService(t)
	h := NewMessageHandler(gs, st.Block)
	// sign vote with key not in authority set
	fakeKey, err := ed25519.NewKeypairFromPrivateKeyString("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20")
	require.NoError(t, err)

	vote := NewVote(testHash, 123)
	just := &Justification{
		Vote:        vote,
		Signature:   createSignedVoteMsg(t, vote.number, 77, gs.state.setID, fakeKey, precommit),
		AuthorityID: fakeKey.Public().(*ed25519.PublicKey).AsBytes(),
	}

	err = h.verifyJustification(just, vote, 77, gs.state.setID, precommit)
	require.EqualError(t, err, ErrVoterNotFound.Error())
}

func TestMessageHandler_VerifyPreVoteJustification(t *testing.T) {
	gs, st := newTestService(t)
	h := NewMessageHandler(gs, st.Block)

	just := buildTestJustifications(t, int(gs.state.threshold()), 1, gs.state.setID, kr, prevote)
	msg := &catchUpResponse{
		Round:                1,
		SetID:                gs.state.setID,
		PreVoteJustification: just,
	}

	prevote, err := h.verifyPreVoteJustification(msg)
	require.NoError(t, err)
	require.Equal(t, testHash, prevote)
}

func TestMessageHandler_VerifyPreCommitJustification(t *testing.T) {
	gs, st := newTestService(t)
	h := NewMessageHandler(gs, st.Block)

	round := uint64(1)
	just := buildTestJustifications(t, int(gs.state.threshold()), round, gs.state.setID, kr, precommit)
	msg := &catchUpResponse{
		Round:                  round,
		SetID:                  gs.state.setID,
		PreCommitJustification: just,
		Hash:                   testHash,
		Number:                 round,
	}

	err := h.verifyPreCommitJustification(msg)
	require.NoError(t, err)
}

func TestMessageHandler_HandleCatchUpResponse(t *testing.T) {
	gs, st := newTestService(t)

	err := st.Block.SetHeader(testHeader)
	require.NoError(t, err)

	h := NewMessageHandler(gs, st.Block)

	round := uint64(77)
	gs.state.round = round + 1

	pvJust := buildTestJustifications(t, int(gs.state.threshold()), round, gs.state.setID, kr, prevote)
	pcJust := buildTestJustifications(t, int(gs.state.threshold()), round, gs.state.setID, kr, precommit)
	msg := &catchUpResponse{
		Round:                  round,
		SetID:                  gs.state.setID,
		PreVoteJustification:   pvJust,
		PreCommitJustification: pcJust,
		Hash:                   testHash,
		Number:                 round,
	}

	cm, err := msg.ToConsensusMessage()
	require.NoError(t, err)

	out, err := h.HandleMessage(cm)
	require.NoError(t, err)
	require.Nil(t, out)
	require.Equal(t, round+1, gs.state.round)
}
