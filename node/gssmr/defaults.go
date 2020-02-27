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

package gssmr

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/ChainSafe/gossamer/dot"
)

const (
	// DefaultNode Default node implementation
	DefaultNode = "gssmr"
	// DefaultConfigPath Default toml configuration path
	DefaultConfigPath = "./node/gssmr/config.toml"
	// DefaultGenesisPath Default genesis configuration path
	DefaultGenesisPath = "./node/gssmr/genesis.json"

	// DefaultNetworkPort network port
	DefaultNetworkPort = 7001
	// DefaultNetworkProtocolID ID
	DefaultNetworkProtocolID = "/gossamer/gssmr/0"

	// DefaultRPCHTTPHost Default host interface for the HTTP RPC server
	DefaultRPCHTTPHost = "localhost"
	// DefaultRPCHTTPPort http port
	DefaultRPCHTTPPort = 8545
)

var (
	// DefaultNetworkBootnodes Must be non-nil to match toml parsing semantics
	DefaultNetworkBootnodes = []string{}

	// DefaultRPCModules holds defaults RPC modules
	DefaultRPCModules = []string{"system", "author"}
)

var (
	// DefaultGlobalConfig Global
	DefaultGlobalConfig = dot.GlobalConfig{
		DataDir:   DefaultDataDir(),
		Roles:     byte(1), // full node
		Authority: true,    // BABE block producer
	}

	// DefaultNetworkConfig Network
	DefaultNetworkConfig = dot.NetworkConfig{
		Bootnodes:   DefaultNetworkBootnodes,
		ProtocolID:  DefaultNetworkProtocolID,
		Port:        DefaultNetworkPort,
		NoBootstrap: false,
		NoMdns:      false,
	}

	// DefaultRPCConfig RPC
	DefaultRPCConfig = dot.RPCConfig{
		Host:    DefaultRPCHTTPHost,
		Port:    DefaultRPCHTTPPort,
		Modules: DefaultRPCModules,
	}
)

// DefaultConfig is the default settings used when a config.toml file is not passed in during instantiation
func DefaultConfig() *dot.Config {
	return &dot.Config{
		Global:  DefaultGlobalConfig,
		Network: DefaultNetworkConfig,
		RPC:     DefaultRPCConfig,
	}
}

// DefaultDataDir is the default data directory for the default node
func DefaultDataDir() string {
	// Try to place the data folder in the user's home dir
	home := HomeDir()
	if home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Gossamer")
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "Gossamer")
		} else {
			return filepath.Join(home, ".gossamer", DefaultNode)
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	return ""
}

// HomeDir returns the current HOME directory
func HomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}
