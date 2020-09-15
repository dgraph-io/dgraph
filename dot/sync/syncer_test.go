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
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/common/variadic"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/runtime/extrinsic"
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

func newTestSyncer(t *testing.T, cfg *Config) *Service {
	if cfg == nil {
		cfg = &Config{}
	}

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
		cfg.Runtime = runtime.NewTestRuntime(t, runtime.SUBSTRATE_TEST_RUNTIME)
	}

	if cfg.TransactionQueue == nil {
		cfg.TransactionQueue = stateSrvc.TransactionQueue
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
	syncer := newTestSyncer(t, nil)
	number := big.NewInt(12)
	req := syncer.HandleSeenBlocks(number)
	require.NotNil(t, req)
	require.Equal(t, uint64(1), req.StartingBlock.Value().(uint64))
	require.Equal(t, number, syncer.highestSeenBlock)
}

func TestHandleSeenBlocks_NotHighestSeen(t *testing.T) {
	syncer := newTestSyncer(t, nil)

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
	syncer := newTestSyncer(t, nil)

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
	syncer := newTestSyncer(t, nil)

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
	syncer := newTestSyncer(t, nil)
	syncer.highestSeenBlock = big.NewInt(132)

	responder := newTestSyncer(t, nil)
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
	syncer := newTestSyncer(t, nil)
	syncer.highestSeenBlock = big.NewInt(20)
	addTestBlocksToState(t, 4, syncer.blockState)

	responder := newTestSyncer(t, nil)
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
	syncer := newTestSyncer(t, nil)

	ext := []byte("nootwashere")
	tx := &transaction.ValidTransaction{
		Extrinsic: ext,
		Validity:  nil,
	}

	syncer.transactionQueue.Push(tx)

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

	inQueue := syncer.transactionQueue.Pop()
	require.Nil(t, inQueue, "queue should be empty")
}

func TestCoreExecuteBlockData_bytes(t *testing.T) {
	syncer := newTestSyncer(t, nil)

	// from bob test
	data, err := hex.DecodeString("ac558d2fa7ea8924147de3ede2ab0ff83ba4ad50b388ef14cfee21887e87185ff00812e3eb9ccf2955b647062349e0e33cbb0d9e936f8185f11a545236d2b41aaf03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c1113140000")

	// from TestWatchForResponses
	//data, err := hex.DecodeString("a0bc81cac20fbff59e86f0bf373782757db7016a9b3b07c343a81841facc4f82017db9db5ed9967b80143100189ba69d9e4deab85ac3570e5df25686cabe32964a0400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000100000000")
	require.Nil(t, err)

	res, err := syncer.executeBlockBytes(data)
	require.Nil(t, err) // expect error since header.ParentHash is empty

	// if execute block return a non-empty byte array, something when wrong
	require.Equal(t, []byte{}, res)
}

func TestCoreExecuteBlock(t *testing.T) {
	syncer := newTestSyncer(t, nil)
	ph, err := hex.DecodeString("972a70b03bb1764fa0c9b631cb825860567ae6098f1ef2261f3cbbd34b000057")
	require.Nil(t, err)
	sr, err := hex.DecodeString("0812e3eb9ccf2955b647062349e0e33cbb0d9e936f8185f11a545236d2b41aaf")
	require.Nil(t, err)
	er, err := hex.DecodeString("03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314")
	require.Nil(t, err)

	cHeader := &types.Header{
		ParentHash:     common.BytesToHash(ph), // executeBlock fails empty or 0 hash
		Number:         big.NewInt(341),
		StateRoot:      common.BytesToHash(sr),
		ExtrinsicsRoot: common.BytesToHash(er),
		Digest:         nil,
	}

	block := &types.Block{
		Header: cHeader,
		Body:   types.NewBody([]byte{}),
	}

	res, err := syncer.executeBlock(block)
	require.Nil(t, err)

	// if execute block returns a non-empty byte array, something went wrong
	require.Equal(t, []byte{}, res)
}

