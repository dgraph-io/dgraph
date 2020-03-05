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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/secp256k1"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/utils"
	"github.com/ChainSafe/gossamer/node/gssmr"

	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
	"golang.org/x/crypto/ssh/terminal"
)

// handleAccounts manages the flags for the account subcommand
// first, if the generate flag is set, if so, it generates a new keypair
// then, if the import flag is set, if so, it imports a keypair
// finally, if the list flag is set, it lists all the keys in the keystore
func handleAccounts(ctx *cli.Context) error {
	err := startLogger(ctx)
	if err != nil {
		log.Error("[gossamer] Failed to start logger", "error", err)
		return err
	}

	// key directory is datadir/keystore/
	var datadir string
	if dir := ctx.String(DataDirFlag.Name); dir != "" {
		datadir, err = filepath.Abs(dir)
		if err != nil {
			log.Error("[gossamer] Failed to create absolute filepath", "error", err)
			return err
		}
	}

	// check if we want to generate a new keypair
	// can specify key type using --ed25519, --sr25519, --secp256k1
	// otherwise defaults to sr25519
	if keygen := ctx.Bool(GenerateFlag.Name); keygen {
		log.Info("[gossamer] Generating keypair...")

		// check if --ed25519, --sr25519, --secp256k1 is set
		keytype := crypto.Sr25519Type
		if flagtype := ctx.Bool(Sr25519Flag.Name); flagtype {
			keytype = crypto.Sr25519Type
		} else if flagtype := ctx.Bool(Ed25519Flag.Name); flagtype {
			keytype = crypto.Ed25519Type
		} else if flagtype := ctx.Bool(Secp256k1Flag.Name); flagtype {
			keytype = crypto.Secp256k1Type
		}

		// check if --password is set
		var password []byte = nil
		if pwdflag := ctx.String(PasswordFlag.Name); pwdflag != "" {
			password = []byte(pwdflag)
		}

		// generate keypair
		_, err = generateKeypair(keytype, datadir, password)
		if err != nil {
			log.Error("[gossamer] Failed to generate keypair", "error", err)
			return err
		}
	}

	// check if --import is set
	if keyimport := ctx.String(ImportFlag.Name); keyimport != "" {
		log.Info("[gossamer] Importing keypair...")

		// import keypair
		_, err = importKey(keyimport, datadir)
		if err != nil {
			log.Error("[gossamer] Failed to import key", "error", err)
			return err
		}
	}

	// check if --list is set
	if keylist := ctx.Bool(ListFlag.Name); keylist {
		_, err = listKeys(datadir)
		if err != nil {
			log.Error("[gossamer] Failed to list keys", "error", err)
			return err
		}
	}

	return nil
}

// importKey imports a key specified by its filename to datadir/keystore/
// it saves it under the filename "[publickey].key"
// it returns the absolute path of the imported key file
func importKey(filename, datadir string) (string, error) {
	keystorepath, err := keystoreDir(datadir)
	if err != nil {
		return "", fmt.Errorf("failed to get keystore directory: %s", err)
	}

	importdata, err := ioutil.ReadFile(filepath.Clean(filename))
	if err != nil {
		return "", fmt.Errorf("failed to read import file: %s", err)
	}

	ksjson := new(keystore.EncryptedKeystore)
	err = json.Unmarshal(importdata, ksjson)
	if err != nil {
		return "", fmt.Errorf("failed to read import data: %s", err)
	}

	keystorefile, err := filepath.Abs(keystorepath + "/" + ksjson.PublicKey[2:] + ".key")
	if err != nil {
		return "", fmt.Errorf("failed to create keystore filepath: %s", err)
	}

	err = ioutil.WriteFile(keystorefile, importdata, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write to keystore file: %s", err)
	}

	log.Info("[gossamer] Key imported", "pubkey", ksjson.PublicKey, "file", keystorefile)

	return keystorefile, nil
}

// listKeys lists all the keys in the datadir/keystore/ directory and returns them as a list of filepaths
func listKeys(datadir string) ([]string, error) {
	keys, err := getKeyFiles(datadir)
	if err != nil {
		return nil, err
	}

	for i, key := range keys {
		fmt.Printf("[%d] %s\n", i, key)
	}

	return keys, nil
}

