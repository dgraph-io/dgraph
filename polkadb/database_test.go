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

package polkadb

import (
	"bytes"
	"testing"

	"fmt"

	"github.com/dgraph-io/badger"
)

type data struct {
	input    string
	expected string
}

func testSetup() []data {
	tests := []data{
		{"camel", "camel"},
		{"walrus", "walrus"},
		{"296204", "296204"},
		{"\x00123\x00", "\x00123\x00"},
	}
	return tests
}

func TestBadgerDB_PutGetDel(t *testing.T) {
	db, remove := newTestDBService(t)
	defer remove()

	testPutGetter(db.StateDB.Db, t)
	testHasGetter(db.StateDB.Db, t)
	testUpdateGetter(db.StateDB.Db, t)
	testDelGetter(db.StateDB.Db, t)
	testGetPath(db.StateDB.Db, t)
}

func testPutGetter(db Database, t *testing.T) {
	tests := testSetup()
	for _, v := range tests {
		v := v
		t.Run("PutGetter", func(t *testing.T) {
			err := db.Put([]byte(v.input), []byte(v.input))
			if err != nil {
				t.Fatalf("put failed: %v", err)
			}
			data, err := db.Get([]byte(v.input))
			if err != nil {
				t.Fatalf("get failed: %v", err)
			}
			if !bytes.Equal(data, []byte(v.expected)) {
				t.Fatalf("get returned wrong result, got %q expected %q", string(data), v.expected)
			}
		})
	}
}

func testHasGetter(db Database, t *testing.T) {
	tests := testSetup()

	for _, v := range tests {
		exists, err := db.Has([]byte(v.input))
		if err != nil {
			t.Fatalf("has operation failed: %v", err)
		}
		if !exists {
			t.Fatalf("has operation returned wrong result, got %t expected %t", exists, true)
		}
	}
}

func testUpdateGetter(db Database, t *testing.T) {
	tests := testSetup()

	for _, v := range tests {
		v := v
		t.Run("UpdateGetter", func(t *testing.T) {
			err := db.Put([]byte(v.input), []byte("?"))
			if err != nil {
				t.Fatalf("put override failed: %v", err)
			}
			data, err := db.Get([]byte(v.input))
			if err != nil {
				t.Fatalf("get failed: %v", err)
			}
			if !bytes.Equal(data, []byte("?")) {
				t.Fatalf("get returned wrong result, got %q expected ?", string(data))
			}
		})
	}
}

func testDelGetter(db Database, t *testing.T) {
	tests := testSetup()

	for _, v := range tests {
		v := v
		t.Run("DelGetter", func(t *testing.T) {
			v := v
			err := db.Del([]byte(v.input))
			if err != nil {
				t.Fatalf("delete %q failed: %v", v.input, err)
			}
			d, _ := db.Get([]byte(v.input))
			if len(d) > 1 {
				t.Fatalf("failed to delete value %q", v.input)
			}
		})
	}
}

func testGetPath(db Database, t *testing.T) {
	dir := db.Path()
	if len(dir) <= 0 {
		t.Fatalf("failed to set database path")
	}
}

func TestBadgerDB_Batch(t *testing.T) {
	db, remove := newTestDBService(t)
	defer remove()
	testBatchPut(db.StateDB.Db, t)
}

func batchTestSetup(db Database) (func(i int) []byte, func(i int) []byte, Batch) {
	testKey := func(i int) []byte {
		return []byte(fmt.Sprintf("%04d", i))
	}
	testValue := func(i int) []byte {
		return []byte(fmt.Sprintf("%05d", i))
	}
	b := db.NewBatch()
	return testKey, testValue, b
}

func testBatchPut(db Database, t *testing.T) {
	k, v, b := batchTestSetup(db)

	for i := 0; i < 10000; i++ {
		err := b.Put(k(i), v(i))
		if err != nil {
			t.Fatalf("failed to add key-value to batch mapping  %q", err)
		}
		err = b.Write()
		if err != nil {
			t.Fatalf("failed to write batch %q", err)
		}
		size := b.ValueSize()
		if size == 0 {
			t.Fatalf("failed to set size of data in each batch, got %v", size)
		}
		err = b.Delete([]byte(k(i)))
		if err != nil {
			t.Fatalf("failed to delete batch key %v", k(i))
		}
		b.Reset()
		if b.ValueSize() != 0 {
			t.Fatalf("failed to reset batch mapping to zero, got %v, expected %v", b.ValueSize(), 0)
		}
	}
}

func TestBadgerDB_Iterator(t *testing.T) {
	db, remove := newTestDBService(t)
	defer remove()

	testNewIterator(db.StateDB.Db, t)
	testNextKeyIterator(db.StateDB.Db, t)
	testSeekKeyValueIterator(db.StateDB.Db, t)
}

func testIteratorSetup(db Database, t *testing.T) {
	k, v, b := batchTestSetup(db)

	for i := 0; i < 5; i++ {
		err := b.Put(k(i), v(i))
		if err != nil {
			t.Fatalf("failed to add key-value to batch mapping  %q", err)
		}
		err = b.Write()
		if err != nil {
			t.Fatalf("failed to write batch %q", err)
		}
	}
}

func testNewIterator(db Database, t *testing.T) {
	testIteratorSetup(db, t)

	it := db.NewIterator()
	defer func() {
		if it.Released() != true {
			it.Release()
		}
	}()
	if it.init {
		t.Fatalf("failed to init iterator")
	}
	if it.released {
		t.Fatalf("failed to set release to false")
	}
	i, ok := interface{}(it.iter).(*badger.Iterator)
	if !ok {
		t.Fatalf("failed to set badger Iterator type %v", i)
	}
	txn, ok := interface{}(it.txn).(*badger.Txn)
	if !ok {
		t.Fatalf("failed to set badger Txn type %v", txn)
	}
}

func testNextKeyIterator(db Database, t *testing.T) {
	testIteratorSetup(db, t)

	it := db.NewIterator()
	defer func() {
		if it.Released() != true {
			it.Release()
		}
	}()

	ok := it.Next()
	if !ok {
		t.Fatalf("failed to rewind the iterator to the zero-th position")
	}
	for it.Next() {
		if it.Key() == nil {
			t.Fatalf("failed to retrieve keys %v", it.Key())
		}
	}
}

func testKVData() []data {
	testKeyValue := []data{
		{"0003", "00003"},
		{"0001", "00001"},
		{"0002", "00002"},
		{"0000", "00000"},
		{"0004", "00004"},
	}
	return testKeyValue
}

func testSeekKeyValueIterator(db Database, t *testing.T) {
	testIteratorSetup(db, t)
	kv := testKVData()

	it := db.NewIterator()
	defer func() {
		if it.Released() != true {
			it.Release()
		}
	}()

	for _, k := range kv {
		k := k
		t.Run("SeekKeyValueIterator", func(t *testing.T) {
			it.Seek([]byte(k.input))
			if !bytes.Equal(it.Key(), []byte(k.input)) {
				t.Fatalf("failed to retrieve presented key, got %v, expected %v", it.Key(), k.input)
			}
			it.Seek([]byte(k.input))
			if !bytes.Equal(it.Value(), []byte(k.expected)) {
				t.Fatalf("failed to retrieve presented key, got %v, expected %v", it.Key(), k.expected)
			}
		})
	}
}
