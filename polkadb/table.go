package polkadb

import (
	log "github.com/ChainSafe/log15"
)

type table struct {
	db     Database
	prefix string
}

type tableBatch struct {
	batch  Batch
	prefix string
}

// NewTable returns a Database object that prefixes all keys with a given
// string.
func NewTable(db Database, prefix string) Database {
	return &table{db: db, prefix: prefix}
}

// Put adds keys with the prefix value given to NewTable
func (dt *table) Put(key []byte, value []byte) error {
	return dt.db.Put(append([]byte(dt.prefix), key...), value)
}

// Has checks keys with the prefix value given to NewTable
func (dt *table) Has(key []byte) (bool, error) {
	return dt.db.Has(append([]byte(dt.prefix), key...))
}

// Get retrieves keys with the prefix value given to NewTable
func (dt *table) Get(key []byte) ([]byte, error) {
	return dt.db.Get(append([]byte(dt.prefix), key...))
}

// Del removes keys with the prefix value given to NewTable
func (dt *table) Del(key []byte) error {
	return dt.db.Del(append([]byte(dt.prefix), key...))
}

// Close closes table db
func (dt *table) Close() error {
	err := dt.db.Close()
	if err != nil {
		log.Info("Database closed")
		return err
	} else {
		log.Crit("Failed to close database")
		return nil
	}
}

// NewIterator initializes type Iterable
func (dt *table) NewIterator() Iterable {
	return Iterable{}
}

// Path returns table prefix
func (dt *table) Path() string {
	return dt.prefix
}

// NewTableBatch returns a Batch object which prefixes all keys with a given string.
func NewTableBatch(db Database, prefix string) Batch {
	return &tableBatch{db.NewBatch(), prefix}
}

// NewBatch returns tableBatch with a Batch type and the given prefix
func (dt *table) NewBatch() Batch {
	return &tableBatch{dt.db.NewBatch(), dt.prefix}
}

// Put encodes key-values with prefix given to NewBatchTable and adds them to a mapping for batch writes, sets the size of item value
func (tb *tableBatch) Put(key, value []byte) error {
	return tb.batch.Put(append([]byte(tb.prefix), key...), value)
}

// Write performs batched writes with the provided prefix
func (tb *tableBatch) Write() error {
	return tb.batch.Write()
}

// ValueSize returns the amount of data in the batch accounting for the given prefix
func (tb *tableBatch) ValueSize() int {
	return tb.batch.ValueSize()
}

// // Reset clears batch key-values and resets the size to zero
func (tb *tableBatch) Reset() {
	tb.batch.Reset()
}

// Delete removes the key from the batch and database
func (tb *tableBatch) Delete(k []byte) error {
	err := tb.batch.Delete(k)
	if err != nil {
		return err
	}
	return nil
}
