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
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/services"

	database "github.com/ChainSafe/chaindb"
	log "github.com/ChainSafe/log15"
)

var logger = log.New("pkg", "dot")

// Node is a container for all the components of a node.
type Node struct {
	Name     string
	Services *services.ServiceRegistry // registry of all node services
	StopFunc func()                    // func to call when node stops, currently used for profiling
	wg       sync.WaitGroup
}

// InitNode initializes a new dot node from the provided dot node configuration
// and JSON formatted genesis file.
func InitNode(cfg *Config) error {
	err := setupLogger(cfg)
	if err != nil {
		return err
	}

	logger.Info(
		"initializing node...",
		"name", cfg.Global.Name,
		"id", cfg.Global.ID,
		"basepath", cfg.Global.BasePath,
		"genesis-raw", cfg.Init.GenesisRaw,
	)

	// create genesis from configuration file
	gen, err := genesis.NewGenesisFromJSONRaw(cfg.Init.GenesisRaw)
	if err != nil {
		return fmt.Errorf("failed to load genesis from file: %w", err)
	}

	// create trie from genesis
	t, err := genesis.NewTrieFromGenesis(gen)
	if err != nil {
		return fmt.Errorf("failed to create trie from genesis: %w", err)
	}

	// create genesis block from trie
	header, err := genesis.NewGenesisBlockFromTrie(t)
	if err != nil {
		return fmt.Errorf("failed to create genesis block from trie: %w", err)
	}

	// create new state service
	stateSrvc := state.NewService(cfg.Global.BasePath, cfg.Global.lvl)

	var genEpochInfo *types.EpochInfo
	if !cfg.Init.TestFirstEpoch {
		// load genesis trie state for loading runtime info
		genTrie := state.NewTrieState(t) //nolint

		// create genesis runtime
		r, err := genesis.NewRuntimeFromGenesis(gen, genTrie) //nolint
		if err != nil {
			return fmt.Errorf("failed to create genesis runtime: %w", err)
		}

		babeCfg, err := r.BabeConfiguration()
		if err != nil {
			return fmt.Errorf("failed to fetch genesis babe configuration: %w", err)
		}

		genEpochInfo = &types.EpochInfo{
			Duration:   babeCfg.EpochLength,
			FirstBlock: 1,
			Randomness: babeCfg.Randomness,
		}

		r.Stop()
	} else {
		genEpochInfo = &types.EpochInfo{
			Duration: 200,
		}
	}

	// declare genesis data
	data := gen.GenesisData()

	// set genesis data using configuration values (assumes the genesis values
	// have already been set for the configuration, which allows for us to take
	// into account dynamic genesis values if the corresponding flag values are
	// provided when using the dot package with the gossamer command)
	data.Name = cfg.Global.Name
	data.ID = cfg.Global.ID
	data.Bootnodes = common.StringArrayToBytes(cfg.Network.Bootnodes)
	data.ProtocolID = cfg.Network.ProtocolID

	// initialize state service with genesis data, block, and trie
	err = stateSrvc.Initialize(data, header, t, genEpochInfo)
	if err != nil {
		return fmt.Errorf("failed to initialize state service: %s", err)
	}

	logger.Info(
		"node initialized",
		"name", cfg.Global.Name,
		"id", cfg.Global.ID,
		"basepath", cfg.Global.BasePath,
		"genesis-raw", cfg.Init.GenesisRaw,
		"block", header.Number,
	)

	return nil
}

// NodeInitialized returns true if, within the configured data directory for the
// node, the state database has been created and the genesis data has been loaded
func NodeInitialized(basepath string, expected bool) bool {
	// check if key registry exists
	registry := path.Join(basepath, "KEYREGISTRY")
	_, err := os.Stat(registry)
	if os.IsNotExist(err) {
		if expected {
			logger.Warn(
				"node has not been initialized",
				"basepath", basepath,
				"error", "failed to locate KEYREGISTRY file in data directory",
			)
		}
		return false
	}

	// check if manifest exists
	manifest := path.Join(basepath, "MANIFEST")
	_, err = os.Stat(manifest)
	if os.IsNotExist(err) {
		if expected {
			logger.Warn(
				"node has not been initialized",
				"basepath", basepath,
				"error", "failed to locate MANIFEST file in data directory",
			)
		}
		return false
	}

	// initialize database using data directory
	db, err := database.NewBadgerDB(basepath)
	if err != nil {
		logger.Error(
			"failed to create database",
			"basepath", basepath,
			"error", err,
		)
		return false
	}

	// load genesis data from initialized node database
	_, err = state.LoadGenesisData(db)
	if err != nil {
		logger.Warn(
			"node has not been initialized",
			"basepath", basepath,
			"error", err,
		)
		return false
	}

	// close database
	err = db.Close()
	if err != nil {
		logger.Error("failed to close database", "error", err)
	}

	return true
}

