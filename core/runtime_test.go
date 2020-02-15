package core

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/consensus/babe"
	"github.com/ChainSafe/gossamer/crypto/sr25519"
	"github.com/ChainSafe/gossamer/runtime"
	"github.com/ChainSafe/gossamer/tests"
	"github.com/ChainSafe/gossamer/trie"
)

func TestRetrieveAuthorityData(t *testing.T) {
	tt := trie.NewEmptyTrie(nil)

	value, err := common.HexToBytes("0x08eea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364b64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")
	if err != nil {
		t.Fatal(err)
	}

	err = tt.Put(tests.AuthorityDataKey, value)
	if err != nil {
		t.Fatal(err)
	}

	rt := runtime.NewTestRuntimeWithTrie(t, tests.POLKADOT_RUNTIME, tt)
	s := &Service{
		rt: rt,
	}

	auths, err := s.retrieveAuthorityData()
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
