// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.
//
// Copyright (c) 2011 The LevelDB Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file. See the AUTHORS file for names of contributors.

#pragma once
#ifndef __STDC_FORMAT_MACROS
#define __STDC_FORMAT_MACROS
#endif

#include <fcntl.h>
#include <inttypes.h>
#ifndef OS_WIN
#include <unistd.h>
#endif

#include <algorithm>
#include <map>
#include <set>
#include <string>
#include <thread>
#include <unordered_set>
#include <utility>
#include <vector>

#include "db/db_impl.h"
#include "db/dbformat.h"
#include "db/filename.h"
#include "memtable/hash_linklist_rep.h"
#include "rocksdb/cache.h"
#include "rocksdb/compaction_filter.h"
#include "rocksdb/convenience.h"
#include "rocksdb/db.h"
#include "rocksdb/env.h"
#include "rocksdb/filter_policy.h"
#include "rocksdb/options.h"
#include "rocksdb/slice.h"
#include "rocksdb/statistics.h"
#include "rocksdb/table.h"
#include "rocksdb/utilities/checkpoint.h"
#include "table/block_based_table_factory.h"
#include "table/mock_table.h"
#include "table/plain_table_factory.h"
#include "table/scoped_arena_iterator.h"
#include "util/compression.h"
#include "util/mock_env.h"
#include "util/mutexlock.h"

#include "util/string_util.h"
// SyncPoint is not supported in Released Windows Mode.
#if !(defined NDEBUG) || !defined(OS_WIN)
#include "util/sync_point.h"
#endif  // !(defined NDEBUG) || !defined(OS_WIN)
#include "util/testharness.h"
#include "util/testutil.h"
#include "util/xfunc.h"
#include "utilities/merge_operators.h"

namespace rocksdb {

namespace anon {
class AtomicCounter {
 public:
  explicit AtomicCounter(Env* env = NULL)
      : env_(env), cond_count_(&mu_), count_(0) {}

  void Increment() {
    MutexLock l(&mu_);
    count_++;
    cond_count_.SignalAll();
  }

  int Read() {
    MutexLock l(&mu_);
    return count_;
  }

  bool WaitFor(int count) {
    MutexLock l(&mu_);

    uint64_t start = env_->NowMicros();
    while (count_ < count) {
      uint64_t now = env_->NowMicros();
      cond_count_.TimedWait(now + /*1s*/ 1 * 1000 * 1000);
      if (env_->NowMicros() - start > /*10s*/ 10 * 1000 * 1000) {
        return false;
      }
      if (count_ < count) {
        GTEST_LOG_(WARNING) << "WaitFor is taking more time than usual";
      }
    }

    return true;
  }

  void Reset() {
    MutexLock l(&mu_);
    count_ = 0;
    cond_count_.SignalAll();
  }

 private:
  Env* env_;
  port::Mutex mu_;
  port::CondVar cond_count_;
  int count_;
};

struct OptionsOverride {
  std::shared_ptr<const FilterPolicy> filter_policy = nullptr;

  // Used as a bit mask of individual enums in which to skip an XF test point
  int skip_policy = 0;
};

}  // namespace anon

// A hacky skip list mem table that triggers flush after number of entries.
class SpecialMemTableRep : public MemTableRep {
 public:
  explicit SpecialMemTableRep(MemTableAllocator* allocator,
                              MemTableRep* memtable, int num_entries_flush)
      : MemTableRep(allocator),
        memtable_(memtable),
        num_entries_flush_(num_entries_flush),
        num_entries_(0) {}

  virtual KeyHandle Allocate(const size_t len, char** buf) override {
    return memtable_->Allocate(len, buf);
  }

  // Insert key into the list.
  // REQUIRES: nothing that compares equal to key is currently in the list.
  virtual void Insert(KeyHandle handle) override {
    memtable_->Insert(handle);
    num_entries_++;
  }

  // Returns true iff an entry that compares equal to key is in the list.
  virtual bool Contains(const char* key) const override {
    return memtable_->Contains(key);
  }

  virtual size_t ApproximateMemoryUsage() override {
    // Return a high memory usage when number of entries exceeds the threshold
    // to trigger a flush.
    return (num_entries_ < num_entries_flush_) ? 0 : 1024 * 1024 * 1024;
  }

  virtual void Get(const LookupKey& k, void* callback_args,
                   bool (*callback_func)(void* arg,
                                         const char* entry)) override {
    memtable_->Get(k, callback_args, callback_func);
  }

