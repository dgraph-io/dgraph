package crypto

import (
	"reflect"
	"testing"
)

func TestEd25519SignAndVerify(t *testing.T) {
	kp, err := GenerateEd25519Keypair()
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("helloworld")
	sig := kp.Sign(msg)

	ok := Verify(kp.Public(), msg, sig)
	if !ok {
		t.Fatal("Fail: did not verify ed25519 sig")
	}
}

func TestPublicKeys(t *testing.T) {
	kp, err := GenerateEd25519Keypair()
	if err != nil {
		t.Fatal(err)
	}

	kp2 := NewEd25519Keypair(kp.Private())
	if !reflect.DeepEqual(kp.Public(), kp2.Public()) {
		t.Fatal("Fail: pubkeys do not match")
	}
}
