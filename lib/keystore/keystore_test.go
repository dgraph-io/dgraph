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
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/secp256k1"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/stretchr/testify/require"
)

var testPassword = []byte("1234")

func TestKeystore(t *testing.T) {
	ks := NewKeystore()

	kp, err := sr25519.GenerateKeypair()
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

func TestGetSr25519PublicKeys(t *testing.T) {
	ks := NewKeystore()

	expectedPubkeys := []crypto.PublicKey{}
	numKps := 12

	for i := 0; i < numKps; i++ {
		kp, err := sr25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
		expectedPubkeys = append(expectedPubkeys, kp.Public())
	}

	for i := 0; i < numKps; i++ {
		kp, err := ed25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
	}

	pubkeys := ks.Sr25519PublicKeys()
	sort.Slice(pubkeys, func(i, j int) bool {
		return strings.Compare(string(pubkeys[i].Address()), string(pubkeys[j].Address())) < 0
	})
	sort.Slice(expectedPubkeys, func(i, j int) bool {
		return strings.Compare(string(expectedPubkeys[i].Address()), string(expectedPubkeys[j].Address())) < 0
	})

	if !reflect.DeepEqual(pubkeys, expectedPubkeys) {
		t.Fatalf("Fail: got %v expected %v", pubkeys, expectedPubkeys)
	}
}

func TestGetEd25519PublicKeys(t *testing.T) {
	ks := NewKeystore()

	expectedPubkeys := []crypto.PublicKey{}
	numKps := 10

	for i := 0; i < numKps; i++ {
		kp, err := ed25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
		expectedPubkeys = append(expectedPubkeys, kp.Public())
	}

	for i := 0; i < numKps; i++ {
		kp, err := secp256k1.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
	}

	pubkeys := ks.Ed25519PublicKeys()
	sort.Slice(pubkeys, func(i, j int) bool {
		return strings.Compare(string(pubkeys[i].Address()), string(pubkeys[j].Address())) < 0
	})
	sort.Slice(expectedPubkeys, func(i, j int) bool {
		return strings.Compare(string(expectedPubkeys[i].Address()), string(expectedPubkeys[j].Address())) < 0
	})

	if !reflect.DeepEqual(pubkeys, expectedPubkeys) {
		t.Fatalf("Fail: got %v expected %v", pubkeys, expectedPubkeys)
	}
}

func TestGetSecp256k1PublicKeys(t *testing.T) {
	ks := NewKeystore()

	expectedPubkeys := []crypto.PublicKey{}
	numKps := 10

	for i := 0; i < numKps; i++ {
		kp, err := secp256k1.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
		expectedPubkeys = append(expectedPubkeys, kp.Public())
	}

	for i := 0; i < numKps; i++ {
		kp, err := sr25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		ks.Insert(kp)
	}

	pubkeys := ks.Secp256k1PublicKeys()
	sort.Slice(pubkeys, func(i, j int) bool {
		return strings.Compare(string(pubkeys[i].Address()), string(pubkeys[j].Address())) < 0
	})
	sort.Slice(expectedPubkeys, func(i, j int) bool {
		return strings.Compare(string(expectedPubkeys[i].Address()), string(expectedPubkeys[j].Address())) < 0
	})

	if !reflect.DeepEqual(pubkeys, expectedPubkeys) {
		t.Fatalf("Fail: got %v expected %v", pubkeys, expectedPubkeys)
	}
}

func TestGenerateKey_Sr25519(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyfile, err := GenerateKeypair("sr25519", testdir, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	keys, err := utils.KeystoreFilepaths(testdir)
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) != 1 {
		t.Fatal("Fail: expected 1 key in keystore")
	}

	if strings.Compare(keys[0], filepath.Base(keyfile)) != 0 {
		t.Fatalf("Fail: got %s expected %s", keys[0], keyfile)
	}
}

func TestGenerateKey_Ed25519(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyfile, err := GenerateKeypair("ed25519", testdir, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	keys, err := utils.KeystoreFilepaths(testdir)
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) != 1 {
		t.Fatal("Fail: expected 1 key in keystore")
	}

	if strings.Compare(keys[0], filepath.Base(keyfile)) != 0 {
		t.Fatalf("Fail: got %s expected %s", keys[0], keyfile)
	}

	contents, err := ioutil.ReadFile(keyfile)
	if err != nil {
		t.Fatal(err)
	}

	kscontents := new(EncryptedKeystore)
	err = json.Unmarshal(contents, kscontents)
	if err != nil {
		t.Fatal(err)
	}

	if kscontents.Type != "ed25519" {
		t.Fatalf("Fail: got %s expected %s", kscontents.Type, "ed25519")
	}
}

func TestGenerateKey_Secp256k1(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyfile, err := GenerateKeypair("secp256k1", testdir, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	keys, err := utils.KeystoreFilepaths(testdir)
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) != 1 {
		t.Fatal("Fail: expected 1 key in keystore")
	}

	if strings.Compare(keys[0], filepath.Base(keyfile)) != 0 {
		t.Fatalf("Fail: got %s expected %s", keys[0], keyfile)
	}

	contents, err := ioutil.ReadFile(keyfile)
	if err != nil {
		t.Fatal(err)
	}

	kscontents := new(EncryptedKeystore)
	err = json.Unmarshal(contents, kscontents)
	if err != nil {
		t.Fatal(err)
	}

	if kscontents.Type != "secp256k1" {
		t.Fatalf("Fail: got %s expected %s", kscontents.Type, "secp256k1")
	}
}

func TestGenerateKey_NoType(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyfile, err := GenerateKeypair("", testdir, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	contents, err := ioutil.ReadFile(keyfile)
	if err != nil {
		t.Fatal(err)
	}

	kscontents := new(EncryptedKeystore)
	err = json.Unmarshal(contents, kscontents)
	if err != nil {
		t.Fatal(err)
	}

	if kscontents.Type != "sr25519" {
		t.Fatalf("Fail: got %s expected %s", kscontents.Type, "sr25519")
	}
}

func TestImportKey_ShouldFail(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	_, err := ImportKeypair("./notakey.key", testdir)
	if err == nil {
		t.Fatal("did not err")
	}
}

func TestImportKey(t *testing.T) {
	basePath := utils.NewTestBasePath(t, "keystore")
	defer utils.RemoveTestDir(t)

	keypath := basePath

	importkeyfile, err := GenerateKeypair("sr25519", keypath, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(importkeyfile)

	keyfile, err := ImportKeypair(importkeyfile, basePath)
	if err != nil {
		t.Fatal(err)
	}

	keys, err := utils.KeystoreFilepaths(basePath)
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) != 1 {
		t.Fatal("fail")
	}

	if strings.Compare(keys[0], filepath.Base(keyfile)) != 0 {
		t.Fatalf("Fail: got %s expected %s", keys[0], keyfile)
	}
}

func TestListKeys(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	expected := []string{}

	for i := 0; i < 5; i++ {
		var err error
		var keyfile string
		if i%2 == 0 {
			// if testPassword == nil {
			// 	testPassword = getPassword("Enter password to encrypt keystore file:")
			// }
			keyfile, err = GenerateKeypair("sr25519", testdir, testPassword)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			keyfile, err = GenerateKeypair("ed25519", testdir, testPassword)
			if err != nil {
				t.Fatal(err)
			}
		}

		expected = append(expected, keyfile)
	}

	keys, err := utils.KeystoreFilepaths(testdir)
	if err != nil {
		t.Fatal(err)
	}

	if len(keys) != len(expected) {
		t.Fatalf("Fail: expected %d keys in keystore, got %d", len(expected), len(keys))
	}

	sort.Slice(expected, func(i, j int) bool { return strings.Compare(expected[i], expected[j]) < 0 })

	for i, key := range keys {
		if strings.Compare(key, filepath.Base(expected[i])) != 0 {
			t.Fatalf("Fail: got %s expected %s", key, filepath.Base(expected[i]))
		}
	}
}

func TestUnlockKeys(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyfile, err := GenerateKeypair("sr25519", testdir, testPassword)
	require.Nil(t, err)

	t.Log(keyfile)

	ks := NewKeystore()

	err = UnlockKeys(ks, testdir, "0", string(testPassword))
	require.Nil(t, err)

	priv, err := ReadFromFileAndDecrypt(keyfile, testPassword)
	require.Nil(t, err)

	pub, err := priv.Public()
	require.Nil(t, err)

	kp, err := PrivateKeyToKeypair(priv)
	require.Nil(t, err)

	expected := ks.Get(pub.Address())
	if !reflect.DeepEqual(expected, kp) {
		t.Fatalf("Fail: got %v expected %v", expected, kp)
	}
}
