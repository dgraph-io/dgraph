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
	"sync"

	"github.com/ChainSafe/gossamer/polkadb"
)

// Database is a wrapper around a polkadb
type Database struct {
	Db     polkadb.Database
	Batch  polkadb.Batch
	Lock   sync.RWMutex
	Hasher *Hasher
}

// WriteToDB writes the trie to the underlying database batch writer
// Stores the merkle value of the node as the key and the encoded node as the value
// This does not actually write to the db, just to the batch writer
// Commit must be called afterwards to finish writing to the db
func (t *Trie) WriteToDB() error {
	t.db.Batch = t.db.Db.NewBatch()
	return t.writeToDB(t.root)
}

// writeToDB recursively attempts to write each node in the trie to the db batch writer
func (t *Trie) writeToDB(n node) error {
	_, err := t.writeNodeToDB(n)
	if err != nil {
		return err
	}

	switch n := n.(type) {
	case *branch:
		for _, child := range n.children {
			if child != nil {
				err = t.writeToDB(child)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// writeNodeToDB returns true if node is written to db batch writer, false otherwise
// if node is clean, it will not attempt to be written to the db
// otherwise if it's dirty, try to write it to db
func (t *Trie) writeNodeToDB(n node) (bool, error) {
	if !n.isDirty() {
		return false, nil
	}

	encRoot, err := Encode(n)
	if err != nil {
		return false, err
	}

	hash, err := t.db.Hasher.Hash(n)
	if err != nil {
		return false, err
	}

	t.db.Lock.Lock()
	err = t.db.Batch.Put(hash, encRoot)
	t.db.Lock.Unlock()

	n.setDirty(false)
	return true, err
}

// Commit writes the contents of the db's batch writer to the db
func (t *Trie) Commit() error {
	return t.db.Batch.Write()
}
