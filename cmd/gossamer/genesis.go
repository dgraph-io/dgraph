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

	// get node configuration
	cfg, err := getConfig(ctx)
	if err != nil {
		return err
	}

	datadir := cfg.Global.DataDir

	// get genesis configuration path
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

	// create and load trie from genesis
	t, err := newTrieFromGenesis(gen)
	if err != nil {
		log.Error("[gossamer] Failed to create trie from genesis", "error", err)
		return err
	}

	// generates genesis block header from trie and stores it in state database
	err = loadGenesisBlock(t, datadir)
	if err != nil {
		log.Error("[gossamer] Failed to load genesis block with state service", "error", err)
		return err
	}

	// initialize trie database
	err = initializeTrieDatabase(t, datadir, gen)
	if err != nil {
		log.Error("[gossamer] Failed to initialize trie database", "error", err)
		return err
	}

	log.Info(
		"[gossamer] Node initialized",
		"datadir", datadir,
		"genesis", genPath,
	)

	return nil
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

// newTrieFromGenesis creates a new trie and loads it with the raw genesis data
func newTrieFromGenesis(gen *genesis.Genesis) (*trie.Trie, error) {
	// create new empty trie
	t := trie.NewEmptyTrie(nil)

	// set raw genesis data for parent
	genRaw := gen.GenesisFields().Raw[0]

	// load raw genesis data into trie
	err := t.Load(genRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to load raw genesis data into trie: %s", err)
	}

	return t, nil
}

// loadGenesisBlock generates the genesis block from the trie (assumes trie is
// already loaded with the raw genesis data), and then loads the genesis block
// header with state service (storing the genesis block in the state database)
func loadGenesisBlock(t *trie.Trie, datadir string) error {
	// create state root from trie hash
	stateRoot, err := t.Hash()
	if err != nil {
		return fmt.Errorf("failed to create state root from trie hash: %s", err)
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
		return fmt.Errorf("failed to create genesis block header: %s", err)
	}

	// create new state service
	stateSrv := state.NewService(datadir)

	// initialize state service with genesis block header
	err = stateSrv.Initialize(header, t)
	if err != nil {
		return fmt.Errorf("failed to initialize state service: %s", err)
	}

	return nil
}

// initializeTrieDatabase initializes and sets the trie database and then stores
// the encoded trie, the genesis hash, and the gensis data in the trie database
func initializeTrieDatabase(t *trie.Trie, datadir string, gen *genesis.Genesis) error {
	// initialize database within data directory
	db, err := database.NewBadgerDB(datadir)
	if err != nil {
		return fmt.Errorf("failed to open database: %s", err)
	}

	// set trie database to initialized database
	t.SetDb(&trie.Database{
		DB: db,
	})

	// store genesis data in trie database
	err = storeGenesisData(t, gen)
	if err != nil {
		log.Error("[gossamer] Failed to store genesis data in trie database", "error", err)
		return err
	}

	// close trie database
	err = db.Close()
	if err != nil {
		log.Error("[gossamer] Failed to close trie database", "error", err)
	}

	return nil
}

// storeGenesisData stores the encoded trie, the genesis hash, and the gensis
// data in the trie database
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
