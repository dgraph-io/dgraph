package keystore

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/crypto"
)

func TestKeystore(t *testing.T) {
	ks := NewKeystore()

	kp, err := crypto.GenerateSr25519Keypair()
	if err != nil {
		t.Fatal(err)
	}

	addr := kp.Public().Address()
	ks.Insert(kp)
	kp2 := ks.Get(addr)

	if !reflect.DeepEqual(kp, kp2) {
		t.Fatalf("Fail: got %v expected %v", kp2, kp)
	}
}
