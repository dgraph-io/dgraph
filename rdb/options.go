package rdb

// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"

// Options represent all of the available options when opening a database with Open.
type Options struct {
	c    *C.rdb_options_t
	bbto *BlockBasedTableOptions
}

// NewDefaultOptions creates the default Options.
func NewDefaultOptions() *Options {
	return NewNativeOptions(C.rdb_options_create())
}

// NewNativeOptions creates a Options object.
func NewNativeOptions(c *C.rdb_options_t) *Options {
	return &Options{c: c}
}

// SetCreateIfMissing specifies whether the database
// should be created if it is missing.
// Default: false
func (opts *Options) SetCreateIfMissing(value bool) {
	C.rdb_options_set_create_if_missing(opts.c, boolToChar(value))
}

// SetBlockBasedTableFactory sets the block based table factory.
func (opts *Options) SetBlockBasedTableFactory(value *BlockBasedTableOptions) {
	opts.bbto = value
	C.rdb_options_set_block_based_table_factory(opts.c, value.c)
}
