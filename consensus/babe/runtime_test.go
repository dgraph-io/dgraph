package babe

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/crypto/sr25519"
	"github.com/ChainSafe/gossamer/runtime"
	"github.com/ChainSafe/gossamer/tests"
	"github.com/ChainSafe/gossamer/trie"
)

func TestConfigurationFromRuntime_noAuth(t *testing.T) {
	babesession := createTestSession(t, nil)
	err := babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

	// see: https://github.com/paritytech/substrate/blob/7b1d822446982013fa5b7ad5caff35ca84f8b7d0/core/test-runtime/src/lib.rs#L621
	expected := &BabeConfiguration{
		SlotDuration:       1000,
		EpochLength:        6,
		C1:                 3,
		C2:                 10,
		GenesisAuthorities: nil,
		Randomness:         0,
		SecondarySlots:     false,
	}

	if !reflect.DeepEqual(babesession.config, expected) {
		t.Errorf("Fail: got %v expected %v\n", babesession.config, expected)
	}
}

func TestConfigurationFromRuntime_withAuthorities(t *testing.T) {
	tt := trie.NewEmptyTrie(nil)

	key, err := common.HexToBytes("0xe3b47b6c84c0493481f97c5197d2554f")
	if err != nil {
		t.Fatal(err)
	}

	value, err := common.HexToBytes("0x08eea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364b64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")
	if err != nil {
		t.Fatal(err)
	}

	err = tt.Put(key, value)
	if err != nil {
		t.Fatal(err)
	}

	rt := runtime.NewTestRuntimeWithTrie(t, tests.POLKADOT_RUNTIME, tt)

	kp, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &SessionConfig{
		Runtime: rt,
		Keypair: kp,
	}

	babesession := createTestSession(t, cfg)
	err = babesession.configurationFromRuntime()
	if err != nil {
		t.Fatal(err)
	}

	authA, _ := common.HexToHash("0xeea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364")
	authB, _ := common.HexToHash("0xb64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")

	expectedAuthData := []*AuthorityDataRaw{
		{ID: authA, Weight: 1},
		{ID: authB, Weight: 1},
	}

	// see: https://github.com/paritytech/substrate/blob/7b1d822446982013fa5b7ad5caff35ca84f8b7d0/core/test-runtime/src/lib.rs#L621
	expected := &BabeConfiguration{
		SlotDuration:       1000,
		EpochLength:        6,
		C1:                 3,
		C2:                 10,
		GenesisAuthorities: expectedAuthData,
		Randomness:         0,
		SecondarySlots:     false,
	}

	if !reflect.DeepEqual(babesession.config, expected) {
		t.Errorf("Fail: got %v expected %v\n", babesession.config, expected)
	}
}
