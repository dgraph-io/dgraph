//  Copyright (c) 2013, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.
//
// Copyright (c) 2011 The LevelDB Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file. See the AUTHORS file for names of contributors.
#pragma once

#ifndef ROCKSDB_LITE

#include <functional>
#include <limits>
#include <list>
#include <memory>
#include <string>
#include <thread>
#include <vector>

#include "db/db_test_util.h"
#include "rocksdb/cache.h"
#include "table/block_builder.h"
#include "util/arena.h"
#include "util/testharness.h"
#include "utilities/persistent_cache/volatile_tier_impl.h"

namespace rocksdb {

//
// Unit tests for testing PersistentCacheTier
//
class PersistentCacheTierTest : public testing::Test {
 public:
  explicit PersistentCacheTierTest()
      : path_(test::TmpDir(Env::Default()) + "/cache_test") {}

  virtual ~PersistentCacheTierTest() {
    if (cache_) {
      Status s = cache_->Close();
      assert(s.ok());
    }
  }

 protected:
  // Flush cache
  void Flush() {
    if (cache_) {
      cache_->Flush();
    }
  }

  // create threaded workload
  template <class T>
  std::list<std::thread> SpawnThreads(const size_t n, const T& fn) {
    std::list<std::thread> threads;
    for (size_t i = 0; i < n; i++) {
      std::thread th(fn);
      threads.push_back(std::move(th));
    }
    return threads;
  }

  // Wait for threads to join
  void Join(std::list<std::thread>&& threads) {
    for (auto& th : threads) {
      th.join();
    }
    threads.clear();
  }

  // Run insert workload in threads
  void Insert(const size_t nthreads, const size_t max_keys) {
    key_ = 0;
    max_keys_ = max_keys;
    // spawn threads
    auto fn = std::bind(&PersistentCacheTierTest::InsertImpl, this);
    auto threads = SpawnThreads(nthreads, fn);
    // join with threads
    Join(std::move(threads));
    // Flush cache
    Flush();
  }

  // Run verification on the cache
  void Verify(const size_t nthreads = 1, const bool eviction_enabled = false) {
    stats_verify_hits_ = 0;
    stats_verify_missed_ = 0;
    key_ = 0;
    // spawn threads
    auto fn =
        std::bind(&PersistentCacheTierTest::VerifyImpl, this, eviction_enabled);
    auto threads = SpawnThreads(nthreads, fn);
    // join with threads
    Join(std::move(threads));
  }

  // pad 0 to numbers
  std::string PaddedNumber(const size_t data, const size_t pad_size) {
    assert(pad_size);
    char* ret = new char[pad_size];
    int pos = static_cast<int>(pad_size) - 1;
    size_t count = 0;
    size_t t = data;
    // copy numbers
    while (t) {
      count++;
      ret[pos--] = '0' + t % 10;
      t = t / 10;
    }
    // copy 0s
    while (pos >= 0) {
      ret[pos--] = '0';
    }
    // post condition
    assert(count <= pad_size);
    assert(pos == -1);
    std::string result(ret, pad_size);
    delete[] ret;
    return result;
  }

  // Insert workload implementation
  void InsertImpl() {
    const std::string prefix = "key_prefix_";

    while (true) {
      size_t i = key_++;
      if (i >= max_keys_) {
        break;
      }

      char data[4 * 1024];
      memset(data, '0' + (i % 10), sizeof(data));
      auto k = prefix + PaddedNumber(i, /*count=*/8);
      Slice key(k);
      while (!cache_->Insert(key, data, sizeof(data)).ok()) {
        Env::Default()->SleepForMicroseconds(1 * 1000 * 1000);
      }
    }
  }

  // Verification implementation
  void VerifyImpl(const bool eviction_enabled = false) {
    const std::string prefix = "key_prefix_";
    while (true) {
      size_t i = key_++;
      if (i >= max_keys_) {
        break;
      }

      char edata[4 * 1024];
      memset(edata, '0' + (i % 10), sizeof(edata));
      auto k = prefix + PaddedNumber(i, /*count=*/8);
      Slice key(k);
      unique_ptr<char[]> block;
      size_t block_size;

      if (eviction_enabled) {
        if (!cache_->Lookup(key, &block, &block_size).ok()) {
          // assume that the key is evicted
          stats_verify_missed_++;
          continue;
        }
      }

      ASSERT_OK(cache_->Lookup(key, &block, &block_size));
      ASSERT_EQ(block_size, sizeof(edata));
      ASSERT_EQ(memcmp(edata, block.get(), sizeof(edata)), 0);
      stats_verify_hits_++;
    }
  }

