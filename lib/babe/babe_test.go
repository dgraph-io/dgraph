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
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
)

var emptyHash = trie.EmptyHash
var maxThreshold = big.NewInt(0).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

var genesisHeader = &types.Header{
	Number:    big.NewInt(0),
	StateRoot: emptyHash,
}

var emptyHeader = &types.Header{
	Number: big.NewInt(0),
}

func createTestService(t *testing.T, cfg *ServiceConfig) *Service {
	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.NODE_RUNTIME, tt, log.LvlCrit)

	babeCfg, err := rt.BabeConfiguration()
	require.NoError(t, err)

	if cfg == nil {
		cfg = &ServiceConfig{
			Runtime: rt,
		}
	}

	if cfg.Runtime == nil {
		cfg.Runtime = rt
	}

	if cfg.Keypair == nil {
		cfg.Keypair, err = sr25519.GenerateKeypair()
		require.NoError(t, err)
	}

	if cfg.AuthData == nil {
		auth := &types.BABEAuthorityData{
			ID:     cfg.Keypair.Public().(*sr25519.PublicKey),
			Weight: 1,
		}
		cfg.AuthData = []*types.BABEAuthorityData{auth}
	}

	if cfg.TransactionQueue == nil {
		cfg.TransactionQueue = state.NewTransactionQueue()
	}

	if cfg.BlockState == nil || cfg.StorageState == nil {
		dbSrv := state.NewService("", log.LvlInfo)
		dbSrv.UseMemDB()

		genesisData := new(genesis.Data)

		err = dbSrv.Initialize(genesisData, genesisHeader, tt)
		require.NoError(t, err)

		err = dbSrv.Start()
		require.NoError(t, err)

		cfg.BlockState = dbSrv.Block
		cfg.StorageState = dbSrv.Storage
	}

	babeService, err := NewService(cfg)
	require.NoError(t, err)

	babeService.started.Store(true)
	babeService.config = babeCfg
	if cfg.EpochThreshold == nil {
		babeService.epochThreshold = maxThreshold
	}

	return babeService
}

func TestSlotDuration(t *testing.T) {
	bs := &Service{
		config: &types.BabeConfiguration{
			SlotDuration: 1000,
		},
	}

	dur := bs.slotDuration()
	require.Equal(t, dur.Milliseconds(), int64(1000))
}

func TestCalculateThreshold(t *testing.T) {
	// C = 1
	var C1 uint64 = 1
	var C2 uint64 = 1

	expected := new(big.Int).Lsh(big.NewInt(1), 256)

	threshold, err := CalculateThreshold(C1, C2, 3)
	require.NoError(t, err)

	if threshold.Cmp(expected) != 0 {
		t.Fatalf("Fail: got %d expected %d", threshold, expected)
	}

	// C = 1/2
	C2 = 2

	theta := float64(1) / float64(3)
	c := float64(C1) / float64(C2)
	pp := 1 - c
	pp_exp := math.Pow(pp, theta)
	p := 1 - pp_exp
	p_rat := new(big.Rat).SetFloat64(p)
	q := new(big.Int).Lsh(big.NewInt(1), 256)
	expected = q.Mul(q, p_rat.Num()).Div(q, p_rat.Denom())

	threshold, err = CalculateThreshold(C1, C2, 3)
	require.NoError(t, err)

	if threshold.Cmp(expected) != 0 {
		t.Fatalf("Fail: got %d expected %d", threshold, expected)
	}
}

func TestCalculateThreshold_Failing(t *testing.T) {
	var C1 uint64 = 5
	var C2 uint64 = 4

	_, err := CalculateThreshold(C1, C2, 3)
	if err == nil {
		t.Fatal("Fail: did not err for c>1")
	}
}

func TestRunLottery(t *testing.T) {
	babeService := createTestService(t, nil)
	babeService.epochThreshold = maxThreshold

	outAndProof, err := babeService.runLottery(0)
	if err != nil {
		t.Fatal(err)
	}

	if outAndProof == nil {
		t.Fatal("proof was nil when over threshold")
	}
}

func TestRunLottery_False(t *testing.T) {
	babeService := createTestService(t, nil)
	babeService.epochThreshold = big.NewInt(0)

	outAndProof, err := babeService.runLottery(0)
	if err != nil {
		t.Fatal(err)
	}

	if outAndProof != nil {
		t.Fatal("proof was not nil when under threshold")
	}
}

func TestBabeAnnounceMessage(t *testing.T) {
	TransactionQueue := state.NewTransactionQueue()

	cfg := &ServiceConfig{
		TransactionQueue: TransactionQueue,
		LogLvl:           log.LvlInfo,
	}

	babeService := createTestService(t, cfg)

	babeService.config = &types.BabeConfiguration{
		SlotDuration:       1,
		EpochLength:        6,
		C1:                 1,
		C2:                 10,
		GenesisAuthorities: []*types.BABEAuthorityDataRaw{},
		Randomness:         [32]byte{},
		SecondarySlots:     false,
	}

	babeService.authorityIndex = 0
	babeService.authorityData = []*types.BABEAuthorityData{
		{ID: nil, Weight: 1},
		{ID: nil, Weight: 1},
		{ID: nil, Weight: 1},
	}

	err := babeService.Start()
	if err != nil {
		t.Fatal(err)
	}

	babeService.epochThreshold = maxThreshold

	newBlocks := babeService.GetBlockChannel()
	block := <-newBlocks
	blockNumber := big.NewInt(int64(1))
	if !reflect.DeepEqual(block.Header.Number, blockNumber) {
		t.Fatalf("Didn't receive the correct block: %+v\nExpected block: %+v", block.Header.Number, blockNumber)
	}
}

func TestDetermineAuthorityIndex(t *testing.T) {
	kpA, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	kpB, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	pubA := kpA.Public().(*sr25519.PublicKey)
	pubB := kpB.Public().(*sr25519.PublicKey)

	authData := []*types.BABEAuthorityData{
		{ID: pubA, Weight: 1},
		{ID: pubB, Weight: 1},
	}

	bs := &Service{
		authorityData: authData,
		keypair:       kpA,
		logger:        log.New("BABE"),
	}

	err = bs.setAuthorityIndex()
	if err != nil {
		t.Fatal(err)
	}

	if bs.authorityIndex != 0 {
		t.Fatalf("Fail: got %d expected %d", bs.authorityIndex, 0)
	}

	bs = &Service{
		authorityData: authData,
		keypair:       kpB,
		logger:        log.New("BABE"),
	}

	err = bs.setAuthorityIndex()
	if err != nil {
		t.Fatal(err)
	}

	if bs.authorityIndex != 1 {
		t.Fatalf("Fail: got %d expected %d", bs.authorityIndex, 1)
	}
}

func TestStartAndStop(t *testing.T) {
	bs := createTestService(t, &ServiceConfig{
		LogLvl: log.LvlCrit,
	})
	err := bs.Start()
	require.NoError(t, err)
	err = bs.Stop()
	require.NoError(t, err)
}