  uint64_t ApproximateNumEntries(const Slice& start_ikey,
                                 const Slice& end_ikey) override {
    return memtable_->ApproximateNumEntries(start_ikey, end_ikey);
  }

  virtual MemTableRep::Iterator* GetIterator(Arena* arena = nullptr) override {
    return memtable_->GetIterator(arena);
  }

  virtual ~SpecialMemTableRep() override {}

 private:
  unique_ptr<MemTableRep> memtable_;
  int num_entries_flush_;
  int num_entries_;
};

// The factory for the hacky skip list mem table that triggers flush after
// number of entries exceeds a threshold.
class SpecialSkipListFactory : public MemTableRepFactory {
 public:
  // After number of inserts exceeds `num_entries_flush` in a mem table, trigger
  // flush.
  explicit SpecialSkipListFactory(int num_entries_flush)
      : num_entries_flush_(num_entries_flush) {}

  virtual MemTableRep* CreateMemTableRep(
      const MemTableRep::KeyComparator& compare, MemTableAllocator* allocator,
      const SliceTransform* transform, Logger* logger) override {
    return new SpecialMemTableRep(
        allocator, factory_.CreateMemTableRep(compare, allocator, transform, 0),
        num_entries_flush_);
  }
  virtual const char* Name() const override { return "SkipListFactory"; }

 private:
  SkipListFactory factory_;
  int num_entries_flush_;
};

// Special Env used to delay background operations
class SpecialEnv : public EnvWrapper {
 public:
  explicit SpecialEnv(Env* base);

  Status NewWritableFile(const std::string& f, unique_ptr<WritableFile>* r,
                         const EnvOptions& soptions) override {
    class SSTableFile : public WritableFile {
     private:
      SpecialEnv* env_;
      unique_ptr<WritableFile> base_;

     public:
      SSTableFile(SpecialEnv* env, unique_ptr<WritableFile>&& base)
          : env_(env), base_(std::move(base)) {}
      Status Append(const Slice& data) override {
        if (env_->table_write_callback_) {
          (*env_->table_write_callback_)();
        }
        if (env_->drop_writes_.load(std::memory_order_acquire)) {
          // Drop writes on the floor
          return Status::OK();
        } else if (env_->no_space_.load(std::memory_order_acquire)) {
          return Status::IOError("No space left on device");
        } else {
          env_->bytes_written_ += data.size();
          return base_->Append(data);
        }
      }
      Status Truncate(uint64_t size) override { return base_->Truncate(size); }
      Status Close() override {
// SyncPoint is not supported in Released Windows Mode.
#if !(defined NDEBUG) || !defined(OS_WIN)
        // Check preallocation size
        // preallocation size is never passed to base file.
        size_t preallocation_size = preallocation_block_size();
        TEST_SYNC_POINT_CALLBACK("DBTestWritableFile.GetPreallocationStatus",
                                 &preallocation_size);
#endif  // !(defined NDEBUG) || !defined(OS_WIN)
        return base_->Close();
      }
      Status Flush() override { return base_->Flush(); }
      Status Sync() override {
        ++env_->sync_counter_;
        while (env_->delay_sstable_sync_.load(std::memory_order_acquire)) {
          env_->SleepForMicroseconds(100000);
        }
        return base_->Sync();
      }
      void SetIOPriority(Env::IOPriority pri) override {
        base_->SetIOPriority(pri);
      }
      Env::IOPriority GetIOPriority() override {
        return base_->GetIOPriority();
      }
    };
    class ManifestFile : public WritableFile {
     public:
      ManifestFile(SpecialEnv* env, unique_ptr<WritableFile>&& b)
          : env_(env), base_(std::move(b)) {}
      Status Append(const Slice& data) override {
        if (env_->manifest_write_error_.load(std::memory_order_acquire)) {
          return Status::IOError("simulated writer error");
        } else {
          return base_->Append(data);
        }
      }
      Status Truncate(uint64_t size) override { return base_->Truncate(size); }
      Status Close() override { return base_->Close(); }
      Status Flush() override { return base_->Flush(); }
      Status Sync() override {
        ++env_->sync_counter_;
        if (env_->manifest_sync_error_.load(std::memory_order_acquire)) {
          return Status::IOError("simulated sync error");
        } else {
          return base_->Sync();
        }
      }
      uint64_t GetFileSize() override { return base_->GetFileSize(); }

     private:
      SpecialEnv* env_;
      unique_ptr<WritableFile> base_;
    };
    class WalFile : public WritableFile {
     public:
      WalFile(SpecialEnv* env, unique_ptr<WritableFile>&& b)
          : env_(env), base_(std::move(b)) {}
      Status Append(const Slice& data) override {
#if !(defined NDEBUG) || !defined(OS_WIN)
        TEST_SYNC_POINT("SpecialEnv::WalFile::Append:1");
#endif
        Status s;
        if (env_->log_write_error_.load(std::memory_order_acquire)) {
          s = Status::IOError("simulated writer error");
        } else {
          int slowdown =
              env_->log_write_slowdown_.load(std::memory_order_acquire);
          if (slowdown > 0) {
            env_->SleepForMicroseconds(slowdown);
          }
          s = base_->Append(data);
        }
#if !(defined NDEBUG) || !defined(OS_WIN)
        TEST_SYNC_POINT("SpecialEnv::WalFile::Append:2");
#endif
        return s;
      }
      Status Truncate(uint64_t size) override { return base_->Truncate(size); }
      Status Close() override { return base_->Close(); }
      Status Flush() override { return base_->Flush(); }
      Status Sync() override {
        ++env_->sync_counter_;
        return base_->Sync();
      }
      bool IsSyncThreadSafe() const override {
        return env_->is_wal_sync_thread_safe_.load();
      }

     private:
      SpecialEnv* env_;
      unique_ptr<WritableFile> base_;
    };

    if (non_writeable_rate_.load(std::memory_order_acquire) > 0) {
      uint32_t random_number;
      {
        MutexLock l(&rnd_mutex_);
        random_number = rnd_.Uniform(100);
      }
      if (random_number < non_writeable_rate_.load()) {
        return Status::IOError("simulated random write error");
      }
    }

    new_writable_count_++;

    if (non_writable_count_.load() > 0) {
      non_writable_count_--;
      return Status::IOError("simulated write error");
    }

    Status s = target()->NewWritableFile(f, r, soptions);
    if (s.ok()) {
      if (strstr(f.c_str(), ".sst") != nullptr) {
        r->reset(new SSTableFile(this, std::move(*r)));
      } else if (strstr(f.c_str(), "MANIFEST") != nullptr) {
        r->reset(new ManifestFile(this, std::move(*r)));
      } else if (strstr(f.c_str(), "log") != nullptr) {
        r->reset(new WalFile(this, std::move(*r)));
      }
    }
    return s;
  }