func TestHandleBlockResponse_NoBlockData(t *testing.T) {
	syncer := newTestSyncer(t, nil)
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
	syncer := newTestSyncer(t, nil)

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

func newBlockBuilder(t *testing.T, cfg *babe.ServiceConfig) *babe.Service { //nolint
	if cfg.Runtime == nil {
		cfg.Runtime = runtime.NewTestRuntime(t, runtime.SUBSTRATE_TEST_RUNTIME)
	}

	if cfg.Keypair == nil {
		kp, err := sr25519.GenerateKeypair()
		require.Nil(t, err)
		cfg.Keypair = kp
	}

	cfg.AuthData = []*types.Authority{
		{
			Key:    cfg.Keypair.Public().(*sr25519.PublicKey),
			Weight: 1,
		},
	}

	b, err := babe.NewService(cfg)
	require.NoError(t, err)

	return b
}

func TestExecuteBlock(t *testing.T) {
	t.Skip()
	// skip until block builder is separate from BABE

	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.SUBSTRATE_TEST_RUNTIME, tt, log.LvlTrace)

	// load authority into runtime
	kp, err := sr25519.GenerateKeypair()
	require.NoError(t, err)

	pubkey := kp.Public().Encode()
	err = tt.Put(runtime.TestAuthorityDataKey, append([]byte{4}, pubkey...))
	require.NoError(t, err)

	cfg := &Config{
		Runtime: rt,
	}

	syncer := newTestSyncer(t, cfg)

	bcfg := &babe.ServiceConfig{
		Runtime:          syncer.runtime,
		TransactionQueue: syncer.transactionQueue,
		Keypair:          kp,
		BlockState:       syncer.blockState.(*state.BlockState),
		EpochThreshold:   babe.MaxThreshold,
	}

	builder := newBlockBuilder(t, bcfg)
	parent, err := syncer.blockState.(*state.BlockState).BestBlockHeader()
	require.NoError(t, err)

	var block *types.Block
	for i := 0; i < maxRetries; i++ {
		slot := babe.NewSlot(1, 0, 0)
		block, err = builder.BuildBlock(parent, *slot)
		require.NoError(t, err)
		if err == nil {
			break
		}
	}

	require.NoError(t, err)
	_, err = syncer.executeBlock(block)
	require.NoError(t, err)
}

func TestExecuteBlock_WithExtrinsic(t *testing.T) {
	t.Skip()
	// skip until block builder is separate from BABE

	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.SUBSTRATE_TEST_RUNTIME, tt, log.LvlTrace)

	// load authority into runtime
	kp, err := sr25519.GenerateKeypair()
	require.NoError(t, err)

	pubkey := kp.Public().Encode()
	err = tt.Put(runtime.TestAuthorityDataKey, append([]byte{4}, pubkey...))
	require.NoError(t, err)

	cfg := &Config{
		Runtime: rt,
	}

	syncer := newTestSyncer(t, cfg)

	bcfg := &babe.ServiceConfig{
		Runtime:          syncer.runtime,
		TransactionQueue: syncer.transactionQueue,
		Keypair:          kp,
		BlockState:       syncer.blockState.(*state.BlockState),
		EpochThreshold:   babe.MaxThreshold,
	}

	key := []byte("noot")
	value := []byte("washere")
	ext := extrinsic.NewStorageChangeExt(key, optional.NewBytes(true, value))
	enc, err := ext.Encode()
	require.NoError(t, err)

	tx := transaction.NewValidTransaction(enc, new(transaction.Validity))
	_, err = syncer.transactionQueue.Push(tx)
	require.NoError(t, err)

	builder := newBlockBuilder(t, bcfg)
	parent, err := syncer.blockState.(*state.BlockState).BestBlockHeader()
	require.NoError(t, err)

	var block *types.Block
	for i := 0; i < maxRetries; i++ {
		slot := babe.NewSlot(uint64(time.Now().Unix()), 100000, 1)
		block, err = builder.BuildBlock(parent, *slot)
		if err == nil {
			break
		}
	}

	require.NoError(t, err)
	require.Equal(t, true, bytes.Contains(*block.Body, enc))
	_, err = syncer.executeBlock(block)
	require.NoError(t, err)
}
