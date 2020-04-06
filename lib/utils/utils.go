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

package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// PathExists returns true if the named file or directory exists, otherwise false
func PathExists(p string) bool {
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// HomeDir returns the user's current HOME directory
func HomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}

// ExpandDir expands a tilde prefix path to a full home path
func ExpandDir(targetPath string) string {
	if strings.HasPrefix(targetPath, "~\\") || strings.HasPrefix(targetPath, "~/") {
		if homeDir := HomeDir(); homeDir != "" {
			targetPath = homeDir + targetPath[1:]
		}
	} else if strings.HasPrefix(targetPath, ".\\") || strings.HasPrefix(targetPath, "./") {
		targetPath, _ = filepath.Abs(targetPath)
	}
	return path.Clean(os.ExpandEnv(targetPath))
}

// DataDir attempts to create a data directory using the given name within the
// gossamer directory within the user's HOME directory, returns absolute path
// or, if unable to locate HOME directory, returns within current directory
func DataDir(name string) string {
	home := HomeDir()
	if home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Gossamer", name)
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "Gossamer", name)
		} else {
			return filepath.Join(home, ".gossamer", name)
		}
	}
	return name
}

// KeystoreDir returns the absolute filepath of the keystore directory
func KeystoreDir(datadir string) (keystorepath string, err error) {
	// datadir specified, set keystore filepath to absolute path of [datadir]/keystore
	if datadir != "" {
		datadir = ExpandDir(datadir)
		keystorepath, err = filepath.Abs(datadir + "/keystore")
		if err != nil {
			return "", fmt.Errorf("failed to create absolute filepath: %s", err)
		}
	}

	// if datadir does not exist, create it
	if _, err = os.Stat(keystorepath); os.IsNotExist(err) {
		err = os.Mkdir(keystorepath, os.ModePerm)
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

// KeystoreFiles returns the filenames of all the keys in the datadir's keystore
func KeystoreFiles(datadir string) ([]string, error) {
	keystorepath, err := KeystoreDir(datadir)
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

// KeystoreFilepaths lists all the keys in the datadir/keystore/ directory and returns them as a list of filepaths
func KeystoreFilepaths(datadir string) ([]string, error) {
	keys, err := KeystoreFiles(datadir)
	if err != nil {
		return nil, err
	}

	for i, key := range keys {
		fmt.Printf("[%d] %s\n", i, key)
	}

	return keys, nil
}
