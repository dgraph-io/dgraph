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

package main

import (
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/lib/keystore"

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
