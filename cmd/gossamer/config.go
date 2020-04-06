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
	"strconv"
	"strings"

	"github.com/ChainSafe/gossamer/dot"

	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

// loadConfigFile loads a default config file if --node is specified, a specific config if --config is specified,
// or the default gossamer config otherwise.
func loadConfigFile(ctx *cli.Context) (cfg *dot.Config, err error) {
	// check --node flag and load node configuration from defaults.go
	if name := ctx.GlobalString(NodeFlag.Name); name != "" {
		switch name {
		case "gssmr":
			log.Debug("[cmd] Loading node implementation...", "name", name)
			cfg = dot.GssmrConfig() // "gssmr" = dot.GssmrConfig()
		case "ksmcc":
			log.Debug("[cmd] Loading node implementation...", "name", name)
			cfg = dot.KsmccConfig() // "ksmcc" = dot.KsmccConfig()
		default:
			return nil, fmt.Errorf("unknown node implementation: %s", name)
		}
	}

	// check --config flag and load toml configuration from config.toml
	if config := ctx.GlobalString(ConfigFlag.Name); config != "" {
		log.Info("[cmd] Loading toml configuration...", "config", config)
		if cfg == nil {
			cfg = &dot.Config{} // if configuration has not been set, create empty dot configuration
		} else {
			log.Warn(
				"[cmd] Overwriting node implementation with toml configuration values",
				"name", cfg.Global.Name,
				"config", config,
			)
		}
		err = dot.LoadConfig(cfg, config) // load toml configuration values into dot configuration
		if err != nil {
			return nil, err
		}
	}

	// if configuration has not been set, load "gssmr" node implemenetation from node/gssmr/defaults.go
	if cfg == nil {
		log.Info("[cmd] Loading node implementation...", "name", "gssmr")
		cfg = dot.GssmrConfig()
	}

	return cfg, nil
}

// createDotConfig creates a new dot configuration from the provided flag values
func createDotConfig(ctx *cli.Context) (cfg *dot.Config, err error) {
	cfg, err = loadConfigFile(ctx)
	if err != nil {
		log.Error("[cmd] Failed to load toml configuration", "error", err)
		return nil, err
	}

	// set dot configuration values
	setDotGlobalConfig(ctx, &cfg.Global)
	setDotAccountConfig(ctx, &cfg.Account)
	setDotNetworkConfig(ctx, &cfg.Network)
	setDotCoreConfig(ctx, &cfg.Core)
	setDotRPCConfig(ctx, &cfg.RPC)

	// return dot configuration ready for node initialization
	return cfg, nil
}

func createInitConfig(ctx *cli.Context) (cfg *dot.Config, err error) {
	cfg, err = loadConfigFile(ctx)
	if err != nil {
		log.Error("[cmd] Failed to load toml configuration", "error", err)
		return nil, err
	}

	setDotInitConfig(ctx, &cfg.Init)
	setDotGlobalConfig(ctx, &cfg.Global)

	return cfg, nil
}

func setDotInitConfig(ctx *cli.Context, cfg *dot.InitConfig) {
	// check --genesis flag and update node configuration
	if genesis := ctx.String(GenesisFlag.Name); genesis != "" {
		cfg.Genesis = genesis
	}
}

// setDotGlobalConfig sets dot.GlobalConfig using flag values from the cli context
func setDotGlobalConfig(ctx *cli.Context, cfg *dot.GlobalConfig) {
	// check --datadir flag and update node configuration
	if datadir := ctx.GlobalString(DataDirFlag.Name); datadir != "" {
		cfg.DataDir = datadir
	}

	log.Debug(
		"[cmd] Global configuration",
		"name", cfg.Name,
		"id", cfg.ID,
		"datadir", cfg.DataDir,
	)
}

// setDotAccountConfig sets dot.AccountConfig using flag values from the cli context
func setDotAccountConfig(ctx *cli.Context, cfg *dot.AccountConfig) {
	// check --key flag and update node configuration
	if key := ctx.GlobalString(KeyFlag.Name); key != "" {
		cfg.Key = key
	}

	// check --unlock flag and update node configuration
	if unlock := ctx.GlobalString(UnlockFlag.Name); unlock != "" {
		cfg.Unlock = unlock
	}

	log.Debug(
		"[cmd] Account configuration",
		"key", cfg.Key,
		"unlock", cfg.Unlock,
	)
}

