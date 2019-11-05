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

package trie

import (
	"bytes"
	"fmt"
	"io"

	scale "github.com/ChainSafe/gossamer/codec"
	"github.com/ChainSafe/gossamer/common"
	"github.com/ChainSafe/gossamer/config/genesis"
	"github.com/ChainSafe/gossamer/polkadb"
)

var (
	LatestHashKey  = []byte("latest_hash")
	GenesisDataKey = []byte("genesis_data")
)

// Database is a wrapper around a polkadb
type Database struct {
	Db     polkadb.Database
	Batch  polkadb.Batch
	Hasher *Hasher
}

func NewDatabase(db polkadb.Database) *Database {
	batch := db.NewBatch()

	return &Database{
		Db:    db,
		Batch: batch,
	}
}

func (db *Database) Store(key, value []byte) error {
	return db.Db.Put(key, value)
}

func (db *Database) Load(key []byte) ([]byte, error) {
	return db.Db.Get(key)
}

func (db *Database) StoreLatestHash(hash []byte) error {
	return db.Db.Put(LatestHashKey, hash)
}

func (db *Database) LoadLatestHash() (common.Hash, error) {
	hashbytes, err := db.Db.Get(LatestHashKey)
	if err != nil {
		return common.Hash{}, err
	}

	return common.NewHash(hashbytes), nil
}

type Genesis struct {
	Name       []byte
	Id         []byte
	ProtocolId []byte
	Bootnodes  [][]byte
}

func NewGenesisFromData(gen *genesis.Genesis) *Genesis {
	bnodes := common.StringArrayToBytes(gen.Bootnodes)
	return &Genesis{
		Name:       []byte(gen.Name),
		Id:         []byte(gen.Id),
		ProtocolId: []byte(gen.ProtocolId),
		Bootnodes:  bnodes,
	}
}

func (db *Database) StoreGenesisData(gen *Genesis) error {
	data := &Genesis{
		Name:       gen.Name,
		Id:         gen.Id,
		Bootnodes:  gen.Bootnodes,
		ProtocolId: gen.ProtocolId,
	}

	enc, err := scale.Encode(data)
	if err != nil {
		return err
	}

	return db.Store(GenesisDataKey, enc)
}

func (db *Database) LoadGenesisData() (*Genesis, error) {
	enc, err := db.Load(GenesisDataKey)
	if err != nil {
		return nil, err
	}

	data, err := scale.Decode(enc, &Genesis{
		Bootnodes: [][]byte{{}},
	})
	if err != nil {
		return nil, err
	}

	return data.(*Genesis), nil
}

// Encode traverses the trie recursively, encodes each node, SCALE encodes the encoded node, and appends them all together
func (t *Trie) Encode() ([]byte, error) {
	return encode(t.root, []byte{})
}

func encode(n node, enc []byte) ([]byte, error) {
	nenc, err := n.Encode()
	if err != nil {
		return enc, err
	}

	scnenc, err := scale.Encode(nenc)
	if err != nil {
		return nil, err
	}

	enc = append(enc, scnenc...)

	switch n := n.(type) {
	case *branch:
		for _, child := range n.children {
			if child != nil {
				enc, err = encode(child, enc)
				if err != nil {
					return enc, err
				}
			}
		}
	}

	return enc, nil
}

// Decode decodes a trie from the DB and sets the receiver to it
// The encoded trie must have been encoded with t.Encode
func (t *Trie) Decode(enc []byte) error {
	r := &bytes.Buffer{}
	_, err := r.Write(enc)
	if err != nil {
		return err
	}

	sd := &scale.Decoder{Reader: r}
	scroot, err := sd.Decode([]byte{})
	if err != nil {
		return err
	}

	n := &bytes.Buffer{}
	_, err = n.Write(scroot.([]byte))
	if err != nil {
		return err
	}

	t.root, err = Decode(n)
	if err != nil {
		return err
	}

	return decode(r, t.root)
}

func decode(r io.Reader, prev node) error {
	sd := &scale.Decoder{Reader: r}

	if b, ok := prev.(*branch); ok {
		for i, child := range b.children {
			if child != nil {
				// there's supposed to be a child here, decode the next node and place it
				// when we decode a branch node, we only know if a child is supposed to exist at a certain index (due to the
				// bitmap). we also have the hashes of the children, but we can't reconstruct the children from that. so
				// instead, we put an empty leaf node where the child should be, so when we reconstruct it in this function,
				// we can see that it's non-nil and we should decode the next node from the reader and place it here
				scnode, err := sd.Decode([]byte{})
				if err != nil {
					return err
				}

				n := &bytes.Buffer{}
				_, err = n.Write(scnode.([]byte))
				if err != nil {
					return err
				}

				b.children[i], err = Decode(n)
				if err != nil {
					return fmt.Errorf("could not decode child at %d: %s", i, err)
				}

				err = decode(r, b.children[i])
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// StoreInDB encodes the entire trie and writes it to the DB
// The key to the DB entry is the root hash of the trie
func (t *Trie) StoreInDB() error {
	enc, err := t.Encode()
	if err != nil {
		return err
	}

	roothash, err := t.Hash()
	if err != nil {
		return err
	}

	return t.db.Store(roothash[:], enc)
}

// LoadFromDB loads an encoded trie from the DB where the key is `root`
func (t *Trie) LoadFromDB(root common.Hash) error {
	enctrie, err := t.db.Load(root[:])
	if err != nil {
		return err
	}

	return t.Decode(enctrie)
}

// StoreHash stores the current root hash in the database at `LatestHashKey`
func (t *Trie) StoreHash() error {
	hash, err := t.Hash()
	if err != nil {
		return err
	}

	return t.db.StoreLatestHash(hash[:])
}

// LoadHash retrieves the hash stored at `LatestHashKey` from the DB
func (t *Trie) LoadHash() (common.Hash, error) {
	return t.db.LoadLatestHash()
}
