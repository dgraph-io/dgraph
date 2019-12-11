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
	"os"

	log "github.com/ChainSafe/log15"
	"github.com/dgraph-io/badger/v2"
	"github.com/golang/snappy"
)

// BadgerDB contains directory path to data and db instance
type BadgerDB struct {
	config Config
	db     *badger.DB
}

//Config defines configurations for BadgerService instance
type Config struct {
	DataDir string
}

// NewBadgerDB initializes badgerDB instance
func NewBadgerDB(file string) (*BadgerDB, error) {
	opts := badger.DefaultOptions(file)
	opts.ValueDir = file
	opts.Logger = nil
	opts.WithSyncWrites(false)
	opts.WithNumCompactors(20)

	if err := os.MkdirAll(file, os.ModePerm); err != nil {
		return nil, err
	}
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	return &BadgerDB{
		config: Config{
			DataDir: file,
		},
		db: db,
	}, nil
}

// Path returns the path to the database directory.
func (db *BadgerDB) Path() string {
	return db.config.DataDir
}

// Batch struct contains a database instance, key-value mapping for batch writes and length of item value for batch write
type batchWriter struct {
	db   *BadgerDB
	b    map[string][]byte
	size int
}

// NewBatch returns batchWriter with a badgerDB instance and an initialized mapping
func (db *BadgerDB) NewBatch() Batch {
	return &batchWriter{
		db: db,
		b:  make(map[string][]byte),
	}
}

// Put puts the given key / value to the queue
func (db *BadgerDB) Put(key []byte, value []byte) error {
	return db.db.Update(func(txn *badger.Txn) error {
		err := txn.Set(snappy.Encode(nil, key), snappy.Encode(nil, value))
		return err
	})
}

// Has checks the given key exists already; returning true or false
func (db *BadgerDB) Has(key []byte) (exists bool, err error) {
	err = db.db.View(func(txn *badger.Txn) error {
		item, errr := txn.Get(snappy.Encode(nil, key))
		if item != nil {
			exists = true
		}
		if errr == badger.ErrKeyNotFound {
			exists = false
			errr = nil
		}
		return errr
	})
	return exists, err
}

// Get returns the given key
func (db *BadgerDB) Get(key []byte) (data []byte, err error) {
	err = db.db.View(func(txn *badger.Txn) error {
		item, e := txn.Get(snappy.Encode(nil, key))
		if e != nil {
			return e
		}
		val, e := item.ValueCopy(nil)
		if e != nil {
			return e
		}
		data, _ = snappy.Decode(nil, val)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Del removes the key from the queue and database
func (db *BadgerDB) Del(key []byte) error {
	return db.db.Update(func(txn *badger.Txn) error {
		err := txn.Delete(snappy.Encode(nil, key))
		if err == badger.ErrKeyNotFound {
			err = nil
		}
		return err
	})
}

// Close closes a DB
func (db *BadgerDB) Close() error {
	err := db.db.Close()
	if err == nil {
		log.Info("Database closed")
		return err
	} else {
		log.Crit("Failed to close database", "err", err)
		return nil
	}
}

// Iterable struct contains a transaction, iterator and context fields released, initialized
type Iterable struct {
	txn      *badger.Txn
	iter     *badger.Iterator
	released bool
	init     bool
}

// NewIterator returns a new iterator within the Iterator struct along with a new transaction
func (db *BadgerDB) NewIterator() Iterable {
	txn := db.db.NewTransaction(false)
	opts := badger.DefaultIteratorOptions
	iter := txn.NewIterator(opts)
	return Iterable{
		txn:      txn,
		iter:     iter,
		released: false,
		init:     false,
	}
}

// Release closes the iterator, discards the created transaction and sets released value to true
func (i *Iterable) Release() {
	i.iter.Close()
	i.txn.Discard()
	i.released = true
}

// Released returns the boolean indicating whether the iterator and transaction was successfully released
func (i *Iterable) Released() bool {
	return i.released
}

// Next rewinds the iterator to the zero-th position if uninitialized, and then will advance the iterator by one
// returns bool to ensure access to the item
func (i *Iterable) Next() bool {
	if !i.init {
		i.iter.Rewind()
		i.init = true
	}
	i.iter.Next()
	return i.iter.Valid()
}

// Seek will look for the provided key if present and go to that position. If
// absent, it would seek to the next smallest key
func (i *Iterable) Seek(key []byte) {
	i.iter.Seek(snappy.Encode(nil, key))
}

// Key returns an item key
func (i *Iterable) Key() []byte {
	ret, err := snappy.Decode(nil, i.iter.Item().Key())
	if err != nil {
		log.Warn("key retrieval error ", "error", err)
	}
	return ret
}

// Value returns a copy of the value of the item
func (i *Iterable) Value() []byte {
	val, err := i.iter.Item().ValueCopy(nil)
	if err != nil {
		log.Warn("value retrieval error ", "error", err)
	}
	ret, err := snappy.Decode(nil, val)
	if err != nil {
		log.Warn("value decoding error ", "error", err)
	}
	return ret
}

// Put encodes key-values and adds them to a mapping for batch writes, sets the size of item value
func (b *batchWriter) Put(key, value []byte) error {
	encodedKey := snappy.Encode(nil, key)
	encodedVal := snappy.Encode(nil, value)
	b.b[string(encodedKey)] = encodedVal
	b.size += len(value)
	return nil
}

// Write performs batched writes
func (b *batchWriter) Write() error {
	wb := b.db.db.NewWriteBatch()
	defer wb.Cancel()

	for k, v := range b.b {
		err := wb.Set([]byte(k), v)
		if err != nil {
			log.Warn("error writing batch txs ", "error", err)
		}
	}
	if err := wb.Flush(); err != nil {
		log.Warn("error stored by write batch ", "error", err)
	}
	return nil
}

// ValueSize returns the amount of data in the batch
func (b *batchWriter) ValueSize() int {
	return b.size
}

// Delete removes the key from the batch and database
func (b *batchWriter) Delete(key []byte) error {
	err := b.db.db.NewWriteBatch().Delete(key)
	if err != nil {
		log.Warn("error batch deleting key ", "error", err)
	}
	b.size++
	return nil
}

// Reset clears batch key-values and resets the size to zero
func (b *batchWriter) Reset() {
	b.b = make(map[string][]byte)
	b.size = 0
}
