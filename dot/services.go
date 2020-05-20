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

package dot

import (
	"fmt"
	"math/big"

	"github.com/ChainSafe/gossamer/dot/core"
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/rpc"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/system"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	log "github.com/ChainSafe/log15"
)

// State Service

// createStateService creates the state service and initialize state database
func createStateService(cfg *Config) (*state.Service, error) {
	log.Info("[dot] creating state service...")

	stateSrvc := state.NewService(cfg.Global.DataDir)

	// start state service (initialize state database)
	err := stateSrvc.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start state service: %s", err)
	}

	// load most recent state from database
	latestState, err := state.LoadLatestStorageHash(stateSrvc.DB())
	if err != nil {
		return nil, fmt.Errorf("failed to load latest state root hash: %s", err)
	}

	// load most recent state from database
	err = stateSrvc.Storage.LoadFromDB(latestState)
	if err != nil {
		return nil, fmt.Errorf("failed to load latest state from database: %s", err)
	}

	return stateSrvc, nil
}

// Core Service

// createCoreService creates the core service from the provided core configuration
func createCoreService(cfg *Config, ks *keystore.Keystore, stateSrvc *state.Service, coreMsgs chan network.Message, networkMsgs chan network.Message, syncChan chan *big.Int) (*core.Service, *runtime.Runtime, error) {
	log.Info(
		"[dot] creating core service...",
		"authority", cfg.Core.Authority,
	)

	// load runtime code from trie
	code, err := stateSrvc.Storage.GetStorage([]byte(":code"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve :code from trie: %s", err)
	}

	// create runtime executor
	rt, err := runtime.NewRuntime(code, stateSrvc.Storage, ks, runtime.RegisterImports)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create runtime executor: %s", err)
	}

	// set core configuration
	coreConfig := &core.Config{
		BlockState:       stateSrvc.Block,
		StorageState:     stateSrvc.Storage,
		TransactionQueue: stateSrvc.TransactionQueue,
		Keystore:         ks,
		Runtime:          rt,
		MsgRec:           networkMsgs, // message channel from network service to core service
		MsgSend:          coreMsgs,    // message channel from core service to network service
		IsBabeAuthority:  cfg.Core.Authority,
		SyncChan:         syncChan,
	}

	// create new core service
	coreSrvc, err := core.NewService(coreConfig)
	if err != nil {
		log.Error("[dot] failed to create core service", "error", err)
		return nil, nil, err
	}

	return coreSrvc, rt, nil
}

// Network Service

// createNetworkService creates a network service from the command configuration and genesis data
func createNetworkService(cfg *Config, stateSrvc *state.Service, coreMsgs chan network.Message, networkMsgs chan network.Message, syncChan chan *big.Int) (*network.Service, error) {
	log.Info(
		"[dot] creating network service...",
		"roles", cfg.Core.Roles,
		"port", cfg.Network.Port,
		"bootnodes", cfg.Network.Bootnodes,
		"protocol", cfg.Network.ProtocolID,
		"nobootstrap", cfg.Network.NoBootstrap,
		"nomdns", cfg.Network.NoMDNS,
	)

	// network service configuation
	networkConfig := network.Config{
		BlockState:   stateSrvc.Block,
		NetworkState: stateSrvc.Network,
		DataDir:      cfg.Global.DataDir,
		Roles:        cfg.Core.Roles,
		Port:         cfg.Network.Port,
		Bootnodes:    cfg.Network.Bootnodes,
		ProtocolID:   cfg.Network.ProtocolID,
		NoBootstrap:  cfg.Network.NoBootstrap,
		NoMDNS:       cfg.Network.NoMDNS,
		MsgRec:       coreMsgs,    // message channel from core service to network service
		MsgSend:      networkMsgs, // message channel from network service to core service
		SyncChan:     syncChan,
	}

	networkSrvc, err := network.NewService(&networkConfig)
	if err != nil {
		log.Error("[dot] failed to create network service", "error", err)
		return nil, err
	}

	return networkSrvc, nil
}

// RPC Service

// createRPCService creates the RPC service from the provided core configuration
func createRPCService(cfg *Config, stateSrvc *state.Service, coreSrvc *core.Service, networkSrvc *network.Service, rt *runtime.Runtime, sysSrvc *system.Service) *rpc.HTTPServer {
	log.Info(
		"[dot] creating rpc service...",
		"host", cfg.RPC.Host,
		"rpc port", cfg.RPC.Port,
		"mods", cfg.RPC.Modules,
		"ws port", cfg.RPC.WSPort,
	)
	rpcService := rpc.NewService()
	rpcConfig := &rpc.HTTPServerConfig{
		BlockAPI:            stateSrvc.Block,
		StorageAPI:          stateSrvc.Storage,
		NetworkAPI:          networkSrvc,
		CoreAPI:             coreSrvc,
		RuntimeAPI:          rt,
		TransactionQueueAPI: stateSrvc.TransactionQueue,
		RPCAPI:              rpcService,
		SystemAPI:           sysSrvc,
		Host:                cfg.RPC.Host,
		RPCPort:             cfg.RPC.Port,
		WSEnabled:           cfg.RPC.WSEnabled,
		WSPort:              cfg.RPC.WSPort,
		Modules:             cfg.RPC.Modules,
	}

	return rpc.NewHTTPServer(rpcConfig)
}

// System service
// creates a service for providing system related information
func createSystemService(cfg *types.SystemInfo) *system.Service {
	return system.NewService(cfg)
}
