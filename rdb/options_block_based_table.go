package rdb

// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"

// BlockBasedTableOptions represents block-based table options.
type BlockBasedTableOptions struct {
	c *C.rdb_block_based_table_options_t

	// Hold references for GC.
	cache     *Cache
	compCache *Cache

	// We keep these so we can free their memory in Destroy.
	cFp *C.rdb_filterpolicy_t
}

// NewDefaultBlockBasedTableOptions creates a default BlockBasedTableOptions object.
func NewDefaultBlockBasedTableOptions() *BlockBasedTableOptions {
	return NewNativeBlockBasedTableOptions(C.rdb_block_based_options_create())
}

// NewNativeBlockBasedTableOptions creates a BlockBasedTableOptions object.
func NewNativeBlockBasedTableOptions(c *C.rdb_block_based_table_options_t) *BlockBasedTableOptions {
	return &BlockBasedTableOptions{c: c}
}

// Destroy deallocates the BlockBasedTableOptions object.
func (opts *BlockBasedTableOptions) Destroy() {
	C.rdb_block_based_options_destroy(opts.c)
	opts.c = nil
	opts.cache = nil
	opts.compCache = nil
}

// SetBlockSize sets the approximate size of user data packed per block.
// Note that the block size specified here corresponds opts uncompressed data.
// The actual size of the unit read from disk may be smaller if
// compression is enabled. This parameter can be changed dynamically.
// Default: 4K
func (opts *BlockBasedTableOptions) SetBlockSize(blockSize int) {
	C.rdb_block_based_options_set_block_size(opts.c, C.size_t(blockSize))
}

// SetFilterPolicy sets the filter policy opts reduce disk reads.
// Many applications will benefit from passing the result of
// NewBloomFilterPolicy() here.
// Default: nil
func (opts *BlockBasedTableOptions) SetFilterPolicy(fp FilterPolicy) {
	if nfp, ok := fp.(nativeFilterPolicy); ok {
		opts.cFp = nfp.c
	} else {
		idx := registerFilterPolicy(fp)
		opts.cFp = C.rdbc_filterpolicy_create(C.uintptr_t(idx))
	}
	C.rdb_block_based_options_set_filter_policy(opts.c, opts.cFp)
}

// SetNoBlockCache specify whether block cache should be used or not.
// Default: false
func (opts *BlockBasedTableOptions) SetNoBlockCache(value bool) {
	C.rdb_block_based_options_set_no_block_cache(opts.c, boolToChar(value))
}

// SetBlockCache sets the control over blocks (user data is stored in a set of blocks, and
// a block is the unit of reading from disk).
//
// If set, use the specified cache for blocks.
// If nil, rocksdb will auoptsmatically create and use an 8MB internal cache.
// Default: nil
func (opts *BlockBasedTableOptions) SetBlockCache(cache *Cache) {
	opts.cache = cache
	C.rdb_block_based_options_set_block_cache(opts.c, cache.c)
}

// SetBlockCacheCompressed sets the cache for compressed blocks.
// If nil, rocksdb will not use a compressed block cache.
// Default: nil
func (opts *BlockBasedTableOptions) SetBlockCacheCompressed(cache *Cache) {
	opts.compCache = cache
	C.rdb_block_based_options_set_block_cache_compressed(opts.c, cache.c)
}

// SetWholeKeyFiltering specify if whole keys in the filter (not just prefixes)
// should be placed.
// This must generally be true for gets opts be efficient.
// Default: true
func (opts *BlockBasedTableOptions) SetWholeKeyFiltering(value bool) {
	C.rdb_block_based_options_set_whole_key_filtering(opts.c, boolToChar(value))
}