  Status NewRandomAccessFile(const std::string& f,
                             unique_ptr<RandomAccessFile>* r,
                             const EnvOptions& soptions) override {
    class CountingFile : public RandomAccessFile {
     public:
      CountingFile(unique_ptr<RandomAccessFile>&& target,
                   anon::AtomicCounter* counter,
                   std::atomic<size_t>* bytes_read)
          : target_(std::move(target)),
            counter_(counter),
            bytes_read_(bytes_read) {}
      virtual Status Read(uint64_t offset, size_t n, Slice* result,
                          char* scratch) const override {
        counter_->Increment();
        Status s = target_->Read(offset, n, result, scratch);
        *bytes_read_ += result->size();
        return s;
      }

     private:
      unique_ptr<RandomAccessFile> target_;
      anon::AtomicCounter* counter_;
      std::atomic<size_t>* bytes_read_;
    };

    Status s = target()->NewRandomAccessFile(f, r, soptions);
    random_file_open_counter_++;
    if (s.ok() && count_random_reads_) {
      r->reset(new CountingFile(std::move(*r), &random_read_counter_,
                                &random_read_bytes_counter_));
    }
    return s;
  }

  Status NewSequentialFile(const std::string& f, unique_ptr<SequentialFile>* r,
                           const EnvOptions& soptions) override {
    class CountingFile : public SequentialFile {
     public:
      CountingFile(unique_ptr<SequentialFile>&& target,
                   anon::AtomicCounter* counter)
          : target_(std::move(target)), counter_(counter) {}
      virtual Status Read(size_t n, Slice* result, char* scratch) override {
        counter_->Increment();
        return target_->Read(n, result, scratch);
      }
      virtual Status Skip(uint64_t n) override { return target_->Skip(n); }

     private:
      unique_ptr<SequentialFile> target_;
      anon::AtomicCounter* counter_;
    };

    Status s = target()->NewSequentialFile(f, r, soptions);
    if (s.ok() && count_sequential_reads_) {
      r->reset(new CountingFile(std::move(*r), &sequential_read_counter_));
    }
    return s;
  }

