package main

import (
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/keystore"
	"github.com/stretchr/testify/require"
)

func TestUnlock(t *testing.T) {
	testKeystoreDir := path.Join(os.TempDir(), "gossamer-test")
	defer os.RemoveAll(testKeystoreDir)

	var testPassword = []byte("1234")

	keyfile, err := generateKeypair("sr25519", testKeystoreDir, testPassword)
	require.Nil(t, err)

	t.Log(keyfile)

	ctx, err := createCliContext("unlock",
		[]string{"datadir", "unlock", "password"},
		[]interface{}{testKeystoreDir, "0", string(testPassword)},
	)
	require.Nil(t, err)

	ks := keystore.NewKeystore()

	err = unlockKeys(ctx, testKeystoreDir, ks)
	require.Nil(t, err)

	priv, err := keystore.ReadFromFileAndDecrypt(keyfile, testPassword)
	require.Nil(t, err)

	pub, err := priv.Public()
	require.Nil(t, err)

	kp, err := keystore.PrivateKeyToKeypair(priv)
	require.Nil(t, err)

	kpRes := ks.Get(pub.Address())
	if !reflect.DeepEqual(kpRes, kp) {
		t.Fatalf("Fail: got %v expected %v", kpRes, kp)
	}
}

func TestUnlockFlag(t *testing.T) {
	testKeystoreDir := path.Join(os.TempDir(), "gossamer-test")
	defer os.RemoveAll(testKeystoreDir)

	var testPassword = []byte("1234")

	_, err := generateKeypair("sr25519", testKeystoreDir, testPassword)
	require.Nil(t, err)

	genesisPath := createTempGenesisFile(t)
	defer os.Remove(genesisPath)

	ctx, err := createCliContext("load genesis",
		[]string{"datadir", "genesis"},
		[]interface{}{testKeystoreDir, genesisPath},
	)
	require.Nil(t, err)

	command := initCommand
	err = command.Run(ctx)
	require.Nil(t, err)

	ctx, err = createCliContext("unlock",
		[]string{"datadir", "genesis", "unlock", "password"},
		[]interface{}{testKeystoreDir, genesisPath, "0", string(testPassword)},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = makeNode(ctx)
	require.Nil(t, err)
}
