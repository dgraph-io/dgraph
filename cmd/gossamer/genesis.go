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
	"math/big"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/database"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/node/gssmr"

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
	if ctx.String(DataDirFlag.Name) != "" {
		dataDir = expandTildeOrDot(ctx.String(DataDirFlag.Name))
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

	// initialize database with genesis storage state
	db, err := database.NewBadgerDB(dataDir)
	if err != nil {
		return err
	}

	defer func() {
		err = db.Close()
		if err != nil {
			log.Error("Loading genesis: cannot close db", "error", err)
		}
	}()

	// set up trie database
	t.SetDb(&trie.Database{
		DB: db,
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

	// store node name, ID, network protocol, bootnodes in state database
	return t.Db().StoreGenesisData(gen.GenesisData())
}

// initializeGenesisState given raw genesis state data, return the initialized state trie and genesis block header.
func initializeGenesisState(gen genesis.Fields) (*trie.Trie, *types.Header, error) {
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
	if file := ctx.String(GenesisFlag.Name); file != "" {
		return file
	} else if file := ctx.GlobalString(GenesisFlag.Name); file != "" {
		return file
	} else {
		return gssmr.DefaultGenesisPath
	}
}
