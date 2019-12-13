package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/keystore"
)

func TestUnlock(t *testing.T) {
	var testKeystoreDir = "./test_keystore/"
	var testPassword = []byte("1234")

	keyfile, err := generateKeypair("sr25519", testKeystoreDir, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(keyfile)

	defer os.RemoveAll(testKeystoreDir)

	ctx, err := createCliContext("unlock",
		[]string{"datadir", "unlock", "password"},
		[]interface{}{testKeystoreDir, "0", string(testPassword)},
	)
	if err != nil {
		t.Fatal(err)
	}

	ks := keystore.NewKeystore()

	err = unlockKeys(ctx, testKeystoreDir, ks)
	if err != nil {
		t.Fatal(err)
	}

	priv, err := keystore.ReadFromFileAndDecrypt(keyfile, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	pub, err := priv.Public()
	if err != nil {
		t.Fatal(err)
	}

	kp, err := keystore.PrivateKeyToKeypair(priv)
	if err != nil {
		t.Fatal(err)
	}

	kpRes := ks.Get(pub.Address())
	if !reflect.DeepEqual(kpRes, kp) {
		t.Fatalf("Fail: got %v expected %v", kpRes, kp)
	}
}

func TestUnlockFlag(t *testing.T) {
	var testKeystoreDir = "./test_keystore/"
	var testPassword = []byte("1234")

	_, err := generateKeypair("sr25519", testKeystoreDir, testPassword)
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testKeystoreDir)

	genesispath := createTempGenesisFile(t)
	defer os.Remove(genesispath)

	ctx, err := createCliContext("load genesis",
		[]string{"datadir", "genesis"},
		[]interface{}{testKeystoreDir, genesispath},
	)
	if err != nil {
		t.Fatal(err)
	}

	command := initCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	ctx, err = createCliContext("unlock",
		[]string{"datadir", "unlock", "password"},
		[]interface{}{testKeystoreDir, "0", string(testPassword)},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = makeNode(ctx)
	if err != nil {
		t.Fatal(err)
	}
}
