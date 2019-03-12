package polkadb

import (
	"log"

	"github.com/dgraph-io/badger"
	"github.com/golang/snappy"
)

// BadgerDB struct contains directory path to data and db instance
type BadgerDB struct {
	path string
	db   *badger.DB
}

type table struct {
	db     Database
	prefix string
}

// NewBadgerDB opens and returns a new DB object
func NewBadgerDB(file string) (*BadgerDB, error) {
	opts := badger.DefaultOptions
	opts.Dir = file
	opts.ValueDir = file
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return &BadgerDB{
		path: file,
		db:   db,
	}, nil
}

// Path returns the path to the database directory.
func (db *BadgerDB) Path() string {
	return db.path
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
		item, err := txn.Get(snappy.Encode(nil, key))
		if item != nil {
			exists = true
		}
		if err == badger.ErrKeyNotFound {
			exists = false
			err = nil
		}
		return err
	})
	return exists, err
}

// Get returns the given key
func (db *BadgerDB) Get(key []byte) (data []byte, err error) {
	_ = db.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(snappy.Encode(nil, key))
		if err != nil {
			return err
		}
		val, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		data, _ = snappy.Decode(nil, val)
		return nil
	})
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
func (db *BadgerDB) Close() {
	err := db.db.Close()
	if err == nil {
		log.Println("Database closed")
	} else {
		log.Fatal("Failed to close database", "err", err)
	}
}

// NewTable returns a Database object that prefixes all keys with a given
// string.
func NewTable(db Database, prefix string) Database {
	return &table{
		db:     db,
		prefix: prefix,
	}
}

func (dt *table) Put(key []byte, value []byte) error {
	return dt.db.Put(append([]byte(dt.prefix), key...), value)
}

func (dt *table) Has(key []byte) (bool, error) {
	return dt.db.Has(append([]byte(dt.prefix), key...))
}

func (dt *table) Get(key []byte) ([]byte, error) {
	return dt.db.Get(append([]byte(dt.prefix), key...))
}

func (dt *table) Del(key []byte) error {
	return dt.db.Del(append([]byte(dt.prefix), key...))
}
