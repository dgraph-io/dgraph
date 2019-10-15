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
	cfg "github.com/ChainSafe/gossamer/config"
	"github.com/urfave/cli"
)

var (
	// BadgerDB directory
	DataDirFlag = cli.StringFlag{
		Name:  "datadir",
		Usage: "Data directory for the database",
		Value: cfg.DefaultDataDir(),
	}
	// cli service settings
	VerbosityFlag = cli.StringFlag{
		Name:  "verbosity",
		Usage: "Supports levels crit (silent) to trce (trace)",
		Value: "info",
	}
	//Genesis
	GenesisFlag = cli.StringFlag{
		Name:  "genesis",
		Usage: "Path to genesis JSON file",
		Value: cfg.DefaultGenesisPath,
	}
)

// P2P flags
var (
	// P2P service settings
	BootnodesFlag = cli.StringFlag{
		Name:  "bootnodes",
		Usage: "Comma separated enode URLs for P2P discovery bootstrap",
		Value: "",
	}
	P2pPortFlag = cli.UintFlag{
		Name:  "p2pport",
		Usage: "Set P2P listening port",
		Value: cfg.DefaultP2PPort,
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
		Value: cfg.DefaultRpcHttpHost,
	}
	RpcPortFlag = cli.IntFlag{
		Name:  "rpcport",
		Usage: "HTTP-RPC server listening port",
		Value: cfg.DefaultRpcHttpPort,
	}
	RpcModuleFlag = cli.StringFlag{
		Name:  "rpcmods",
		Usage: "API modules to enable via HTTP-RPC, comma separated list",
		Value: "",
	}
)
