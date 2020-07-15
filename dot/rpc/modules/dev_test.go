package modules

import (
	"testing"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/ChainSafe/chaindb"
	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

func newBlockState(t *testing.T) *state.BlockState {
	db := chaindb.NewMemDatabase()
	bs, err := state.NewBlockStateFromGenesis(db, genesisHeader)
	require.NoError(t, err)
	return bs
}

func newBABEService(t *testing.T) *babe.Service {
	kr, err := keystore.NewSr25519Keyring()
	require.NoError(t, err)

	bs := newBlockState(t)
	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.NODE_RUNTIME, tt, log.LvlInfo)

	err = tt.Put(common.MustHexToBytes("0x886726f904d8372fdabb7707870c2fad"), common.MustHexToBytes("0x24d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d01000000000000008eaf04151687736326c9fea17e25fc5287613693c912909cb226aa4794f26a48010000000000000090b5ab205c6974c9ea841be688864633dc9ca8a357843eeacf2314649965fe220100000000000000306721211d5404bd9da88e0204360a1a9ab8b87c66c1bc2fcdd37f3c2222cc200100000000000000e659a7a1628cdd93febc04a4e0646ea20e9f5f0ce097d9a05290d4a9e054df4e01000000000000001cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c01000000000000004603307f855321776922daeea21ee31720388d097cdaac66f05a6f8462b317570100000000000000be1d9d59de1283380100550a7b024501cb62d6cc40e3db35fcc5cf341814986e01000000000000001206960f920a23f7f4c43cc9081ec2ed0721f31a9bef2c10fd7602e16e08a32c0100000000000000"))
	require.NoError(t, err)

	cfg := &babe.ServiceConfig{
		BlockState: bs,
		Keypair:    kr.Alice,
		Runtime:    rt,
	}

	babe, err := babe.NewService(cfg)
	require.NoError(t, err)
	return babe
}

func TestDevControl_Babe(t *testing.T) {
	bs := newBABEService(t)
	m := NewDevModule(bs, nil)

	var res string
	err := m.Control(nil, &[]string{"babe", "stop"}, &res)
	require.NoError(t, err)
	require.Equal(t, blockProducerStoppedMsg, res)
	require.True(t, bs.IsStopped())

	err = m.Control(nil, &[]string{"babe", "start"}, &res)
	require.NoError(t, err)
	require.Equal(t, blockProducerStartedMsg, res)
	require.False(t, bs.IsStopped())
}

func TestDevControl_Network(t *testing.T) {
	net := newNetworkService(t)
	m := NewDevModule(nil, net)

	var res string
	err := m.Control(nil, &[]string{"network", "stop"}, &res)
	require.NoError(t, err)
	require.Equal(t, networkStoppedMsg, res)
	require.True(t, net.IsStopped())

	err = m.Control(nil, &[]string{"network", "start"}, &res)
	require.NoError(t, err)
	require.Equal(t, networkStartedMsg, res)
	require.False(t, net.IsStopped())
}
