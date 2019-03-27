package polkadb

import (
	"errors"
	"sync"
)

// MemDatabase test memory database, data is not persisted
type MemDatabase struct {
	db   map[string][]byte
	lock sync.RWMutex
}

// NewMemDatabase returns an initialized mapping used for test database
func NewMemDatabase() (*MemDatabase, error) {
	return &MemDatabase{
		db: make(map[string][]byte),
	}, nil
}

// Put puts the given key / value into the mapping
func (db *MemDatabase) Put(k []byte, v []byte) error {
	db.lock.Lock()
	defer db.lock.Unlock()

	db.db[string(k)] = v
	return nil
}

// Has checks the given key exists already; returning true or false
func (db *MemDatabase) Has(k []byte) (bool, error) {
	db.lock.RLock()
	defer db.lock.RUnlock()

	_, ok := db.db[string(k)]
	return ok, nil
}

// Get returns the given key []byte
func (db *MemDatabase) Get(k []byte) ([]byte, error) {
	db.lock.RLock()
	defer db.lock.RUnlock()

	if v, ok := db.db[string(k)]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}

// Keys returns [][]byte of mapping keys
func (db *MemDatabase) Keys() [][]byte {
	db.lock.RLock()
	defer db.lock.RUnlock()

	keys := [][]byte{}
	for key := range db.db {
		keys = append(keys, []byte(key))
	}
	return keys
}

// Delete removes the key from the mapping
func (db *MemDatabase) Del(key []byte) error {
	db.lock.Lock()
	defer db.lock.Unlock()

	delete(db.db, string(key))
	return nil
}

// NewBatch ...
func (db *MemDatabase) NewBatch() Batch {
	return nil
}
