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
	"os"
	"testing"

	db "github.com/ChainSafe/gossamer/polkadb"
)

func newTrie() (*Trie, error) {
	hasher, err := NewHasher()
	if err != nil {
		return nil, err
	}

	db, err := db.NewBadgerService("./gossamer_data")
	if err != nil {
		return nil, err
	}

	trie := &Trie{
		db: &Database{
			Db:     db,
			Hasher: hasher,
		},
		root: nil,
	}

	trie.db.Batch = trie.db.Db.NewBatch()

	return trie, nil
}

func (t *Trie) closeDb() {
	t.db.Db.Close()
	if err := os.RemoveAll("./gossamer_data"); err != nil {
		fmt.Println("removal of temp directory gossamer_data failed")
	}
}

func TestWriteToDB(t *testing.T) {
	trie, err := newTrie()
	if err != nil {
		t.Fatal(err)
	}

	rt := generateRandomTests(20000)
	var val []byte
	for _, test := range rt {
		err = trie.Put(test.key, test.value)
		if err != nil {
			t.Errorf("Fail to put with key %x and value %x: %s", test.key, test.value, err.Error())
		}

		val, err = trie.Get(test.key)
		if err != nil {
			t.Errorf("Fail to get key %x: %s", test.key, err.Error())
		} else if !bytes.Equal(val, test.value) {
			t.Errorf("Fail to get key %x with value %x: got %x", test.key, test.value, val)
		}
	}

	err = trie.WriteToDB()
	if err != nil {
		t.Errorf("Fail: could not write to batch writer: %s", err)
	}

	err = trie.Commit()
	if err != nil {
		t.Errorf("Fail: could not commit (batch write) to DB: %s", err)
	}

	trie.closeDb()
}

func TestWriteDirty(t *testing.T) {
	trie, err := newTrie()
	if err != nil {
		t.Fatal(err)
	}

	dirtyNode := &leaf{key: generateRandBytes(10), value: generateRandBytes(10), dirty: true}
	written, err := trie.writeNodeToDB(dirtyNode)
	if err != nil {
		t.Errorf("Fail: could not write to db: %s", err)
	} else if !written {
		t.Errorf("Fail: did not write dirty node to db")
	}

	cleanNode := &leaf{key: generateRandBytes(10), value: generateRandBytes(10), dirty: false}
	written, err = trie.writeNodeToDB(cleanNode)
	if err != nil {
		t.Errorf("Fail: could not write to db: %s", err)
	} else if written {
		t.Errorf("Fail: wrote clean node to db")
	}

	trie.closeDb()
}
