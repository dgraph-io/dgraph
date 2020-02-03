package main

import (
	"fmt"
	"math/big"
	"path/filepath"

	"github.com/ChainSafe/gossamer/cmd/utils"
	"github.com/ChainSafe/gossamer/common"
	cfg "github.com/ChainSafe/gossamer/config"
	"github.com/ChainSafe/gossamer/config/genesis"
	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/state"
	"github.com/ChainSafe/gossamer/trie"
	log "github.com/ChainSafe/log15"
	"github.com/urfave/cli"
)

func loadGenesis(ctx *cli.Context) error {
	currentConfig, err := getConfig(ctx)
	if err != nil {
		return err
	}

	// read genesis file
	genesisPath := getGenesisPath(ctx)
	dataDir := expandTildeOrDot(currentConfig.Global.DataDir)
	if ctx.String(utils.DataDirFlag.Name) != "" {
		dataDir = expandTildeOrDot(ctx.String(utils.DataDirFlag.Name))
	}

	log.Debug("Loading genesis", "genesisPath", genesisPath, "dataDir", dataDir)

	// read genesis configuration file
	gen, err := genesis.LoadGenesisFromJSON(genesisPath)
	if err != nil {
		return err
	}

	log.Info("🕸\t Initializing node", "Name", gen.Name, "ID", gen.ID, "ProtocolID", gen.ProtocolID, "Bootnodes", gen.Bootnodes)

	// initialize stateDB and blockDB
	stateSrv := state.NewService(dataDir)

	t, header, err := initializeGenesisState(gen.GenesisFields())
	if err != nil {
		return err
	}

	// initialize DB with genesis header
	err = stateSrv.Initialize(header, t)
	if err != nil {
		return fmt.Errorf("cannot initialize state service: %s", err)
	}

	stateDataDir := filepath.Join(dataDir, "state")
	stateDb, err := state.NewStorageState(stateDataDir, t)
	if err != nil {
		return fmt.Errorf("cannot create state db: %s", err)
	}

	defer func() {
		err = stateDb.Db.Db.Close()
		if err != nil {
			log.Error("Loading genesis: cannot close stateDB", "error", err)
		}
	}()

	// set up trie database
	t.SetDb(&trie.Database{
		DB: stateDb.Db.Db,
	})

	// write initial genesis data to DB
	err = t.StoreInDB()
	if err != nil {
		return fmt.Errorf("cannot store genesis data in db: %s", err)
	}

	err = t.StoreHash()
	if err != nil {
		return fmt.Errorf("cannot store genesis hash in db: %s", err)
	}

	// store node name, ID, p2p protocol, bootnodes in DB
	return t.Db().StoreGenesisData(gen.GenesisData())
}

// initializeGenesisState given raw genesis state data, return the initialized state trie and genesis block header.
func initializeGenesisState(gen genesis.GenesisFields) (*trie.Trie, *types.Header, error) {
	t := trie.NewEmptyTrie(nil)
	err := t.Load(gen.Raw[0])
	if err != nil {
		return nil, nil, fmt.Errorf("cannot load trie with initial state: %s", err)
	}

	stateRoot, err := t.Hash()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot create state root: %s", err)
	}

	header, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), stateRoot, trie.EmptyHash, [][]byte{})
	if err != nil {
		return nil, nil, fmt.Errorf("cannot create genesis header: %s", err)
	}

	return t, header, nil
}

// getGenesisPath gets the path to the genesis file
func getGenesisPath(ctx *cli.Context) string {
	// Check local string genesis flags first
	if file := ctx.String(utils.GenesisFlag.Name); file != "" {
		return file
	} else if file := ctx.GlobalString(utils.GenesisFlag.Name); file != "" {
		return file
	} else {
		return cfg.DefaultGenesisPath
	}
}
