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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/trie"
)

var genesisHeader = &types.Header{
	Number:    big.NewInt(0),
	StateRoot: trie.EmptyHash,
}

func createTestSession(t *testing.T, cfg *SessionConfig) *Session {
	rt := runtime.NewTestRuntime(t, runtime.POLKADOT_RUNTIME_c768a7e4c70e)

	babeCfg, err := rt.BabeConfiguration()
	if err != nil {
		t.Fatal(err)
	}

	if cfg == nil {
		cfg = &SessionConfig{
			Runtime: rt,
		}
	}

	if cfg.Kill == nil {
		cfg.Kill = make(chan struct{})
	}

	if cfg.EpochDone == nil {
		cfg.EpochDone = new(sync.WaitGroup)
		cfg.EpochDone.Add(1)
	}

	if cfg.NewBlocks == nil {
		cfg.NewBlocks = make(chan types.Block)
	}

	if cfg.Runtime == nil {
		cfg.Runtime = rt
	}

	cfg.SyncLock = &sync.Mutex{}

	if cfg.Keypair == nil {
		cfg.Keypair, err = sr25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
	}

	if cfg.AuthData == nil {
		auth := &types.AuthorityData{
			ID:     cfg.Keypair.Public().(*sr25519.PublicKey),
			Weight: 1,
		}
		cfg.AuthData = []*types.AuthorityData{auth}
	}

	if cfg.TransactionQueue == nil {
		cfg.TransactionQueue = state.NewTransactionQueue()
	}

	if cfg.BlockState == nil || cfg.StorageState == nil {
		dbSrv := state.NewService("")
		dbSrv.UseMemDB()

		genesisData := new(genesis.Data)

		err = dbSrv.Initialize(genesisData, genesisHeader, trie.NewEmptyTrie())
		if err != nil {
			t.Fatal(err)
		}

		err = dbSrv.Start()
		if err != nil {
			t.Fatal(err)
		}

		cfg.BlockState = dbSrv.Block
		cfg.StorageState = dbSrv.Storage
	}

	babesession, err := NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}

	babesession.config = babeCfg

	return babesession
}

func TestKill(t *testing.T) {
	killChan := make(chan struct{})

	cfg := &SessionConfig{
		Kill: killChan,
	}

	babesession := createTestSession(t, cfg)
	err := babesession.Start()
	if err != nil {
		t.Fatal(err)
	}

	if atomic.LoadUint32(&babesession.started) == uint32(0) {
		t.Fatalf("did not start session")
	}

	close(killChan)

	babeSessionKilled := true
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		if atomic.LoadUint32(&babesession.started) == uint32(1) {
			babeSessionKilled = false
		} else {
			break
		}
	}

	require.True(t, babeSessionKilled, "did not kill session")
}

func TestCalculateThreshold(t *testing.T) {
	// C = 1
	var C1 uint64 = 1
	var C2 uint64 = 1
	var authorityIndex uint64 = 0
	authorityWeights := []uint64{1, 1, 1}

	expected := new(big.Int).Lsh(big.NewInt(1), 128)

	threshold, err := calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err != nil {
		t.Fatal(err)
	}

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
	q := new(big.Int).Lsh(big.NewInt(1), 128)
	expected = q.Mul(q, p_rat.Num()).Div(q, p_rat.Denom())

	threshold, err = calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err != nil {
		t.Fatal(err)
	}

	if threshold.Cmp(expected) != 0 {
		t.Fatalf("Fail: got %d expected %d", threshold, expected)
	}
}

func TestCalculateThreshold_AuthorityWeights(t *testing.T) {
	var C1 uint64 = 5
	var C2 uint64 = 17
	var authorityIndex uint64 = 3
	authorityWeights := []uint64{3, 1, 4, 6, 10}

	theta := float64(6) / float64(24)
	c := float64(C1) / float64(C2)
	pp := 1 - c
	pp_exp := math.Pow(pp, theta)
	p := 1 - pp_exp
	p_rat := new(big.Rat).SetFloat64(p)
	q := new(big.Int).Lsh(big.NewInt(1), 128)
	expected := q.Mul(q, p_rat.Num()).Div(q, p_rat.Denom())

	threshold, err := calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err != nil {
		t.Fatal(err)
	}

	if threshold.Cmp(expected) != 0 {
		t.Fatalf("Fail: got %d expected %d", threshold, expected)
	}
}

func TestRunLottery(t *testing.T) {
	babesession := createTestSession(t, nil)
	babesession.epochThreshold = big.NewInt(0)

	outAndProof, err := babesession.runLottery(0)
	if err != nil {
		t.Fatal(err)
	}

	if outAndProof == nil {
		t.Fatal("proof was nil when over threshold")
	}
}

func TestRunLottery_False(t *testing.T) {
	babesession := createTestSession(t, nil)
	babesession.epochThreshold = big.NewInt(0).SetBytes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

	outAndProof, err := babesession.runLottery(0)
	if err != nil {
		t.Fatal(err)
	}

	if outAndProof != nil {
		t.Fatal("proof was not nil when under threshold")
	}
}

func TestCalculateThreshold_Failing(t *testing.T) {
	var C1 uint64 = 5
	var C2 uint64 = 4
	var authorityIndex uint64 = 3
	authorityWeights := []uint64{3, 1, 4, 6, 10}

	_, err := calculateThreshold(C1, C2, authorityIndex, authorityWeights)
	if err == nil {
		t.Fatal("Fail: did not err for c>1")
	}
}

func TestBabeAnnounceMessage(t *testing.T) {
	newBlocks := make(chan types.Block)
	TransactionQueue := state.NewTransactionQueue()

	cfg := &SessionConfig{
		NewBlocks:        newBlocks,
		TransactionQueue: TransactionQueue,
	}

	babesession := createTestSession(t, cfg)

	babesession.config = &types.BabeConfiguration{
		SlotDuration:       1,
		EpochLength:        6,
		C1:                 1,
		C2:                 10,
		GenesisAuthorities: []*types.AuthorityDataRaw{},
		Randomness:         0,
		SecondarySlots:     false,
	}

	babesession.authorityIndex = 0
	babesession.authorityData = []*types.AuthorityData{
		{ID: nil, Weight: 1},
		{ID: nil, Weight: 1},
		{ID: nil, Weight: 1},
	}

	err := babesession.Start()
	if err != nil {
		t.Fatal(err)
	}

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

	authData := []*types.AuthorityData{
		{ID: pubA, Weight: 1},
		{ID: pubB, Weight: 1},
	}

	bs := &Session{
		authorityData: authData,
		keypair:       kpA,
	}

	err = bs.setAuthorityIndex()
	if err != nil {
		t.Fatal(err)
	}

	if bs.authorityIndex != 0 {
		t.Fatalf("Fail: got %d expected %d", bs.authorityIndex, 0)
	}

	bs = &Session{
		authorityData: authData,
		keypair:       kpB,
	}

	err = bs.setAuthorityIndex()
	if err != nil {
		t.Fatal(err)
	}

	if bs.authorityIndex != 1 {
		t.Fatalf("Fail: got %d expected %d", bs.authorityIndex, 1)
	}
}
