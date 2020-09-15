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

package babe

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
)

func newTestVerificationManager(t *testing.T, descriptor *Descriptor) *VerificationManager {
	dbSrv := state.NewService("", log.LvlInfo)
	dbSrv.UseMemDB()

	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.NODE_RUNTIME, tt, log.LvlCrit)

	genesisData := new(genesis.Data)

	err := dbSrv.Initialize(genesisData, genesisHeader, trie.NewEmptyTrie(), firstEpochInfo)
	require.NoError(t, err)

	err = dbSrv.Start()
	require.NoError(t, err)

	vm, err := NewVerificationManagerFromRuntime(dbSrv.Block, rt)
	require.NoError(t, err)

	if descriptor != nil {
		vm.descriptors[dbSrv.Block.GenesisHash()] = descriptor
	}

	return vm
}

func TestVerificationManager_SetAuthorityChangeAtBlock(t *testing.T) {
	descriptor := &Descriptor{
		AuthorityData: []*types.Authority{{Weight: 1}},
		Randomness:    [types.RandomnessLength]byte{77},
		Threshold:     big.NewInt(99),
	}

	vm := newTestVerificationManager(t, descriptor)
	require.Equal(t, []common.Hash{vm.blockState.GenesisHash()}, vm.branches[0])
	require.Equal(t, descriptor, vm.descriptors[vm.blockState.GenesisHash()])

	block1a := &types.Header{
		Number:     big.NewInt(1),
		ParentHash: vm.blockState.GenesisHash(),
	}

	block1b := &types.Header{
		ExtrinsicsRoot: common.Hash{0x8},
		Number:         big.NewInt(1),
		ParentHash:     vm.blockState.GenesisHash(),
	}

	err := vm.blockState.AddBlock(&types.Block{
		Header: block1a,
		Body:   &types.Body{},
	})
	require.NoError(t, err)
	err = vm.blockState.AddBlock(&types.Block{
		Header: block1b,
		Body:   &types.Body{},
	})
	require.NoError(t, err)

	authsA := []*types.Authority{{Weight: 77}}
	vm.SetAuthorityChangeAtBlock(block1a, authsA)
	require.Equal(t, []int64{1, 0}, vm.branchNums)
	require.Equal(t, []common.Hash{block1a.Hash()}, vm.branches[1])

	expected := &Descriptor{
		AuthorityData: authsA,
		Randomness:    descriptor.Randomness,
		Threshold:     descriptor.Threshold,
	}
	require.Equal(t, expected, vm.descriptors[block1a.Hash()])

	authsB := []*types.Authority{{Weight: 88}}
	vm.SetAuthorityChangeAtBlock(block1b, authsB)
	require.Equal(t, []int64{1, 0}, vm.branchNums)
	require.Equal(t, []common.Hash{block1a.Hash(), block1b.Hash()}, vm.branches[1])

	expected = &Descriptor{
		AuthorityData: authsB,
		Randomness:    descriptor.Randomness,
		Threshold:     descriptor.Threshold,
	}
	require.Equal(t, expected, vm.descriptors[block1b.Hash()])
}

func TestVerificationManager_VerifyBlock(t *testing.T) {
	babeService := createTestService(t, &ServiceConfig{
		EpochThreshold: maxThreshold,
	})
	descriptor := babeService.Descriptor()

	vm := newTestVerificationManager(t, descriptor)

	block, _ := createTestBlock(t, babeService, genesisHeader, [][]byte{}, 1)
	err := vm.blockState.AddBlock(block)
	require.NoError(t, err)

	ok, err := vm.VerifyBlock(block.Header)
	require.NoError(t, err)
	require.Equal(t, true, ok)
}

