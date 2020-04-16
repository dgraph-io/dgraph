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
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/transaction"
	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/stretchr/testify/require"
)

func TestRetrieveAuthorityData(t *testing.T) {
	tt := trie.NewEmptyTrie()

	value, err := common.HexToBytes("0x08eea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364b64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")
	if err != nil {
		t.Fatal(err)
	}

	err = tt.Put(testAuthorityDataKey, value)
	if err != nil {
		t.Fatal(err)
	}

	rt := runtime.NewTestRuntimeWithTrie(t, runtime.POLKADOT_RUNTIME_c768a7e4c70e, tt)
	s := &Service{
		rt: rt,
	}

	auths, err := s.grandpaAuthorities()
	if err != nil {
		t.Fatal(err)
	}

	authABytes, _ := common.HexToBytes("0xeea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364")
	authBBytes, _ := common.HexToBytes("0xb64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")

	authA, _ := sr25519.NewPublicKey(authABytes)
	authB, _ := sr25519.NewPublicKey(authBBytes)

	expected := []*babe.AuthorityData{
		{ID: authA, Weight: 1},
		{ID: authB, Weight: 1},
	}

	if !reflect.DeepEqual(auths, expected) {
		t.Fatalf("Fail: got %v expected %v", auths, expected)
	}
}

func TestValidateTransaction(t *testing.T) {
	s := NewTestService(t, nil)

	// https://github.com/paritytech/substrate/blob/5420de3face1349a97eb954ae71c5b0b940c31de/core/transaction-pool/src/tests.rs#L95
	tx := []byte{1, 212, 53, 147, 199, 21, 253, 211, 28, 97, 20, 26, 189, 4, 169, 159, 214, 130, 44, 133, 88, 133, 76, 205, 227, 154, 86, 132, 231, 165, 109, 162, 125, 142, 175, 4, 21, 22, 135, 115, 99, 38, 201, 254, 161, 126, 37, 252, 82, 135, 97, 54, 147, 201, 18, 144, 156, 178, 38, 170, 71, 148, 242, 106, 72, 69, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 216, 5, 113, 87, 87, 40, 221, 120, 247, 252, 137, 201, 74, 231, 222, 101, 85, 108, 102, 39, 31, 190, 210, 14, 215, 124, 19, 160, 180, 203, 54, 110, 167, 163, 149, 45, 12, 108, 80, 221, 65, 238, 57, 237, 199, 16, 10, 33, 185, 8, 244, 184, 243, 139, 5, 87, 252, 245, 24, 225, 37, 154, 163, 142}

	validity, err := s.ValidateTransaction(tx)
	require.Nil(t, err)

	// https://github.com/paritytech/substrate/blob/ea2644a235f4b189c8029b9c9eac9d4df64ee91e/core/test-runtime/src/system.rs#L190
	expected := &transaction.Validity{
		Priority: 69,
		Requires: [][]byte{},
		// https://github.com/paritytech/substrate/blob/ea2644a235f4b189c8029b9c9eac9d4df64ee91e/core/test-runtime/src/system.rs#L173
		Provides:  [][]byte{{146, 157, 61, 99, 63, 98, 30, 242, 128, 49, 150, 90, 140, 165, 187, 249}},
		Longevity: 64,
		Propagate: true,
	}

	require.Equal(t, expected, validity)
}

func TestCheckForRuntimeChanges(t *testing.T) {
	tt := trie.NewEmptyTrie()
	rt := runtime.NewTestRuntimeWithTrie(t, runtime.POLKADOT_RUNTIME_c768a7e4c70e, tt)

	kp, err := sr25519.GenerateKeypair()
	require.Nil(t, err)

	pubkey := kp.Public().Encode()
	err = tt.Put(testAuthorityDataKey, append([]byte{4}, pubkey...))
	require.Nil(t, err)

	ks := keystore.NewKeystore()
	ks.Insert(kp)

	cfg := &Config{
		Runtime:          rt,
		Keystore:         ks,
		TransactionQueue: transaction.NewPriorityQueue(),
		IsBabeAuthority:  false,
	}

	s := NewTestService(t, cfg)

	_, err = runtime.GetRuntimeBlob(runtime.TESTS_FP, runtime.TEST_WASM_URL)
	require.Nil(t, err)

	testRuntime, err := ioutil.ReadFile(runtime.TESTS_FP)
	require.Nil(t, err)

	err = s.storageState.SetStorage([]byte(":code"), testRuntime)
	require.Nil(t, err)

	err = s.checkForRuntimeChanges()
	require.Nil(t, err)
}
