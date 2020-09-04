package modules

import (
	"testing"

	"github.com/ChainSafe/chaindb"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/trie"
	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

func newState(t *testing.T) (*state.BlockState, *state.EpochState) {
	db := chaindb.NewMemDatabase()
	bs, err := state.NewBlockStateFromGenesis(db, genesisHeader)
	require.NoError(t, err)
	es, err := state.NewEpochStateFromGenesis(db, &types.EpochInfo{
		Duration: 200,
	})
	require.NoError(t, err)
	return bs, es
}

func newBABEService(t *testing.T) *babe.Service {
	kr, err := keystore.NewSr25519Keyring()
	require.NoError(t, err)

	bs, es := newState(t)
	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.NODE_RUNTIME, tt, log.LvlInfo)

	err = tt.Put(common.MustHexToBytes("0x886726f904d8372fdabb7707870c2fad"), common.MustHexToBytes("0x24d43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d01000000000000008eaf04151687736326c9fea17e25fc5287613693c912909cb226aa4794f26a48010000000000000090b5ab205c6974c9ea841be688864633dc9ca8a357843eeacf2314649965fe220100000000000000306721211d5404bd9da88e0204360a1a9ab8b87c66c1bc2fcdd37f3c2222cc200100000000000000e659a7a1628cdd93febc04a4e0646ea20e9f5f0ce097d9a05290d4a9e054df4e01000000000000001cbd2d43530a44705ad088af313e18f80b53ef16b36177cd4b77b846f2a5f07c01000000000000004603307f855321776922daeea21ee31720388d097cdaac66f05a6f8462b317570100000000000000be1d9d59de1283380100550a7b024501cb62d6cc40e3db35fcc5cf341814986e01000000000000001206960f920a23f7f4c43cc9081ec2ed0721f31a9bef2c10fd7602e16e08a32c0100000000000000"))
	require.NoError(t, err)

	cfg := &babe.ServiceConfig{
		BlockState: bs,
		EpochState: es,
		Keypair:    kr.Alice().(*sr25519.Keypair),
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
	require.True(t, bs.IsPaused())

	err = m.Control(nil, &[]string{"babe", "start"}, &res)
	require.NoError(t, err)
	require.Equal(t, blockProducerStartedMsg, res)
	require.False(t, bs.IsPaused())
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

func TestDevModule_SetBlockProducerAuthorities(t *testing.T) {
	bs := newBABEService(t)
	m := NewDevModule(bs, nil)
	aBefore := bs.Authorities()
	req := &[]interface{}{[]interface{}{"5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY", float64(1)}}
	var res string
	err := m.SetBlockProducerAuthorities(nil, req, &res)
	require.NoError(t, err)
	require.Equal(t, "set 1 block producer authorities", res)
	aAfter := bs.Authorities()
	// authorities before and after should be different since they were changed
	require.NotEqual(t, aBefore, aAfter)
}

func TestDevModule_SetBlockProducerAuthorities_NotFound(t *testing.T) {
	bs := newBABEService(t)
	m := NewDevModule(bs, nil)
	aBefore := bs.Authorities()

	req := &[]interface{}{[]interface{}{"5FrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY", float64(1)}}
	var res string
	err := m.SetBlockProducerAuthorities(nil, req, &res)
	require.EqualError(t, err, "key not in BABE authority data")
	aAfter := bs.Authorities()
	// authorities before and after should be equal since they should not have changed (due to key error)
	require.Equal(t, aBefore, aAfter)
}

func TestDevModule_SetBABEEpochThreshold(t *testing.T) {
	bs := newBABEService(t)
	m := NewDevModule(bs, nil)
	req := "123"
	var res string
	err := m.SetBABEEpochThreshold(nil, &req, &res)
	require.NoError(t, err)

	require.Equal(t, "set BABE Epoch Threshold to 123", res)
}

func TestDevModule_SetBABERandomness(t *testing.T) {
	bs := newBABEService(t)
	m := NewDevModule(bs, nil)
	req := &[]string{"0x000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"}
	var res string
	err := m.SetBABERandomness(nil, req, &res)
	require.NoError(t, err)
}

func TestDevModule_SetBABERandomness_WrongLength(t *testing.T) {
	bs := newBABEService(t)
	m := NewDevModule(bs, nil)
	req := &[]string{"0x0001"}
	var res string
	err := m.SetBABERandomness(nil, req, &res)
	require.EqualError(t, err, "expected randomness value of 32 bytes, received 2 bytes")
}
