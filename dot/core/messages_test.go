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

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/common/variadic"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/transaction"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

// BlockRequestMessage 1

// tests the ProcessBlockRequestMessage method
func TestService_ProcessBlockRequestMessage(t *testing.T) {
	msgSend := make(chan network.Message, 10)

	cfg := &Config{
		MsgSend: msgSend,
	}

	s := NewTestService(t, cfg)
	s.started.Store(true)

	addTestBlocksToState(t, 2, s.blockState)

	bestHash := s.blockState.BestBlockHash()
	bestBlock, err := s.blockState.GetBlockByNumber(big.NewInt(1))
	require.Nil(t, err)

	// set some nils and check no error is thrown
	bds := &types.BlockData{
		Hash:          bestHash,
		Header:        nil,
		Receipt:       nil,
		MessageQueue:  nil,
		Justification: nil,
	}
	err = s.blockState.CompareAndSetBlockData(bds)
	require.Nil(t, err)

	// set receipt message and justification
	bds = &types.BlockData{
		Hash:          bestHash,
		Receipt:       optional.NewBytes(true, []byte("asdf")),
		MessageQueue:  optional.NewBytes(true, []byte("ghjkl")),
		Justification: optional.NewBytes(true, []byte("qwerty")),
	}

	endHash := s.blockState.BestBlockHash()
	start, err := variadic.NewUint64OrHash(uint64(1))
	if err != nil {
		t.Fatal(err)
	}

	err = s.blockState.CompareAndSetBlockData(bds)

	require.Nil(t, err)

	testsCases := []struct {
		description      string
		value            *network.BlockRequestMessage
		expectedMsgType  int
		expectedMsgValue *network.BlockResponseMessage
	}{
		{
			description: "test get Header and Body",
			value: &network.BlockRequestMessage{
				ID:            1,
				RequestedData: 3,
				StartingBlock: start,
				EndBlockHash:  optional.NewHash(true, endHash),
				Direction:     1,
				Max:           optional.NewUint32(false, 0),
			},
			expectedMsgType: network.BlockResponseMsgType,
			expectedMsgValue: &network.BlockResponseMessage{
				ID: 1,
				BlockData: []*types.BlockData{
					{
						Hash:   optional.NewHash(true, bestHash).Value(),
						Header: bestBlock.Header.AsOptional(),
						Body:   bestBlock.Body.AsOptional(),
					},
				},
			},
		},
		{
			description: "test get Header",
			value: &network.BlockRequestMessage{
				ID:            2,
				RequestedData: 1,
				StartingBlock: start,
				EndBlockHash:  optional.NewHash(true, endHash),
				Direction:     1,
				Max:           optional.NewUint32(false, 0),
			},
			expectedMsgType: network.BlockResponseMsgType,
			expectedMsgValue: &network.BlockResponseMessage{
				ID: 2,
				BlockData: []*types.BlockData{
					{
						Hash:   optional.NewHash(true, bestHash).Value(),
						Header: bestBlock.Header.AsOptional(),
						Body:   optional.NewBody(false, nil),
					},
				},
			},
		},
		{
			description: "test get Receipt",
			value: &network.BlockRequestMessage{
				ID:            2,
				RequestedData: 4,
				StartingBlock: start,
				EndBlockHash:  optional.NewHash(true, endHash),
				Direction:     1,
				Max:           optional.NewUint32(false, 0),
			},
			expectedMsgType: network.BlockResponseMsgType,
			expectedMsgValue: &network.BlockResponseMessage{
				ID: 2,
				BlockData: []*types.BlockData{
					{
						Hash:    optional.NewHash(true, bestHash).Value(),
						Header:  optional.NewHeader(false, nil),
						Body:    optional.NewBody(false, nil),
						Receipt: bds.Receipt,
					},
				},
			},
		},
		{
			description: "test get MessageQueue",
			value: &network.BlockRequestMessage{
				ID:            2,
				RequestedData: 8,
				StartingBlock: start,
				EndBlockHash:  optional.NewHash(true, endHash),
				Direction:     1,
				Max:           optional.NewUint32(false, 0),
			},
			expectedMsgType: network.BlockResponseMsgType,
			expectedMsgValue: &network.BlockResponseMessage{
				ID: 2,
				BlockData: []*types.BlockData{
					{
						Hash:         optional.NewHash(true, bestHash).Value(),
						Header:       optional.NewHeader(false, nil),
						Body:         optional.NewBody(false, nil),
						MessageQueue: bds.MessageQueue,
					},
				},
			},
		},
		{
			description: "test get Justification",
			value: &network.BlockRequestMessage{
				ID:            2,
				RequestedData: 16,
				StartingBlock: start,
				EndBlockHash:  optional.NewHash(true, endHash),
				Direction:     1,
				Max:           optional.NewUint32(false, 0),
			},
			expectedMsgType: network.BlockResponseMsgType,
			expectedMsgValue: &network.BlockResponseMessage{
				ID: 2,
				BlockData: []*types.BlockData{
					{
						Hash:          optional.NewHash(true, bestHash).Value(),
						Header:        optional.NewHeader(false, nil),
						Body:          optional.NewBody(false, nil),
						Justification: bds.Justification,
					},
				},
			},
		},
	}

	for _, test := range testsCases {
		t.Run(test.description, func(t *testing.T) {

			err := s.ProcessBlockRequestMessage(test.value)
			require.Nil(t, err)

			select {
			case resp := <-msgSend:
				msgType := resp.GetType()

				require.Equal(t, test.expectedMsgType, msgType)

				require.Equal(t, test.expectedMsgValue.ID, resp.(*network.BlockResponseMessage).ID)

				require.Len(t, resp.(*network.BlockResponseMessage).BlockData, 2)

				require.Equal(t, test.expectedMsgValue.BlockData[0].Hash, bestHash)
				require.Equal(t, test.expectedMsgValue.BlockData[0].Header, resp.(*network.BlockResponseMessage).BlockData[0].Header)
				require.Equal(t, test.expectedMsgValue.BlockData[0].Body, resp.(*network.BlockResponseMessage).BlockData[0].Body)

				if test.expectedMsgValue.BlockData[0].Receipt != nil {
					require.Equal(t, test.expectedMsgValue.BlockData[0].Receipt, resp.(*network.BlockResponseMessage).BlockData[1].Receipt)
				}

				if test.expectedMsgValue.BlockData[0].MessageQueue != nil {
					require.Equal(t, test.expectedMsgValue.BlockData[0].MessageQueue, resp.(*network.BlockResponseMessage).BlockData[1].MessageQueue)
				}

				if test.expectedMsgValue.BlockData[0].Justification != nil {
					require.Equal(t, test.expectedMsgValue.BlockData[0].Justification, resp.(*network.BlockResponseMessage).BlockData[1].Justification)
				}
			case <-time.After(testMessageTimeout):
				t.Error("timeout waiting for message")
			}
		})
	}
}

