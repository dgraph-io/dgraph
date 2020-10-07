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
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/runtime/extrinsic"
	"github.com/ChainSafe/gossamer/lib/runtime/wasmer"
	"github.com/ChainSafe/gossamer/lib/transaction"
	"github.com/ChainSafe/gossamer/lib/trie"
	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

func addTestBlocksToState(t *testing.T, depth int, blockState BlockState) []*types.Header {
	return addTestBlocksToStateWithParent(t, blockState.BestBlockHash(), depth, blockState)
}

func addTestBlocksToStateWithParent(t *testing.T, previousHash common.Hash, depth int, blockState BlockState) []*types.Header {
	prevHeader, err := blockState.(*state.BlockState).GetHeader(previousHash)
	require.NoError(t, err)
	previousNum := prevHeader.Number

	headers := []*types.Header{}

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
		require.NoError(t, err)
		headers = append(headers, block.Header)
	}

	return headers
}

func TestStartService(t *testing.T) {
	s := NewTestService(t, nil)

	// TODO: improve dot tests #687
	require.NotNil(t, s)

	err := s.Start()
	require.Nil(t, err)

	err = s.Stop()
	require.NoError(t, err)
}

func TestAnnounceBlock(t *testing.T) {
	net := new(mockNetwork)
	newBlocks := make(chan types.Block)

	cfg := &Config{
		NewBlocks: newBlocks,
		Network:   net,
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

	time.Sleep(testMessageTimeout)
	require.Equal(t, network.BlockAnnounceMsgType, net.Message.Type())
}

func TestHandleRuntimeChanges(t *testing.T) {
	tt := trie.NewEmptyTrie()
	rt := wasmer.NewTestInstanceWithTrie(t, runtime.NODE_RUNTIME, tt, log.LvlTrace)

	kp, err := sr25519.GenerateKeypair()
	require.Nil(t, err)

	ks := keystore.NewGlobalKeystore()
	ks.Acco.Insert(kp)

	cfg := &Config{
		Runtime:          rt,
		Keystore:         ks,
		TransactionState: state.NewTransactionState(),
		IsBlockProducer:  false,
	}

	s := NewTestService(t, cfg)

	_, err = runtime.GetRuntimeBlob(runtime.TESTS_FP, runtime.TEST_WASM_URL)
	require.Nil(t, err)

	testRuntime, err := ioutil.ReadFile(runtime.TESTS_FP)
	require.Nil(t, err)

	ts, err := s.storageState.TrieState(nil)
	require.NoError(t, err)

	err = ts.Set([]byte(":code"), testRuntime)
	require.Nil(t, err)

	root, err := ts.Root()
	require.NoError(t, err)

	s.storageState.StoreTrie(root, ts)
	head := &types.Header{
		ParentHash: s.blockState.BestBlockHash(),
		Number:     big.NewInt(1),
		StateRoot:  root,
		Digest:     [][]byte{},
	}

	err = s.blockState.AddBlock(&types.Block{
		Header: head,
		Body:   types.NewBody([]byte{}),
	})
	require.NoError(t, err)

	bestHeader, err := s.blockState.BestBlockHeader()
	require.NoError(t, err)
	require.Equal(t, head, bestHeader)

	err = s.handleRuntimeChanges(testGenesisHeader)
	require.NoError(t, err)
}

func TestService_HasKey(t *testing.T) {
	ks := keystore.NewGlobalKeystore()
	kr, err := keystore.NewSr25519Keyring()
	require.NoError(t, err)
	ks.Acco.Insert(kr.Alice())

	cfg := &Config{
		Keystore: ks,
	}
	svc := NewTestService(t, cfg)

	res, err := svc.HasKey(kr.Alice().Public().Hex(), "babe")
	require.NoError(t, err)
	require.True(t, res)
}

func TestService_HasKey_UnknownType(t *testing.T) {
	ks := keystore.NewGlobalKeystore()
	kr, err := keystore.NewSr25519Keyring()
	require.NoError(t, err)
	ks.Acco.Insert(kr.Alice())

	cfg := &Config{
		Keystore: ks,
	}
	svc := NewTestService(t, cfg)

	res, err := svc.HasKey(kr.Alice().Public().Hex(), "xxxx")
	require.EqualError(t, err, "unknown key type: xxxx")
	require.False(t, res)
}

func TestHandleChainReorg_NoReorg(t *testing.T) {
	s := NewTestService(t, nil)
	addTestBlocksToState(t, 4, s.blockState.(*state.BlockState))

	head, err := s.blockState.BestBlockHeader()
	require.NoError(t, err)

	err = s.handleChainReorg(head.ParentHash, head.Hash())
	require.NoError(t, err)
}

func TestHandleChainReorg_WithReorg_NoTransactions(t *testing.T) {
	s := NewTestService(t, nil)
	height := 5
	branch := 3
	branches := map[int]int{branch: 1}
	state.AddBlocksToStateWithFixedBranches(t, s.blockState.(*state.BlockState), height, branches, 0)

	leaves := s.blockState.(*state.BlockState).Leaves()
	require.Equal(t, 2, len(leaves))

	head := s.blockState.BestBlockHash()
	var other common.Hash
	if leaves[0] == head {
		other = leaves[1]
	} else {
		other = leaves[0]
	}

	err := s.handleChainReorg(other, head)
	require.NoError(t, err)
}

func TestHandleChainReorg_WithReorg_Transactions(t *testing.T) {
	cfg := &Config{
		// TODO: change to NODE_RUNTIME
		Runtime: wasmer.NewTestInstance(t, runtime.SUBSTRATE_TEST_RUNTIME),
	}

	s := NewTestService(t, cfg)
	height := 5
	branch := 3
	addTestBlocksToState(t, height, s.blockState.(*state.BlockState))

	// create extrinsic
	ext := extrinsic.NewIncludeDataExt([]byte("nootwashere"))
	tx, err := ext.Encode()
	require.NoError(t, err)

	validity, err := s.rt.ValidateTransaction(tx)
	require.NoError(t, err)

	// get common ancestor
	ancestor, err := s.blockState.(*state.BlockState).GetBlockByNumber(big.NewInt(int64(branch - 1)))
	require.NoError(t, err)

	// build "re-org" chain
	body, err := types.NewBodyFromExtrinsics([]types.Extrinsic{tx})
	require.NoError(t, err)

	block := &types.Block{
		Header: &types.Header{
			ParentHash: ancestor.Header.Hash(),
			Number:     big.NewInt(0).Add(ancestor.Header.Number, big.NewInt(1)),
			Digest:     [][]byte{{1}},
		},
		Body: body,
	}

	err = s.blockState.AddBlock(block)
	require.NoError(t, err)

	leaves := s.blockState.(*state.BlockState).Leaves()
	require.Equal(t, 2, len(leaves))

	head := s.blockState.BestBlockHash()
	var other common.Hash
	if leaves[0] == head {
		other = leaves[1]
	} else {
		other = leaves[0]
	}

	err = s.handleChainReorg(other, head)
	require.NoError(t, err)

	pending := s.transactionState.(*state.TransactionState).Pending()
	require.Equal(t, 1, len(pending))
	require.Equal(t, transaction.NewValidTransaction(tx, validity), pending[0])
}