// setDotCoreConfig sets dot.CoreConfig using flag values from the cli context
func setDotCoreConfig(ctx *cli.Context, cfg *dot.CoreConfig) {
	// check --roles flag and update node configuration
	if roles := ctx.GlobalString(RolesFlag.Name); roles != "" {
		// convert string to byte
		b, err := strconv.Atoi(roles)
		if err != nil {
			log.Error("[cmd] Failed to convert Roles to byte", "error", err)
		} else if byte(b) == 4 {
			// if roles byte is 4, act as an authority (see Table D.2)
			log.Debug("[cmd] Authority enabled", "roles", 4)
			cfg.Authority = true
		} else if byte(b) > 4 {
			// if roles byte is greater than 4, invalid roles byte (see Table D.2)
			log.Error("[cmd] Invalid roles option provided, authority disabled", "roles", byte(b))
			cfg.Authority = false
		} else {
			// if roles byte is less than 4, do not act as an authority (see Table D.2)
			log.Debug("[cmd] Authority disabled", "roles", byte(b))
			cfg.Authority = false
		}
	}

	// check --roles flag and update node configuration
	if roles := ctx.GlobalString(RolesFlag.Name); roles != "" {
		b, err := strconv.Atoi(roles)
		if err != nil {
			log.Error("[cmd] Failed to convert Roles to byte", "error", err)
		} else if byte(b) > 4 {
			// if roles byte is greater than 4, invalid roles byte (see Table D.2)
			log.Error("[cmd] Invalid roles option provided", "roles", byte(b))
		} else {
			cfg.Roles = byte(b)
		}
	}

	log.Debug(
		"[cmd] Core configuration",
		"authority", cfg.Authority,
		"roles", cfg.Roles,
	)
}

// setDotNetworkConfig sets dot.NetworkConfig using flag values from the cli context
func setDotNetworkConfig(ctx *cli.Context, cfg *dot.NetworkConfig) {
	// check --port flag and update node configuration
	if port := ctx.GlobalUint(PortFlag.Name); port != 0 {
		cfg.Port = uint32(port)
	}

	// check --bootnodes flag and update node configuration
	if bootnodes := ctx.GlobalString(BootnodesFlag.Name); bootnodes != "" {
		cfg.Bootnodes = strings.Split(ctx.GlobalString(BootnodesFlag.Name), ",")
	}

	// format bootnodes
	if len(cfg.Bootnodes) == 0 {
		cfg.Bootnodes = []string(nil)
	}

	// check --protocol flag and update node configuration
	if protocol := ctx.GlobalString(ProtocolFlag.Name); protocol != "" {
		cfg.ProtocolID = protocol
	}

	// check --nobootstrap flag and update node configuration
	if nobootstrap := ctx.GlobalBool(NoBootstrapFlag.Name); nobootstrap {
		cfg.NoBootstrap = true
	}

	// check --nomdns flag and update node configuration
	if nomdns := ctx.GlobalBool(NoMDNSFlag.Name); nomdns {
		cfg.NoMDNS = true
	}

	log.Debug(
		"[cmd] Network configuration",
		"port", cfg.Port,
		"bootnodes", cfg.Bootnodes,
		"protocol", cfg.ProtocolID,
		"nobootstrap", cfg.NoBootstrap,
		"nomdns", cfg.NoMDNS,
	)
}

// setDotRPCConfig sets dot.RPCConfig using flag values from the cli context
func setDotRPCConfig(ctx *cli.Context, cfg *dot.RPCConfig) {
	// check --rpc flag and update node configuration
	if enabled := ctx.GlobalBool(RPCEnabledFlag.Name); enabled {
		cfg.Enabled = true
	} else if ctx.IsSet(RPCEnabledFlag.Name) && !enabled {
		cfg.Enabled = false
	}

	// check --rpcport flag and update node configuration
	if port := ctx.GlobalUint(RPCPortFlag.Name); port != 0 {
		cfg.Port = uint32(port)
	}

	// check --rpchost flag and update node configuration
	if host := ctx.GlobalString(RPCHostFlag.Name); host != "" {
		cfg.Host = host
	}

	// check --rpcmods flag and update node configuration
	if modules := ctx.GlobalString(RPCModulesFlag.Name); modules != "" {
		cfg.Modules = strings.Split(ctx.GlobalString(RPCModulesFlag.Name), ",")
	}

	// format rpc modules
	if len(cfg.Modules) == 0 {
		cfg.Modules = []string(nil)
	}

	log.Debug(
		"[cmd] RPC configuration",
		"enabled", cfg.Enabled,
		"port", cfg.Port,
		"host", cfg.Host,
		"modules", cfg.Modules,
	)
}
