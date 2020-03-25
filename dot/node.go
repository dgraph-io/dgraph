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
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/services"

	log "github.com/ChainSafe/log15"
)

// Node is a container for all the components of a node.
type Node struct {
	Name      string
	Services  *services.ServiceRegistry // registry of all node services
	IsStarted chan struct{}             // signals node startup complete
	stop      chan struct{}             // used to signal node shutdown
	syncChan  chan *big.Int
}

// InitNode initializes a new dot node from the provided dot node configuration
// and JSON formatted genesis file.
func InitNode(cfg *Config) error {
	dataDir := cfg.Global.DataDir
	genPath := cfg.Global.Genesis

	log.Info(
		"[dot] Initializing node...",
		"datadir", dataDir,
		"genesis", genPath,
	)

	// load Genesis from genesis configuration file
	gen, err := genesis.LoadGenesisFromJSON(genPath)
	if err != nil {
		log.Error("[dot] Failed to load genesis from file", "error", err)
		return err
	}

	log.Info(
		"[dot] Loading genesis...",
		"name", gen.Name,
		"id", gen.ID,
		"protocol", gen.ProtocolID,
		"bootnodes", gen.Bootnodes,
	)

	// create and load trie from genesis
	t, err := newTrieFromGenesis(gen)
	if err != nil {
		log.Error("[dot] Failed to create trie from genesis", "error", err)
		return err
	}

	// generates genesis block header from trie and store it in state database
	err = loadGenesisBlock(t, dataDir)
	if err != nil {
		log.Error("[dot] Failed to load genesis block with state service", "error", err)
		return err
	}

	// initialize trie database
	err = initTrieDatabase(t, dataDir, gen)
	if err != nil {
		log.Error("[dot] Failed to initialize trie database", "error", err)
		return err
	}

	log.Info(
		"[dot] Node initialized",
		"datadir", dataDir,
		"genesis", genPath,
	)

	return nil
}

// NodeInitialized returns true if, within the configured data directory for the
// node, the state database has been created and the genesis data has been loaded
func NodeInitialized(cfg *Config) bool {

	// check if key registry exists
	registry := path.Join(cfg.Global.DataDir, "KEYREGISTRY")
	_, err := os.Stat(registry)
	if os.IsNotExist(err) {
		log.Warn(
			"[dot] Node has not been initialized",
			"datadir", cfg.Global.DataDir,
			"error", "failed to locate KEYREGISTRY file in data directory",
		)
		return false
	}

	// check if manifest exists
	manifest := path.Join(cfg.Global.DataDir, "MANIFEST")
	_, err = os.Stat(manifest)
	if os.IsNotExist(err) {
		log.Warn(
			"[dot] Node has not been initialized",
			"datadir", cfg.Global.DataDir,
			"error", "failed to locate MANIFEST file in data directory",
		)
		return false
	}

	// TODO: investigate cheap way to confirm valid genesis data has been loaded

	return true
}

// NewNode creates a new dot node from a dot node configuration
func NewNode(cfg *Config, ks *keystore.Keystore) (*Node, error) {

	// if authority node, should have at least 1 key in keystore
	if cfg.Core.Authority && ks.NumSr25519Keys() == 0 {
		return nil, fmt.Errorf("no keys provided for authority node")
	}

	// Node Services

	log.Info(
		"[dot] Creating node services...",
		"datadir", cfg.Global.DataDir,
	)

	var nodeSrvcs []services.Service

	// Message Channels (send and receive messages between services)

	coreMsgs := make(chan network.Message, 128)    // message channel from core service to network service
	networkMsgs := make(chan network.Message, 128) // message channel from network service to core service

	// State Service

	// create state service and append state service to node services
	stateSrvc, err := createStateService(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create state service: %s", err)
	}
	nodeSrvcs = append(nodeSrvcs, stateSrvc)

	// Syncer
	syncChan := make(chan *big.Int, 128)

	// Core Service

	// create core service and append core service to node services
	coreSrvc, err := createCoreService(cfg, ks, stateSrvc, coreMsgs, networkMsgs, syncChan)
	if err != nil {
		return nil, fmt.Errorf("failed to create core service: %s", err)
	}
	nodeSrvcs = append(nodeSrvcs, coreSrvc)

	// Network Service

	networkSrvc := &network.Service{} // TODO: rpc service without network service

	// check if network service is enabled
	if enabled := NetworkServiceEnabled(cfg); enabled {

		// create network service and append network service to node services
		networkSrvc, err = createNetworkService(cfg, stateSrvc, coreMsgs, networkMsgs, syncChan)
		if err != nil {
			return nil, fmt.Errorf("failed to create network service: %s", err)
		}
		nodeSrvcs = append(nodeSrvcs, networkSrvc)

	} else {

		// do not create or append network service if network service is not enabled
		log.Debug("[dot] Network service disabled", "network", enabled, "roles", cfg.Global.Roles)

	}

	// RPC Service

	// check if rpc service is enabled
	if enabled := RPCServiceEnabled(cfg); enabled {

		// create rpc service and append rpc service to node services
		rpcSrvc := createRPCService(cfg, stateSrvc, coreSrvc, networkSrvc)
		nodeSrvcs = append(nodeSrvcs, rpcSrvc)

	} else {

		// do not create or append rpc service if rpc service is not enabled
		log.Debug("[dot] RPC service disabled by default", "rpc", enabled)

	}

	node := &Node{
		Name:      cfg.Global.Name,
		Services:  services.NewServiceRegistry(),
		IsStarted: make(chan struct{}),
		stop:      nil,
		syncChan:  syncChan,
	}

	for _, srvc := range nodeSrvcs {
		node.Services.RegisterService(srvc)
	}

	return node, nil
}

// Start starts all dot node services
func (n *Node) Start() {
	log.Info("[dot] Starting node services...")

	// start all dot node services
	n.Services.StartAll()

	// open node stop channel
	n.stop = make(chan struct{})

	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)
		<-sigc
		log.Info("[dot] Signal interrupt, shutting down...")
		n.Stop()
		os.Exit(130)
	}()

	// move on when routine catches SIGINT or SIGTERM calls
	close(n.IsStarted)

	// wait for node stop channel to be closed
	<-n.stop
}

// Stop stops all dot node services
func (n *Node) Stop() {

	// stop all node services
	n.Services.StopAll()

	// close node stop channel if not already closed
	if n.stop != nil {
		close(n.stop)
	}

	close(n.syncChan)
}