  virtual void SleepForMicroseconds(int micros) override {
    sleep_counter_.Increment();
    if (no_sleep_ || time_elapse_only_sleep_) {
      addon_time_.fetch_add(micros);
    }
    if (!no_sleep_) {
      target()->SleepForMicroseconds(micros);
    }
  }

  virtual Status GetCurrentTime(int64_t* unix_time) override {
    Status s;
    if (!time_elapse_only_sleep_) {
      s = target()->GetCurrentTime(unix_time);
    }
    if (s.ok()) {
      *unix_time += addon_time_.load();
    }
    return s;
  }

  virtual uint64_t NowNanos() override {
    return (time_elapse_only_sleep_ ? 0 : target()->NowNanos()) +
           addon_time_.load() * 1000;
  }

  virtual uint64_t NowMicros() override {
    return (time_elapse_only_sleep_ ? 0 : target()->NowMicros()) +
           addon_time_.load();
  }

  Random rnd_;
  port::Mutex rnd_mutex_;  // Lock to pretect rnd_

  // sstable Sync() calls are blocked while this pointer is non-nullptr.
  std::atomic<bool> delay_sstable_sync_;

  // Drop writes on the floor while this pointer is non-nullptr.
  std::atomic<bool> drop_writes_;

  // Simulate no-space errors while this pointer is non-nullptr.
  std::atomic<bool> no_space_;

  // Simulate non-writable file system while this pointer is non-nullptr
  std::atomic<bool> non_writable_;

  // Force sync of manifest files to fail while this pointer is non-nullptr
  std::atomic<bool> manifest_sync_error_;

  // Force write to manifest files to fail while this pointer is non-nullptr
  std::atomic<bool> manifest_write_error_;

  // Force write to log files to fail while this pointer is non-nullptr
  std::atomic<bool> log_write_error_;

  // Slow down every log write, in micro-seconds.
  std::atomic<int> log_write_slowdown_;

  bool count_random_reads_;
  anon::AtomicCounter random_read_counter_;
  std::atomic<size_t> random_read_bytes_counter_;
  std::atomic<int> random_file_open_counter_;

  bool count_sequential_reads_;
  anon::AtomicCounter sequential_read_counter_;

  anon::AtomicCounter sleep_counter_;

  std::atomic<int64_t> bytes_written_;

  std::atomic<int> sync_counter_;

  std::atomic<uint32_t> non_writeable_rate_;

  std::atomic<uint32_t> new_writable_count_;

  std::atomic<uint32_t> non_writable_count_;

  std::function<void()>* table_write_callback_;

  std::atomic<int64_t> addon_time_;

  bool time_elapse_only_sleep_;

  bool no_sleep_;

  std::atomic<bool> is_wal_sync_thread_safe_{true};
};

#ifndef ROCKSDB_LITE
class OnFileDeletionListener : public EventListener {
 public:
  OnFileDeletionListener() : matched_count_(0), expected_file_name_("") {}

  void SetExpectedFileName(const std::string file_name) {
    expected_file_name_ = file_name;
  }

  void VerifyMatchedCount(size_t expected_value) {
    ASSERT_EQ(matched_count_, expected_value);
  }

  void OnTableFileDeleted(const TableFileDeletionInfo& info) override {
    if (expected_file_name_ != "") {
      ASSERT_EQ(expected_file_name_, info.file_path);
      expected_file_name_ = "";
      matched_count_++;
    }
  }