  // template for insert test
  void RunInsertTest(const size_t nthreads, const size_t max_keys) {
    Insert(nthreads, max_keys);
    Verify(nthreads);
    ASSERT_EQ(stats_verify_hits_, max_keys);
    ASSERT_EQ(stats_verify_missed_, 0);

    cache_->Close();
    cache_.reset();
  }

  // template for insert with eviction test
  void RunInsertTestWithEviction(const size_t nthreads, const size_t max_keys) {
    Insert(nthreads, max_keys);
    Verify(nthreads, /*eviction_enabled=*/true);
    ASSERT_EQ(stats_verify_hits_ + stats_verify_missed_, max_keys);
    ASSERT_GT(stats_verify_hits_, 0);
    ASSERT_GT(stats_verify_missed_, 0);

    cache_->Close();
    cache_.reset();
  }

  const std::string path_;
  shared_ptr<Logger> log_;
  std::shared_ptr<PersistentCacheTier> cache_;
  std::atomic<size_t> key_{0};
  size_t max_keys_ = 0;
  std::atomic<size_t> stats_verify_hits_{0};
  std::atomic<size_t> stats_verify_missed_{0};
};

//
// RocksDB tests
//
class PersistentCacheDBTest : public DBTestBase {
 public:
  PersistentCacheDBTest() : DBTestBase("/cache_test") {
    rocksdb::SyncPoint::GetInstance()->EnableProcessing();
    rocksdb::SyncPoint::GetInstance()->SetCallBack(
        "GetUniqueIdFromFile:FS_IOC_GETVERSION",
        PersistentCacheDBTest::UniqueIdCallback);
  }

  static void UniqueIdCallback(void* arg) {
    int* result = reinterpret_cast<int*>(arg);
    if (*result == -1) {
      *result = 0;
    }

    rocksdb::SyncPoint::GetInstance()->ClearTrace();
    rocksdb::SyncPoint::GetInstance()->SetCallBack(
        "GetUniqueIdFromFile:FS_IOC_GETVERSION", UniqueIdCallback);
  }

  std::shared_ptr<PersistentCacheTier> MakeVolatileCache() {
    return std::make_shared<VolatileCacheTier>();
  }

  static uint64_t TestGetTickerCount(const Options& options,
                                     Tickers ticker_type) {
    return static_cast<uint32_t>(
        options.statistics->getTickerCount(ticker_type));
  }

  // insert data to table
  void Insert(const Options& options,
              const BlockBasedTableOptions& table_options, const int num_iter,
              std::vector<std::string>* values) {
    CreateAndReopenWithCF({"pikachu"}, options);
    // default column family doesn't have block cache
    Options no_block_cache_opts;
    no_block_cache_opts.statistics = options.statistics;
    no_block_cache_opts = CurrentOptions(no_block_cache_opts);
    BlockBasedTableOptions table_options_no_bc;
    table_options_no_bc.no_block_cache = true;
    no_block_cache_opts.table_factory.reset(
        NewBlockBasedTableFactory(table_options_no_bc));
    ReopenWithColumnFamilies(
        {"default", "pikachu"},
        std::vector<Options>({no_block_cache_opts, options}));

    Random rnd(301);

    // Write 8MB (80 values, each 100K)
    ASSERT_EQ(NumTableFilesAtLevel(0, 1), 0);
    std::string str;
    for (int i = 0; i < num_iter; i++) {
      if (i % 4 == 0) {  // high compression ratio
        str = RandomString(&rnd, 1000);
      }
      values->push_back(str);
      ASSERT_OK(Put(1, Key(i), (*values)[i]));
    }

    // flush all data from memtable so that reads are from block cache
    ASSERT_OK(Flush(1));
  }

  // verify data
  void Verify(const int num_iter, const std::vector<std::string>& values) {
    for (int j = 0; j < 2; ++j) {
      for (int i = 0; i < num_iter; i++) {
        ASSERT_EQ(Get(1, Key(i)), values[i]);
      }
    }
  }

