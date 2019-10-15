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
	"runtime"

	"github.com/ChainSafe/gossamer/internal/api"
	"github.com/ChainSafe/gossamer/p2p"
	"github.com/ChainSafe/gossamer/polkadb"
	"github.com/ChainSafe/gossamer/rpc"
)

const (
	DefaultRpcHttpHost = "localhost" // Default host interface for the HTTP RPC server
	DefaultRpcHttpPort = 8545        // Default port for

	// P2P
	DefaultP2PPort     = 7001
	DefaultP2PRandSeed = int64(0)

	DefaultGenesisPath = "./genesis.json"
)

var DefaultP2PBootstrap []string

var DefaultRpcModules = []api.Module{"system"}

var (
	// P2P
	DefaultP2PConfig = p2p.Config{
		Port:           DefaultP2PPort,
		RandSeed:       DefaultP2PRandSeed,
		BootstrapNodes: DefaultP2PBootstrap,
		NoBootstrap:    false,
		NoMdns:         false,
	}

	// DB
	DefaultDBConfig = polkadb.Config{
		DataDir: DefaultDataDir(),
	}

	// RPC
	DefaultRpcConfig = rpc.Config{
		Host:    DefaultRpcHttpHost,
		Port:    DefaultRpcHttpPort,
		Modules: DefaultRpcModules,
	}
)

// DefaultConfig is the default settings used when a config.toml file is not passed in during instantiation
func DefaultConfig() *Config {
	return &Config{
		P2pCfg: DefaultP2PConfig,
		DbCfg:  DefaultDBConfig,
		RpcCfg: DefaultRpcConfig,
	}
}

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