 private:
  size_t matched_count_;
  std::string expected_file_name_;
};
#endif

class DBTestBase : public testing::Test {
 protected:
  // Sequence of option configurations to try
  enum OptionConfig {
    kDefault = 0,
    kBlockBasedTableWithPrefixHashIndex = 1,
    kBlockBasedTableWithWholeKeyHashIndex = 2,
    kPlainTableFirstBytePrefix = 3,
    kPlainTableCappedPrefix = 4,
    kPlainTableCappedPrefixNonMmap = 5,
    kPlainTableAllBytesPrefix = 6,
    kVectorRep = 7,
    kHashLinkList = 8,
    kHashCuckoo = 9,
    kMergePut = 10,
    kFilter = 11,
    kFullFilterWithNewTableReaderForCompactions = 12,
    kUncompressed = 13,
    kNumLevel_3 = 14,
    kDBLogDir = 15,
    kWalDirAndMmapReads = 16,
    kManifestFileSize = 17,
    kPerfOptions = 18,
    kDeletesFilterFirst = 19,
    kHashSkipList = 20,
    kUniversalCompaction = 21,
    kUniversalCompactionMultiLevel = 22,
    kCompressedBlockCache = 23,
    kInfiniteMaxOpenFiles = 24,
    kxxHashChecksum = 25,
    kFIFOCompaction = 26,
    kOptimizeFiltersForHits = 27,
    kRowCache = 28,
    kRecycleLogFiles = 29,
    kConcurrentSkipList = 30,
    kEnd = 31,
    kLevelSubcompactions = 31,
    kUniversalSubcompactions = 32,
    kBlockBasedTableWithIndexRestartInterval = 33,
  };
  int option_config_;

 public:
  std::string dbname_;
  std::string alternative_wal_dir_;
  std::string alternative_db_log_dir_;
  MockEnv* mem_env_;
  SpecialEnv* env_;
  DB* db_;
  std::vector<ColumnFamilyHandle*> handles_;

  Options last_options_;

  // Skip some options, as they may not be applicable to a specific test.
  // To add more skip constants, use values 4, 8, 16, etc.
  enum OptionSkip {
    kNoSkip = 0,
    kSkipDeletesFilterFirst = 1,
    kSkipUniversalCompaction = 2,
    kSkipMergePut = 4,
    kSkipPlainTable = 8,
    kSkipHashIndex = 16,
    kSkipNoSeekToLast = 32,
    kSkipHashCuckoo = 64,
    kSkipFIFOCompaction = 128,
    kSkipMmapReads = 256,
  };

  explicit DBTestBase(const std::string path);

  ~DBTestBase();

  static std::string RandomString(Random* rnd, int len) {
    std::string r;
    test::RandomString(rnd, len, &r);
    return r;
  }

  static std::string Key(int i) {
    char buf[100];
    snprintf(buf, sizeof(buf), "key%06d", i);
    return std::string(buf);
  }

  static bool ShouldSkipOptions(int option_config, int skip_mask = kNoSkip);

  // Switch to a fresh database with the next option configuration to
  // test.  Return false if there are no more configurations to test.
  bool ChangeOptions(int skip_mask = kNoSkip);

  // Switch between different compaction styles (we have only 2 now).
  bool ChangeCompactOptions();

  // Switch between different filter policy
  // Jump from kDefault to kFilter to kFullFilter
  bool ChangeFilterOptions();

  // Return the current option configuration.
  Options CurrentOptions(
      const anon::OptionsOverride& options_override = anon::OptionsOverride());

  Options CurrentOptions(
      const Options& defaultOptions,
      const anon::OptionsOverride& options_override = anon::OptionsOverride());

  DBImpl* dbfull() { return reinterpret_cast<DBImpl*>(db_); }

  void CreateColumnFamilies(const std::vector<std::string>& cfs,
                            const Options& options);

  void CreateAndReopenWithCF(const std::vector<std::string>& cfs,
                             const Options& options);

  void ReopenWithColumnFamilies(const std::vector<std::string>& cfs,
                                const std::vector<Options>& options);

  void ReopenWithColumnFamilies(const std::vector<std::string>& cfs,
                                const Options& options);

  Status TryReopenWithColumnFamilies(const std::vector<std::string>& cfs,
                                     const std::vector<Options>& options);

  Status TryReopenWithColumnFamilies(const std::vector<std::string>& cfs,
                                     const Options& options);

  void Reopen(const Options& options);

  void Close();

  void DestroyAndReopen(const Options& options);

  void Destroy(const Options& options);

  Status ReadOnlyReopen(const Options& options);

  Status TryReopen(const Options& options);

  Status Flush(int cf = 0);

  Status Put(const Slice& k, const Slice& v, WriteOptions wo = WriteOptions());

  Status Put(int cf, const Slice& k, const Slice& v,
             WriteOptions wo = WriteOptions());

  Status Delete(const std::string& k);

  Status Delete(int cf, const std::string& k);

  Status SingleDelete(const std::string& k);

  Status SingleDelete(int cf, const std::string& k);

