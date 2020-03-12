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

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/database"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
)

// newTrieFromGenesis creates a new trie and loads it with the raw genesis data
func newTrieFromGenesis(gen *genesis.Genesis) (*trie.Trie, error) {
	t := trie.NewEmptyTrie(nil)

	raw := gen.GenesisFields().Raw[0]

	err := t.Load(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to create trie from genesis: %s", err)
	}

	return t, nil
}

// loadGenesisBlock
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
	stateSrvc := state.NewService(datadir)

	// initialize state service with genesis block header
	err = stateSrvc.Initialize(header, t)
	if err != nil {
		return fmt.Errorf("failed to initialize state service: %s", err)
	}

	return nil
}

// initTrieDatabase initializes and sets the trie database and then stores
// the encoded trie, the genesis hash, and the genesis data in the trie database
func initTrieDatabase(t *trie.Trie, datadir string, gen *genesis.Genesis) error {
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
		log.Error("[dot] Failed to store genesis data in trie database", "error", err)
		return err
	}

	// close trie database
	err = db.Close()
	if err != nil {
		log.Error("[dot] Failed to close trie database", "error", err)
	}

	return nil
}

// storeGenesisData stores the encoded trie, the genesis hash, and the genesis
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
