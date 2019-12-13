package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ChainSafe/gossamer/keystore"
)

var testKeystoreDir = "./test_datadir/"
var testPassword = []byte("1234")

func TestGenerateCommand(t *testing.T) {
	ctx, err := createCliContext("account generate", []string{"generate", "datadir"}, []interface{}{true, testKeystoreDir})
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testKeystoreDir)

	command := accountCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenerateCommand_Password(t *testing.T) {
	ctx, err := createCliContext("account generate", []string{"generate", "datadir", "password"}, []interface{}{true, testKeystoreDir, string(testPassword)})
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testKeystoreDir)

	command := accountCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenerateCommand_Type(t *testing.T) {
	ctx, err := createCliContext("account generate", []string{"generate", "datadir", "type"}, []interface{}{true, testKeystoreDir, "ed25519"})
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testKeystoreDir)

	command := accountCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenerateKey_Sr25519(t *testing.T) {
	keyfile, err := generateKeypair("sr25519", testKeystoreDir, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testKeystoreDir)

	keys, err := listKeys(testKeystoreDir)
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
	keyfile, err := generateKeypair("ed25519", testKeystoreDir, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testKeystoreDir)

	keys, err := listKeys(testKeystoreDir)
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

	kscontents := new(keystore.EncryptedKeystore)
	err = json.Unmarshal(contents, kscontents)
	if err != nil {
		t.Fatal(err)
	}

	if kscontents.Type != "ed25519" {
		t.Fatalf("Fail: got %s expected %s", kscontents.Type, "ed25519")
	}
}

func TestGenerateKey_Secp256k1(t *testing.T) {
	keyfile, err := generateKeypair("secp256k1", testKeystoreDir, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testKeystoreDir)

	keys, err := listKeys(testKeystoreDir)
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

	kscontents := new(keystore.EncryptedKeystore)
	err = json.Unmarshal(contents, kscontents)
	if err != nil {
		t.Fatal(err)
	}

	if kscontents.Type != "secp256k1" {
		t.Fatalf("Fail: got %s expected %s", kscontents.Type, "secp256k1")
	}
}

func TestGenerateKey_NoType(t *testing.T) {
	keyfile, err := generateKeypair("", testKeystoreDir, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testKeystoreDir)

	contents, err := ioutil.ReadFile(keyfile)
	if err != nil {
		t.Fatal(err)
	}

	kscontents := new(keystore.EncryptedKeystore)
	err = json.Unmarshal(contents, kscontents)
	if err != nil {
		t.Fatal(err)
	}

	if kscontents.Type != "sr25519" {
		t.Fatalf("Fail: got %s expected %s", kscontents.Type, "sr25519")
	}
}

func TestImportCommand(t *testing.T) {
	filename := ""
	defer os.RemoveAll(testKeystoreDir)

	ctx, err := createCliContext("account import", []string{"import", "datadir"}, []interface{}{filename, testKeystoreDir})
	if err != nil {
		t.Fatal(err)
	}

	command := accountCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestImportKey_ShouldFail(t *testing.T) {
	_, err := importKey("./notakey.key", testKeystoreDir)
	if err == nil {
		t.Fatal("did not err")
	}
}

func TestImportKey(t *testing.T) {
	keypath := "../../"

	importkeyfile, err := generateKeypair("sr25519", keypath, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(importkeyfile)

	keyfile, err := importKey(importkeyfile, testKeystoreDir)
	if err != nil {
		t.Fatal(err)
	}

	keys, err := listKeys(testKeystoreDir)
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testKeystoreDir)

	if len(keys) != 1 {
		t.Fatal("fail")
	}

	if strings.Compare(keys[0], filepath.Base(keyfile)) != 0 {
		t.Fatalf("Fail: got %s expected %s", keys[0], keyfile)
	}
}

func TestListCommand(t *testing.T) {
	defer os.RemoveAll(testKeystoreDir)

	ctx, err := createCliContext("account list", []string{"list", "datadir"}, []interface{}{true, testKeystoreDir})
	if err != nil {
		t.Fatal(err)
	}

	command := accountCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestListKeys(t *testing.T) {
	expected := []string{}
	defer os.RemoveAll(testKeystoreDir)

	for i := 0; i < 5; i++ {
		var err error
		var keyfile string
		if i%2 == 0 {
			keyfile, err = generateKeypair("sr25519", testKeystoreDir, testPassword)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			keyfile, err = generateKeypair("ed25519", testKeystoreDir, testPassword)
			if err != nil {
				t.Fatal(err)
			}
		}

		expected = append(expected, keyfile)
	}

	keys, err := listKeys(testKeystoreDir)
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
