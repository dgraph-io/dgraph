package sr25519

import (
	"crypto/rand"
	"reflect"
	"testing"
)

func TestNewKeypairFromSeed(t *testing.T) {
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	if err != nil {
		t.Fatal(err)
	}

	kp, err := NewKeypairFromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}

	if kp.public == nil || kp.private == nil {
		t.Fatal("key is nil")
	}
}

func TestSignAndVerify(t *testing.T) {
	kp, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("helloworld")
	sig, err := kp.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	pub := kp.Public().(*PublicKey)
	ok, err := pub.Verify(msg, sig)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Fail: did not verify sr25519 sig")
	}
}

func TestPublicKeys(t *testing.T) {
	kp, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	priv := kp.Private().(*PrivateKey)
	kp2, err := NewKeypair(priv.key)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kp.Public(), kp2.Public()) {
		t.Fatalf("Fail: pubkeys do not match got %x expected %x", kp2.Public(), kp.Public())
	}
}

func TestEncodeAndDecodePrivateKey(t *testing.T) {
	kp, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	enc := kp.Private().Encode()
	res := new(PrivateKey)
	err = res.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}

	exp := kp.Private().(*PrivateKey).key.Encode()
	if !reflect.DeepEqual(res.key.Encode(), exp) {
		t.Fatalf("Fail: got %x expected %x", res.key.Encode(), exp)
	}
}

func TestEncodeAndDecodePublicKey(t *testing.T) {
	kp, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	enc := kp.Public().Encode()
	res := new(PublicKey)
	err = res.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}

	exp := kp.Public().(*PublicKey).key.Encode()
	if !reflect.DeepEqual(res.key.Encode(), exp) {
		t.Fatalf("Fail: got %v expected %v", res.key, exp)
	}
}

func TestVrfSignAndVerify(t *testing.T) {
	kp, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("helloworld")
	out, proof, err := kp.VrfSign(msg)
	if err != nil {
		t.Fatal(err)
	}

	pub := kp.Public().(*PublicKey)
	ok, err := pub.VrfVerify(msg, out, proof)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Fail: did not verify vrf")
	}
}
