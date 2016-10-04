package rdb

// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"
import (
	"errors"
	"unsafe"
)

// Range is a range of keys in the database. GetApproximateSizes calls with it
// begin at the key Start and end right before the key Limit.
type Range struct {
	Start []byte
	Limit []byte
}

// DB is a reusable handle to a RocksDB database on disk, created by Open.
type DB struct {
	c    *C.rdb_t
	name string
	opts *Options
}

// OpenDb opens a database with the specified options.
func OpenDb(opts *Options, name string) (*DB, error) {
	var (
		cErr  *C.char
		cName = C.CString(name)
	)
	defer C.free(unsafe.Pointer(cName))
	db := C.rdb_open(opts.c, cName, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return nil, errors.New(C.GoString(cErr))
	}
	return &DB{
		name: name,
		c:    db,
		opts: opts,
	}, nil
}

// OpenDbForReadOnly opens a database with the specified options for readonly usage.
func OpenDbForReadOnly(opts *Options, name string, errorIfLogFileExist bool) (*DB, error) {
	var (
		cErr  *C.char
		cName = C.CString(name)
	)
	defer C.free(unsafe.Pointer(cName))
	db := C.rdb_open_for_read_only(opts.c, cName, boolToChar(errorIfLogFileExist), &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return nil, errors.New(C.GoString(cErr))
	}
	return &DB{
		name: name,
		c:    db,
		opts: opts,
	}, nil
}

// Close closes the database.
func (db *DB) Close() {
	C.rdb_close(db.c)
}

// Get returns the data associated with the key from the database.
func (db *DB) Get(opts *ReadOptions, key []byte) (*Slice, error) {
	var (
		cErr    *C.char
		cValLen C.size_t
		cKey    = byteToChar(key)
	)
	cValue := C.rdb_get(db.c, opts.c, cKey, C.size_t(len(key)), &cValLen, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return nil, errors.New(C.GoString(cErr))
	}
	return NewSlice(cValue, cValLen), nil
}

// GetBytes is like Get but returns a copy of the data.
func (db *DB) GetBytes(opts *ReadOptions, key []byte) ([]byte, error) {
	var (
		cErr    *C.char
		cValLen C.size_t
		cKey    = byteToChar(key)
	)
	cValue := C.rdb_get(db.c, opts.c, cKey, C.size_t(len(key)), &cValLen, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return nil, errors.New(C.GoString(cErr))
	}
	if cValue == nil {
		return nil, nil
	}
	defer C.free(unsafe.Pointer(cValue))
	return C.GoBytes(unsafe.Pointer(cValue), C.int(cValLen)), nil
}

// Put writes data associated with a key to the database.
func (db *DB) Put(opts *WriteOptions, key, value []byte) error {
	var (
		cErr   *C.char
		cKey   = byteToChar(key)
		cValue = byteToChar(value)
	)
	C.rdb_put(db.c, opts.c, cKey, C.size_t(len(key)), cValue, C.size_t(len(value)), &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return errors.New(C.GoString(cErr))
	}
	return nil
}

// Delete removes the data associated with the key from the database.
func (db *DB) Delete(opts *WriteOptions, key []byte) error {
	var (
		cErr *C.char
		cKey = byteToChar(key)
	)
	C.rdb_delete(db.c, opts.c, cKey, C.size_t(len(key)), &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return errors.New(C.GoString(cErr))
	}
	return nil
}

// Write writes a WriteBatch to the database
func (db *DB) Write(opts *WriteOptions, batch *WriteBatch) error {
	var cErr *C.char
	C.rdb_write(db.c, opts.c, batch.c, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return errors.New(C.GoString(cErr))
	}
	return nil
}

// NewIterator returns an Iterator over the the database that uses the
// ReadOptions given.
func (db *DB) NewIterator(opts *ReadOptions) *Iterator {
	cIter := C.rdb_create_iterator(db.c, opts.c)
	return NewNativeIterator(unsafe.Pointer(cIter))
}

// GetProperty returns the value of a database property.
func (db *DB) GetProperty(propName string) string {
	cprop := C.CString(propName)
	defer C.free(unsafe.Pointer(cprop))
	cValue := C.rdb_property_value(db.c, cprop)
	defer C.free(unsafe.Pointer(cValue))
	return C.GoString(cValue)
}

// GetStats returns stats of our data store.
func (db *DB) GetStats() string { return db.GetProperty("rocksdb.stats") }
