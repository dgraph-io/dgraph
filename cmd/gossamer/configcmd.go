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
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/dot/core"
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/rpc"
	"github.com/ChainSafe/gossamer/dot/rpc/json2"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/keyring"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/services"
	"github.com/ChainSafe/gossamer/lib/utils"
	"github.com/ChainSafe/gossamer/node/gssmr"
	"github.com/ChainSafe/gossamer/node/ksmcc"

	log "github.com/ChainSafe/log15"
	"github.com/naoina/toml"
	"github.com/urfave/cli"
)

var channelSize = 128

// makeNode sets up node; opening badgerDB instance and returning the Node container
func makeNode(ctx *cli.Context) (*dot.Node, *dot.Config, error) {
	cfg, err := getConfig(ctx)
	if err != nil {
		return nil, nil, err
	}

	dataDir := cfg.Global.DataDir

	log.Info(
		"[gossamer] Setting up node services...",
		"datadir", dataDir,
	)

	var srvcs []services.Service

	// Create service, initialize stateDB and blockDB
	stateSrv := state.NewService(dataDir)
	srvcs = append(srvcs, stateSrv)

	err = stateSrv.Start()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start db service: %s", err)
	}

	ks, err := loadKeystore(ctx, dataDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load keystore: %s", err)
	}

	// Trie, runtime: load most recent state from DB, load runtime code from trie and create runtime executor
	rt, err := loadStateAndRuntime(stateSrv.Storage, ks)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load state and runtime: %s", err)
	}

	// load genesis from JSON file
	gendata, err := stateSrv.Storage.LoadGenesisData()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load genesis data: %s", err)
	}

	// TODO: Configure node based on Roles #601

	// Network
	networkSrvc, networkMsgSend, networkMsgRec := createNetworkService(cfg, gendata, stateSrv)
	srvcs = append(srvcs, networkSrvc)

	// BABE authority configuration; flag overwrites config option
	if auth := ctx.GlobalBool(AuthorityFlag.Name); auth && !cfg.Global.Authority {
		cfg.Global.Authority = true
		// if authority, should have at least 1 key in keystore
		if ks.NumSr25519Keys() == 0 {
			return nil, nil, fmt.Errorf("no keys provided for authority node")
		}
	} else if ctx.IsSet(AuthorityFlag.Name) && !auth && cfg.Global.Authority {
		cfg.Global.Authority = false
	}

	// Core
	coreConfig := &core.Config{
		BlockState:       stateSrv.Block,
		StorageState:     stateSrv.Storage,
		TransactionQueue: stateSrv.TransactionQueue,
		Keystore:         ks,
		Runtime:          rt,
		MsgRec:           networkMsgSend, // message channel from network service to core service
		MsgSend:          networkMsgRec,  // message channel from core service to network service
		IsBabeAuthority:  cfg.Global.Authority,
	}

	coreSrvc, err := createCoreService(coreConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create core service: %s", err)
	}

	srvcs = append(srvcs, coreSrvc)

	// RPC
	if ctx.GlobalBool(RPCEnabledFlag.Name) {
		rpcSrvr := setupRPC(cfg.RPC, stateSrv, networkSrvc, coreSrvc, stateSrv.TransactionQueue)
		srvcs = append(srvcs, rpcSrvr)
	}

	return dot.NewNode(gendata.Name, srvcs), cfg, nil
}

func loadKeystore(ctx *cli.Context, dataDir string) (*keystore.Keystore, error) {
	ks := keystore.NewKeystore()

	// load test keys if specified
	if key := ctx.String(KeyFlag.Name); key != "" {
		ring, err := keyring.NewKeyring()
		if err != nil {
			return nil, fmt.Errorf("failed to create test keyring")
		}

		switch strings.ToLower(key) {
		case "alice":
			ks.Insert(ring.Alice)
		case "bob":
			ks.Insert(ring.Bob)
		case "charlie":
			ks.Insert(ring.Charlie)
		case "dave":
			ks.Insert(ring.Dave)
		case "eve":
			ks.Insert(ring.Eve)
		case "fred":
			ks.Insert(ring.Fred)
		case "george":
			ks.Insert(ring.George)
		case "heather":
			ks.Insert(ring.Heather)
		default:
			log.Error(fmt.Sprintf("unknown test key %s: options: alice | bob | charlie | dave | eve | fred | george | heather", key))
		}
	}

	// unlock keys, if specified
	if keyindices := ctx.String(UnlockFlag.Name); keyindices != "" {
		err := unlockKeys(ctx, dataDir, ks)
		if err != nil {
			return nil, fmt.Errorf("failed to unlock keys: %s", err)
		}
	}

	return ks, nil
}