// getKeyFiles returns the filenames of all the keys in the datadir's keystore
func getKeyFiles(datadir string) ([]string, error) {
	keystorepath, err := keystoreDir(datadir)
	if err != nil {
		return nil, fmt.Errorf("failed to get keystore directory: %s", err)
	}

	files, err := ioutil.ReadDir(keystorepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read keystore directory: %s", err)
	}

	keys := []string{}

	for _, f := range files {
		ext := filepath.Ext(f.Name())
		if ext == ".key" {
			keys = append(keys, f.Name())
		}
	}

	return keys, nil
}

// generateKeypair create a new keypair with the corresponding type and saves it to datadir/keystore/[public key].key
// in json format encrypted using the specified password
// it returns the resulting filepath of the new key
func generateKeypair(keytype, datadir string, password []byte) (string, error) {
	if password == nil {
		password = getPassword("Enter password to encrypt keystore file:")
	}

	if keytype == "" {
		keytype = crypto.Sr25519Type
	}

	var kp crypto.Keypair
	var err error
	if keytype == crypto.Sr25519Type {
		// generate sr25519 keys
		kp, err = sr25519.GenerateKeypair()
		if err != nil {
			return "", fmt.Errorf("failed to generate sr25519 keypair: %s", err)
		}
	} else if keytype == crypto.Ed25519Type {
		// generate ed25519 keys
		kp, err = ed25519.GenerateKeypair()
		if err != nil {
			return "", fmt.Errorf("failed to generate ed25519 keypair: %s", err)
		}
	} else if keytype == crypto.Secp256k1Type {
		// generate secp256k1 keys
		kp, err = secp256k1.GenerateKeypair()
		if err != nil {
			return "", fmt.Errorf("failed to generate secp256k1 keypair: %s", err)
		}
	}

	keystorepath, err := keystoreDir(datadir)
	if err != nil {
		return "", fmt.Errorf("failed to get keystore directory: %s", err)
	}

	pub := hex.EncodeToString(kp.Public().Encode())
	fp, err := filepath.Abs(keystorepath + "/" + pub + ".key")
	if err != nil {
		return "", fmt.Errorf("failed to create absolute filepath: %s", err)
	}

	file, err := os.OpenFile(fp, os.O_EXCL|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}

	defer func() {
		err = file.Close()
		if err != nil {
			log.Error("[gossamer] Failed to close keystore file")
		}
	}()

	err = keystore.EncryptAndWriteToFile(file, kp.Private(), password)
	if err != nil {
		return "", fmt.Errorf("failed to write key to file: %s", err)
	}

	log.Info("[gossamer] Generated key", "type", keytype, "pubkey", pub, "file", fp)

	return fp, nil
}

// keystoreDir returns the absolute filepath of the keystore directory
func keystoreDir(datadir string) (keystorepath string, err error) {
	// datadir specified, set keystore filepath to absolute path of [datadir]/keystore
	if datadir != "" {
		keystorepath, err = filepath.Abs(datadir + "/keystore")
		if err != nil {
			return "", fmt.Errorf("failed to create absolute filepath: %s", err)
		}
	} else {
		// datadir not specified, use default datadir and set keystore filepath to absolute path of [datadir]/keystore
		datadir = utils.DataDir(gssmr.DefaultNode)
		keystorepath, err = filepath.Abs(datadir + "/keystore")
		if err != nil {
			return "", fmt.Errorf("failed to create keystore filepath: %s", err)
		}
	}

	// if datadir does not exist, create it
	if _, err = os.Stat(datadir); os.IsNotExist(err) {
		err = os.Mkdir(datadir, os.ModePerm)
		if err != nil {
			return "", fmt.Errorf("failed to create data directory: %s", err)
		}
	}

	// if datadir/keystore does not exist, create it
	if _, err = os.Stat(keystorepath); os.IsNotExist(err) {
		err = os.Mkdir(keystorepath, os.ModePerm)
		if err != nil {
			return "", fmt.Errorf("failed to create keystore directory: %s", err)
		}
	}

	return keystorepath, nil
}

// prompt user to enter password for encrypted keystore
func getPassword(msg string) []byte {
	for {
		fmt.Println(msg)
		fmt.Print("> ")
		password, err := terminal.ReadPassword(syscall.Stdin)
		if err != nil {
			fmt.Printf("invalid input: %s\n", err)
		} else {
			fmt.Printf("\n")
			return password
		}
	}
}
