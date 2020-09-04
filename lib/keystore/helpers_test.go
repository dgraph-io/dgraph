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
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/stretchr/testify/require"
)

func TestLoadKeystore(t *testing.T) {
	ks := NewBasicKeystore("test", crypto.Sr25519Type)
	err := LoadKeystore("alice", ks)
	require.Nil(t, err)
	require.Equal(t, 1, ks.Size())

	ks = NewBasicKeystore("test", crypto.Ed25519Type)
	err = LoadKeystore("bob", ks)
	require.Nil(t, err)
	require.Equal(t, 1, ks.Size())
}

var testKeyTypes = []struct {
	testType     string
	expectedType string
}{
	{testType: "babe", expectedType: crypto.Sr25519Type},
	{testType: "gran", expectedType: crypto.Ed25519Type},
	{testType: "acco", expectedType: crypto.Sr25519Type},
	{testType: "aura", expectedType: crypto.Sr25519Type},
	{testType: "imon", expectedType: crypto.Sr25519Type},
	{testType: "audi", expectedType: crypto.Sr25519Type},
	{testType: "dumy", expectedType: crypto.Sr25519Type},
	{testType: "xxxx", expectedType: crypto.UnknownType},
}

func TestDetermineKeyType(t *testing.T) {
	for _, test := range testKeyTypes {
		output := DetermineKeyType(test.testType)
		require.Equal(t, test.expectedType, output)
	}
}

func TestGenerateKey_Sr25519(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyfile, err := GenerateKeypair("sr25519", nil, testdir, testPassword)
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

	keyfile, err := GenerateKeypair("ed25519", nil, testdir, testPassword)
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

	keyfile, err := GenerateKeypair("secp256k1", nil, testdir, testPassword)
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

	keyfile, err := GenerateKeypair("", nil, testdir, testPassword)
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

	importkeyfile, err := GenerateKeypair("sr25519", nil, keypath, testPassword)
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
			keyfile, err = GenerateKeypair("sr25519", nil, testdir, testPassword)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			keyfile, err = GenerateKeypair("ed25519", nil, testdir, testPassword)
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

	keyfile, err := GenerateKeypair("sr25519", nil, testdir, testPassword)
	require.Nil(t, err)

	t.Log(keyfile)

	ks := NewBasicKeystore("test", crypto.Sr25519Type)

	err = UnlockKeys(ks, testdir, "0", string(testPassword))
	require.Nil(t, err)

	priv, err := ReadFromFileAndDecrypt(keyfile, testPassword)
	require.Nil(t, err)

	pub, err := priv.Public()
	require.Nil(t, err)

	kp, err := PrivateKeyToKeypair(priv)
	require.Nil(t, err)

	expected := ks.GetKeypair(pub)
	if !reflect.DeepEqual(expected, kp) {
		t.Fatalf("Fail: got %v expected %v", expected, kp)
	}
}

func TestImportRawPrivateKey_NoType(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyfile, err := ImportRawPrivateKey("0x33a6f3093f158a7109f679410bef1a0c54168145e0cecb4df006c1c2fffb1f09", "", testdir, testPassword)
	require.NoError(t, err)

	contents, err := ioutil.ReadFile(keyfile)
	require.NoError(t, err)

	kscontents := new(EncryptedKeystore)
	err = json.Unmarshal(contents, kscontents)
	require.NoError(t, err)
	require.Equal(t, "sr25519", kscontents.Type)
	require.Equal(t, "0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d", kscontents.PublicKey)
}

func TestImportRawPrivateKey_Sr25519(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyfile, err := ImportRawPrivateKey("0x33a6f3093f158a7109f679410bef1a0c54168145e0cecb4df006c1c2fffb1f09", "sr25519", testdir, testPassword)
	require.NoError(t, err)

	contents, err := ioutil.ReadFile(keyfile)
	require.NoError(t, err)

	kscontents := new(EncryptedKeystore)
	err = json.Unmarshal(contents, kscontents)
	require.NoError(t, err)
	require.Equal(t, "sr25519", kscontents.Type)
	require.Equal(t, "0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d", kscontents.PublicKey)
}

func TestImportRawPrivateKey_Ed25519(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyfile, err := ImportRawPrivateKey("0x33a6f3093f158a7109f679410bef1a0c54168145e0cecb4df006c1c2fffb1f09", "ed25519", testdir, testPassword)
	require.NoError(t, err)

	contents, err := ioutil.ReadFile(keyfile)
	require.NoError(t, err)

	kscontents := new(EncryptedKeystore)
	err = json.Unmarshal(contents, kscontents)
	require.NoError(t, err)
	require.Equal(t, "ed25519", kscontents.Type)
	require.Equal(t, "0x6dfb362eb332449782b7260bcff6d8777242acdea3293508b22d33ce7336a8b3", kscontents.PublicKey)
}

func TestImportRawPrivateKey_Secp256k1(t *testing.T) {
	testdir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	keyfile, err := ImportRawPrivateKey("0x33a6f3093f158a7109f679410bef1a0c54168145e0cecb4df006c1c2fffb1f09", "secp256k1", testdir, testPassword)
	require.NoError(t, err)

	contents, err := ioutil.ReadFile(keyfile)
	require.NoError(t, err)

	kscontents := new(EncryptedKeystore)
	err = json.Unmarshal(contents, kscontents)
	require.NoError(t, err)
	require.Equal(t, "secp256k1", kscontents.Type)
	require.Equal(t, "0x03409094a319b2961660c3ebcc7d206266182c1b3e60d341b5fb17e6851865825c", kscontents.PublicKey)
}