func loadStateAndRuntime(ss *state.StorageState, ks *keystore.Keystore) (*runtime.Runtime, error) {
	latestState, err := ss.LoadHash()
	if err != nil {
		return nil, fmt.Errorf("failed to load latest state root hash: %s", err)
	}

	err = ss.LoadFromDB(latestState)
	if err != nil {
		return nil, fmt.Errorf("failed to load latest state from database: %s", err)
	}

	code, err := ss.GetStorage([]byte(":code"))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve :code from trie: %s", err)
	}

	return runtime.NewRuntime(code, ss, ks)
}

// getConfig gets the configuration for the node using --node and/or --config,
// then applies the remaining cli flag options to the configuration
func getConfig(ctx *cli.Context) (cfg *dot.Config, err error) {

	// check --node flag and apply node defaults to config
	if name := ctx.GlobalString(NodeFlag.Name); name != "" {
		switch name {
		case "gssmr":
			log.Trace("[gossamer] Node implementation", "name", name)
			cfg = gssmr.DefaultConfig()
		case "ksmcc":
			log.Trace("[gossamer] Node implementation", "name", name)
			cfg = ksmcc.DefaultConfig()
		default:
			return nil, fmt.Errorf("unknown node implementation: %s", name)
		}
	} else {
		log.Trace("[gossamer] Node implementation", "name", "gssmr")
		cfg = gssmr.DefaultConfig()
	}

	// check --config flag and apply toml configuration to config
	if name := ctx.GlobalString(ConfigFlag.Name); name != "" {
		log.Trace("[gossamer] Loading toml configuration...", "path", name)
		err = dot.LoadConfig(name, cfg)
		if err != nil {
			return nil, err
		}
	}

	// check --datadir flag and expand path of node data directory
	if name := ctx.GlobalString(DataDirFlag.Name); name != "" {
		cfg.Global.DataDir = utils.ExpandDir(name)
	} else {
		cfg.Global.DataDir = utils.ExpandDir(cfg.Global.DataDir)
	}

	log.Trace("[gossamer] Expanded data directory", "path", cfg.Global.DataDir)

	// parse remaining flags
	setGlobalConfig(ctx, &cfg.Global)
	setNetworkConfig(ctx, &cfg.Network)
	setRPCConfig(ctx, &cfg.RPC)

	return cfg, nil
}

func setGlobalConfig(ctx *cli.Context, cfg *dot.GlobalConfig) {
	newDataDir := cfg.DataDir
	if dir := ctx.GlobalString(DataDirFlag.Name); dir != "" {
		newDataDir = utils.ExpandDir(dir)
	}
	cfg.DataDir, _ = filepath.Abs(newDataDir)

	newRoles := cfg.Roles
	if roles := ctx.GlobalString(RolesFlag.Name); roles != "" {
		b, err := strconv.Atoi(roles)
		if err != nil {
			log.Error("[gossamer] Failed to convert Roles to byte", "error", err)
		} else {
			newRoles = byte(b)
		}
	}
	cfg.Roles = newRoles
}

func setNetworkConfig(ctx *cli.Context, fig *dot.NetworkConfig) {
	// Bootnodes
	if bnodes := ctx.GlobalString(BootnodesFlag.Name); bnodes != "" {
		fig.Bootnodes = strings.Split(ctx.GlobalString(BootnodesFlag.Name), ",")
	}

	if protocol := ctx.GlobalString(ProtocolIDFlag.Name); protocol != "" {
		fig.ProtocolID = protocol
	}

	if port := ctx.GlobalUint(PortFlag.Name); port != 0 {
		fig.Port = uint32(port)
	}

	// NoBootstrap
	if off := ctx.GlobalBool(NoBootstrapFlag.Name); off {
		fig.NoBootstrap = true
	}

	// NoMDNS
	if off := ctx.GlobalBool(NoMDNSFlag.Name); off {
		fig.NoMDNS = true
	}
}

