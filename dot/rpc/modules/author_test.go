package modules

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/core"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/transaction"
	"github.com/ChainSafe/gossamer/lib/trie"
	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

// https://github.com/paritytech/substrate/blob/5420de3face1349a97eb954ae71c5b0b940c31de/core/transaction-pool/src/tests.rs#L95
var testExt = []byte{1, 212, 53, 147, 199, 21, 253, 211, 28, 97, 20, 26, 189, 4, 169, 159, 214, 130, 44, 133, 88, 133, 76, 205, 227, 154, 86, 132, 231, 165, 109, 162, 125, 142, 175, 4, 21, 22, 135, 115, 99, 38, 201, 254, 161, 126, 37, 252, 82, 135, 97, 54, 147, 201, 18, 144, 156, 178, 38, 170, 71, 148, 242, 106, 72, 69, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 216, 5, 113, 87, 87, 40, 221, 120, 247, 252, 137, 201, 74, 231, 222, 101, 85, 108, 102, 39, 31, 190, 210, 14, 215, 124, 19, 160, 180, 203, 54, 110, 167, 163, 149, 45, 12, 108, 80, 221, 65, 238, 57, 237, 199, 16, 10, 33, 185, 8, 244, 184, 243, 139, 5, 87, 252, 245, 24, 225, 37, 154, 163, 142}

// invalid transaction (above tx, with last byte changed)
var testInvalidExt = []byte{1, 212, 53, 147, 199, 21, 253, 211, 28, 97, 20, 26, 189, 4, 169, 159, 214, 130, 44, 133, 88, 133, 76, 205, 227, 154, 86, 132, 231, 165, 109, 162, 125, 142, 175, 4, 21, 22, 135, 115, 99, 38, 201, 254, 161, 126, 37, 252, 82, 135, 97, 54, 147, 201, 18, 144, 156, 178, 38, 170, 71, 148, 242, 106, 72, 69, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 216, 5, 113, 87, 87, 40, 221, 120, 247, 252, 137, 201, 74, 231, 222, 101, 85, 108, 102, 39, 31, 190, 210, 14, 215, 124, 19, 160, 180, 203, 54, 110, 167, 163, 149, 45, 12, 108, 80, 221, 65, 238, 57, 237, 199, 16, 10, 33, 185, 8, 244, 184, 243, 139, 5, 87, 252, 245, 24, 225, 37, 154, 163, 143}

func TestAuthorModule_Pending(t *testing.T) {
	txQueue := state.NewTransactionQueue()
	auth := NewAuthorModule(nil, nil, nil, txQueue)

	res := new(PendingExtrinsicsResponse)
	err := auth.PendingExtrinsics(nil, nil, res)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(*res, PendingExtrinsicsResponse([][]byte{})) {
		t.Errorf("Fail: expected: %+v got: %+v\n", res, &[][]byte{})
	}

	vtx := &transaction.ValidTransaction{
		Extrinsic: types.NewExtrinsic(testExt),
		Validity:  new(transaction.Validity),
	}

	_, err = txQueue.Push(vtx)
	require.NoError(t, err)

	err = auth.PendingExtrinsics(nil, nil, res)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := vtx.Encode()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(*res, PendingExtrinsicsResponse([][]byte{expected})) {
		t.Errorf("Fail: expected: %+v got: %+v\n", res, &[][]byte{expected})
	}
}

