package rdb

// #cgo CXXFLAGS: -std=c++11 -O2
// #cgo LDFLAGS: -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy
// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"

// WriteOptions represent all of the available options when writing to a
// database.
type WriteOptions struct {
	c *C.rocksdb_writeoptions_t
}

// NewDefaultWriteOptions creates a default WriteOptions object.
func NewDefaultWriteOptions() *WriteOptions {
	return NewNativeWriteOptions(C.rocksdb_writeoptions_create())
}

// NewNativeWriteOptions creates a WriteOptions object.
func NewNativeWriteOptions(c *C.rocksdb_writeoptions_t) *WriteOptions {
	return &WriteOptions{c}
}

// SetSync sets the sync mode. If true, the write will be flushed
// from the operating system buffer cache before the write is considered complete.
// If this flag is true, writes will be slower.
// Default: false
func (opts *WriteOptions) SetSync(value bool) {
	C.rocksdb_writeoptions_set_sync(opts.c, boolToChar(value))
}

// Destroy deallocates the WriteOptions object.
func (opts *WriteOptions) Destroy() {
	C.rocksdb_writeoptions_destroy(opts.c)
	opts.c = nil
}