func TestVerificationManager_VerifyBlock_Branches(t *testing.T) {
	babeService := createTestService(t, &ServiceConfig{
		EpochThreshold: maxThreshold,
	})
	descriptor := babeService.Descriptor()

	block1a := &types.Header{
		Number:     big.NewInt(1),
		ParentHash: babeService.blockState.GenesisHash(),
	}

	block1b := &types.Header{
		ExtrinsicsRoot: common.Hash{0x8},
		Number:         big.NewInt(1),
		ParentHash:     babeService.blockState.GenesisHash(),
	}

	vm := newTestVerificationManager(t, descriptor)
	err := babeService.blockState.AddBlock(&types.Block{
		Header: block1a,
		Body:   &types.Body{},
	})
	require.NoError(t, err)
	err = babeService.blockState.AddBlock(&types.Block{
		Header: block1b,
		Body:   &types.Body{},
	})
	require.NoError(t, err)
	err = vm.blockState.AddBlock(&types.Block{
		Header: block1a,
		Body:   &types.Body{},
	})
	require.NoError(t, err)
	err = vm.blockState.AddBlock(&types.Block{
		Header: block1b,
		Body:   &types.Body{},
	})
	require.NoError(t, err)

	// create a branch at block 1 on chain A
	randomnessA := [types.RandomnessLength]byte{0x77}

	descriptorA := &Descriptor{
		AuthorityData: descriptor.AuthorityData,
		Randomness:    randomnessA,
		Threshold:     maxThreshold,
	}

	vm.setDescriptorChangeAtBlock(block1a, descriptorA)
	require.Equal(t, []int64{1, 0}, vm.branchNums)

	// create and verify block that's descendant of block B, should verify
	block, _ := createTestBlock(t, babeService, block1b, [][]byte{}, 1)
	require.NoError(t, err)

	ok, err := vm.VerifyBlock(block.Header)
	require.NoError(t, err)
	require.Equal(t, true, ok)

	// create and verify block that's descendant of block A, should verify
	babeService.randomness = randomnessA
	block, _ = createTestBlock(t, babeService, block1a, [][]byte{}, 1)
	require.NoError(t, err)

	ok, err = vm.VerifyBlock(block.Header)
	require.NoError(t, err)
	require.Equal(t, true, ok)
}

func TestVerifySlotWinner(t *testing.T) {
	kp, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &ServiceConfig{
		Keypair: kp,
	}

	babeService := createTestService(t, cfg)

	// create proof that we can authorize this block
	babeService.epochThreshold = maxThreshold
	babeService.authorityIndex = 0
	var slotNumber uint64 = 1

	addAuthorshipProof(t, babeService, slotNumber)

	slot := Slot{
		start:    uint64(time.Now().Unix()),
		duration: uint64(10000000),
		number:   slotNumber,
	}

	// create babe header
	babeHeader, err := babeService.buildBlockBabeHeader(slot)
	if err != nil {
		t.Fatal(err)
	}

	authorityData := make([]*types.Authority, 1)
	authorityData[0] = &types.Authority{
		Key: kp.Public().(*sr25519.PublicKey),
	}
	babeService.authorityData = authorityData

	verifier, err := newVerifier(babeService.blockState, babeService.Descriptor())
	if err != nil {
		t.Fatal(err)
	}

	ok, err := verifier.verifySlotWinner(slot.number, babeHeader)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("did not verify slot winner")
	}
}

func TestVerifyAuthorshipRight(t *testing.T) {
	babeService := createTestService(t, nil)
	block, _ := createTestBlock(t, babeService, genesisHeader, [][]byte{}, 1)

	verifier, err := newVerifier(babeService.blockState, babeService.Descriptor())
	if err != nil {
		t.Fatal(err)
	}

	ok, err := verifier.verifyAuthorshipRight(block.Header)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("did not verify authorship right")
	}
}

func TestVerifyAuthorshipRight_Equivocation(t *testing.T) {
	kp, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &ServiceConfig{
		Keypair: kp,
	}

	babeService := createTestService(t, cfg)

	babeService.authorityData = make([]*types.Authority, 1)
	babeService.authorityData[0] = &types.Authority{
		Key: kp.Public().(*sr25519.PublicKey),
	}

	// create and add first block
	block, _ := createTestBlock(t, babeService, genesisHeader, [][]byte{}, 1)
	block.Header.Hash()

	err = babeService.blockState.AddBlock(block)
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := newVerifier(babeService.blockState, babeService.Descriptor())
	if err != nil {
		t.Fatal(err)
	}

	ok, err := verifier.verifyAuthorshipRight(block.Header)
	require.NoError(t, err)
	require.True(t, ok)

	// create new block
	block2, _ := createTestBlock(t, babeService, genesisHeader, [][]byte{}, 1)
	block2.Header.Hash()

	t.Log(block2.Header)

	err = babeService.blockState.AddBlock(block2)
	if err != nil {
		t.Fatal(err)
	}

	ok, err = verifier.verifyAuthorshipRight(block2.Header)
	require.NotNil(t, err)
	require.False(t, ok)
	require.Equal(t, ErrProducerEquivocated, err)
}
