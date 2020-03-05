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
	"path"

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

// initializeNode creates a Genesis instance from the configured genesis file
// and then initializes the state database using the Genesis instance
func initializeNode(ctx *cli.Context) error {
	cfg, err := getConfig(ctx)
	if err != nil {
		return err
	}

	datadir := cfg.Global.DataDir
	genPath := getGenesisPath(ctx)

	log.Info(
		"[gossamer] Initializing node...",
		"datadir", datadir,
		"genesis", genPath,
	)

	// load Genesis from genesis configuration file
	gen, err := genesis.LoadGenesisFromJSON(genPath)
	if err != nil {
		log.Error("[gossamer] Failed to load genesis from file", "error", err)
		return err
	}

	log.Info(
		"[gossamer] Loading genesis...",
		"Name", gen.Name,
		"ID", gen.ID,
		"ProtocolID", gen.ProtocolID,
		"Bootnodes", gen.Bootnodes,
	)

	// initialize stateDB and blockDB
	stateSrv := state.NewService(datadir)

	// initialize genesis state from genesis data
	t, header, err := initializeGenesisState(gen)
	if err != nil {
		return err
	}

	// initialize state service with genesis block header
	err = stateSrv.Initialize(header, t)
	if err != nil {
		return fmt.Errorf("failed to initialize state service: %s", err)
	}

	// create and/or open database instance
	db, err := database.NewBadgerDB(datadir)
	if err != nil {
		return err
	}
	defer func() {
		err = db.Close()
		if err != nil {
			log.Error("[gossamer] Failed to close database", "error", err)
		}
	}()

	// set trie database to open database instance
	t.SetDb(&trie.Database{
		DB: db,
	})

	// store genesis data in trie database
	err = storeGenesisData(t, gen)
	if err != nil {
		return fmt.Errorf("failed to store genesis data in trie database: %s", err)
	}

	return nil
}

// initializeGenesisState given raw genesis state data, return the initialized state trie and genesis block header.
func initializeGenesisState(gen *genesis.Genesis) (*trie.Trie, *types.Header, error) {
	t := trie.NewEmptyTrie(nil)

	// set raw genesis data for parent
	genRaw := gen.GenesisFields().Raw[0]

	// load raw genesis data into trie
	err := t.Load(genRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load raw genesis data into trie: %s", err)
	}

	// create state root from trie hash
	stateRoot, err := t.Hash()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create state root from trie hash: %s", err)
	}

	// create genesis block header
	header, err := types.NewHeader(
		common.NewHash([]byte{0}), // parentHash
		big.NewInt(0),             // number
		stateRoot,                 // stateRoot
		trie.EmptyHash,            // extrinsicsRoot
		[][]byte{},                // digest
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create genesis block header: %s", err)
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
	} else if name := ctx.GlobalString(NodeFlag.Name); name != "" {
		return path.Join("node", name, "genesis.json")
	} else {
		return gssmr.DefaultGenesisPath
	}
}

// storeGenesisData stores genesis data in trie database
func storeGenesisData(t *trie.Trie, gen *genesis.Genesis) error {

	// encode trie and write to trie database
	err := t.StoreInDB()
	if err != nil {
		return fmt.Errorf("failed to encode trie and write to database: %s", err)
	}

	// store genesis hash in trie database
	err = t.StoreHash()
	if err != nil {
		return fmt.Errorf("failed to store genesis hash in database: %s", err)
	}

	// store genesis data in trie database
	err = t.Db().StoreGenesisData(gen.GenesisData())
	if err != nil {
		return fmt.Errorf("failed to store genesis data in database: %s", err)
	}

	return nil
}
