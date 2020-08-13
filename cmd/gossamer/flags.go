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
	// ForceFlag disables all confirm prompts ("Y" to all)
	ForceFlag = cli.BoolFlag{
		Name:  "force",
		Usage: "Disable all confirm prompts (the same as answering \"Y\" to all)",
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
	// LogFlag cli service settings
	LogFlag = cli.StringFlag{
		Name:  "log",
		Usage: "Supports levels crit (silent) to trce (trace)",
		Value: log.LvlInfo.String(),
	}
	// NameFlag node implementation name
	NameFlag = cli.StringFlag{
		Name:  "name",
		Usage: "Node implementation name",
	}
	// ChainFlag is chain id used to load default configuration for specified chain
	ChainFlag = cli.StringFlag{
		Name:  "chain",
		Usage: "Chain id used to load default configuration for specified chain",
	}
	// ConfigFlag TOML configuration file
	ConfigFlag = cli.StringFlag{
		Name:  "config",
		Usage: "TOML configuration file",
	}
	// BasePathFlag data directory for node
	BasePathFlag = cli.StringFlag{
		Name:  "basepath",
		Usage: "Data directory for the node",
	}
	CPUProfFlag = cli.StringFlag{
		Name:  "cpuprof",
		Usage: "File to write CPU profile to",
	}
	MemProfFlag = cli.StringFlag{
		Name:  "memprof",
		Usage: "File to write memory profile to",
	}
)

// Initialization-only flags
var (
	// GenesisRawFlag Path to raw genesis JSON file
	GenesisRawFlag = cli.StringFlag{
		Name:  "genesis-raw",
		Usage: "Path to raw genesis JSON file",
	}
)

// BuildSpec-only flags
var (
	RawFlag = cli.BoolFlag{
		Name:  "raw",
		Usage: "Output as raw genesis JSON",
	}
	GenesisFlag = cli.StringFlag{
		Name:  "genesis",
		Usage: "Path to human-readable genesis JSON file",
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
		Usage: "Comma separated node URLs for network discovery bootstrap",
	}
	// ProtocolFlag Set protocol id
	ProtocolFlag = cli.StringFlag{
		Name:  "protocol",
		Usage: "Set protocol id",
	}
	// NoBootstrapFlag Disables network bootstrapping
	NoBootstrapFlag = cli.BoolFlag{
		Name:  "nobootstrap",
		Usage: "Disables network bootstrapping (mDNS still enabled)",
	}
	// NoMDNSFlag Disables network mDNS
	NoMDNSFlag = cli.BoolFlag{
		Name:  "nomdns",
		Usage: "Disables network mDNS discovery",
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
	WSPortFlag = cli.IntFlag{
		Name:  "wsport",
		Usage: "Websockets server listening port",
	}
	WSEnabledFlag = cli.BoolFlag{
		Name:  "ws",
		Usage: "Enable the websockets server",
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
	// ImportRawFlag imports a raw private key
	ImportRawFlag = cli.StringFlag{
		Name:  "import-raw",
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

// flag sets that are shared by multiple commands
var (
	// GlobalFlags are flags that are valid for use with the root command and all subcommands
	GlobalFlags = []cli.Flag{
		LogFlag,
		NameFlag,
		ChainFlag,
		ConfigFlag,
		BasePathFlag,
		CPUProfFlag,
		MemProfFlag,
	}

	// StartupFlags are flags that are valid for use with the root command and the export subcommand
	StartupFlags = []cli.Flag{
		// keystore flags
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
		WSEnabledFlag,
		WSPortFlag,
	}
)

// local flag sets for the root gossamer command and all subcommands
var (
	// RootFlags are the flags that are valid for use with the root gossamer command
	RootFlags = append(GlobalFlags, StartupFlags...)

	// InitFlags are flags that are valid for use with the init subcommand
	InitFlags = append([]cli.Flag{
		ForceFlag,
		GenesisRawFlag,
	}, GlobalFlags...)

	BuildSpecFlags = append([]cli.Flag{
		RawFlag,
		GenesisFlag,
	}, GlobalFlags...)

	// ExportFlags are the flags that are valid for use with the export subcommand
	ExportFlags = append([]cli.Flag{
		ForceFlag,
		GenesisRawFlag,
	}, append(GlobalFlags, StartupFlags...)...)

	// AccountFlags are flags that are valid for use with the account subcommand
	AccountFlags = append([]cli.Flag{
		GenerateFlag,
		PasswordFlag,
		ImportFlag,
		ImportRawFlag,
		ListFlag,
		Ed25519Flag,
		Sr25519Flag,
		Secp256k1Flag,
	}, GlobalFlags...)
)

// FixFlagOrder allow us to use various flag order formats (ie, `gossamer init
// --config config.toml` and `gossamer --config config.toml init`). FixFlagOrder
// only fixes global flags, all local flags must come after the subcommand (ie,
// `gossamer --force --config config.toml init` will not recognize `--force` but
// `gossamer init --force --config config.toml` will work as expected).
func FixFlagOrder(f func(ctx *cli.Context) error) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		trace := "trace"

		// loop through all flags (global and local)
		for _, flagName := range ctx.FlagNames() {

			// check if flag is set as global or local flag
			if ctx.GlobalIsSet(flagName) {
				// log global flag if log equals trace
				if ctx.String(LogFlag.Name) == trace {
					log.Trace("[cmd] global flag set", "name", flagName)
				}
			} else if ctx.IsSet(flagName) {
				// check if global flag using set as global flag
				err := ctx.GlobalSet(flagName, ctx.String(flagName))
				if err == nil {
					// log fixed global flag if log equals trace
					if ctx.String(LogFlag.Name) == trace {
						log.Trace("[cmd] global flag fixed", "name", flagName)
					}
				} else {
					// if not global flag, log local flag if log equals trace
					if ctx.String(LogFlag.Name) == trace {
						log.Trace("[cmd] local flag set", "name", flagName)
					}
				}
			}
		}

		return f(ctx)
	}
}
