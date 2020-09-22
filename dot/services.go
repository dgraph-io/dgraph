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
	"errors"
	"fmt"
	"math/big"

	database "github.com/ChainSafe/chaindb"

	"github.com/ChainSafe/gossamer/dot/core"
	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/rpc"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/sync"
	"github.com/ChainSafe/gossamer/dot/system"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/babe"
	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/grandpa"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"

	log "github.com/ChainSafe/log15"
)

// State Service

// createStateService creates the state service and initialize state database
func createStateService(cfg *Config) (*state.Service, error) {
	logger.Info("creating state service...")

	lvl, err := log.LvlFromString(cfg.Log.StateLvl)
	if err != nil {
		return nil, err
	}

	stateSrvc := state.NewService(cfg.Global.BasePath, lvl)

	// start state service (initialize state database)
	err = stateSrvc.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start state service: %s", err)
	}

	// load most recent state from database
	latestState, err := state.LoadLatestStorageHash(stateSrvc.DB())
	if err != nil {
		return nil, fmt.Errorf("failed to load latest state root hash: %s", err)
	}

	// load most recent state from database
	_, err = stateSrvc.Storage.LoadFromDB(latestState)
	if err != nil {
		return nil, fmt.Errorf("failed to load latest state from database: %s", err)
	}

	return stateSrvc, nil
}

func createRuntime(cfg *Config, st *state.Service, ks *keystore.GenericKeystore) (*runtime.Runtime, error) {
	// load runtime code from trie
	code, err := st.Storage.GetStorage(nil, []byte(":code"))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve :code from trie: %s", err)
	}

	lvl, err := log.LvlFromString(cfg.Log.RuntimeLvl)
	if err != nil {
		return nil, err
	}

	ts, err := st.Storage.TrieState(nil)
	if err != nil {
		return nil, err
	}

	ns := runtime.NodeStorage{
		LocalStorage:      database.NewMemDatabase(),
		PersistentStorage: database.NewTable(st.DB(), "offlinestorage"),
	}
	rtCfg := &runtime.Config{
		Storage:     ts,
		Keystore:    ks,
		Imports:     runtime.RegisterImports_NodeRuntime,
		LogLvl:      lvl,
		NodeStorage: ns,
		Role:        cfg.Core.Roles,
	}

	// create runtime executor
	rt, err := runtime.NewRuntime(code, rtCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime executor: %s", err)
	}

	return rt, nil
}

func createBABEService(cfg *Config, rt *runtime.Runtime, st *state.Service, ks keystore.Keystore) (*babe.Service, error) {
	logger.Info(
		"creating BABE service...",
		"authority", cfg.Core.Authority,
	)

	if ks.Name() != "babe" || ks.Type() != crypto.Sr25519Type {
		return nil, ErrInvalidKeystoreType
	}

	kps := ks.Keypairs()
	logger.Info("keystore", "keys", kps)
	if len(kps) == 0 {
		return nil, ErrNoKeysProvided
	}

	// get best slot to determine next start slot
	header, err := st.Block.BestBlockHeader()
	if err != nil {
		return nil, fmt.Errorf("failed to get latest block: %s", err)
	}

	var bestSlot uint64
	if header.Number.Cmp(big.NewInt(0)) == 0 {
		bestSlot = 0
	} else {
		bestSlot, err = st.Block.GetSlotForBlock(header.Hash())
		if err != nil {
			return nil, fmt.Errorf("failed to get slot for latest block: %s", err)
		}
	}

	lvl, err := log.LvlFromString(cfg.Log.BlockProducerLvl)
	if err != nil {
		return nil, err
	}

	threshold, ok := cfg.Core.BabeThreshold.(*big.Int)
	if !ok && threshold != nil {
		return nil, errors.New("invalid BabeThreshold in configuration")
	}

	bcfg := &babe.ServiceConfig{
		LogLvl:           lvl,
		Keypair:          kps[0].(*sr25519.Keypair),
		Runtime:          rt,
		BlockState:       st.Block,
		StorageState:     st.Storage,
		TransactionQueue: st.TransactionQueue,
		EpochState:       st.Epoch,
		StartSlot:        bestSlot + 1,
		EpochThreshold:   threshold,
		SlotDuration:     cfg.Core.SlotDuration,
	}

	// create new BABE service
	bs, err := babe.NewService(bcfg)
	if err != nil {
		logger.Error("failed to initialize BABE service", "error", err)
		return nil, err
	}

	return bs, nil
}

