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
	"github.com/ChainSafe/gossamer/dot/rpc/json2"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"

	log "github.com/ChainSafe/log15"
)

// State Service

// createStateService creates the state service and initialize state database
func createStateService(cfg *Config) (*state.Service, error) {
	log.Info("[dot] Creating state service...")

	stateSrvc := state.NewService(cfg.Global.DataDir)

	// start state service (initialize state database)
	err := stateSrvc.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start state service: %s", err)
	}

	// load most recent state from database
	latestState, err := stateSrvc.Storage.LoadHash()
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
func createCoreService(cfg *Config, ks *keystore.Keystore, stateSrvc *state.Service, coreMsgs chan network.Message, networkMsgs chan network.Message, syncChan chan *big.Int) (*core.Service, error) {
	log.Info(
		"[dot] Creating core service...",
		"authority", cfg.Core.Authority,
	)

	// load runtime code from trie
	code, err := stateSrvc.Storage.GetStorage([]byte(":code"))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve :code from trie: %s", err)
	}

	// create runtime executor
	rt, err := runtime.NewRuntime(code, stateSrvc.Storage, ks)
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime executor: %s", err)
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
		log.Error("[dot] Failed to create core service", "error", err)
		return nil, err
	}

	return coreSrvc, nil
}

// Network Service

// createNetworkService creates a network service from the command configuration and genesis data
func createNetworkService(cfg *Config, stateSrvc *state.Service, coreMsgs chan network.Message, networkMsgs chan network.Message, syncChan chan *big.Int) (*network.Service, error) {
	log.Info(
		"[dot] Creating network service...",
		"roles", cfg.Global.Roles,
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
		Roles:        cfg.Global.Roles,
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
		log.Error("[dot] Failed to create network service", "error", err)
		return nil, err
	}

	return networkSrvc, nil
}

// RPC Service

// createRPCService creates the RPC service from the provided core configuration
func createRPCService(cfg *Config, stateSrvc *state.Service, coreSrvc *core.Service, networkSrvc *network.Service) *rpc.HTTPServer {
	log.Info(
		"[dot] Creating rpc service...",
		"host", cfg.RPC.Host,
		"port", cfg.RPC.Port,
		"mods", cfg.RPC.Modules,
	)

	rpcConfig := &rpc.HTTPServerConfig{
		BlockAPI:            stateSrvc.Block,
		StorageAPI:          stateSrvc.Storage,
		NetworkAPI:          networkSrvc,
		CoreAPI:             coreSrvc,
		TransactionQueueAPI: stateSrvc.TransactionQueue,
		Codec:               &json2.Codec{},
		Host:                cfg.RPC.Host,
		Port:                cfg.RPC.Port,
		Modules:             cfg.RPC.Modules,
	}

	return rpc.NewHTTPServer(rpcConfig)
}
