package grandpa

import (
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/scale"

	"github.com/stretchr/testify/require"
)

var testVote = &Vote{
	hash:   common.Hash{0xa, 0xb, 0xc, 0xd},
	number: 999,
}

var testVote2 = &Vote{
	hash:   common.Hash{0xa, 0xb, 0xc, 0xd},
	number: 333,
}

var testSignature = [64]byte{1, 2, 3, 4}
var testAuthorityID = [32]byte{5, 6, 7, 8}

func TestVoteMessageToConsensusMessage(t *testing.T) {
	gs, st := newTestService(t)

	v, err := NewVoteFromHash(st.Block.BestBlockHash(), st.Block)
	require.NoError(t, err)

	gs.state.setID = 99
	gs.state.round = 77
	v.number = 0x7777

	// test precommit
	vm, err := gs.createVoteMessage(v, precommit, gs.keypair)
	require.NoError(t, err)

	cm, err := vm.ToConsensusMessage()
	require.NoError(t, err)

	expected := &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              common.MustHexToBytes("0x014d000000000000006300000000000000017db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a7777000000000000a28633c3a1046351931209fe9182fd530dc659d54ece48e9f88f4277e47f39eb78a84d50e3d37e1b50786d88abafceb5137044b6122fb6b7b5ae8ff62787cc0e34602b88f60513f1c805d87ef52896934baf6a662bc37414dbdbf69356b1a691"),
	}

	require.Equal(t, expected, cm)

	// test prevote
	vm, err = gs.createVoteMessage(v, prevote, gs.keypair)
	require.NoError(t, err)

	cm, err = vm.ToConsensusMessage()
	require.NoError(t, err)

	expected = &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              common.MustHexToBytes("0x004d000000000000006300000000000000007db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a7777000000000000215cea37b45853e63d4cc2f0a04c7a33aec9fc5683ac46b03a01e6c41ce46e4339bb7456667f14d109b49e8af26090f7087991f3b22494df997551ae44a0ef0034602b88f60513f1c805d87ef52896934baf6a662bc37414dbdbf69356b1a691"),
	}

	require.Equal(t, expected, cm)
}

func TestFinalizationMessageToConsensusMessage(t *testing.T) {
	gs, _ := newTestService(t)
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

	expected := &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              common.MustHexToBytes("0x024d000000000000007db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a0000000000000000040a0b0c0d00000000000000000000000000000000000000000000000000000000e7030000000000000102030400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000034602b88f60513f1c805d87ef52896934baf6a662bc37414dbdbf69356b1a691"),
	}

	require.Equal(t, expected, cm)
}

func TestNewCatchUpResponse(t *testing.T) {
	gs, _ := newTestService(t)

	round := uint64(1)
	setID := uint64(1)

	testHeader := &types.Header{
		Number: big.NewInt(1),
	}

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

	expected := &catchUpResponse{
		Round:                  round,
		SetID:                  setID,
		PreVoteJustification:   FullJustification(pvj),
		PreCommitJustification: FullJustification(pcj),
		Hash:                   v.hash,
		Number:                 v.number,
	}

	require.Equal(t, expected, resp)
}