func TestAuthorModule_SubmitExtrinsic(t *testing.T) {
	t.Skip()
	// setup auth module
	txQueue := state.NewTransactionQueue()

	auth := setupAuthModule(t, txQueue)

	// create and submit extrinsic
	ext := Extrinsic(fmt.Sprintf("0x%x", testExt))

	res := new(ExtrinsicHashResponse)

	err := auth.SubmitExtrinsic(nil, &ext, res)
	require.Nil(t, err)

	// setup expected results
	val := &transaction.Validity{
		Priority:  69,
		Requires:  [][]byte{},
		Provides:  [][]byte{{146, 157, 61, 99, 63, 98, 30, 242, 128, 49, 150, 90, 140, 165, 187, 249}},
		Longevity: 64,
		Propagate: true,
	}
	expected := &transaction.ValidTransaction{
		Extrinsic: types.NewExtrinsic(testExt),
		Validity:  val,
	}
	expectedHash := ExtrinsicHashResponse("0xb20777f4db60ea55b1aeedde2d7b7aff3efeda736b7e2a840b5713348f766078")

	inQueue := txQueue.Pop()

	// compare results
	require.Equal(t, expected, inQueue)
	require.Equal(t, expectedHash, *res)
}

func TestAuthorModule_SubmitExtrinsic_invalid(t *testing.T) {
	t.Skip()
	// setup service
	// setup auth module
	txQueue := state.NewTransactionQueue()
	auth := setupAuthModule(t, txQueue)

	// create and submit extrinsic
	ext := Extrinsic(fmt.Sprintf("0x%x", testInvalidExt))

	res := new(ExtrinsicHashResponse)

	err := auth.SubmitExtrinsic(nil, &ext, res)
	require.EqualError(t, err, runtime.ErrInvalidTransaction.Message)
}

func TestAuthorModule_SubmitExtrinsic_invalid_input(t *testing.T) {
	// setup service
	// setup auth module
	txQueue := state.NewTransactionQueue()
	auth := setupAuthModule(t, txQueue)

	// create and submit extrinsic
	ext := Extrinsic(fmt.Sprintf("%x", "1"))

	res := new(ExtrinsicHashResponse)

	err := auth.SubmitExtrinsic(nil, &ext, res)
	require.EqualError(t, err, "could not byteify non 0x prefixed string")
}

func TestAuthorModule_SubmitExtrinsic_InQueue(t *testing.T) {
	t.Skip()
	// setup auth module
	txQueue := state.NewTransactionQueue()

	auth := setupAuthModule(t, txQueue)

	// create and submit extrinsic
	ext := Extrinsic(fmt.Sprintf("0x%x", testExt))

	res := new(ExtrinsicHashResponse)

	// setup expected results
	val := &transaction.Validity{
		Priority:  69,
		Requires:  [][]byte{},
		Provides:  [][]byte{{146, 157, 61, 99, 63, 98, 30, 242, 128, 49, 150, 90, 140, 165, 187, 249}},
		Longevity: 64,
		Propagate: true,
	}
	expected := &transaction.ValidTransaction{
		Extrinsic: types.NewExtrinsic(testExt),
		Validity:  val,
	}

	_, err := txQueue.Push(expected)
	require.Nil(t, err)

	// this should cause error since transaction is already in txQueue
	err = auth.SubmitExtrinsic(nil, &ext, res)
	require.EqualError(t, err, transaction.ErrTransactionExists.Error())

}

func TestAuthorModule_InsertKey_Valid(t *testing.T) {
	cs := core.NewTestService(t, nil)

	auth := NewAuthorModule(nil, cs, nil, nil)
	req := &KeyInsertRequest{"babe", "0xb7e9185065667390d2ad952a5324e8c365c9bf503dcf97c67a5ce861afe97309", "0x6246ddf254e0b4b4e7dffefc8adf69d212b98ac2b579c362b473fec8c40b4c0a"}
	res := &KeyInsertResponse{}
	err := auth.InsertKey(nil, req, res)
	require.Nil(t, err)

	require.Len(t, *res, 0) // zero len result on success
}

func TestAuthorModule_InsertKey_Valid_gran_keytype(t *testing.T) {
	cs := core.NewTestService(t, nil)

	auth := NewAuthorModule(nil, cs, nil, nil)
	req := &KeyInsertRequest{"gran", "0xb7e9185065667390d2ad952a5324e8c365c9bf503dcf97c67a5ce861afe97309b7e9185065667390d2ad952a5324e8c365c9bf503dcf97c67a5ce861afe97309", "0xb7e9185065667390d2ad952a5324e8c365c9bf503dcf97c67a5ce861afe97309"}
	res := &KeyInsertResponse{}
	err := auth.InsertKey(nil, req, res)
	require.Nil(t, err)

	require.Len(t, *res, 0) // zero len result on success
}

