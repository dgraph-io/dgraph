package crypto

import (
	"reflect"
	"testing"
)

func TestSr25519SignAndVerify(t *testing.T) {
	kp, err := GenerateSr25519Keypair()
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("helloworld")
	sig, err := kp.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	pub := kp.Public().(*Sr25519PublicKey)
	ok := pub.Verify(msg, sig)
	if !ok {
		t.Fatal("Fail: did not verify sr25519 sig")
	}
}

func TestSr25519PublicKeys(t *testing.T) {
	kp, err := GenerateSr25519Keypair()
	if err != nil {
		t.Fatal(err)
	}

	priv := kp.Private().(*Sr25519PrivateKey)
	kp2, err := NewSr25519Keypair(priv.key)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kp.Public(), kp2.Public()) {
		t.Fatalf("Fail: pubkeys do not match got %x expected %x", kp2.Public(), kp.Public())
	}
}

func TestSr25519EncodeAndDecodePrivateKey(t *testing.T) {
	kp, err := GenerateSr25519Keypair()
	if err != nil {
		t.Fatal(err)
	}

	enc := kp.Private().Encode()
	res := new(Sr25519PrivateKey)
	err = res.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}

	exp := kp.Private().(*Sr25519PrivateKey).key.Encode()
	if !reflect.DeepEqual(res.key.Encode(), exp) {
		t.Fatalf("Fail: got %x expected %x", res.key.Encode(), exp)
	}
}

func TestSr25519EncodeAndDecodePublicKey(t *testing.T) {
	kp, err := GenerateSr25519Keypair()
	if err != nil {
		t.Fatal(err)
	}

	enc := kp.Public().Encode()
	res := new(Sr25519PublicKey)
	err = res.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}

	exp := kp.Public().(*Sr25519PublicKey).key.Encode()
	if !reflect.DeepEqual(res.key.Encode(), exp) {
		t.Fatalf("Fail: got %v expected %v", res.key, exp)
	}
}
