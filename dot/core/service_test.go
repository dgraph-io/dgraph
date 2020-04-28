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
	"io/ioutil"
	"math/big"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/transaction"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/stretchr/testify/require"
)

// testMessageTimeout is the wait time for messages to be exchanged
var testMessageTimeout = time.Second

// newTestServiceWithFirstBlock creates a new test service with a test block
func newTestServiceWithFirstBlock(t *testing.T) *Service {
	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.POLKADOT_RUNTIME_c768a7e4c70e, tt)

	kp, err := sr25519.GenerateKeypair()
	require.Nil(t, err)

	err = tt.Put(runtime.TestAuthorityDataKey, append([]byte{4}, kp.Public().Encode()...))
	require.Nil(t, err)

	ks := keystore.NewKeystore()
	ks.Insert(kp)

	cfg := &Config{
		Runtime:         rt,
		Keystore:        ks,
		IsBabeAuthority: true,
	}

	s := NewTestService(t, cfg)

	preDigest, err := common.HexToBytes("0x014241424538e93dcef2efc275b72b4fa748332dc4c9f13be1125909cf90c8e9109c45da16b04bc5fdf9fe06a4f35e4ae4ed7e251ff9ee3d0d840c8237c9fb9057442dbf00f210d697a7b4959f792a81b948ff88937e30bf9709a8ab1314f71284da89a40000000000000000001100000000000000")
	require.Nil(t, err)

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
				Digest:     [][]byte{},
			},
			Body: &types.Body{},
		}

		previousHash = block.Header.Hash()

		err := blockState.AddBlock(block)
		require.Nil(t, err)
	}
}

func TestStartService(t *testing.T) {
	s := NewTestService(t, nil)

	// TODO: improve dot tests #687
	require.NotNil(t, s)

	err := s.Start()
	require.Nil(t, err)

	s.Stop()
}

func TestNotAuthority(t *testing.T) {
	cfg := &Config{
		Keystore:        keystore.NewKeystore(),
		IsBabeAuthority: false,
	}

	s := NewTestService(t, cfg)
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

	s := NewTestService(t, cfg)
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

func TestCheckForRuntimeChanges(t *testing.T) {
	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.POLKADOT_RUNTIME_c768a7e4c70e, tt)

	kp, err := sr25519.GenerateKeypair()
	require.Nil(t, err)

	pubkey := kp.Public().Encode()
	err = tt.Put(runtime.TestAuthorityDataKey, append([]byte{4}, pubkey...))
	require.Nil(t, err)

	ks := keystore.NewKeystore()
	ks.Insert(kp)

	cfg := &Config{
		Runtime:          rt,
		Keystore:         ks,
		TransactionQueue: transaction.NewPriorityQueue(),
		IsBabeAuthority:  false,
	}

	s := NewTestService(t, cfg)

	_, err = runtime.GetRuntimeBlob(runtime.TESTS_FP, runtime.TEST_WASM_URL)
	require.Nil(t, err)

	testRuntime, err := ioutil.ReadFile(runtime.TESTS_FP)
	require.Nil(t, err)

	err = s.storageState.SetStorage([]byte(":code"), testRuntime)
	require.Nil(t, err)

	err = s.checkForRuntimeChanges()
	require.Nil(t, err)
}
