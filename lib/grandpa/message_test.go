package grandpa

import (
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/keystore"

	"github.com/stretchr/testify/require"
)

func TestDecodeMessage_VoteMessage(t *testing.T) {
	gs := &Service{}

	cm := &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              common.MustHexToBytes("0x004d000000000000006300000000000000017db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a777700000000000050de12b09722c4676f022d7200001b90183b3cf7e2e0a5ec009859b3c0956db6ccf35ac019ff5fd73640e3f0dcf658a92b56842b7821f4b7e77eb891931d370034602b88f60513f1c805d87ef52896934baf6a662bc37414dbdbf69356b1a691"),
	}

	msg, err := gs.DecodeMessage(cm)
	require.NoError(t, err)

	sigb := common.MustHexToBytes("0x50de12b09722c4676f022d7200001b90183b3cf7e2e0a5ec009859b3c0956db6ccf35ac019ff5fd73640e3f0dcf658a92b56842b7821f4b7e77eb891931d3700")
	sig := [64]byte{}
	copy(sig[:], sigb)

	expected := &VoteMessage{
		SetID: 77,
		Round: 99,
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
	gs := &Service{}

	cm := &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              common.MustHexToBytes("0x014d000000000000007db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a0100000000000000"),
	}

	msg, err := gs.DecodeMessage(cm)
	require.NoError(t, err)

	expected := &FinalizationMessage{
		Round: 77,
		Vote: &Vote{
			hash:   common.MustHexToHash("0x7db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a"),
			number: 1,
		},
	}

	require.Equal(t, expected, msg)
}

func TestVoteMessageToConsensusMessage(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters(t)
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState: st.Block,
		Voters:     voters,
		Keypair:    kr.Alice,
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	v, err := NewVoteFromHash(st.Block.BestBlockHash(), st.Block)
	require.NoError(t, err)

	gs.state.setID = 77
	gs.state.round = 99
	v.number = 0x7777
	vm, err := gs.createVoteMessage(v, precommit, gs.keypair)
	require.NoError(t, err)

	cm, err := vm.ToConsensusMessage()
	require.NoError(t, err)

	expected := &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              common.MustHexToBytes("0x004d000000000000006300000000000000017db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a777700000000000036e6eca85489bebbb0f687ca5404748d5aa2ffabee34e3ed272cc7b2f6d0a82c65b99bc7cd90dbc21bb528289ebf96705dbd7d96918d34d815509b4e0e2a030f34602b88f60513f1c805d87ef52896934baf6a662bc37414dbdbf69356b1a691"),
	}

	require.Equal(t, expected, cm)
}

func TestFinalizationMessageToConsensusMessage(t *testing.T) {
	st := newTestState(t)
	voters := newTestVoters(t)
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	cfg := &Config{
		BlockState: st.Block,
		Voters:     voters,
		Keypair:    kr.Alice,
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)

	fm, err := gs.newFinalizationMessage(gs.head, 77)
	require.NoError(t, err)

	cm, err := fm.ToConsensusMessage()
	require.NoError(t, err)

	expected := &ConsensusMessage{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              common.MustHexToBytes("0x014d000000000000007db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a0000000000000000"),
	}

	require.Equal(t, expected, cm)
}