func TestAuthorModule_InsertKey_InValid(t *testing.T) {
	cs := core.NewTestService(t, nil)

	auth := NewAuthorModule(nil, cs, nil, nil)
	req := &KeyInsertRequest{"babe", "0xb7e9185065667390d2ad952a5324e8c365c9bf503dcf97c67a5ce861afe97309", "0x0000000000000000000000000000000000000000000000000000000000000000"}
	res := &KeyInsertResponse{}
	err := auth.InsertKey(nil, req, res)
	require.EqualError(t, err, "generated public key does not equal provide public key")
}

func TestAuthorModule_InsertKey_UnknownKeyType(t *testing.T) {
	cs := core.NewTestService(t, nil)

	auth := NewAuthorModule(nil, cs, nil, nil)
	req := &KeyInsertRequest{"mack", "0xb7e9185065667390d2ad952a5324e8c365c9bf503dcf97c67a5ce861afe97309", "0x6246ddf254e0b4b4e7dffefc8adf69d212b98ac2b579c362b473fec8c40b4c0a"}
	res := &KeyInsertResponse{}
	err := auth.InsertKey(nil, req, res)
	require.EqualError(t, err, "cannot decode key: invalid key type")

}

func TestAuthorModule_HasKey(t *testing.T) {
	auth := setupAuthModule(t, nil)
	kr, err := keystore.NewSr25519Keyring()
	require.Nil(t, err)

	var res bool
	req := []string{kr.Alice.Public().Hex(), "babe"}
	err = auth.HasKey(nil, &req, &res)
	require.NoError(t, err)
	require.True(t, res)
}

func TestAuthorModule_HasKey_NotFound(t *testing.T) {
	auth := setupAuthModule(t, nil)
	kr, err := keystore.NewSr25519Keyring()
	require.Nil(t, err)

	var res bool
	req := []string{kr.Bob.Public().Hex(), "babe"}
	err = auth.HasKey(nil, &req, &res)
	require.NoError(t, err)
	require.False(t, res)
}

func TestAuthorModule_HasKey_InvalidKey(t *testing.T) {
	auth := setupAuthModule(t, nil)

	var res bool
	req := []string{"0xaa11", "babe"}
	err := auth.HasKey(nil, &req, &res)
	require.EqualError(t, err, "cannot create public key: input is not 32 bytes")
	require.False(t, res)
}

func TestAuthorModule_HasKey_InvalidKeyType(t *testing.T) {
	auth := setupAuthModule(t, nil)
	kr, err := keystore.NewSr25519Keyring()
	require.Nil(t, err)

	var res bool
	req := []string{kr.Alice.Public().Hex(), "xxxx"}
	err = auth.HasKey(nil, &req, &res)
	require.EqualError(t, err, "unknown key type: xxxx")
	require.False(t, res)
}

func newCoreService(t *testing.T) *core.Service {
	// setup service
	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.NODE_RUNTIME, tt, log.LvlInfo)
	ks := keystore.NewKeystore()

	// insert alice key for testing
	kr, err := keystore.NewSr25519Keyring()
	require.NoError(t, err)
	ks.Insert(kr.Alice)

	cfg := &core.Config{
		Runtime:          rt,
		Keystore:         ks,
		TransactionQueue: transaction.NewPriorityQueue(),
		IsBlockProducer:  false,
	}

	return core.NewTestService(t, cfg)
}

func setupAuthModule(t *testing.T, txq *state.TransactionQueue) *AuthorModule {
	cs := newCoreService(t)
	rt := runtime.NewTestRuntime(t, runtime.NODE_RUNTIME)
	return NewAuthorModule(nil, cs, rt, txq)
}
