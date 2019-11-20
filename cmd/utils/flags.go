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
	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

var (
	Ed25519KeyType = "ed25519"
	Sr25519KeyType = "sr25519"
)

var (
	// BadgerDB directory
	DataDirFlag = cli.StringFlag{
		Name:  "datadir",
		Usage: "Data directory for the database",
	}
	// cli service settings
	VerbosityFlag = cli.StringFlag{
		Name:  "verbosity",
		Usage: "Supports levels crit (silent) to trce (trace)",
		Value: log.LvlInfo.String(),
	}
	// Genesis
	GenesisFlag = cli.StringFlag{
		Name:  "genesis",
		Usage: "Path to genesis JSON file",
	}
	// config file
	ConfigFileFlag = cli.StringFlag{
		Name:  "config",
		Usage: "TOML configuration file",
	}
)

// P2P flags
var (
	// P2P service settings
	BootnodesFlag = cli.StringFlag{
		Name:  "bootnodes",
		Usage: "Comma separated enode URLs for P2P discovery bootstrap",
	}
	P2pPortFlag = cli.UintFlag{
		Name:  "p2pport",
		Usage: "Set P2P listening port",
	}
	NoBootstrapFlag = cli.BoolFlag{
		Name:  "nobootstrap",
		Usage: "Disables p2p bootstrapping (mdns still enabled)",
	}

	NoMdnsFlag = cli.BoolFlag{
		Name:  "nomdns",
		Usage: "Disables p2p mdns discovery",
	}
)

// RPC flags
var (
	RpcEnabledFlag = cli.BoolFlag{
		Name:  "rpc",
		Usage: "Enable the HTTP-RPC server",
	}
	RpcHostFlag = cli.StringFlag{
		Name:  "rpchost",
		Usage: "HTTP-RPC server listening hostname",
	}
	RpcPortFlag = cli.IntFlag{
		Name:  "rpcport",
		Usage: "HTTP-RPC server listening port",
	}
	RpcModuleFlag = cli.StringFlag{
		Name:  "rpcmods",
		Usage: "API modules to enable via HTTP-RPC, comma separated list",
	}
)

// Account management flags
var (
	GenerateFlag = cli.BoolFlag{
		Name:  "generate",
		Usage: "Generate a new keypair. If type is not specified, defaults to sr25519",
	}
	PasswordFlag = cli.StringFlag{
		Name:  "password",
		Usage: "Password used to encrypt the keystore. Used with --generate or --unlock",
	}
	ImportFlag = cli.StringFlag{
		Name:  "import",
		Usage: "Import encrypted keystore file generated with gossamer",
	}
	ListFlag = cli.BoolFlag{
		Name:  "list",
		Usage: "List node keys",
	}
	Ed25519Flag = cli.BoolFlag{
		Name:  "ed25519",
		Usage: "Specify account type as ed25519",
	}
	Sr25519Flag = cli.BoolFlag{
		Name:  "sr25519",
		Usage: "Specify account type as sr25519",
	}
	// TODO: account unlocking
)
