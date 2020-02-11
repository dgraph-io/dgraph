package core

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/consensus/babe"
	"github.com/ChainSafe/gossamer/keystore"
	"github.com/ChainSafe/gossamer/runtime"
	"github.com/ChainSafe/gossamer/tests"
)

func TestRetrieveAuthorityData(t *testing.T) {
	rt := runtime.NewTestRuntime(t, tests.POLKADOT_RUNTIME)

	cfg := &Config{
		Runtime:  rt,
		Keystore: keystore.NewKeystore(),
	}

	s, err := NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}

	auths, err := s.retrieveAuthorityData()
	if err != nil {
		t.Fatal(err)
	}

	expected := []*babe.AuthorityData{}
	if !reflect.DeepEqual(auths, expected) {
		t.Fatalf("Fail: got %v expected %v", auths, expected)
	}
}
