package keystore

import (
	"bytes"
	"crypto/rand"
	"os"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/crypto"
)

func TestEncryptAndDecrypt(t *testing.T) {
	password := []byte("noot")
	msg := []byte("helloworld")

	ciphertext, err := Encrypt(msg, password)
	if err != nil {
		t.Fatal(err)
	}

	res, err := Decrypt(ciphertext, password)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(msg, res) {
		t.Fatalf("Fail to decrypt: got %x expected %x", res, msg)
	}
}

func TestEncryptAndDecryptPrivateKey(t *testing.T) {
	buf := make([]byte, 64)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	priv, err := crypto.NewEd25519PrivateKey(buf)
	if err != nil {
		t.Fatal(err)
	}

	password := []byte("noot")

	data, err := EncryptPrivateKey(priv, password)
	if err != nil {
		t.Fatal(err)
	}

	res, err := DecryptPrivateKey(data, password)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(priv, res) {
		t.Fatalf("Fail: got %v expected %v", res, priv)
	}
}

func TestEncryptAndDecryptFromFile(t *testing.T) {
	filename := "./test_key"
	password := []byte("noot")

	buf := make([]byte, 64)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	priv, err := crypto.NewEd25519PrivateKey(buf)
	if err != nil {
		t.Fatal(err)
	}

	err = EncryptAndWriteToFile(filename, priv, password)
	if err != nil {
		t.Fatal(err)
	}

	res, err := ReadFromFileAndDecrypt(filename, password)
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(filename)

	if !reflect.DeepEqual(priv, res) {
		t.Fatalf("Fail: got %v expected %v", res, priv)
	}
}