  // test template
  void RunTest(const std::function<std::shared_ptr<PersistentCacheTier>(bool)>&
                   new_pcache) {
    if (!Snappy_Supported()) {
      return;
    }

    // number of insertion interations
    int num_iter = 100 * 1024;

    for (int iter = 0; iter < 5; iter++) {
      Options options;
      options.write_buffer_size = 64 * 1024;  // small write buffer
      options.statistics = rocksdb::CreateDBStatistics();
      options = CurrentOptions(options);

      // setup page cache
      std::shared_ptr<PersistentCacheTier> pcache;
      BlockBasedTableOptions table_options;
      table_options.cache_index_and_filter_blocks = true;

      const uint64_t uint64_max = std::numeric_limits<uint64_t>::max();

      switch (iter) {
        case 0:
          // page cache, block cache, no-compressed cache
          pcache = new_pcache(/*is_compressed=*/true);
          table_options.persistent_cache = pcache;
          table_options.block_cache = NewLRUCache(uint64_max);
          table_options.block_cache_compressed = nullptr;
          options.table_factory.reset(NewBlockBasedTableFactory(table_options));
          break;
        case 1:
          // page cache, block cache, compressed cache
          pcache = new_pcache(/*is_compressed=*/true);
          table_options.persistent_cache = pcache;
          table_options.block_cache = NewLRUCache(uint64_max);
          table_options.block_cache_compressed = NewLRUCache(uint64_max);
          options.table_factory.reset(NewBlockBasedTableFactory(table_options));
          break;
        case 2:
          // page cache, block cache, compressed cache + KNoCompression
          // both block cache and compressed cache, but DB is not compressed
          // also, make block cache sizes bigger, to trigger block cache hits
          pcache = new_pcache(/*is_compressed=*/true);
          table_options.persistent_cache = pcache;
          table_options.block_cache = NewLRUCache(uint64_max);
          table_options.block_cache_compressed = NewLRUCache(uint64_max);
          options.table_factory.reset(NewBlockBasedTableFactory(table_options));
          options.compression = kNoCompression;
          break;
        case 3:
          // page cache, no block cache, no compressed cache
          pcache = new_pcache(/*is_compressed=*/false);
          table_options.persistent_cache = pcache;
          table_options.block_cache = nullptr;
          table_options.block_cache_compressed = nullptr;
          options.table_factory.reset(NewBlockBasedTableFactory(table_options));
          break;
        case 4:
          // page cache, no block cache, no compressed cache
          // Page cache caches compressed blocks
          pcache = new_pcache(/*is_compressed=*/true);
          table_options.persistent_cache = pcache;
          table_options.block_cache = nullptr;
          table_options.block_cache_compressed = nullptr;
          options.table_factory.reset(NewBlockBasedTableFactory(table_options));
          break;
        default:
          ASSERT_TRUE(false);
      }

      std::vector<std::string> values;
      // insert data
      Insert(options, table_options, num_iter, &values);
      // flush all data in cache to device
      pcache->Flush();
      // verify data
      Verify(num_iter, values);

      auto block_miss = TestGetTickerCount(options, BLOCK_CACHE_MISS);
      auto compressed_block_hit =
          TestGetTickerCount(options, BLOCK_CACHE_COMPRESSED_HIT);
      auto compressed_block_miss =
          TestGetTickerCount(options, BLOCK_CACHE_COMPRESSED_MISS);
      auto page_hit = TestGetTickerCount(options, PERSISTENT_CACHE_HIT);
      auto page_miss = TestGetTickerCount(options, PERSISTENT_CACHE_MISS);

      // check that we triggered the appropriate code paths in the cache
      switch (iter) {
        case 0:
          // page cache, block cache, no-compressed cache
          ASSERT_GT(page_miss, 0);
          ASSERT_GT(page_hit, 0);
          ASSERT_GT(block_miss, 0);
          ASSERT_EQ(compressed_block_miss, 0);
          ASSERT_EQ(compressed_block_hit, 0);
          break;
        case 1:
          // page cache, block cache, compressed cache
          ASSERT_GT(page_miss, 0);
          ASSERT_GT(block_miss, 0);
          ASSERT_GT(compressed_block_miss, 0);
          break;
        case 2:
          // page cache, block cache, compressed cache + KNoCompression
          ASSERT_GT(page_miss, 0);
          ASSERT_GT(page_hit, 0);
          ASSERT_GT(block_miss, 0);
          ASSERT_GT(compressed_block_miss, 0);
          // remember kNoCompression
          ASSERT_EQ(compressed_block_hit, 0);
          break;
        case 3:
        case 4:
          // page cache, no block cache, no compressed cache
          ASSERT_GT(page_miss, 0);
          ASSERT_GT(page_hit, 0);
          ASSERT_EQ(compressed_block_hit, 0);
          ASSERT_EQ(compressed_block_miss, 0);
          break;
        default:
          ASSERT_TRUE(false);
      }

      options.create_if_missing = true;
      DestroyAndReopen(options);

      pcache->Close();
    }
  }
};

}  // namespace rocksdb

#endif