// Core Service

// createCoreService creates the core service from the provided core configuration
//func createCoreService(cfg *Config, bp BlockProducer, fg core.FinalityGadget, verifier *babe.VerificationManager, rt *runtime.Runtime, ks *keystore.Keystore, stateSrvc *state.Service, coreMsgs chan network.Message, networkMsgs chan network.Message) (*core.Service, error) {
func createCoreService(cfg *Config, bp BlockProducer, fg core.FinalityGadget, verifier *babe.VerificationManager, rt *runtime.Runtime, ks *keystore.GlobalKeystore, stateSrvc *state.Service, net *network.Service) (*core.Service, error) {
	logger.Info(
		"creating core service...",
		"authority", cfg.Core.Authority,
	)

	lvl, err := log.LvlFromString(cfg.Log.CoreLvl)
	if err != nil {
		return nil, err
	}

	handler := grandpa.NewMessageHandler(fg.(*grandpa.Service), stateSrvc.Block)

	// set core configuration
	coreConfig := &core.Config{
		LogLvl:                  lvl,
		BlockState:              stateSrvc.Block,
		StorageState:            stateSrvc.Storage,
		TransactionQueue:        stateSrvc.TransactionQueue,
		BlockProducer:           bp,
		FinalityGadget:          fg,
		ConsensusMessageHandler: handler,
		Keystore:                ks,
		Runtime:                 rt,
		IsBlockProducer:         cfg.Core.BabeAuthority,
		IsFinalityAuthority:     cfg.Core.GrandpaAuthority,
		Verifier:                verifier,
		Network:                 net,
	}

	// create new core service
	coreSrvc, err := core.NewService(coreConfig)
	if err != nil {
		logger.Error("failed to create core service", "error", err)
		return nil, err
	}

	return coreSrvc, nil
}

// Network Service

// createNetworkService creates a network service from the command configuration and genesis data
func createNetworkService(cfg *Config, stateSrvc *state.Service, syncer *sync.Service) (*network.Service, error) {
	logger.Info(
		"creating network service...",
		"roles", cfg.Core.Roles,
		"port", cfg.Network.Port,
		"bootnodes", cfg.Network.Bootnodes,
		"protocol", cfg.Network.ProtocolID,
		"nobootstrap", cfg.Network.NoBootstrap,
		"nomdns", cfg.Network.NoMDNS,
	)

	lvl, err := log.LvlFromString(cfg.Log.NetworkLvl)
	if err != nil {
		return nil, err
	}

	// network service configuation
	networkConfig := network.Config{
		LogLvl:       lvl,
		BlockState:   stateSrvc.Block,
		NetworkState: stateSrvc.Network,
		BasePath:     cfg.Global.BasePath,
		Roles:        cfg.Core.Roles,
		Port:         cfg.Network.Port,
		Bootnodes:    cfg.Network.Bootnodes,
		ProtocolID:   cfg.Network.ProtocolID,
		NoBootstrap:  cfg.Network.NoBootstrap,
		NoMDNS:       cfg.Network.NoMDNS,
		Syncer:       syncer,
	}

	networkSrvc, err := network.NewService(&networkConfig)
	if err != nil {
		logger.Error("failed to create network service", "error", err)
		return nil, err
	}

	return networkSrvc, nil
}

// RPC Service

