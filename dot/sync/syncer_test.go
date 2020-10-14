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

package sync

import (
	"math/big"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/common/variadic"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/runtime/wasmer"
	"github.com/ChainSafe/gossamer/lib/scale"
	"github.com/ChainSafe/gossamer/lib/transaction"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

var maxRetries = 8 //nolint

var testGenesisHeader = &types.Header{
	Number:    big.NewInt(0),
	StateRoot: trie.EmptyHash,
}

func newTestSyncer(t *testing.T) *Service {
	cfg := &Config{}
	stateSrvc := state.NewService("", log.LvlInfo)
	stateSrvc.UseMemDB()

	genesisData := new(genesis.Data)
	err := stateSrvc.Initialize(genesisData, testGenesisHeader, trie.NewEmptyTrie(), firstEpochInfo)
	if err != nil {
		t.Fatal(err)
	}

	err = stateSrvc.Start()
	require.NoError(t, err)

	if cfg.BlockState == nil {
		cfg.BlockState = stateSrvc.Block
	}

	if cfg.StorageState == nil {
		cfg.StorageState = stateSrvc.Storage
	}

	if cfg.Runtime == nil {
		cfg.Runtime = wasmer.NewTestInstance(t, runtime.NODE_RUNTIME)
	}

	if cfg.TransactionState == nil {
		cfg.TransactionState = stateSrvc.Transaction
	}

	if cfg.Verifier == nil {
		cfg.Verifier = &mockVerifier{}
	}

	if cfg.LogLvl == 0 {
		cfg.LogLvl = log.LvlDebug
	}

	syncer, err := NewService(cfg)
	require.NoError(t, err)
	return syncer
}

func TestHandleSeenBlocks(t *testing.T) {
	syncer := newTestSyncer(t)
	number := big.NewInt(12)
	req := syncer.HandleSeenBlocks(number)
	require.NotNil(t, req)
	require.Equal(t, uint64(1), req.StartingBlock.Value().(uint64))
	require.Equal(t, number, syncer.highestSeenBlock)
}

func TestHandleSeenBlocks_NotHighestSeen(t *testing.T) {
	syncer := newTestSyncer(t)

	number := big.NewInt(12)
	req := syncer.HandleSeenBlocks(number)
	require.NotNil(t, req)
	require.Equal(t, number, syncer.highestSeenBlock)

	lower := big.NewInt(11)
	req = syncer.HandleSeenBlocks(lower)
	require.Nil(t, req)
	require.Equal(t, number, syncer.highestSeenBlock)
}

func TestHandleSeenBlocks_GreaterThanHighestSeen_NotSynced(t *testing.T) {
	syncer := newTestSyncer(t)

	number := big.NewInt(12)
	req := syncer.HandleSeenBlocks(number)
	require.NotNil(t, req)
	require.Equal(t, number, syncer.highestSeenBlock)

	number = big.NewInt(16)
	req = syncer.HandleSeenBlocks(number)
	require.NotNil(t, req)
	require.Equal(t, number, syncer.highestSeenBlock)
	require.Equal(t, req.StartingBlock.Value().(uint64), uint64(12))
}

func TestHandleSeenBlocks_GreaterThanHighestSeen_Synced(t *testing.T) {
	syncer := newTestSyncer(t)

	number := big.NewInt(12)
	req := syncer.HandleSeenBlocks(number)
	require.NotNil(t, req)
	require.Equal(t, number, syncer.highestSeenBlock)

	// synced to block 12
	syncer.synced = true

	number = big.NewInt(16)
	req = syncer.HandleSeenBlocks(number)
	require.NotNil(t, req)
	require.Equal(t, number, syncer.highestSeenBlock)
	require.Equal(t, uint64(13), req.StartingBlock.Value().(uint64))
}

func TestHandleBlockResponse(t *testing.T) {
	syncer := newTestSyncer(t)
	syncer.highestSeenBlock = big.NewInt(132)

	responder := newTestSyncer(t)
	addTestBlocksToState(t, 130, responder.blockState)

	startNum := 1
	start, err := variadic.NewUint64OrHash(startNum)
	require.NoError(t, err)

	req := &network.BlockRequestMessage{
		ID:            1,
		RequestedData: 3,
		StartingBlock: start,
	}

	resp, err := responder.CreateBlockResponse(req)
	require.NoError(t, err)

	req2 := syncer.HandleBlockResponse(resp)
	require.NotNil(t, req2)

	// msg should contain blocks 1 to 129 (maxResponseSize # of blocks)
	require.Equal(t, uint64(startNum+int(maxResponseSize)), req2.StartingBlock.Value().(uint64))

	resp2, err := responder.CreateBlockResponse(req)
	require.NoError(t, err)
	syncer.HandleBlockResponse(resp2)
	// response should contain blocks 13 to 20, and we should be synced
	require.True(t, syncer.synced)
}

func TestHandleBlockResponse_MissingBlocks(t *testing.T) {
	syncer := newTestSyncer(t)
	syncer.highestSeenBlock = big.NewInt(20)
	addTestBlocksToState(t, 4, syncer.blockState)

	responder := newTestSyncer(t)
	addTestBlocksToState(t, 16, responder.blockState)

	startNum := 16
	start, err := variadic.NewUint64OrHash(startNum)
	require.NoError(t, err)

	req := &network.BlockRequestMessage{
		ID:            1,
		RequestedData: 3,
		StartingBlock: start,
	}

	// resp contains blocks 16 + (16 + maxResponseSize)
	resp, err := responder.CreateBlockResponse(req)
	require.NoError(t, err)

	// request should start from block 5 (best block number + 1)
	syncer.synced = false
	req2 := syncer.HandleBlockResponse(resp)
	require.NotNil(t, req2)
	require.Equal(t, uint64(5), req2.StartingBlock.Value().(uint64))
}

func TestRemoveIncludedExtrinsics(t *testing.T) {
	syncer := newTestSyncer(t)

	ext := []byte("nootwashere")
	tx := &transaction.ValidTransaction{
		Extrinsic: ext,
		Validity:  nil,
	}

	syncer.transactionState.(*state.TransactionState).Push(tx)

	exts := []types.Extrinsic{ext}
	body, err := types.NewBodyFromExtrinsics(exts)
	require.NoError(t, err)

	bd := &types.BlockData{
		Body: body.AsOptional(),
	}

	msg := &network.BlockResponseMessage{
		BlockData: []*types.BlockData{bd},
	}

	_, _, err = syncer.processBlockResponseData(msg)
	require.NoError(t, err)

	inQueue := syncer.transactionState.(*state.TransactionState).Pop()
	require.Nil(t, inQueue, "queue should be empty")
}

func TestHandleBlockResponse_NoBlockData(t *testing.T) {
	syncer := newTestSyncer(t)
	msg := &network.BlockResponseMessage{
		ID:        0,
		BlockData: nil,
	}
	low, high, err := syncer.processBlockResponseData(msg)
	require.Nil(t, err)
	require.Equal(t, int64(0), high)
	require.Equal(t, maxInt64, low)
}

func TestHandleBlockResponse_BlockData(t *testing.T) {
	syncer := newTestSyncer(t)

	cHeader := &optional.CoreHeader{
		ParentHash:     syncer.blockState.BestBlockHash(), // executeBlock fails empty or 0 hash
		Number:         big.NewInt(1),
		StateRoot:      trie.EmptyHash,
		ExtrinsicsRoot: common.Hash{},
		Digest:         nil,
	}
	header := optional.NewHeader(true, cHeader)
	bd := []*types.BlockData{{
		Hash:          common.Hash{},
		Header:        header,
		Body:          optional.NewBody(true, optional.CoreBody{}),
		Receipt:       nil,
		MessageQueue:  nil,
		Justification: nil,
	}}
	msg := &network.BlockResponseMessage{
		ID:        0,
		BlockData: bd,
	}
	low, high, err := syncer.processBlockResponseData(msg)
	require.Nil(t, err)
	require.Equal(t, int64(1), low)
	require.Equal(t, int64(1), high)
}

func buildBlock(t *testing.T, instance runtime.Instance, parent *types.Header) *types.Block {
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     big.NewInt(0).Add(parent.Number, big.NewInt(1)),
		Digest:     [][]byte{},
	}

	err := instance.InitializeBlock(header)
	require.NoError(t, err)

	idata := types.NewInherentsData()
	err = idata.SetInt64Inherent(types.Timstap0, uint64(time.Now().Unix()))
	require.NoError(t, err)

	err = idata.SetInt64Inherent(types.Babeslot, 1)
	require.NoError(t, err)

	err = idata.SetBigIntInherent(types.Finalnum, big.NewInt(0))
	require.NoError(t, err)

	ienc, err := idata.Encode()
	require.NoError(t, err)

	// Call BlockBuilder_inherent_extrinsics which returns the inherents as extrinsics
	inherentExts, err := instance.InherentExtrinsics(ienc)
	require.NoError(t, err)

	// decode inherent extrinsics
	exts, err := scale.Decode(inherentExts, [][]byte{})
	require.NoError(t, err)

	bodyExts := []types.Extrinsic{}

	// apply each inherent extrinsic
	for _, ext := range exts.([][]byte) {
		in, err := scale.Encode(ext) //nolint
		require.NoError(t, err)
		t.Log(in)

		ret, err := instance.ApplyExtrinsic(in)
		require.NoError(t, err)
		require.Equal(t, ret, []byte{0, 0})

		bodyExts = append(bodyExts, in)
	}

	res, err := instance.FinalizeBlock()
	require.NoError(t, err)

	body, err := types.NewBodyFromExtrinsics(bodyExts)
	require.NoError(t, err)

	return &types.Block{
		Header: res,
		Body:   body,
	}
}

func TestCoreExecuteBlock(t *testing.T) {
	t.Skip() // this currently fails due to mismatching ExtrinsicRoots
	syncer := newTestSyncer(t)

	parent, err := syncer.blockState.(*state.BlockState).BestBlockHeader()
	require.NoError(t, err)

	block := buildBlock(t, syncer.runtime, parent)
	res, err := syncer.executeBlock(block)
	require.Nil(t, err)

	// if execute block returns a non-empty byte array, something went wrong
	require.Equal(t, []byte{}, res)
}
