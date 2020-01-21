package types

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/common"
)

func TestChangesTrieRootDigest(t *testing.T) {
	d := &ChangesTrieRootDigest{
		Hash: common.Hash{0, 91, 50, 25, 214, 94, 119, 36, 71, 216, 33, 152, 85, 184, 34, 120, 61, 161, 164, 223, 76, 53, 40, 246, 76, 38, 235, 204, 43, 31, 179, 28},
	}

	enc := d.Encode()
	d2, err := DecodeDigestItem(enc)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(d, d2) {
		t.Fatalf("Fail: got %v expected %v", d2, d)
	}
}

func TestPreRuntimeDigest(t *testing.T) {
	d := &PreRuntimeDigest{
		ConsensusEngineID: BabeEngineID,
		Data:              []byte{1, 3, 5, 7},
	}

	enc := d.Encode()
	d2, err := DecodeDigestItem(enc)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(d, d2) {
		t.Fatalf("Fail: got %v expected %v", d2, d)
	}
}

func TestConsensusDigest(t *testing.T) {
	d := &ConsensusDigest{
		ConsensusEngineID: BabeEngineID,
		Data:              []byte{1, 3, 5, 7},
	}

	enc := d.Encode()
	d2, err := DecodeDigestItem(enc)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(d, d2) {
		t.Fatalf("Fail: got %v expected %v", d2, d)
	}
}

func TestSealDigest(t *testing.T) {
	d := &SealDigest{
		ConsensusEngineID: BabeEngineID,
		Data:              []byte{1, 3, 5, 7},
	}

	enc := d.Encode()
	d2, err := DecodeDigestItem(enc)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(d, d2) {
		t.Fatalf("Fail: got %v expected %v", d2, d)
	}
}