// BlockResponseMessage 2

// tests the ProcessBlockResponseMessage method
func TestService_ProcessBlockResponseMessage(t *testing.T) {
	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.NODE_RUNTIME, tt, log.LvlTrace)

	kp, err := sr25519.GenerateKeypair()
	require.Nil(t, err)

	ks := keystore.NewKeystore()
	ks.Insert(kp)
	msgSend := make(chan network.Message, 10)

	cfg := &Config{
		Runtime:         rt,
		Keystore:        ks,
		IsBlockProducer: false,
		MsgSend:         msgSend,
	}

	s := NewTestService(t, cfg)

	hash := common.NewHash([]byte{0})
	body := optional.CoreBody{0xa, 0xb, 0xc, 0xd}

	parentHash := testGenesisHeader.Hash()
	stateRoot, err := common.HexToHash("0x2747ab7c0dc38b7f2afba82bd5e2d6acef8c31e09800f660b75ec84a7005099f")
	require.Nil(t, err)

	extrinsicsRoot, err := common.HexToHash("0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	require.Nil(t, err)

	preDigest, err := common.HexToBytes("0x014241424538e93dcef2efc275b72b4fa748332dc4c9f13be1125909cf90c8e9109c45da16b04bc5fdf9fe06a4f35e4ae4ed7e251ff9ee3d0d840c8237c9fb9057442dbf00f210d697a7b4959f792a81b948ff88937e30bf9709a8ab1314f71284da89a40000000000000000001100000000000000")
	if err != nil {
		t.Fatal(err)
	}

	header := &types.Header{
		ParentHash:     parentHash,
		Number:         big.NewInt(1),
		StateRoot:      stateRoot,
		ExtrinsicsRoot: extrinsicsRoot,
		Digest:         [][]byte{preDigest},
	}

	bds := []*types.BlockData{{
		Hash:          header.Hash(),
		Header:        header.AsOptional(),
		Body:          types.NewBody([]byte{}).AsOptional(),
		Receipt:       optional.NewBytes(false, nil),
		MessageQueue:  optional.NewBytes(false, nil),
		Justification: optional.NewBytes(false, nil),
	}, {
		Hash:          hash,
		Header:        optional.NewHeader(false, nil),
		Body:          optional.NewBody(true, body),
		Receipt:       optional.NewBytes(true, []byte("asdf")),
		MessageQueue:  optional.NewBytes(true, []byte("ghjkl")),
		Justification: optional.NewBytes(true, []byte("qwerty")),
	}}

	blockResponse := &network.BlockResponseMessage{
		BlockData: bds,
	}

	err = s.blockState.SetHeader(header)
	require.Nil(t, err)

	err = s.ProcessBlockResponseMessage(blockResponse)
	require.Nil(t, err)

	select {
	case resp := <-s.syncer.respIn:
		msgType := resp.GetType()
		require.Equal(t, network.BlockResponseMsgType, msgType)
	case <-time.After(testMessageTimeout):
		t.Error("timeout waiting for message")
	}
}

