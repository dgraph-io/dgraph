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

package cfg

import (
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/inconshreveable/log15"
)

const (
	DefaultHTTPHost = "localhost" // Default host interface for the HTTP RPC server
	DefaultHTTPPort = 8545
)

// DefaultDataDir is the default data directory to use for the databases and other
// persistence requirements.
func DefaultDataDir() string {
	// Try to place the data folder in the user's home dir
	home := homeDir()
	if home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Gossamer")
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "Gossamer")
		} else {
			return filepath.Join(home, ".gossamer")
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	return ""
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}

// CheckConfig finds file based on ext input
func CheckConfig(ext string) string {
	pathS, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	var file string
	if err = filepath.Walk(pathS, func(path string, f os.FileInfo, _ error) error {
		if !f.IsDir() && f.Name() == "config.toml" {
			r, e := regexp.MatchString(ext, f.Name())
			if e == nil && r {
				file = f.Name()
				return nil
			}
		} else if !f.IsDir() && f.Name() != "Gopkg.toml" {
			r, e := regexp.MatchString(ext, f.Name())
			if e == nil && r {
				file = f.Name()
				return nil
			}
		}
		return nil
	}); err != nil {
		log15.Error("please specify a config file", "err", err)
	}
	return file
}
