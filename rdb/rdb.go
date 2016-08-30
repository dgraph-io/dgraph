package rdb

// #cgo LDFLAGS: -ldgrocksdbc -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy
// #include <stdlib.h>
// #include <dgrocksdbc/rocks.h>
import "C"

import (
	"errors"
	"unsafe"
)

// boolToChar converts a bool value to C.uchar.
func boolToChar(b bool) C.uchar {
	if b {
		return 1
	}
	return 0
}

// Options represent all of the available options when opening a database with Open.
type Options struct {
	c *C.rocksdb_options_t
}

// NewDefaultOptions creates the default Options.
func NewDefaultOptions() *Options {
	return &Options{c: C.rocksdb_options_create()}
}

// DB is a reusable handle to a RocksDB database on disk, created by Open.
type DB struct {
	c    *C.rocksdb_t
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
	db := C.rocksdb_open(opts.c, cName, &cErr)
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
	db := C.rocksdb_open_for_read_only(opts.c, cName, boolToChar(errorIfLogFileExist), &cErr)
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
	C.rocksdb_close(db.c)
}
