package babe

import (
	"bytes"
	"testing"

	"github.com/ChainSafe/gossamer/crypto/sr25519"
)

func TestDecodeNextEpochDescriptor(t *testing.T) {
	length := 3
	auths := []*AuthorityData{}

	for i := 0; i < length; i++ {
		kp, err := sr25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}

		auth := &AuthorityData{
			id:     kp.Public().(*sr25519.PublicKey),
			weight: 1,
		}

		auths = append(auths, auth)
	}

	ned := &NextEpochDescriptor{
		Authorities: auths,
		Randomness:  [sr25519.VrfOutputLength]byte{1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 1, 2, 3, 4, 5, 6, 1, 2},
	}

	enc := ned.Encode()

	res := new(NextEpochDescriptor)
	err := res.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}

	enc2 := res.Encode()

	if !bytes.Equal(enc2, enc) {
		t.Fatalf("Fail: got %v expected %v", enc2, enc)
	}
}
