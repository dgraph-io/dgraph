package rdb

// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"

// ReadOptions represent all of the available options when reading from a
// database.
type ReadOptions struct {
	c *C.rdb_readoptions_t
}

// NewDefaultReadOptions creates a default ReadOptions object.
func NewDefaultReadOptions() *ReadOptions {
	return NewNativeReadOptions(C.rdb_readoptions_create())
}

// NewNativeReadOptions creates a ReadOptions object.
func NewNativeReadOptions(c *C.rdb_readoptions_t) *ReadOptions {
	return &ReadOptions{c}
}

// Destroy deallocates the ReadOptions object.
func (opts *ReadOptions) Destroy() {
	C.rdb_readoptions_destroy(opts.c)
	opts.c = nil
}

// SetFillCache specify whether the "data block"/"index block"/"filter block"
// read for this iteration should be cached in memory?
// Callers may wish to set this field to false for bulk scans.
// Default: true
func (opts *ReadOptions) SetFillCache(value bool) {
	C.rdb_readoptions_set_fill_cache(opts.c, boolToChar(value))
}
