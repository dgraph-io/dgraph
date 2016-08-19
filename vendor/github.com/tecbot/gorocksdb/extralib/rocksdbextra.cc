#include <rocksdb/c.h>
#include <rocksdb/cache.h>
#include <rocksdb/table.h>

#include "rocksdbextra.h"

using rocksdb::BlockBasedTableOptions;

// This is not exposed in RocksDB lib. We have to redefine it here.
// Caution: Have to remain consistent with RocksDB definition.
struct rocksdb_block_based_table_options_t  { BlockBasedTableOptions rep; };

extern size_t rocksdb_block_based_options_usage(
  rocksdb_block_based_table_options_t* options) {
  return options->rep.block_cache->GetUsage();
}