// NewNode creates a new dot node from a dot node configuration
func NewNode(cfg *Config, ks *keystore.GlobalKeystore, stopFunc func()) (*Node, error) {
	err := setupLogger(cfg)
	if err != nil {
		return nil, err
	}

	// if authority node, should have at least 1 key in keystore
	if cfg.Core.Authority && (ks.Babe.Size() == 0 || ks.Gran.Size() == 0) {
		return nil, ErrNoKeysProvided
	}

	// Node Services

	logger.Info(
		"initializing node services...",
		"name", cfg.Global.Name,
		"id", cfg.Global.ID,
		"basepath", cfg.Global.BasePath,
	)

	var nodeSrvcs []services.Service

	// State Service

	// create state service and append state service to node services
	stateSrvc, err := createStateService(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create state service: %s", err)
	}

	// create runtime
	rt, err := createRuntime(cfg, stateSrvc, ks.Acco.(*keystore.GenericKeystore))
	if err != nil {
		return nil, err
	}

	ver, err := createBlockVerifier(cfg, stateSrvc, rt)
	if err != nil {
		return nil, err
	}

	var bp BlockProducer

	if cfg.Core.BabeAuthority {
		// create BABE service
		bp, err = createBABEService(cfg, rt, stateSrvc, ks.Babe)
		if err != nil {
			return nil, err
		}

		nodeSrvcs = append(nodeSrvcs, bp)
	}

	dh, err := createDigestHandler(stateSrvc, bp, ver)
	if err != nil {
		return nil, err
	}

	// create GRANDPA service
	fg, err := createGRANDPAService(cfg, rt, stateSrvc, dh, ks.Gran)
	if err != nil {
		return nil, err
	}
	nodeSrvcs = append(nodeSrvcs, fg)
	dh.SetFinalityGadget(fg)

	// Syncer
	syncer, err := createSyncService(cfg, stateSrvc, bp, dh, ver, rt)
	if err != nil {
		return nil, err
	}

	// Network Service
	var networkSrvc *network.Service

	// check if network service is enabled
	if enabled := networkServiceEnabled(cfg); enabled {
		// create network service and append network service to node services
		networkSrvc, err = createNetworkService(cfg, stateSrvc, syncer)
		if err != nil {
			return nil, fmt.Errorf("failed to create network service: %s", err)
		}
		nodeSrvcs = append(nodeSrvcs, networkSrvc)
	} else {
		// do not create or append network service if network service is not enabled
		logger.Debug("network service disabled", "network", enabled, "roles", cfg.Core.Roles)
	}

	// Core Service

	// create core service and append core service to node services
	coreSrvc, err := createCoreService(cfg, bp, fg, ver, rt, ks, stateSrvc, networkSrvc)
	if err != nil {
		return nil, fmt.Errorf("failed to create core service: %s", err)
	}
	networkSrvc.SetMessageHandler(coreSrvc)
	nodeSrvcs = append(nodeSrvcs, coreSrvc)

	// System Service

	// create system service and append to node services
	sysSrvc := createSystemService(&cfg.System)
	nodeSrvcs = append(nodeSrvcs, sysSrvc)

	// RPC Service

	// check if rpc service is enabled
	if enabled := RPCServiceEnabled(cfg); enabled {

		// create rpc service and append rpc service to node services
		rpcSrvc, err := createRPCService(cfg, stateSrvc, coreSrvc, networkSrvc, bp, rt, sysSrvc)
		if err != nil {
			return nil, err
		}
		nodeSrvcs = append(nodeSrvcs, rpcSrvc)

	} else {

		// do not create or append rpc service if rpc service is not enabled
		logger.Debug("rpc service disabled by default", "rpc", enabled)

	}

	// close state service last
	nodeSrvcs = append(nodeSrvcs, stateSrvc)

	node := &Node{
		Name:     cfg.Global.Name,
		StopFunc: stopFunc,
		Services: services.NewServiceRegistry(),
	}

	for _, srvc := range nodeSrvcs {
		node.Services.RegisterService(srvc)
	}

	return node, nil
}

// Start starts all dot node services
func (n *Node) Start() error {
	logger.Info("starting node services...")

	// start all dot node services
	n.Services.StartAll()

	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)
		<-sigc
		logger.Info("signal interrupt, shutting down...")
		n.Stop()
		os.Exit(130)
	}()

	n.wg.Add(1)
	n.wg.Wait()

	return nil
}

// Stop stops all dot node services
func (n *Node) Stop() {
	if n.StopFunc != nil {
		n.StopFunc()
	}

	// stop all node services
	n.Services.StopAll()
	n.wg.Done()
}
