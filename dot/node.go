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
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/services"

	database "github.com/ChainSafe/chaindb"
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
	log.Info(
		"[dot] initializing node...",
		"name", cfg.Global.Name,
		"id", cfg.Global.ID,
		"datadir", cfg.Global.DataDir,
		"genesis", cfg.Init.Genesis,
	)

	// create genesis from configuration file
	gen, err := genesis.NewGenesisFromJSON(cfg.Init.Genesis)
	if err != nil {
		return fmt.Errorf("failed to load genesis from file: %s", err)
	}

	// create trie from genesis
	t, err := genesis.NewTrieFromGenesis(gen)
	if err != nil {
		return fmt.Errorf("failed to create trie from genesis: %s", err)
	}

	// create genesis block from trie
	header, err := genesis.NewGenesisBlockFromTrie(t)
	if err != nil {
		return fmt.Errorf("failed to create genesis block from trie: %s", err)
	}

	// create new state service
	stateSrvc := state.NewService(cfg.Global.DataDir)

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
	err = stateSrvc.Initialize(data, header, t)
	if err != nil {
		return fmt.Errorf("failed to initialize state service: %s", err)
	}

	log.Info(
		"[dot] node initialized",
		"name", cfg.Global.Name,
		"id", cfg.Global.ID,
		"datadir", cfg.Global.DataDir,
		"genesis", cfg.Init.Genesis,
		"block", header.Number,
	)

	return nil
}

// NodeInitialized returns true if, within the configured data directory for the
// node, the state database has been created and the genesis data has been loaded
func NodeInitialized(datadir string, expected bool) bool {

	// check if key registry exists
	registry := path.Join(datadir, "KEYREGISTRY")
	_, err := os.Stat(registry)
	if os.IsNotExist(err) {
		if expected {
			log.Warn(
				"[dot] node has not been initialized",
				"datadir", datadir,
				"error", "failed to locate KEYREGISTRY file in data directory",
			)
		}
		return false
	}

	// check if manifest exists
	manifest := path.Join(datadir, "MANIFEST")
	_, err = os.Stat(manifest)
	if os.IsNotExist(err) {
		if expected {
			log.Warn(
				"[dot] node has not been initialized",
				"datadir", datadir,
				"error", "failed to locate MANIFEST file in data directory",
			)
		}
		return false
	}

	// initialize database using data directory
	db, err := database.NewBadgerDB(datadir)
	if err != nil {
		log.Error(
			"[dot] failed to create database",
			"datadir", datadir,
			"error", err,
		)
		return false
	}

	// load genesis data from initialized node database
	_, err = state.LoadGenesisData(db)
	if err != nil {
		log.Warn(
			"[dot] node has not been initialized",
			"datadir", datadir,
			"error", err,
		)
		return false
	}

	// close database
	err = db.Close()
	if err != nil {
		log.Error("[dot] failed to close database", "error", err)
	}

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
		"[dot] initializing node services...",
		"name", cfg.Global.Name,
		"id", cfg.Global.ID,
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
		log.Debug("[dot] network service disabled", "network", enabled, "roles", cfg.Core.Roles)

	}

	// RPC Service

	// check if rpc service is enabled
	if enabled := RPCServiceEnabled(cfg); enabled {

		// create rpc service and append rpc service to node services
		rpcSrvc := createRPCService(cfg, stateSrvc, coreSrvc, networkSrvc)
		nodeSrvcs = append(nodeSrvcs, rpcSrvc)

	} else {

		// do not create or append rpc service if rpc service is not enabled
		log.Debug("[dot] rpc service disabled by default", "rpc", enabled)

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
	log.Info("[dot] starting node services...")

	// start all dot node services
	n.Services.StartAll()

	// open node stop channel
	n.stop = make(chan struct{})

	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigc)
		<-sigc
		log.Info("[dot] signal interrupt, shutting down...")
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