// createNetworkService creates a network service from the command configuration and genesis data
func createNetworkService(fig *dot.Config, gendata *genesis.Data, stateService *state.Service) (*network.Service, chan network.Message, chan network.Message) {
	// Default bootnodes and protocol from genesis file
	bootnodes := common.BytesToStringArray(gendata.Bootnodes)
	protocolID := gendata.ProtocolID

	// If bootnodes flag has one or more bootnodes, overwrite genesis bootnodes
	if len(fig.Network.Bootnodes) > 0 {
		bootnodes = fig.Network.Bootnodes
	}

	// If protocol id flag is not an empty string, overwrite
	if fig.Network.ProtocolID != "" {
		protocolID = fig.Network.ProtocolID
	}

	log.Info(
		"[gossamer] Creating network service...",
		"port", fig.Network.Port,
		"protocol", protocolID,
		"bootnodes", bootnodes,
	)

	// network service configuation
	networkConfig := network.Config{
		BlockState:   stateService.Block,
		NetworkState: stateService.Network,
		DataDir:      fig.Global.DataDir,
		Roles:        fig.Global.Roles,
		Port:         fig.Network.Port,
		Bootnodes:    bootnodes,
		ProtocolID:   protocolID,
		NoBootstrap:  fig.Network.NoBootstrap,
		NoMDNS:       fig.Network.NoMDNS,
	}

	networkMsgRec := make(chan network.Message, channelSize)
	networkMsgSend := make(chan network.Message, channelSize)

	networkService, err := network.NewService(&networkConfig, networkMsgSend, networkMsgRec)
	if err != nil {
		log.Error("[gossamer] Failed to create network service", "error", err)
	}

	return networkService, networkMsgSend, networkMsgRec
}

// createCoreService creates the core service from the provided core configuration
func createCoreService(coreConfig *core.Config) (*core.Service, error) {
	log.Info(
		"[gossamer] Creating core service...",
		"authority", coreConfig.IsBabeAuthority,
	)

	coreService, err := core.NewService(coreConfig)
	if err != nil {
		log.Error("[gossamer] Failed to create core service", "error", err)
		return nil, err
	}

	return coreService, nil
}

func setRPCConfig(ctx *cli.Context, fig *dot.RPCConfig) {
	// Modules
	if mods := ctx.GlobalString(RPCModuleFlag.Name); mods != "" {
		fig.Modules = strings.Split(ctx.GlobalString(RPCModuleFlag.Name), ",")
	}

	// Host
	if host := ctx.GlobalString(RPCHostFlag.Name); host != "" {
		fig.Host = host
	}

	// Port
	if port := ctx.GlobalUint(RPCPortFlag.Name); port != 0 {
		fig.Port = uint32(port)
	}

}

func setupRPC(fig dot.RPCConfig, stateSrv *state.Service, networkSrvc *network.Service, coreSrvc *core.Service, txQueue *state.TransactionQueue) *rpc.HTTPServer {
	cfg := &rpc.HTTPServerConfig{
		BlockAPI:            stateSrv.Block,
		StorageAPI:          stateSrv.Storage,
		NetworkAPI:          networkSrvc,
		CoreAPI:             coreSrvc,
		TransactionQueueAPI: txQueue,
		Codec:               &json2.Codec{},
		Host:                fig.Host,
		Port:                fig.Port,
		Modules:             fig.Modules,
	}

	return rpc.NewHTTPServer(cfg)
}

// dumpConfig is the dumpconfig command.
func dumpConfig(ctx *cli.Context) error {
	cfg, err := getConfig(ctx)
	if err != nil {
		return err
	}

	comment := ""

	out, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}

	dump := os.Stdout
	if ctx.NArg() > 0 {
		/* #nosec */
		dump, err = os.OpenFile(filepath.Clean(ctx.Args().Get(0)), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}

		defer func() {
			err = dump.Close()
			if err != nil {
				log.Error("[gossamer] Failed to close connection", "error", err)
			}
		}()
	}
	_, err = dump.WriteString(comment)
	if err != nil {
		log.Error("[gossamer] Failed to write output for dumpconfig command", "error", err)
	}
	_, err = dump.Write(out)
	if err != nil {
		log.Error("[gossamer] Failed to write output for dumpconfig command", "error", err)
	}
	return nil
}
