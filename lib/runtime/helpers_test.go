package runtime

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/runtime/extrinsic"
	"github.com/ChainSafe/gossamer/lib/transaction"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/stretchr/testify/require"
)

func TestValidateTransaction_IncludeData(t *testing.T) {
	rt := NewTestRuntime(t, POLKADOT_RUNTIME_c768a7e4c70e)

	ext := extrinsic.NewIncludeDataExt([]byte("nootwashere"))
	tx, err := ext.Encode()
	require.NoError(t, err)

	validity, err := rt.ValidateTransaction(tx)
	require.Nil(t, err)

	// https://github.com/paritytech/substrate/blob/ea2644a235f4b189c8029b9c9eac9d4df64ee91e/core/test-runtime/src/system.rs#L190
	expected := &transaction.Validity{
		Priority:  0xb,
		Requires:  [][]byte{},
		Provides:  [][]byte{{0x6e, 0x6f, 0x6f, 0x74, 0x77, 0x61, 0x73, 0x68, 0x65, 0x72, 0x65}},
		Longevity: 1,
		Propagate: false,
	}

	require.Equal(t, expected, validity)
}

func TestRetrieveAuthorityData(t *testing.T) {
	tt := trie.NewEmptyTrie()

	value, err := common.HexToBytes("0x08eea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364b64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")
	if err != nil {
		t.Fatal(err)
	}

	err = tt.Put(TestAuthorityDataKey, value)
	if err != nil {
		t.Fatal(err)
	}

	rt := NewTestRuntimeWithTrie(t, POLKADOT_RUNTIME_c768a7e4c70e, tt)

	auths, err := rt.GrandpaAuthorities()
	if err != nil {
		t.Fatal(err)
	}

	authABytes, _ := common.HexToBytes("0xeea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364")
	authBBytes, _ := common.HexToBytes("0xb64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")

	authA, _ := sr25519.NewPublicKey(authABytes)
	authB, _ := sr25519.NewPublicKey(authBBytes)

	expected := []*types.AuthorityData{
		{ID: authA, Weight: 1},
		{ID: authB, Weight: 1},
	}

	if !reflect.DeepEqual(auths, expected) {
		t.Fatalf("Fail: got %v expected %v", auths, expected)
	}
}

func TestConfigurationFromRuntime_noAuth(t *testing.T) {
	rt := NewTestRuntime(t, POLKADOT_RUNTIME_c768a7e4c70e)

	cfg, err := rt.BabeConfiguration()
	if err != nil {
		t.Fatal(err)
	}

	// see: https://github.com/paritytech/substrate/blob/7b1d822446982013fa5b7ad5caff35ca84f8b7d0/core/test-runtime/src/lib.rs#L621
	expected := &types.BabeConfiguration{
		SlotDuration:       1000,
		EpochLength:        6,
		C1:                 3,
		C2:                 10,
		GenesisAuthorities: nil,
		Randomness:         0,
		SecondarySlots:     false,
	}

	if !reflect.DeepEqual(cfg, expected) {
		t.Errorf("Fail: got %v expected %v\n", cfg, expected)
	}
}

func TestConfigurationFromRuntime_withAuthorities(t *testing.T) {
	tt := trie.NewEmptyTrie()

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

	rt := NewTestRuntimeWithTrie(t, POLKADOT_RUNTIME_c768a7e4c70e, tt)

	cfg, err := rt.BabeConfiguration()
	if err != nil {
		t.Fatal(err)
	}

	authA, _ := common.HexToHash("0xeea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364")
	authB, _ := common.HexToHash("0xb64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")

	expectedAuthData := []*types.AuthorityDataRaw{
		{ID: authA, Weight: 1},
		{ID: authB, Weight: 1},
	}

	// see: https://github.com/paritytech/substrate/blob/7b1d822446982013fa5b7ad5caff35ca84f8b7d0/core/test-runtime/src/lib.rs#L621
	expected := &types.BabeConfiguration{
		SlotDuration:       1000,
		EpochLength:        6,
		C1:                 3,
		C2:                 10,
		GenesisAuthorities: expectedAuthData,
		Randomness:         0,
		SecondarySlots:     false,
	}

	if !reflect.DeepEqual(cfg, expected) {
		t.Errorf("Fail: got %v expected %v\n", cfg, expected)
	}
}

func TestInitializeBlock(t *testing.T) {
	rt := NewTestRuntime(t, POLKADOT_RUNTIME_c768a7e4c70e)

	header := &types.Header{
		Number: big.NewInt(77),
	}

	err := rt.InitializeBlock(header)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFinalizeBlock(t *testing.T) {
	rt := NewTestRuntime(t, POLKADOT_RUNTIME_c768a7e4c70e)

	header := &types.Header{
		Number: big.NewInt(77),
	}

	err := rt.InitializeBlock(header)
	if err != nil {
		t.Fatal(err)
	}

	res, err := rt.FinalizeBlock()
	if err != nil {
		t.Fatal(err)
	}

	res.Number = header.Number

	expected := &types.Header{
		StateRoot:      trie.EmptyHash,
		ExtrinsicsRoot: trie.EmptyHash,
		Number:         big.NewInt(77),
		Digest:         [][]byte{},
	}

	res.Hash()
	expected.Hash()

	if !reflect.DeepEqual(res, expected) {
		t.Fatalf("Fail: got %v expected %v", res, expected)
	}
}
