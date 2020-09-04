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

	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/urfave/cli"
)

// accountAction executes the action for the "account" subcommand
// first, if the generate flag is set, if so, it generates a new keypair
// then, if the import flag is set, if so, it imports a keypair
// finally, if the list flag is set, it lists all the keys in the keystore
func accountAction(ctx *cli.Context) error {
	// create dot configuration
	cfg, err := createDotConfig(ctx)
	if err != nil {
		logger.Error("failed to create dot configuration", "error", err)
		return err
	}

	basepath := cfg.Global.BasePath
	var file string

	// check if --ed25519, --sr25519, --secp256k1 is set
	keytype := crypto.Sr25519Type
	if flagtype := ctx.Bool(Sr25519Flag.Name); flagtype {
		keytype = crypto.Sr25519Type
	} else if flagtype := ctx.Bool(Ed25519Flag.Name); flagtype {
		keytype = crypto.Ed25519Type
	} else if flagtype := ctx.Bool(Secp256k1Flag.Name); flagtype {
		keytype = crypto.Secp256k1Type
	}

	// check --generate flag and generate new keypair
	if keygen := ctx.Bool(GenerateFlag.Name); keygen {
		logger.Info("generating keypair...")

		file, err = keystore.GenerateKeypair(keytype, nil, basepath, getKeystorePassword(ctx))
		if err != nil {
			logger.Error("failed to generate keypair", "error", err)
			return err
		}

		logger.Info("generated key", "file", file)
	}

	// check if --import is set
	if keyimport := ctx.String(ImportFlag.Name); keyimport != "" {
		logger.Info("importing keypair...")

		// import keypair
		_, err = keystore.ImportKeypair(keyimport, basepath)
		if err != nil {
			logger.Error("failed to import key", "error", err)
			return err
		}
	}

	// check if --list is set
	if keylist := ctx.Bool(ListFlag.Name); keylist {
		_, err = utils.KeystoreFilepaths(basepath)
		if err != nil {
			logger.Error("failed to list keys", "error", err)
			return err
		}
	}

	// check if --import-raw is set
	if importraw := ctx.String(ImportRawFlag.Name); importraw != "" {
		file, err = keystore.ImportRawPrivateKey(importraw, keytype, basepath, getKeystorePassword(ctx))
		if err != nil {
			logger.Error("failed to import private key", "error", err)
			return err
		}

		logger.Info("imported key", "file", file)
	}

	return nil
}

// getKeystorePassword checks if the --password flag is set, if not,
func getKeystorePassword(ctx *cli.Context) []byte {
	// check if --password is set
	var password []byte
	if pwdflag := ctx.String(PasswordFlag.Name); pwdflag != "" {
		password = []byte(pwdflag)
	}

	if password == nil {
		password = getPassword("Enter password to encrypt keystore file:")
	}

	return password
}

// unlockKeystore compares the length of passwords to the length of accounts,
// prompts the user for a password if no password is provided, and then unlocks
// the accounts within the provided keystore
func unlockKeystore(ks keystore.Keystore, basepath string, unlock string, password string) error {
	var passwords []string

	if password != "" {
		passwords = strings.Split(password, ",")

		// compare length of passwords to length of accounts to unlock (if password provided)
		if len(passwords) != len(unlock) {
			return fmt.Errorf("passwords length does not match unlock length")
		}

	} else {

		// compare length of passwords to length of accounts to unlock (if password not provided)
		if len(passwords) != len(unlock) {
			bytes := getPassword("Enter password to unlock keystore:")
			password = string(bytes)
		}

		err := keystore.UnlockKeys(ks, basepath, unlock, password)
		if err != nil {
			return fmt.Errorf("failed to unlock keys: %s", err)
		}
	}

	return nil
}
