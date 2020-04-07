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
	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

// Node flags
var (
	// UnlockFlag keystore
	UnlockFlag = cli.StringFlag{
		Name:  "unlock",
		Usage: "Unlock an account. eg. --unlock=0,2 to unlock accounts 0 and 2. Can be used with --password=[password] to avoid prompt. For multiple passwords, do --password=password1,password2",
	}
	// KeyFlag specifies a test keyring account to use
	KeyFlag = cli.StringFlag{
		Name:  "key",
		Usage: "Specify a test keyring account to use: eg --key=alice",
	}
	// RolesFlag role of the node (see Table D.2)
	RolesFlag = cli.StringFlag{
		Name:  "roles",
		Usage: "Roles of the gossamer node",
	}
)

// Global node configuration flags
var (
	// VerbosityFlag cli service settings
	VerbosityFlag = cli.StringFlag{
		Name:  "verbosity",
		Usage: "Supports levels crit (silent) to trce (trace)",
		Value: log.LvlInfo.String(),
	}
	// NameFlag node implementation name
	NameFlag = cli.StringFlag{
		Name:  "name",
		Usage: "Node implementation name",
	}
	// NodeFlag node implementation id used to load default node configuration
	NodeFlag = cli.StringFlag{
		Name:  "node",
		Usage: "Node implementation id used to load default node configuration",
	}
	// ConfigFlag TOML configuration file
	ConfigFlag = cli.StringFlag{
		Name:  "config",
		Usage: "TOML configuration file",
	}
	// DataDirFlag data directory for node
	DataDirFlag = cli.StringFlag{
		Name:  "datadir",
		Usage: "Data directory for the node",
	}
)

// Initialization-only flags
var (
	// GenesisFlag Path to genesis JSON file
	GenesisFlag = cli.StringFlag{
		Name:  "genesis",
		Usage: "Path to genesis JSON file",
	}
)

// Network service configuration flags
var (
	// PortFlag Set network listening port
	PortFlag = cli.UintFlag{
		Name:  "port",
		Usage: "Set network listening port",
	}
	// BootnodesFlag Network service settings
	BootnodesFlag = cli.StringFlag{
		Name:  "bootnodes",
		Usage: "Comma separated enode URLs for network discovery bootstrap",
	}
	// ProtocolFlag Set protocol id
	ProtocolFlag = cli.StringFlag{
		Name:  "protocol",
		Usage: "Set protocol id",
	}
	// NoBootstrapFlag Disables network bootstrapping
	NoBootstrapFlag = cli.BoolFlag{
		Name:  "nobootstrap",
		Usage: "Disables network bootstrapping (mdns still enabled)",
	}
	// NoMDNSFlag Disables network mdns
	NoMDNSFlag = cli.BoolFlag{
		Name:  "nomdns",
		Usage: "Disables network mdns discovery",
	}
)

// RPC service configuration flags
var (
	// RPCEnabledFlag Enable the HTTP-RPC
	RPCEnabledFlag = cli.BoolFlag{
		Name:  "rpc",
		Usage: "Enable the HTTP-RPC server",
	}
	// RPCHostFlag HTTP-RPC server listening hostname
	RPCHostFlag = cli.StringFlag{
		Name:  "rpchost",
		Usage: "HTTP-RPC server listening hostname",
	}
	// RPCPortFlag HTTP-RPC server listening port
	RPCPortFlag = cli.IntFlag{
		Name:  "rpcport",
		Usage: "HTTP-RPC server listening port",
	}
	// RPCModulesFlag API modules to enable via HTTP-RPC
	RPCModulesFlag = cli.StringFlag{
		Name:  "rpcmods",
		Usage: "API modules to enable via HTTP-RPC, comma separated list",
	}
)

// Account management flags
var (
	// GenerateFlag Generate a new keypair
	GenerateFlag = cli.BoolFlag{
		Name:  "generate",
		Usage: "Generate a new keypair. If type is not specified, defaults to sr25519",
	}
	// PasswordFlag Password used to encrypt the keystore.
	PasswordFlag = cli.StringFlag{
		Name:  "password",
		Usage: "Password used to encrypt the keystore. Used with --generate or --unlock",
	}
	// ImportFlag Import encrypted keystore
	ImportFlag = cli.StringFlag{
		Name:  "import",
		Usage: "Import encrypted keystore file generated with gossamer",
	}
	// ListFlag List node keys
	ListFlag = cli.BoolFlag{
		Name:  "list",
		Usage: "List node keys",
	}
	// Ed25519Flag Specify account type ed25519
	Ed25519Flag = cli.BoolFlag{
		Name:  "ed25519",
		Usage: "Specify account type as ed25519",
	}
	// Sr25519Flag Specify account type sr25519
	Sr25519Flag = cli.BoolFlag{
		Name:  "sr25519",
		Usage: "Specify account type as sr25519",
	}
	// Secp256k1Flag Specify account type secp256k1
	Secp256k1Flag = cli.BoolFlag{
		Name:  "secp256k1",
		Usage: "Specify account type as secp256k1",
	}
)

var (
	// GlobalFlags are flags that are valid for use with all commands
	GlobalFlags = []cli.Flag{
		VerbosityFlag,
		NameFlag,
		NodeFlag,
		ConfigFlag,
		DataDirFlag,
	}

	// InitFlags are flags that are valid for use with the init subcommand
	InitFlags = append(GlobalFlags, GenesisFlag)

	// AccountFlags are flags that are valid for use with the account subcommand
	AccountFlags = append([]cli.Flag{
		GenerateFlag,
		PasswordFlag,
		ImportFlag,
		ListFlag,
		Ed25519Flag,
		Sr25519Flag,
		Secp256k1Flag,
	}, GlobalFlags...)

	// CLIFlags are the flags that are valid for use with the gossamer command
	CLIFlags = append([]cli.Flag{
		KeyFlag,
		UnlockFlag,

		// network flags
		PortFlag,
		BootnodesFlag,
		ProtocolFlag,
		RolesFlag,
		NoBootstrapFlag,
		NoMDNSFlag,

		// rpc flags
		RPCEnabledFlag,
		RPCHostFlag,
		RPCPortFlag,
		RPCModulesFlag,
	}, GlobalFlags...)
)

// FixFlagOrder allow us to use various flag order formats, eg: (gossamer init --config config.toml and  gossamer --config config.toml init)
func FixFlagOrder(f func(ctx *cli.Context) error) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		for _, flagName := range ctx.FlagNames() {
			if ctx.IsSet(flagName) {
				if err := ctx.Set(flagName, ctx.String(flagName)); err != nil {
					if err := ctx.GlobalSet(flagName, ctx.String(flagName)); err != nil {
						log.Trace("[cmd] Failed to fix flag order", "flag", flagName)
					}
				}
			}
		}
		return f(ctx)
	}
}
