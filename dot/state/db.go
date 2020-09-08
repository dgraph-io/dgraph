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

package state

import (
	"fmt"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/scale"
	"github.com/ChainSafe/gossamer/lib/trie"

	database "github.com/ChainSafe/chaindb"
)

// StoreBestBlockHash stores the hash at the BestBlockHashKey
func StoreBestBlockHash(db database.Database, hash common.Hash) error {
	return db.Put(common.BestBlockHashKey, hash[:])
}

// LoadBestBlockHash loads the hash stored at BestBlockHashKey
func LoadBestBlockHash(db database.Database) (common.Hash, error) {
	hash, err := db.Get(common.BestBlockHashKey)
	if err != nil {
		return common.Hash{}, err
	}

	return common.NewHash(hash), nil
}

// StoreGenesisData stores the given genesis data at the known GenesisDataKey.
func StoreGenesisData(db database.Database, gen *genesis.Data) error {
	enc, err := scale.Encode(gen)
	if err != nil {
		return fmt.Errorf("cannot scale encode genesis data: %s", err)
	}

	return db.Put(common.GenesisDataKey, enc)
}

// LoadGenesisData retrieves the genesis data stored at the known GenesisDataKey.
func LoadGenesisData(db database.Database) (*genesis.Data, error) {
	enc, err := db.Get(common.GenesisDataKey)
	if err != nil {
		return nil, err
	}

	data, err := scale.Decode(enc, &genesis.Data{})
	if err != nil {
		return nil, err
	}

	return data.(*genesis.Data), nil
}

// StoreLatestStorageHash stores the current root hash in the database at LatestStorageHashKey
func StoreLatestStorageHash(db database.Database, t *trie.Trie) error {
	hash, err := t.Hash()
	if err != nil {
		return err
	}

	return db.Put(common.LatestStorageHashKey, hash[:])
}

// LoadLatestStorageHash retrieves the hash stored at LatestStorageHashKey from the DB
func LoadLatestStorageHash(db database.Database) (common.Hash, error) {
	hashbytes, err := db.Get(common.LatestStorageHashKey)
	if err != nil {
		return common.Hash{}, err
	}

	return common.NewHash(hashbytes), nil
}

// StoreTrie encodes the entire trie and writes it to the DB
// The key to the DB entry is the root hash of the trie
func StoreTrie(db database.Database, t *trie.Trie) error {
	enc, err := t.Encode()
	if err != nil {
		return err
	}

	roothash, err := t.Hash()
	if err != nil {
		return err
	}

	if has, err := db.Has(roothash[:]); err == nil && has {
		return nil
	}

	return db.Put(roothash[:], enc)
}

// LoadTrie loads an encoded trie from the DB where the key is `root`
func LoadTrie(db database.Database, t *trie.Trie, root common.Hash) error {
	enctrie, err := db.Get(root[:])
	if err != nil {
		return err
	}

	return t.Decode(enctrie)
}