// createRPCService creates the RPC service from the provided core configuration
func createRPCService(cfg *Config, stateSrvc *state.Service, coreSrvc *core.Service, networkSrvc *network.Service, bp BlockProducer, rt *runtime.Runtime, sysSrvc *system.Service) (*rpc.HTTPServer, error) {
	logger.Info(
		"creating rpc service...",
		"host", cfg.RPC.Host,
		"rpc port", cfg.RPC.Port,
		"mods", cfg.RPC.Modules,
		"ws enabled", cfg.RPC.WSEnabled,
		"ws port", cfg.RPC.WSPort,
	)
	rpcService := rpc.NewService()

	lvl, err := log.LvlFromString(cfg.Log.RPCLvl)
	if err != nil {
		return nil, err
	}

	rpcConfig := &rpc.HTTPServerConfig{
		LogLvl:              lvl,
		BlockAPI:            stateSrvc.Block,
		StorageAPI:          stateSrvc.Storage,
		NetworkAPI:          networkSrvc,
		CoreAPI:             coreSrvc,
		BlockProducerAPI:    bp,
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

	return rpc.NewHTTPServer(rpcConfig), nil
}

// System service
// creates a service for providing system related information
func createSystemService(cfg *types.SystemInfo) *system.Service {
	return system.NewService(cfg)
}

// createGRANDPAService creates a new GRANDPA service
func createGRANDPAService(cfg *Config, rt *runtime.Runtime, st *state.Service, dh *core.DigestHandler, ks keystore.Keystore) (*grandpa.Service, error) {
	ad, err := rt.GrandpaAuthorities()
	if err != nil {
		return nil, err
	}

	if ks.Name() != "gran" || ks.Type() != crypto.Ed25519Type {
		return nil, ErrInvalidKeystoreType
	}

	voters := grandpa.NewVotersFromAuthorityData(ad)

	keys := ks.Keypairs()
	if len(keys) == 0 {
		return nil, errors.New("no ed25519 keys provided for GRANDPA")
	}

	lvl, err := log.LvlFromString(cfg.Log.FinalityGadgetLvl)
	if err != nil {
		return nil, err
	}

	gsCfg := &grandpa.Config{
		LogLvl:        lvl,
		BlockState:    st.Block,
		DigestHandler: dh,
		SetID:         1,
		Voters:        voters,
		Keypair:       keys[0].(*ed25519.Keypair),
		Authority:     cfg.Core.GrandpaAuthority,
	}

	return grandpa.NewService(gsCfg)
}

func createBlockVerifier(cfg *Config, st *state.Service, rt *runtime.Runtime) (*babe.VerificationManager, error) {
	// load BABE verification data from runtime
	babeCfg, err := rt.BabeConfiguration()
	if err != nil {
		return nil, err
	}

	ad, err := types.BABEAuthorityRawToAuthority(babeCfg.GenesisAuthorities)
	if err != nil {
		return nil, err
	}

	var threshold *big.Int
	var ok bool
	// TODO: remove config options, directly set storage values in genesis
	if cfg.Core.BabeThreshold != nil {
		threshold, ok = cfg.Core.BabeThreshold.(*big.Int)
		if !ok {
			return nil, errors.New("invalid BabeThreshold in configuration")
		}
	} else {
		threshold, err = babe.CalculateThreshold(babeCfg.C1, babeCfg.C2, len(babeCfg.GenesisAuthorities))
		if err != nil {
			return nil, err
		}
	}

	descriptor := &babe.Descriptor{
		AuthorityData: ad,
		Randomness:    babeCfg.Randomness,
		Threshold:     threshold,
	}

	ver, err := babe.NewVerificationManager(st.Block, descriptor)
	if err != nil {
		return nil, err
	}

	logger.Info("verifier", "threshold", threshold)
	return ver, nil
}

func createSyncService(cfg *Config, st *state.Service, bp BlockProducer, dh *core.DigestHandler, verifier *babe.VerificationManager, rt *runtime.Runtime) (*sync.Service, error) {
	lvl, err := log.LvlFromString(cfg.Log.SyncLvl)
	if err != nil {
		return nil, err
	}

	syncCfg := &sync.Config{
		LogLvl:           lvl,
		BlockState:       st.Block,
		StorageState:     st.Storage,
		TransactionQueue: st.TransactionQueue,
		BlockProducer:    bp,
		Verifier:         verifier,
		Runtime:          rt,
		DigestHandler:    dh,
	}

	return sync.NewService(syncCfg)
}

func createDigestHandler(st *state.Service, bp BlockProducer, verifier *babe.VerificationManager) (*core.DigestHandler, error) {
	return core.NewDigestHandler(st.Block, bp, nil, verifier)
}
