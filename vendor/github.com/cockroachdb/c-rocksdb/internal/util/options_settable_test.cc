//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.
//
// Copyright (c) 2011 The LevelDB Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file. See the AUTHORS file for names of contributors.

#ifndef __STDC_FORMAT_MACROS
#define __STDC_FORMAT_MACROS
#endif

#include <inttypes.h>
#include <cctype>
#include <cstring>
#include <unordered_map>

#include "rocksdb/cache.h"
#include "rocksdb/convenience.h"
#include "rocksdb/memtablerep.h"
#include "rocksdb/utilities/leveldb_options.h"
#include "util/options_helper.h"
#include "util/options_parser.h"
#include "util/options_sanity_check.h"
#include "util/random.h"
#include "util/stderr_logger.h"
#include "util/testharness.h"
#include "util/testutil.h"

#ifndef GFLAGS
bool FLAGS_enable_print = false;
#else
#include <gflags/gflags.h>
using GFLAGS::ParseCommandLineFlags;
DEFINE_bool(enable_print, false, "Print options generated to console.");
#endif  // GFLAGS

namespace rocksdb {

// Verify options are settable from options strings.
// We take the approach that depends on compiler behavior that copy constructor
// won't touch implicit padding bytes, so that the test is fragile.
// As a result, we only run the tests to verify new fields in options are
// settable through string on limited platforms as it depends on behavior of
// compilers.
#ifndef ROCKSDB_LITE
#ifdef OS_LINUX
#ifndef __clang__

class OptionsSettableTest : public testing::Test {
 public:
  OptionsSettableTest() {}
};

const char kSpecialChar = 'z';
typedef std::vector<std::pair<int, size_t>> OffsetGap;

void FillWithSpecialChar(char* start_ptr, size_t total_size,
                         const OffsetGap& blacklist) {
  size_t offset = 0;
  for (auto& pair : blacklist) {
    std::memset(start_ptr + offset, kSpecialChar, pair.first - offset);
    offset = pair.first + pair.second;
  }
  std::memset(start_ptr + offset, kSpecialChar, total_size - offset);
}

int NumUnsetBytes(char* start_ptr, size_t total_size,
                  const OffsetGap& blacklist) {
  int total_unset_bytes_base = 0;
  size_t offset = 0;
  for (auto& pair : blacklist) {
    for (char* ptr = start_ptr + offset; ptr < start_ptr + pair.first; ptr++) {
      if (*ptr == kSpecialChar) {
        total_unset_bytes_base++;
      }
    }
    offset = pair.first + pair.second;
  }
  for (char* ptr = start_ptr + offset; ptr < start_ptr + total_size; ptr++) {
    if (*ptr == kSpecialChar) {
      total_unset_bytes_base++;
    }
  }
  return total_unset_bytes_base;
}

// If the test fails, likely a new option is added to BlockBasedTableOptions
// but it cannot be set through GetBlockBasedTableOptionsFromString(), or the
// test is not updated accordingly.
// After adding an option, we need to make sure it is settable by
// GetBlockBasedTableOptionsFromString() and add the option to the input string
// passed to the GetBlockBasedTableOptionsFromString() in this test.
// If it is a complicated type, you also need to add the field to
// kBbtoBlacklist, and maybe add customized verification for it.
TEST_F(OptionsSettableTest, BlockBasedTableOptionsAllFieldsSettable) {
  // Items in the form of <offset, size>. Need to be in ascending order
  // and not overlapping. Need to updated if new pointer-option is added.
  const OffsetGap kBbtoBlacklist = {
      {offsetof(struct BlockBasedTableOptions, flush_block_policy_factory),
       sizeof(std::shared_ptr<FlushBlockPolicyFactory>)},
      {offsetof(struct BlockBasedTableOptions, block_cache),
       sizeof(std::shared_ptr<Cache>)},
      {offsetof(struct BlockBasedTableOptions, persistent_cache),
       sizeof(std::shared_ptr<PersistentCache>)},
      {offsetof(struct BlockBasedTableOptions, block_cache_compressed),
       sizeof(std::shared_ptr<Cache>)},
      {offsetof(struct BlockBasedTableOptions, filter_policy),
       sizeof(std::shared_ptr<const FilterPolicy>)},
  };

  // In this test, we catch a new option of BlockBasedTableOptions that is not
  // settable through GetBlockBasedTableOptionsFromString().
  // We count padding bytes of the option struct, and assert it to be the same
  // as unset bytes of an option struct initialized by
  // GetBlockBasedTableOptionsFromString().

  char* bbto_ptr = new char[sizeof(BlockBasedTableOptions)];

  // Count padding bytes by setting all bytes in the memory to a special char,
  // copy a well constructed struct to this memory and see how many special
  // bytes left.
  BlockBasedTableOptions* bbto = new (bbto_ptr) BlockBasedTableOptions();
  FillWithSpecialChar(bbto_ptr, sizeof(BlockBasedTableOptions), kBbtoBlacklist);
  // It based on the behavior of compiler that padding bytes are not changed
  // when copying the struct. It's prone to failure when compiler behavior
  // changes. We verify there is unset bytes to detect the case.
  *bbto = BlockBasedTableOptions();
  int unset_bytes_base =
      NumUnsetBytes(bbto_ptr, sizeof(BlockBasedTableOptions), kBbtoBlacklist);
  ASSERT_GT(unset_bytes_base, 0);
  bbto->~BlockBasedTableOptions();

  // Construct the base option passed into
  // GetBlockBasedTableOptionsFromString().
  bbto = new (bbto_ptr) BlockBasedTableOptions();
  FillWithSpecialChar(bbto_ptr, sizeof(BlockBasedTableOptions), kBbtoBlacklist);
  // This option is not setable:
  bbto->use_delta_encoding = true;

  char* new_bbto_ptr = new char[sizeof(BlockBasedTableOptions)];
  BlockBasedTableOptions* new_bbto =
      new (new_bbto_ptr) BlockBasedTableOptions();
  FillWithSpecialChar(new_bbto_ptr, sizeof(BlockBasedTableOptions),
                      kBbtoBlacklist);

  // Need to update the option string if a new option is added.
  ASSERT_OK(GetBlockBasedTableOptionsFromString(
      *bbto,
      "cache_index_and_filter_blocks=1;"
      "pin_l0_filter_and_index_blocks_in_cache=1;"
      "index_type=kHashSearch;"
      "checksum=kxxHash;hash_index_allow_collision=1;no_block_cache=1;"
      "block_cache=1M;block_cache_compressed=1k;block_size=1024;"
      "block_size_deviation=8;block_restart_interval=4; "
      "index_block_restart_interval=4;"
      "filter_policy=bloomfilter:4:true;whole_key_filtering=1;"
      "skip_table_builder_flush=1;format_version=1;"
      "hash_index_allow_collision=false;",
      new_bbto));

  ASSERT_EQ(unset_bytes_base,
            NumUnsetBytes(new_bbto_ptr, sizeof(BlockBasedTableOptions),
                          kBbtoBlacklist));

  ASSERT_TRUE(new_bbto->block_cache.get() != nullptr);
  ASSERT_TRUE(new_bbto->block_cache_compressed.get() != nullptr);
  ASSERT_TRUE(new_bbto->filter_policy.get() != nullptr);

  bbto->~BlockBasedTableOptions();
  new_bbto->~BlockBasedTableOptions();

  delete[] bbto_ptr;
  delete[] new_bbto_ptr;
}

// If the test fails, likely a new option is added to DBOptions
// but it cannot be set through GetDBOptionsFromString(), or the test is not
// updated accordingly.
// After adding an option, we need to make sure it is settable by
// GetDBOptionsFromString() and add the option to the input string passed to
// DBOptionsFromString()in this test.
// If it is a complicated type, you also need to add the field to
// kDBOptionsBlacklist, and maybe add customized verification for it.
TEST_F(OptionsSettableTest, DBOptionsAllFieldsSettable) {
  const OffsetGap kDBOptionsBlacklist = {
      {offsetof(struct DBOptions, env), sizeof(Env*)},
      {offsetof(struct DBOptions, rate_limiter),
       sizeof(std::shared_ptr<RateLimiter>)},
      {offsetof(struct DBOptions, sst_file_manager),
       sizeof(std::shared_ptr<SstFileManager>)},
      {offsetof(struct DBOptions, info_log), sizeof(std::shared_ptr<Logger>)},
      {offsetof(struct DBOptions, statistics),
       sizeof(std::shared_ptr<Statistics>)},
      {offsetof(struct DBOptions, db_paths), sizeof(std::vector<DbPath>)},
      {offsetof(struct DBOptions, db_log_dir), sizeof(std::string)},
      {offsetof(struct DBOptions, wal_dir), sizeof(std::string)},
      {offsetof(struct DBOptions, listeners),
       sizeof(std::vector<std::shared_ptr<EventListener>>)},
      {offsetof(struct DBOptions, row_cache), sizeof(std::shared_ptr<Cache>)},
      {offsetof(struct DBOptions, wal_filter), sizeof(const WalFilter*)},
  };

  char* options_ptr = new char[sizeof(DBOptions)];

  // Count padding bytes by setting all bytes in the memory to a special char,
  // copy a well constructed struct to this memory and see how many special
  // bytes left.
  DBOptions* options = new (options_ptr) DBOptions();
  FillWithSpecialChar(options_ptr, sizeof(DBOptions), kDBOptionsBlacklist);
  // It based on the behavior of compiler that padding bytes are not changed
  // when copying the struct. It's prone to failure when compiler behavior
  // changes. We verify there is unset bytes to detect the case.
  *options = DBOptions();
  int unset_bytes_base =
      NumUnsetBytes(options_ptr, sizeof(DBOptions), kDBOptionsBlacklist);
  ASSERT_GT(unset_bytes_base, 0);
  options->~DBOptions();

  options = new (options_ptr) DBOptions();
  FillWithSpecialChar(options_ptr, sizeof(DBOptions), kDBOptionsBlacklist);

  char* new_options_ptr = new char[sizeof(DBOptions)];
  DBOptions* new_options = new (new_options_ptr) DBOptions();
  FillWithSpecialChar(new_options_ptr, sizeof(DBOptions), kDBOptionsBlacklist);

  // Need to update the option string if a new option is added.
  ASSERT_OK(
      GetDBOptionsFromString(*options,
                             "wal_bytes_per_sync=4295048118;"
                             "delete_obsolete_files_period_micros=4294967758;"
                             "WAL_ttl_seconds=4295008036;"
                             "WAL_size_limit_MB=4295036161;"
                             "wal_dir=path/to/wal_dir;"
                             "db_write_buffer_size=2587;"
                             "max_subcompactions=64330;"
                             "table_cache_numshardbits=28;"
                             "max_open_files=72;"
                             "max_file_opening_threads=35;"
                             "base_background_compactions=3;"
                             "max_background_compactions=33;"
                             "use_fsync=true;"
                             "use_adaptive_mutex=false;"
                             "max_total_wal_size=4295005604;"
                             "compaction_readahead_size=0;"
                             "new_table_reader_for_compaction_inputs=false;"
                             "keep_log_file_num=4890;"
                             "skip_stats_update_on_db_open=false;"
                             "max_manifest_file_size=4295009941;"
                             "db_log_dir=path/to/db_log_dir;"
                             "skip_log_error_on_recovery=true;"
                             "writable_file_max_buffer_size=1048576;"
                             "paranoid_checks=true;"
                             "is_fd_close_on_exec=false;"
                             "bytes_per_sync=4295013613;"
                             "enable_thread_tracking=false;"
                             "disable_data_sync=false;"
                             "recycle_log_file_num=0;"
                             "disableDataSync=false;"
                             "create_missing_column_families=true;"
                             "log_file_time_to_roll=3097;"
                             "max_background_flushes=35;"
                             "create_if_missing=false;"
                             "error_if_exists=true;"
                             "allow_os_buffer=false;"
                             "delayed_write_rate=4294976214;"
                             "manifest_preallocation_size=1222;"
                             "allow_mmap_writes=false;"
                             "stats_dump_period_sec=70127;"
                             "allow_fallocate=true;"
                             "allow_mmap_reads=false;"
                             "max_log_file_size=4607;"
                             "random_access_max_buffer_size=1048576;"
                             "advise_random_on_open=true;"
                             "fail_if_options_file_error=false;"
                             "allow_concurrent_memtable_write=true;"
                             "wal_recovery_mode=kPointInTimeRecovery;"
                             "enable_write_thread_adaptive_yield=true;"
                             "write_thread_slow_yield_usec=5;"
                             "write_thread_max_yield_usec=1000;"
                             "access_hint_on_compaction_start=NONE;"
                             "info_log_level=DEBUG_LEVEL;"
                             "dump_malloc_stats=false;"
                             "allow_2pc=false;",
                             new_options));

  ASSERT_EQ(unset_bytes_base, NumUnsetBytes(new_options_ptr, sizeof(DBOptions),
                                            kDBOptionsBlacklist));

  options->~DBOptions();
  new_options->~DBOptions();

  delete[] options_ptr;
  delete[] new_options_ptr;
}

// If the test fails, likely a new option is added to ColumnFamilyOptions
// but it cannot be set through GetColumnFamilyOptionsFromString(), or the
// test is not updated accordingly.
// After adding an option, we need to make sure it is settable by
// GetColumnFamilyOptionsFromString() and add the option to the input
// string passed to GetColumnFamilyOptionsFromString()in this test.
// If it is a complicated type, you also need to add the field to
// kColumnFamilyOptionsBlacklist, and maybe add customized verification
// for it.
TEST_F(OptionsSettableTest, ColumnFamilyOptionsAllFieldsSettable) {
  const OffsetGap kColumnFamilyOptionsBlacklist = {
      {offsetof(struct ColumnFamilyOptions, comparator), sizeof(Comparator*)},
      {offsetof(struct ColumnFamilyOptions, merge_operator),
       sizeof(std::shared_ptr<MergeOperator>)},
      {offsetof(struct ColumnFamilyOptions, compaction_filter),
       sizeof(const CompactionFilter*)},
      {offsetof(struct ColumnFamilyOptions, compaction_filter_factory),
       sizeof(std::shared_ptr<CompactionFilterFactory>)},
      {offsetof(struct ColumnFamilyOptions, compression_per_level),
       sizeof(std::vector<CompressionType>)},
      {offsetof(struct ColumnFamilyOptions, prefix_extractor),
       sizeof(std::shared_ptr<const SliceTransform>)},
      {offsetof(struct ColumnFamilyOptions,
                max_bytes_for_level_multiplier_additional),
       sizeof(std::vector<int>)},
      {offsetof(struct ColumnFamilyOptions, memtable_factory),
       sizeof(std::shared_ptr<MemTableRepFactory>)},
      {offsetof(struct ColumnFamilyOptions, table_factory),
       sizeof(std::shared_ptr<TableFactory>)},
      {offsetof(struct ColumnFamilyOptions,
                table_properties_collector_factories),
       sizeof(ColumnFamilyOptions::TablePropertiesCollectorFactories)},
      {offsetof(struct ColumnFamilyOptions, inplace_callback),
       sizeof(UpdateStatus(*)(char*, uint32_t*, Slice, std::string*))},
  };

  char* options_ptr = new char[sizeof(ColumnFamilyOptions)];

  // Count padding bytes by setting all bytes in the memory to a special char,
  // copy a well constructed struct to this memory and see how many special
  // bytes left.
  ColumnFamilyOptions* options = new (options_ptr) ColumnFamilyOptions();
  FillWithSpecialChar(options_ptr, sizeof(ColumnFamilyOptions),
                      kColumnFamilyOptionsBlacklist);
  // It based on the behavior of compiler that padding bytes are not changed
  // when copying the struct. It's prone to failure when compiler behavior
  // changes. We verify there is unset bytes to detect the case.
  *options = ColumnFamilyOptions();

  // Deprecatd option which is not initialized. Need to set it to avoid
  // Valgrind error
  options->max_mem_compaction_level = 0;

  int unset_bytes_base = NumUnsetBytes(options_ptr, sizeof(ColumnFamilyOptions),
                                       kColumnFamilyOptionsBlacklist);
  ASSERT_GT(unset_bytes_base, 0);
  options->~ColumnFamilyOptions();

  options = new (options_ptr) ColumnFamilyOptions();
  FillWithSpecialChar(options_ptr, sizeof(ColumnFamilyOptions),
                      kColumnFamilyOptionsBlacklist);

  // Following options are not settable through
  // GetColumnFamilyOptionsFromString():
  options->rate_limit_delay_max_milliseconds = 33;
  options->compaction_pri = CompactionPri::kOldestSmallestSeqFirst;
  options->compaction_options_universal = CompactionOptionsUniversal();
  options->compression_opts = CompressionOptions();
  options->hard_rate_limit = 0;
  options->soft_rate_limit = 0;
  options->compaction_options_fifo = CompactionOptionsFIFO();
  options->max_mem_compaction_level = 0;

  char* new_options_ptr = new char[sizeof(ColumnFamilyOptions)];
  ColumnFamilyOptions* new_options =
      new (new_options_ptr) ColumnFamilyOptions();
  FillWithSpecialChar(new_options_ptr, sizeof(ColumnFamilyOptions),
                      kColumnFamilyOptionsBlacklist);

  // Need to update the option string if a new option is added.
  ASSERT_OK(GetColumnFamilyOptionsFromString(
      *options,
      "compaction_filter_factory=mpudlojcujCompactionFilterFactory;"
      "table_factory=PlainTable;"
      "prefix_extractor=rocksdb.CappedPrefix.13;"
      "comparator=leveldb.BytewiseComparator;"
      "compression_per_level=kBZip2Compression:kBZip2Compression:"
      "kBZip2Compression:kNoCompression:kZlibCompression:kBZip2Compression:"
      "kSnappyCompression;"
      "max_bytes_for_level_base=986;"
      "bloom_locality=8016;"
      "target_file_size_base=4294976376;"
      "memtable_prefix_bloom_huge_page_tlb_size=2557;"
      "max_successive_merges=5497;"
      "max_sequential_skip_in_iterations=4294971408;"
      "arena_block_size=1893;"
      "target_file_size_multiplier=35;"
      "source_compaction_factor=54;"
      "min_write_buffer_number_to_merge=9;"
      "max_write_buffer_number=84;"
      "write_buffer_size=1653;"
      "max_grandparent_overlap_factor=64;"
      "max_bytes_for_level_multiplier=60;"
      "memtable_factory=SkipListFactory;"
      "compression=kNoCompression;"
      "bottommost_compression=kDisableCompressionOption;"
      "min_partial_merge_operands=7576;"
      "level0_stop_writes_trigger=33;"
      "num_levels=99;"
      "level0_slowdown_writes_trigger=22;"
      "level0_file_num_compaction_trigger=14;"
      "expanded_compaction_factor=34;"
      "compaction_filter=urxcqstuwnCompactionFilter;"
      "soft_rate_limit=530.615385;"
      "soft_pending_compaction_bytes_limit=0;"
      "max_write_buffer_number_to_maintain=84;"
      "verify_checksums_in_compaction=false;"
      "merge_operator=aabcxehazrMergeOperator;"
      "memtable_prefix_bloom_bits=4642;"
      "paranoid_file_checks=true;"
      "inplace_update_num_locks=7429;"
      "optimize_filters_for_hits=false;"
      "level_compaction_dynamic_level_bytes=false;"
      "inplace_update_support=false;"
      "compaction_style=kCompactionStyleFIFO;"
      "memtable_prefix_bloom_probes=2511;"
      "purge_redundant_kvs_while_flush=true;"
      "filter_deletes=false;"
      "hard_pending_compaction_bytes_limit=0;"
      "disable_auto_compactions=false;"
      "report_bg_io_stats=true;",
      new_options));

  ASSERT_EQ(unset_bytes_base,
            NumUnsetBytes(new_options_ptr, sizeof(ColumnFamilyOptions),
                          kColumnFamilyOptionsBlacklist));

  options->~ColumnFamilyOptions();
  new_options->~ColumnFamilyOptions();

  delete[] options_ptr;
  delete[] new_options_ptr;
}
#endif  // !__clang__
#endif  // OS_LINUX
#endif  // !ROCKSDB_LITE

}  // namespace rocksdb

int main(int argc, char** argv) {
  ::testing::InitGoogleTest(&argc, argv);
#ifdef GFLAGS
  ParseCommandLineFlags(&argc, &argv, true);
#endif  // GFLAGS
  return RUN_ALL_TESTS();
}