  std::string Get(const std::string& k, const Snapshot* snapshot = nullptr);

  std::string Get(int cf, const std::string& k,
                  const Snapshot* snapshot = nullptr);

  uint64_t GetNumSnapshots();

  uint64_t GetTimeOldestSnapshots();

  // Return a string that contains all key,value pairs in order,
  // formatted like "(k1->v1)(k2->v2)".
  std::string Contents(int cf = 0);

  std::string AllEntriesFor(const Slice& user_key, int cf = 0);

#ifndef ROCKSDB_LITE
  int NumSortedRuns(int cf = 0);

  uint64_t TotalSize(int cf = 0);

  uint64_t SizeAtLevel(int level);

  size_t TotalLiveFiles(int cf = 0);

  size_t CountLiveFiles();
#endif  // ROCKSDB_LITE

  int NumTableFilesAtLevel(int level, int cf = 0);

  double CompressionRatioAtLevel(int level, int cf = 0);

  int TotalTableFiles(int cf = 0, int levels = -1);

  // Return spread of files per level
  std::string FilesPerLevel(int cf = 0);

  size_t CountFiles();

  uint64_t Size(const Slice& start, const Slice& limit, int cf = 0);

  void Compact(int cf, const Slice& start, const Slice& limit,
               uint32_t target_path_id);

  void Compact(int cf, const Slice& start, const Slice& limit);

  void Compact(const Slice& start, const Slice& limit);

  // Do n memtable compactions, each of which produces an sstable
  // covering the range [small,large].
  void MakeTables(int n, const std::string& small, const std::string& large,
                  int cf = 0);

  // Prevent pushing of new sstables into deeper levels by adding
  // tables that cover a specified range to all levels.
  void FillLevels(const std::string& smallest, const std::string& largest,
                  int cf);

  void MoveFilesToLevel(int level, int cf = 0);

  void DumpFileCounts(const char* label);

  std::string DumpSSTableList();

  void GetSstFiles(std::string path, std::vector<std::string>* files);

  int GetSstFileCount(std::string path);

  // this will generate non-overlapping files since it keeps increasing key_idx
  void GenerateNewFile(Random* rnd, int* key_idx, bool nowait = false);

  void GenerateNewFile(int fd, Random* rnd, int* key_idx, bool nowait = false);

  static const int kNumKeysByGenerateNewRandomFile;
  static const int KNumKeysByGenerateNewFile = 100;

  void GenerateNewRandomFile(Random* rnd, bool nowait = false);

  std::string IterStatus(Iterator* iter);

  Options OptionsForLogIterTest();

  std::string DummyString(size_t len, char c = 'a');

  void VerifyIterLast(std::string expected_key, int cf = 0);

  // Used to test InplaceUpdate

  // If previous value is nullptr or delta is > than previous value,
  //   sets newValue with delta
  // If previous value is not empty,
  //   updates previous value with 'b' string of previous value size - 1.
  static UpdateStatus updateInPlaceSmallerSize(char* prevValue,
                                               uint32_t* prevSize, Slice delta,
                                               std::string* newValue);

  static UpdateStatus updateInPlaceSmallerVarintSize(char* prevValue,
                                                     uint32_t* prevSize,
                                                     Slice delta,
                                                     std::string* newValue);

  static UpdateStatus updateInPlaceLargerSize(char* prevValue,
                                              uint32_t* prevSize, Slice delta,
                                              std::string* newValue);

  static UpdateStatus updateInPlaceNoAction(char* prevValue, uint32_t* prevSize,
                                            Slice delta, std::string* newValue);

  // Utility method to test InplaceUpdate
  void validateNumberOfEntries(int numValues, int cf = 0);

  void CopyFile(const std::string& source, const std::string& destination,
                uint64_t size = 0);

  std::unordered_map<std::string, uint64_t> GetAllSSTFiles(
      uint64_t* total_size = nullptr);

  std::vector<std::uint64_t> ListTableFiles(Env* env, const std::string& path);

#ifndef ROCKSDB_LITE
  uint64_t GetNumberOfSstFilesForColumnFamily(DB* db,
                                              std::string column_family_name);
#endif  // ROCKSDB_LITE

  uint64_t TestGetTickerCount(const Options& options, Tickers ticker_type) {
    return options.statistics->getTickerCount(ticker_type);
  }
};

}  // namespace rocksdb
