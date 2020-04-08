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

package core

import (
	"math/big"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/tests"

	"github.com/stretchr/testify/require"
)

// testMessageTimeout is the wait time for messages to be exchanged
var testMessageTimeout = time.Second

// testGenesisHeader is a test block header
var testGenesisHeader = &types.Header{
	Number:    big.NewInt(0),
	StateRoot: trie.EmptyHash,
}

// newTestService creates a new test core service
func newTestService(t *testing.T, cfg *Config) *Service {
	if cfg == nil {
		rt := runtime.NewTestRuntime(t, runtime.POLKADOT_RUNTIME_c768a7e4c70e)
		cfg = &Config{
			Runtime:         rt,
			IsBabeAuthority: false,
		}
	}

	if cfg.Keystore == nil {
		cfg.Keystore = keystore.NewKeystore()
	}

	if cfg.NewBlocks == nil {
		cfg.NewBlocks = make(chan types.Block)
	}

	if cfg.MsgRec == nil {
		cfg.MsgRec = make(chan network.Message, 10)
	}

	if cfg.MsgSend == nil {
		cfg.MsgSend = make(chan network.Message, 10)
	}

	if cfg.SyncChan == nil {
		cfg.SyncChan = make(chan *big.Int, 10)
	}

	stateSrvc := state.NewService("")
	stateSrvc.UseMemDB()

	genesisData := new(genesis.Data)

	err := stateSrvc.Initialize(genesisData, testGenesisHeader, trie.NewEmptyTrie())
	require.Nil(t, err)

	err = stateSrvc.Start()
	require.Nil(t, err)

	if cfg.BlockState == nil {
		cfg.BlockState = stateSrvc.Block
	}

	if cfg.StorageState == nil {
		cfg.StorageState = stateSrvc.Storage
	}

	s, err := NewService(cfg)
	require.Nil(t, err)

	return s
}

// newTestServiceWithFirstBlock creates a new test service with a test block
func newTestServiceWithFirstBlock(t *testing.T) *Service {
	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.POLKADOT_RUNTIME_c768a7e4c70e, tt)

	kp, err := sr25519.GenerateKeypair()
	require.Nil(t, err)

	err = tt.Put(tests.AuthorityDataKey, append([]byte{4}, kp.Public().Encode()...))
	require.Nil(t, err)

	ks := keystore.NewKeystore()
	ks.Insert(kp)

	cfg := &Config{
		Runtime:         rt,
		Keystore:        ks,
		IsBabeAuthority: true,
	}

	s := newTestService(t, cfg)

	preDigest, err := common.HexToBytes("0x014241424538e93dcef2efc275b72b4fa748332dc4c9f13be1125909cf90c8e9109c45da16b04bc5fdf9fe06a4f35e4ae4ed7e251ff9ee3d0d840c8237c9fb9057442dbf00f210d697a7b4959f792a81b948ff88937e30bf9709a8ab1314f71284da89a40000000000000000001100000000000000")
	require.Nil(t, err)

	s.epochNumber = uint64(2) // test preDigest item is for block in slot 17 epoch 2

	nextEpochData := &babe.NextEpochDescriptor{
		Authorities: s.bs.AuthorityData(),
	}

	consensusDigest := &types.ConsensusDigest{
		ConsensusEngineID: types.BabeEngineID,
		Data:              nextEpochData.Encode(),
	}

	conDigest := consensusDigest.Encode()

	header := &types.Header{
		ParentHash: testGenesisHeader.Hash(),
		Number:     big.NewInt(1),
		Digest:     [][]byte{preDigest, conDigest},
	}

	firstBlock := &types.Block{
		Header: header,
		Body:   &types.Body{},
	}

	err = s.blockState.AddBlock(firstBlock)
	require.Nil(t, err)

	return s
}

func addTestBlocksToState(t *testing.T, depth int, blockState BlockState) {
	previousHash := blockState.BestBlockHash()
	previousNum, err := blockState.BestBlockNumber()
	require.Nil(t, err)

	for i := 1; i <= depth; i++ {
		block := &types.Block{
			Header: &types.Header{
				ParentHash: previousHash,
				Number:     big.NewInt(int64(i)).Add(previousNum, big.NewInt(int64(i))),
			},
			Body: &types.Body{},
		}

		previousHash = block.Header.Hash()

		err := blockState.AddBlock(block)
		require.Nil(t, err)
	}
}

func TestStartService(t *testing.T) {
	s := newTestService(t, nil)
	require.NotNil(t, s) // TODO: improve dot core tests

	err := s.Start()
	require.Nil(t, err)

	s.Stop()
}

func TestNotAuthority(t *testing.T) {
	cfg := &Config{
		Keystore:        keystore.NewKeystore(),
		IsBabeAuthority: false,
	}

	s := newTestService(t, cfg)
	if s.bs != nil {
		t.Fatal("Fail: should not have babe session")
	}
}

func TestAnnounceBlock(t *testing.T) {
	msgSend := make(chan network.Message)
	newBlocks := make(chan types.Block)

	cfg := &Config{
		NewBlocks: newBlocks,
		MsgSend:   msgSend,
	}

	s := newTestService(t, cfg)
	err := s.Start()
	require.Nil(t, err)
	defer s.Stop()

	parent := &types.Header{
		Number:    big.NewInt(0),
		StateRoot: trie.EmptyHash,
	}

	// simulate block sent from BABE session
	newBlocks <- types.Block{
		Header: &types.Header{
			ParentHash: parent.Hash(),
			Number:     big.NewInt(1),
		},
		Body: &types.Body{},
	}

	select {
	case msg := <-msgSend:
		msgType := msg.GetType()
		require.Equal(t, network.BlockAnnounceMsgType, msgType)
	case <-time.After(testMessageTimeout):
		t.Error("timeout waiting for message")
	}
}

// test getBlockEpoch
func TestGetBlockEpoch(t *testing.T) {
	s := newTestServiceWithFirstBlock(t)

	blockHash := s.blockState.BestBlockHash()

	epoch, err := s.getBlockEpoch(blockHash)
	require.Nil(t, err)

	require.Equal(t, s.epochNumber, epoch)
}

// test blockFromCurrentEpoch
func TestVerifyCurrentEpoch(t *testing.T) {
	s := newTestServiceWithFirstBlock(t)

	blockHash := s.blockState.BestBlockHash()

	currentEpoch, err := s.blockFromCurrentEpoch(blockHash)
	require.Nil(t, err)

	require.Equal(t, true, currentEpoch)
}
