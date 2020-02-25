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

package keystore

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/secp256k1"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
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

	priv, err := ed25519.NewPrivateKey(buf)
	if err != nil {
		t.Fatal(err)
	}

	password := []byte("noot")

	data, err := EncryptPrivateKey(priv, password)
	if err != nil {
		t.Fatal(err)
	}

	res, err := DecryptPrivateKey(data, password, "ed25519")
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(priv, res) {
		t.Fatalf("Fail: got %v expected %v", res, priv)
	}
}

func createTestFile(t *testing.T) (*os.File, string) {
	filename := "./test_key"

	fp, err := filepath.Abs(filename)
	if err != nil {
		t.Fatal(err)
	}

	file, err := os.Create(fp)
	if err != nil {
		t.Fatal(err)
	}

	return file, fp
}

func TestEncryptAndDecryptFromFile_Ed25519(t *testing.T) {
	password := []byte("noot")

	file, fp := createTestFile(t)
	defer os.Remove(fp)

	kp, err := ed25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	priv := kp.Private()

	err = EncryptAndWriteToFile(file, priv, password)
	if err != nil {
		t.Fatal(err)
	}

	res, err := ReadFromFileAndDecrypt(fp, password)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(priv.Encode(), res.Encode()) {
		t.Fatalf("Fail: got %v expected %v", res, priv)
	}
}

func TestEncryptAndDecryptFromFile_Sr25519(t *testing.T) {
	password := []byte("noot")
	file, fp := createTestFile(t)
	defer os.Remove(fp)

	kp, err := sr25519.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	priv := kp.Private()

	err = EncryptAndWriteToFile(file, priv, password)
	if err != nil {
		t.Fatal(err)
	}

	res, err := ReadFromFileAndDecrypt(fp, password)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(priv.Encode(), res.Encode()) {
		t.Fatalf("Fail: got %v expected %v", res, priv)
	}
}

func TestEncryptAndDecryptFromFile_Secp256k1(t *testing.T) {
	password := []byte("noot")
	file, fp := createTestFile(t)
	defer os.Remove(fp)

	kp, err := secp256k1.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	priv := kp.Private()

	err = EncryptAndWriteToFile(file, priv, password)
	if err != nil {
		t.Fatal(err)
	}

	res, err := ReadFromFileAndDecrypt(fp, password)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(priv.Encode(), res.Encode()) {
		t.Fatalf("Fail: got %v expected %v", res, priv)
	}
}
