package runtime

import (
	"math/big"
	"os"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime/extrinsic"
	"github.com/ChainSafe/gossamer/lib/transaction"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

var kr, _ = keystore.NewSr25519Keyring()
var maxRetries = 10

func TestExportRuntime(t *testing.T) {
	fp := "runtime.out"
	exportRuntime(t, SUBSTRATE_TEST_RUNTIME, fp)
	err := os.Remove(fp)
	require.NoError(t, err)
}

func TestGrandpaAuthorities(t *testing.T) {
	tt := trie.NewEmptyTrie()

	value, err := common.HexToBytes("0x0108eea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d714103640000000000000000b64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d7170100000000000000")
	require.NoError(t, err)

	err = tt.Put(TestAuthorityDataKey, value)
	require.NoError(t, err)

	rt := NewTestRuntimeWithTrie(t, NODE_RUNTIME, tt, log.LvlTrace)

	auths, err := rt.GrandpaAuthorities()
	require.NoError(t, err)

	authABytes, _ := common.HexToBytes("0xeea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364")
	authBBytes, _ := common.HexToBytes("0xb64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")

	authA, _ := ed25519.NewPublicKey(authABytes)
	authB, _ := ed25519.NewPublicKey(authBBytes)

	expected := []*types.GrandpaAuthorityData{
		{Key: authA, ID: 0},
		{Key: authB, ID: 1},
	}

	require.Equal(t, expected, auths)
}

func TestConfigurationFromRuntime_noAuth(t *testing.T) {
	rt := NewTestRuntime(t, NODE_RUNTIME)

	cfg, err := rt.BabeConfiguration()
	if err != nil {
		t.Fatal(err)
	}

	// see: https://github.com/paritytech/substrate/blob/7b1d822446982013fa5b7ad5caff35ca84f8b7d0/core/test-runtime/src/lib.rs#L621
	expected := &types.BabeConfiguration{
		SlotDuration:       3000,
		EpochLength:        200,
		C1:                 1,
		C2:                 4,
		GenesisAuthorities: nil,
		Randomness:         [32]byte{},
		SecondarySlots:     true,
	}

	if !reflect.DeepEqual(cfg, expected) {
		t.Errorf("Fail: got %v expected %v\n", cfg, expected)
	}
}

func TestConfigurationFromRuntime_withAuthorities(t *testing.T) {
	tt := trie.NewEmptyTrie()

	// randomness key
	rkey, err := common.HexToBytes("0xd5b995311b7ab9b44b649bc5ce4a7aba")
	if err != nil {
		t.Fatal(err)
	}

	rvalue, err := common.HexToHash("0x01")
	if err != nil {
		t.Fatal(err)
	}

	err = tt.Put(rkey, rvalue[:])
	if err != nil {
		t.Fatal(err)
	}

	// authorities key
	akey, err := common.HexToBytes("0x886726f904d8372fdabb7707870c2fad")
	if err != nil {
		t.Fatal(err)
	}

	avalue, err := common.HexToBytes("0x08eea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d714103640100000000000000b64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d7170100000000000000")
	if err != nil {
		t.Fatal(err)
	}

	err = tt.Put(akey, avalue)
	if err != nil {
		t.Fatal(err)
	}

	rt := NewTestRuntimeWithTrie(t, NODE_RUNTIME, tt, log.LvlTrace)

	cfg, err := rt.BabeConfiguration()
	if err != nil {
		t.Fatal(err)
	}

	authA, _ := common.HexToHash("0xeea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364")
	authB, _ := common.HexToHash("0xb64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")

	expectedAuthData := []*types.BABEAuthorityDataRaw{
		{ID: authA, Weight: 1},
		{ID: authB, Weight: 1},
	}

	// see: https://github.com/paritytech/substrate/blob/7b1d822446982013fa5b7ad5caff35ca84f8b7d0/core/test-runtime/src/lib.rs#L621
	expected := &types.BabeConfiguration{
		SlotDuration:       3000,
		EpochLength:        200,
		C1:                 1,
		C2:                 4,
		GenesisAuthorities: expectedAuthData,
		Randomness:         [32]byte{1},
		SecondarySlots:     true,
	}

	if !reflect.DeepEqual(cfg, expected) {
		t.Errorf("Fail: got %v expected %v\n", cfg, expected)
	}
}

func TestInitializeBlock(t *testing.T) {
	rt := NewTestRuntime(t, NODE_RUNTIME)

	header := &types.Header{
		Number: big.NewInt(77),
	}

	err := rt.InitializeBlock(header)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFinalizeBlock(t *testing.T) {
	// TODO: need to add inherents before calling finalize_block (see babe/inherents_test.go)
	// need to move inherents to a different package for use with BABE and runtime
	t.Skip()

	rt := NewTestRuntime(t, NODE_RUNTIME)

	header := &types.Header{
		ParentHash: trie.EmptyHash,
		Number:     big.NewInt(77),
		//StateRoot: trie.EmptyHash,
		//ExtrinsicsRoot: trie.EmptyHash,
		Digest: [][]byte{},
	}

	err := rt.InitializeBlock(header)
	require.NoError(t, err)

	var res *types.Header
	for i := 0; i < 1; i++ {
		res, err = rt.FinalizeBlock()
		if err == nil {
			break
		}
	}
	require.NoError(t, err)

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

// TODO: the following tests need to be updated to use NODE_RUNTIME.
// this will likely result in some of them being removed (need to determine what extrinsic types are valid)
func TestValidateTransaction_AuthoritiesChange(t *testing.T) {
	// TODO: update AuthoritiesChange to need to be signed by an authority
	rt := NewTestRuntime(t, SUBSTRATE_TEST_RUNTIME)

	alice := kr.Alice.Public().Encode()
	bob := kr.Bob.Public().Encode()

	aliceb := [32]byte{}
	copy(aliceb[:], alice)

	bobb := [32]byte{}
	copy(bobb[:], bob)

	ids := [][32]byte{aliceb, bobb}

	ext := extrinsic.NewAuthoritiesChangeExt(ids)
	enc, err := ext.Encode()
	require.NoError(t, err)

	validity, err := rt.ValidateTransaction(enc)
	require.NoError(t, err)

	expected := &transaction.Validity{
		Priority:  1 << 63,
		Requires:  [][]byte{},
		Provides:  [][]byte{},
		Longevity: 1,
		Propagate: true,
	}

	require.Equal(t, expected, validity)
}

func TestValidateTransaction_IncludeData(t *testing.T) {
	rt := NewTestRuntime(t, SUBSTRATE_TEST_RUNTIME)

	ext := extrinsic.NewIncludeDataExt([]byte("nootwashere"))
	tx, err := ext.Encode()
	require.NoError(t, err)

	validity, err := rt.ValidateTransaction(tx)
	require.NoError(t, err)

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

func TestValidateTransaction_StorageChange(t *testing.T) {
	rt := NewTestRuntime(t, SUBSTRATE_TEST_RUNTIME)

	ext := extrinsic.NewStorageChangeExt([]byte("testkey"), optional.NewBytes(true, []byte("testvalue")))
	enc, err := ext.Encode()
	require.NoError(t, err)

	validity, err := rt.ValidateTransaction(enc)
	require.NoError(t, err)

	expected := &transaction.Validity{
		Priority:  0x1,
		Requires:  [][]byte{},
		Provides:  [][]byte{},
		Longevity: 1,
		Propagate: false,
	}

	require.Equal(t, expected, validity)
}

func TestValidateTransaction_Transfer(t *testing.T) {
	rt := NewTestRuntime(t, SUBSTRATE_TEST_RUNTIME)

	alice := kr.Alice.Public().Encode()
	bob := kr.Bob.Public().Encode()

	aliceb := [32]byte{}
	copy(aliceb[:], alice)

	bobb := [32]byte{}
	copy(bobb[:], bob)

	transfer := extrinsic.NewTransfer(aliceb, bobb, 1000, 1)
	ext, err := transfer.AsSignedExtrinsic(kr.Alice.Private().(*sr25519.PrivateKey))
	require.NoError(t, err)
	tx, err := ext.Encode()
	require.NoError(t, err)

	validity, err := rt.ValidateTransaction(tx)
	require.NoError(t, err)

	// https://github.com/paritytech/substrate/blob/ea2644a235f4b189c8029b9c9eac9d4df64ee91e/core/test-runtime/src/system.rs#L190
	expected := &transaction.Validity{
		Priority:  0x3e8,
		Requires:  [][]byte{{0x92, 0x9d, 0x3d, 0x63, 0x3f, 0x62, 0x1e, 0xf2, 0x80, 0x31, 0x96, 0x5a, 0x8c, 0xa5, 0xbb, 0xf9}},
		Provides:  [][]byte{{0x56, 0xf3, 0xd1, 0x60, 0xa1, 0xe7, 0xc8, 0xf6, 0xe1, 0xbc, 0xb1, 0xa1, 0x95, 0x29, 0x5e, 0xc9}},
		Longevity: 0x40,
		Propagate: true,
	}

	require.Equal(t, expected, validity)
}

func TestApplyExtrinsic_AuthoritiesChange(t *testing.T) {
	// TODO: update AuthoritiesChange to need to be signed by an authority
	rt := NewTestRuntime(t, SUBSTRATE_TEST_RUNTIME)

	alice := kr.Alice.Public().Encode()
	bob := kr.Bob.Public().Encode()

	aliceb := [32]byte{}
	copy(aliceb[:], alice)

	bobb := [32]byte{}
	copy(bobb[:], bob)

	ids := [][32]byte{aliceb, bobb}

	ext := extrinsic.NewAuthoritiesChangeExt(ids)
	enc, err := ext.Encode()
	require.NoError(t, err)

	header := &types.Header{
		Number: big.NewInt(77),
	}

	err = rt.InitializeBlock(header)
	require.NoError(t, err)

	res, err := rt.ApplyExtrinsic(enc)
	require.Nil(t, err)

	require.Equal(t, []byte{0, 0}, res)
}

func TestApplyExtrinsic_IncludeData(t *testing.T) {
	rt := NewTestRuntime(t, SUBSTRATE_TEST_RUNTIME)

	header := &types.Header{
		Number: big.NewInt(77),
	}

	err := rt.InitializeBlock(header)
	require.NoError(t, err)

	data := []byte("nootwashere")

	ext := extrinsic.NewIncludeDataExt(data)
	enc, err := ext.Encode()
	require.NoError(t, err)

	res, err := rt.ApplyExtrinsic(enc)
	require.Nil(t, err)

	require.Equal(t, []byte{0, 0}, res)
}

func TestApplyExtrinsic_StorageChange_Set(t *testing.T) {
	rt := NewTestRuntime(t, SUBSTRATE_TEST_RUNTIME)

	header := &types.Header{
		Number: big.NewInt(77),
	}

	err := rt.InitializeBlock(header)
	require.NoError(t, err)

	ext := extrinsic.NewStorageChangeExt([]byte("testkey"), optional.NewBytes(true, []byte("testvalue")))
	tx, err := ext.Encode()
	require.NoError(t, err)

	res, err := rt.ApplyExtrinsic(tx)
	require.NoError(t, err)
	require.Equal(t, []byte{0, 0}, res)

	val, err := rt.storage.GetStorage([]byte("testkey"))
	require.NoError(t, err)
	require.Equal(t, []byte("testvalue"), val)

	for i := 0; i < maxRetries; i++ {
		_, err = rt.FinalizeBlock()
		if err == nil {
			break
		}
	}
	require.NoError(t, err)

	val, err = rt.storage.GetStorage([]byte("testkey"))
	require.NoError(t, err)
	// TODO: why does calling finalize_block modify the storage?
	require.NotEqual(t, []byte("testvalue"), val)
}

func TestApplyExtrinsic_StorageChange_Delete(t *testing.T) {
	rt := NewTestRuntime(t, SUBSTRATE_TEST_RUNTIME)

	header := &types.Header{
		Number: big.NewInt(77),
	}

	err := rt.InitializeBlock(header)
	require.NoError(t, err)

	ext := extrinsic.NewStorageChangeExt([]byte("testkey"), optional.NewBytes(false, []byte{}))
	tx, err := ext.Encode()
	require.NoError(t, err)

	res, err := rt.ApplyExtrinsic(tx)
	require.NoError(t, err)

	require.Equal(t, []byte{0, 0}, res)

	val, err := rt.storage.GetStorage([]byte("testkey"))
	require.NoError(t, err)
	require.Equal(t, []byte(nil), val)
}

// TODO, this test replaced by TestApplyExtrinsic_Transfer_NoBalance_UncheckedExt, should this be removed?
func TestApplyExtrinsic_Transfer_NoBalance(t *testing.T) {
	rt := NewTestRuntime(t, SUBSTRATE_TEST_RUNTIME)

	header := &types.Header{
		Number: big.NewInt(77),
	}

	alice := kr.Alice.Public().Encode()
	bob := kr.Bob.Public().Encode()

	ab := [32]byte{}
	copy(ab[:], alice)

	bb := [32]byte{}
	copy(bb[:], bob)

	transfer := extrinsic.NewTransfer(ab, bb, 1000, 0)
	ext, err := transfer.AsSignedExtrinsic(kr.Alice.Private().(*sr25519.PrivateKey))
	require.NoError(t, err)
	tx, err := ext.Encode()
	require.NoError(t, err)

	err = rt.InitializeBlock(header)
	require.NoError(t, err)

	res, err := rt.ApplyExtrinsic(tx)
	require.NoError(t, err)

	require.Equal(t, []byte{1, 2, 0, 1}, res)
}

// TODO, this test replaced by TestApplyExtrinsic_Transfer_WithBalance_UncheckedExtrinsic, should this be removed?
func TestApplyExtrinsic_Transfer_WithBalance(t *testing.T) {
	rt := NewTestRuntime(t, SUBSTRATE_TEST_RUNTIME)

	header := &types.Header{
		Number: big.NewInt(77),
	}

	alice := kr.Alice.Public().Encode()
	bob := kr.Bob.Public().Encode()

	ab := [32]byte{}
	copy(ab[:], alice)

	bb := [32]byte{}
	copy(bb[:], bob)

	rt.storage.SetBalance(ab, 2000)

	transfer := extrinsic.NewTransfer(ab, bb, 1000, 0)
	ext, err := transfer.AsSignedExtrinsic(kr.Alice.Private().(*sr25519.PrivateKey))
	require.NoError(t, err)
	tx, err := ext.Encode()
	require.NoError(t, err)

	err = rt.InitializeBlock(header)
	require.NoError(t, err)

	res, err := rt.ApplyExtrinsic(tx)
	require.NoError(t, err)
	require.Equal(t, []byte{0, 0}, res)

	// TODO: not sure if alice's balance is getting decremented properly, seems like it's always getting set to the transfer amount
	bal, err := rt.storage.GetBalance(ab)
	require.NoError(t, err)
	require.Equal(t, uint64(1000), bal)

	bal, err = rt.storage.GetBalance(bb)
	require.NoError(t, err)
	require.Equal(t, uint64(1000), bal)
}
