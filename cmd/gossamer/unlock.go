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
	"fmt"
	"strings"

	"github.com/ChainSafe/gossamer/cmd/utils"
	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/keystore"
	"github.com/urfave/cli"
)

// unlockKeys unlocks keys specified by the --unlock flag with the passwords given by --password
// and places them into the keystore
func unlockKeys(ctx *cli.Context, datadir string, ks *keystore.Keystore) error {
	var indices []int
	var passwords []string
	var err error

	keydir, err := keystoreDir(datadir)
	if err != nil {
		return err
	}

	// indices of keys to unlock
	if keyindices := ctx.String(utils.UnlockFlag.Name); keyindices != "" {
		indices, err = common.StringToInts(keyindices)
		if err != nil {
			return err
		}
	}

	// passwords corresponding to the keys
	if passwordsStr := ctx.String(utils.PasswordFlag.Name); passwordsStr != "" {
		passwords = strings.Split(passwordsStr, ",")
	} else {
		// no passwords specified, prompt user
		passwords = []string{}
		for _, idx := range indices {
			pwd := getPassword(fmt.Sprintf("Enter password to decrypt keystore file [%d]:", idx))
			passwords = append(passwords, string(pwd))
		}
	}

	if len(passwords) != len(indices) {
		return fmt.Errorf("number of passwords given does not match number of keys to unlock")
	}

	// get paths to key files
	keyfiles, err := getKeyFiles(datadir)
	if err != nil {
		return err
	}

	if len(keyfiles) < len(indices) {
		return fmt.Errorf("number of accounts to unlock is greater than number of accounts in keystore")
	}

	// for each key to unlock, read its file and decrypt contents and add to keystore
	for i, idx := range indices {
		if idx >= len(keyfiles) {
			return fmt.Errorf("invalid account index: %d", idx)
		}

		keyfile := keyfiles[idx]
		priv, err := keystore.ReadFromFileAndDecrypt(keydir+"/"+keyfile, []byte(passwords[i]))
		if err != nil {
			return fmt.Errorf("cannot decrypt key file %s: %s", keyfile, err)
		}

		kp, err := keystore.PrivateKeyToKeypair(priv)
		if err != nil {
			return fmt.Errorf("cannot create keypair from private key %d: %s", idx, err)
		}

		ks.Insert(kp)
	}

	return nil
}