// BlockAnnounceMessage 3

// tests the ProcessBlockAnnounceMessage method
func TestService_ProcessBlockAnnounceMessage(t *testing.T) {
	msgSend := make(chan network.Message)
	newBlocks := make(chan types.Block)

	cfg := &Config{
		MsgSend:         msgSend,
		Keystore:        keystore.NewKeystore(),
		NewBlocks:       newBlocks,
		IsBlockProducer: false,
	}

	s := NewTestService(t, cfg)
	err := s.Start()
	require.Nil(t, err)

	expected := &network.BlockAnnounceMessage{
		Number:         big.NewInt(1),
		ParentHash:     testGenesisHeader.Hash(),
		StateRoot:      common.Hash{},
		ExtrinsicsRoot: common.Hash{},
		Digest:         nil,
	}

	// simulate block sent from BABE session
	newBlocks <- types.Block{
		Header: &types.Header{
			Number:     big.NewInt(1),
			ParentHash: testGenesisHeader.Hash(),
		},
		Body: types.NewBody([]byte{}),
	}

	select {
	case msg := <-msgSend:
		msgType := msg.GetType()
		require.Equal(t, network.BlockAnnounceMsgType, msgType)
		require.Equal(t, expected, msg)
	case <-time.After(testMessageTimeout):
		t.Error("timeout waiting for message")
	}
}

// TransactionMessage 4

// tests the ProcessTransactionMessage method
func TestService_ProcessTransactionMessage(t *testing.T) {
	// this currently fails due to not being able to call validate_transaction

	t.Skip()
	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.NODE_RUNTIME, tt, log.LvlTrace)

	kp, err := sr25519.GenerateKeypair()
	require.Nil(t, err)

	// TODO: load BABE authority key

	ks := keystore.NewKeystore()
	ks.Insert(kp)

	cfg := &Config{
		Runtime:          rt,
		Keystore:         ks,
		TransactionQueue: transaction.NewPriorityQueue(),
		IsBlockProducer:  true,
	}

	s := NewTestService(t, cfg)

	// https://github.com/paritytech/substrate/blob/5420de3face1349a97eb954ae71c5b0b940c31de/core/transaction-pool/src/tests.rs#L95
	ext := []byte{1, 212, 53, 147, 199, 21, 253, 211, 28, 97, 20, 26, 189, 4, 169, 159, 214, 130, 44, 133, 88, 133, 76, 205, 227, 154, 86, 132, 231, 165, 109, 162, 125, 142, 175, 4, 21, 22, 135, 115, 99, 38, 201, 254, 161, 126, 37, 252, 82, 135, 97, 54, 147, 201, 18, 144, 156, 178, 38, 170, 71, 148, 242, 106, 72, 69, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 216, 5, 113, 87, 87, 40, 221, 120, 247, 252, 137, 201, 74, 231, 222, 101, 85, 108, 102, 39, 31, 190, 210, 14, 215, 124, 19, 160, 180, 203, 54, 110, 167, 163, 149, 45, 12, 108, 80, 221, 65, 238, 57, 237, 199, 16, 10, 33, 185, 8, 244, 184, 243, 139, 5, 87, 252, 245, 24, 225, 37, 154, 163, 142}

	msg := &network.TransactionMessage{Extrinsics: []types.Extrinsic{ext}}

	err = s.ProcessTransactionMessage(msg)
	require.Nil(t, err)

	bsTx := s.transactionQueue.Peek()
	bsTxExt := []byte(bsTx.Extrinsic)

	require.Equal(t, ext, bsTxExt)
}
