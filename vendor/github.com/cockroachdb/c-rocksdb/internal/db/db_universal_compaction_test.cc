//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.
//
// Copyright (c) 2011 The LevelDB Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file. See the AUTHORS file for names of contributors.

#include "db/db_test_util.h"
#include "port/stack_trace.h"
#if !defined(ROCKSDB_LITE)
#include "util/sync_point.h"

namespace rocksdb {

static std::string CompressibleString(Random* rnd, int len) {
  std::string r;
  test::CompressibleString(rnd, 0.8, len, &r);
  return r;
}

class DBTestUniversalCompactionBase
    : public DBTestBase,
      public ::testing::WithParamInterface<std::tuple<int, bool>> {
 public:
  explicit DBTestUniversalCompactionBase(
      const std::string& path) : DBTestBase(path) {}
  virtual void SetUp() override {
    num_levels_ = std::get<0>(GetParam());
    exclusive_manual_compaction_ = std::get<1>(GetParam());
  }
  int num_levels_;
  bool exclusive_manual_compaction_;
};

class DBTestUniversalCompaction : public DBTestUniversalCompactionBase {
 public:
  DBTestUniversalCompaction() :
      DBTestUniversalCompactionBase("/db_universal_compaction_test") {}
};

namespace {
void VerifyCompactionResult(
    const ColumnFamilyMetaData& cf_meta,
    const std::set<std::string>& overlapping_file_numbers) {
#ifndef NDEBUG
  for (auto& level : cf_meta.levels) {
    for (auto& file : level.files) {
      assert(overlapping_file_numbers.find(file.name) ==
             overlapping_file_numbers.end());
    }
  }
#endif
}

class KeepFilter : public CompactionFilter {
 public:
  virtual bool Filter(int level, const Slice& key, const Slice& value,
                      std::string* new_value, bool* value_changed) const
      override {
    return false;
  }

  virtual const char* Name() const override { return "KeepFilter"; }
};

class KeepFilterFactory : public CompactionFilterFactory {
 public:
  explicit KeepFilterFactory(bool check_context = false)
      : check_context_(check_context) {}

  virtual std::unique_ptr<CompactionFilter> CreateCompactionFilter(
      const CompactionFilter::Context& context) override {
    if (check_context_) {
      EXPECT_EQ(expect_full_compaction_.load(), context.is_full_compaction);
      EXPECT_EQ(expect_manual_compaction_.load(), context.is_manual_compaction);
    }
    return std::unique_ptr<CompactionFilter>(new KeepFilter());
  }

  virtual const char* Name() const override { return "KeepFilterFactory"; }
  bool check_context_;
  std::atomic_bool expect_full_compaction_;
  std::atomic_bool expect_manual_compaction_;
};

class DelayFilter : public CompactionFilter {
 public:
  explicit DelayFilter(DBTestBase* d) : db_test(d) {}
  virtual bool Filter(int level, const Slice& key, const Slice& value,
                      std::string* new_value,
                      bool* value_changed) const override {
    db_test->env_->addon_time_.fetch_add(1000);
    return true;
  }

  virtual const char* Name() const override { return "DelayFilter"; }

 private:
  DBTestBase* db_test;
};

class DelayFilterFactory : public CompactionFilterFactory {
 public:
  explicit DelayFilterFactory(DBTestBase* d) : db_test(d) {}
  virtual std::unique_ptr<CompactionFilter> CreateCompactionFilter(
      const CompactionFilter::Context& context) override {
    return std::unique_ptr<CompactionFilter>(new DelayFilter(db_test));
  }

  virtual const char* Name() const override { return "DelayFilterFactory"; }

 private:
  DBTestBase* db_test;
};
}  // namespace

// Make sure we don't trigger a problem if the trigger conditon is given
// to be 0, which is invalid.
TEST_P(DBTestUniversalCompaction, UniversalCompactionSingleSortedRun) {
  Options options = CurrentOptions();

  options.compaction_style = kCompactionStyleUniversal;
  options.num_levels = num_levels_;
  // Config universal compaction to always compact to one single sorted run.
  options.level0_file_num_compaction_trigger = 0;
  options.compaction_options_universal.size_ratio = 10;
  options.compaction_options_universal.min_merge_width = 2;
  options.compaction_options_universal.max_size_amplification_percent = 1;

  options.write_buffer_size = 105 << 10;  // 105KB
  options.arena_block_size = 4 << 10;
  options.target_file_size_base = 32 << 10;  // 32KB
  // trigger compaction if there are >= 4 files
  KeepFilterFactory* filter = new KeepFilterFactory(true);
  filter->expect_manual_compaction_.store(false);
  options.compaction_filter_factory.reset(filter);

  DestroyAndReopen(options);
  ASSERT_EQ(1, db_->GetOptions().level0_file_num_compaction_trigger);

  Random rnd(301);
  int key_idx = 0;

  filter->expect_full_compaction_.store(true);

  for (int num = 0; num < 16; num++) {
    // Write 100KB file. And immediately it should be compacted to one file.
    GenerateNewFile(&rnd, &key_idx);
    dbfull()->TEST_WaitForCompact();
    ASSERT_EQ(NumSortedRuns(0), 1);
  }
}

TEST_P(DBTestUniversalCompaction, OptimizeFiltersForHits) {
  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.compaction_options_universal.size_ratio = 5;
  options.num_levels = num_levels_;
  options.write_buffer_size = 105 << 10;  // 105KB
  options.arena_block_size = 4 << 10;
  options.target_file_size_base = 32 << 10;  // 32KB
  // trigger compaction if there are >= 4 files
  options.level0_file_num_compaction_trigger = 4;
  BlockBasedTableOptions bbto;
  bbto.cache_index_and_filter_blocks = true;
  bbto.filter_policy.reset(NewBloomFilterPolicy(10, false));
  bbto.whole_key_filtering = true;
  options.table_factory.reset(NewBlockBasedTableFactory(bbto));
  options.optimize_filters_for_hits = true;
  options.statistics = rocksdb::CreateDBStatistics();
  options.memtable_factory.reset(new SpecialSkipListFactory(3));

  DestroyAndReopen(options);

  // block compaction from happening
  env_->SetBackgroundThreads(1, Env::LOW);
  test::SleepingBackgroundTask sleeping_task_low;
  env_->Schedule(&test::SleepingBackgroundTask::DoSleepTask, &sleeping_task_low,
                 Env::Priority::LOW);

  for (int num = 0; num < options.level0_file_num_compaction_trigger; num++) {
    Put(Key(num * 10), "val");
    if (num) {
      dbfull()->TEST_WaitForFlushMemTable();
    }
    Put(Key(30 + num * 10), "val");
    Put(Key(60 + num * 10), "val");
  }
  Put("", "");
  dbfull()->TEST_WaitForFlushMemTable();

  // Query set of non existing keys
  for (int i = 5; i < 90; i += 10) {
    ASSERT_EQ(Get(Key(i)), "NOT_FOUND");
  }

  // Make sure bloom filter is used at least once.
  ASSERT_GT(TestGetTickerCount(options, BLOOM_FILTER_USEFUL), 0);
  auto prev_counter = TestGetTickerCount(options, BLOOM_FILTER_USEFUL);

  // Make sure bloom filter is used for all but the last L0 file when looking
  // up a non-existent key that's in the range of all L0 files.
  ASSERT_EQ(Get(Key(35)), "NOT_FOUND");
  ASSERT_EQ(prev_counter + NumTableFilesAtLevel(0) - 1,
            TestGetTickerCount(options, BLOOM_FILTER_USEFUL));
  prev_counter = TestGetTickerCount(options, BLOOM_FILTER_USEFUL);

  // Unblock compaction and wait it for happening.
  sleeping_task_low.WakeUp();
  dbfull()->TEST_WaitForCompact();

  // The same queries will not trigger bloom filter
  for (int i = 5; i < 90; i += 10) {
    ASSERT_EQ(Get(Key(i)), "NOT_FOUND");
  }
  ASSERT_EQ(prev_counter, TestGetTickerCount(options, BLOOM_FILTER_USEFUL));
}

// TODO(kailiu) The tests on UniversalCompaction has some issues:
//  1. A lot of magic numbers ("11" or "12").
//  2. Made assumption on the memtable flush conditions, which may change from
//     time to time.
TEST_P(DBTestUniversalCompaction, UniversalCompactionTrigger) {
  Options options;
  options.compaction_style = kCompactionStyleUniversal;
  options.compaction_options_universal.size_ratio = 5;
  options.num_levels = num_levels_;
  options.write_buffer_size = 105 << 10;  // 105KB
  options.arena_block_size = 4 << 10;
  options.target_file_size_base = 32 << 10;  // 32KB
  // trigger compaction if there are >= 4 files
  options.level0_file_num_compaction_trigger = 4;
  KeepFilterFactory* filter = new KeepFilterFactory(true);
  filter->expect_manual_compaction_.store(false);
  options.compaction_filter_factory.reset(filter);

  options = CurrentOptions(options);
  DestroyAndReopen(options);
  CreateAndReopenWithCF({"pikachu"}, options);

  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBTestWritableFile.GetPreallocationStatus", [&](void* arg) {
        ASSERT_TRUE(arg != nullptr);
        size_t preallocation_size = *(static_cast<size_t*>(arg));
        if (num_levels_ > 3) {
          ASSERT_LE(preallocation_size, options.target_file_size_base * 1.1);
        }
      });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  Random rnd(301);
  int key_idx = 0;

  filter->expect_full_compaction_.store(true);
  // Stage 1:
  //   Generate a set of files at level 0, but don't trigger level-0
  //   compaction.
  for (int num = 0; num < options.level0_file_num_compaction_trigger - 1;
       num++) {
    // Write 100KB
    GenerateNewFile(1, &rnd, &key_idx);
  }

  // Generate one more file at level-0, which should trigger level-0
  // compaction.
  GenerateNewFile(1, &rnd, &key_idx);
  // Suppose each file flushed from mem table has size 1. Now we compact
  // (level0_file_num_compaction_trigger+1)=4 files and should have a big
  // file of size 4.
  ASSERT_EQ(NumSortedRuns(1), 1);

  // Stage 2:
  //   Now we have one file at level 0, with size 4. We also have some data in
  //   mem table. Let's continue generating new files at level 0, but don't
  //   trigger level-0 compaction.
  //   First, clean up memtable before inserting new data. This will generate
  //   a level-0 file, with size around 0.4 (according to previously written
  //   data amount).
  filter->expect_full_compaction_.store(false);
  ASSERT_OK(Flush(1));
  for (int num = 0; num < options.level0_file_num_compaction_trigger - 3;
       num++) {
    GenerateNewFile(1, &rnd, &key_idx);
    ASSERT_EQ(NumSortedRuns(1), num + 3);
  }

  // Generate one more file at level-0, which should trigger level-0
  // compaction.
  GenerateNewFile(1, &rnd, &key_idx);
  // Before compaction, we have 4 files at level 0, with size 4, 0.4, 1, 1.
  // After compaction, we should have 2 files, with size 4, 2.4.
  ASSERT_EQ(NumSortedRuns(1), 2);

  // Stage 3:
  //   Now we have 2 files at level 0, with size 4 and 2.4. Continue
  //   generating new files at level 0.
  for (int num = 0; num < options.level0_file_num_compaction_trigger - 3;
       num++) {
    GenerateNewFile(1, &rnd, &key_idx);
    ASSERT_EQ(NumSortedRuns(1), num + 3);
  }

  // Generate one more file at level-0, which should trigger level-0
  // compaction.
  GenerateNewFile(1, &rnd, &key_idx);
  // Before compaction, we have 4 files at level 0, with size 4, 2.4, 1, 1.
  // After compaction, we should have 3 files, with size 4, 2.4, 2.
  ASSERT_EQ(NumSortedRuns(1), 3);

  // Stage 4:
  //   Now we have 3 files at level 0, with size 4, 2.4, 2. Let's generate a
  //   new file of size 1.
  GenerateNewFile(1, &rnd, &key_idx);
  dbfull()->TEST_WaitForCompact();
  // Level-0 compaction is triggered, but no file will be picked up.
  ASSERT_EQ(NumSortedRuns(1), 4);

  // Stage 5:
  //   Now we have 4 files at level 0, with size 4, 2.4, 2, 1. Let's generate
  //   a new file of size 1.
  filter->expect_full_compaction_.store(true);
  GenerateNewFile(1, &rnd, &key_idx);
  dbfull()->TEST_WaitForCompact();
  // All files at level 0 will be compacted into a single one.
  ASSERT_EQ(NumSortedRuns(1), 1);

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

TEST_P(DBTestUniversalCompaction, UniversalCompactionSizeAmplification) {
  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.num_levels = num_levels_;
  options.write_buffer_size = 100 << 10;     // 100KB
  options.target_file_size_base = 32 << 10;  // 32KB
  options.level0_file_num_compaction_trigger = 3;
  DestroyAndReopen(options);
  CreateAndReopenWithCF({"pikachu"}, options);

  // Trigger compaction if size amplification exceeds 110%
  options.compaction_options_universal.max_size_amplification_percent = 110;
  options = CurrentOptions(options);
  ReopenWithColumnFamilies({"default", "pikachu"}, options);

  Random rnd(301);
  int key_idx = 0;

  //   Generate two files in Level 0. Both files are approx the same size.
  for (int num = 0; num < options.level0_file_num_compaction_trigger - 1;
       num++) {
    // Write 110KB (11 values, each 10K)
    for (int i = 0; i < 11; i++) {
      ASSERT_OK(Put(1, Key(key_idx), RandomString(&rnd, 10000)));
      key_idx++;
    }
    dbfull()->TEST_WaitForFlushMemTable(handles_[1]);
    ASSERT_EQ(NumSortedRuns(1), num + 1);
  }
  ASSERT_EQ(NumSortedRuns(1), 2);

  // Flush whatever is remaining in memtable. This is typically
  // small, which should not trigger size ratio based compaction
  // but will instead trigger size amplification.
  ASSERT_OK(Flush(1));

  dbfull()->TEST_WaitForCompact();

  // Verify that size amplification did occur
  ASSERT_EQ(NumSortedRuns(1), 1);
}

TEST_P(DBTestUniversalCompaction, CompactFilesOnUniversalCompaction) {
  const int kTestKeySize = 16;
  const int kTestValueSize = 984;
  const int kEntrySize = kTestKeySize + kTestValueSize;
  const int kEntriesPerBuffer = 10;

  ChangeCompactOptions();
  Options options;
  options.create_if_missing = true;
  options.compaction_style = kCompactionStyleLevel;
  options.num_levels = 1;
  options.target_file_size_base = options.write_buffer_size;
  options.compression = kNoCompression;
  options = CurrentOptions(options);
  options.write_buffer_size = kEntrySize * kEntriesPerBuffer;
  CreateAndReopenWithCF({"pikachu"}, options);
  ASSERT_EQ(options.compaction_style, kCompactionStyleUniversal);
  Random rnd(301);
  for (int key = 1024 * kEntriesPerBuffer; key >= 0; --key) {
    ASSERT_OK(Put(1, ToString(key), RandomString(&rnd, kTestValueSize)));
  }
  dbfull()->TEST_WaitForFlushMemTable(handles_[1]);
  dbfull()->TEST_WaitForCompact();
  ColumnFamilyMetaData cf_meta;
  dbfull()->GetColumnFamilyMetaData(handles_[1], &cf_meta);
  std::vector<std::string> compaction_input_file_names;
  for (auto file : cf_meta.levels[0].files) {
    if (rnd.OneIn(2)) {
      compaction_input_file_names.push_back(file.name);
    }
  }

  if (compaction_input_file_names.size() == 0) {
    compaction_input_file_names.push_back(
        cf_meta.levels[0].files[0].name);
  }

  // expect fail since universal compaction only allow L0 output
  ASSERT_FALSE(dbfull()
                   ->CompactFiles(CompactionOptions(), handles_[1],
                                  compaction_input_file_names, 1)
                   .ok());

  // expect ok and verify the compacted files no longer exist.
  ASSERT_OK(dbfull()->CompactFiles(
      CompactionOptions(), handles_[1],
      compaction_input_file_names, 0));

  dbfull()->GetColumnFamilyMetaData(handles_[1], &cf_meta);
  VerifyCompactionResult(
      cf_meta,
      std::set<std::string>(compaction_input_file_names.begin(),
          compaction_input_file_names.end()));

  compaction_input_file_names.clear();

  // Pick the first and the last file, expect everything is
  // compacted into one single file.
  compaction_input_file_names.push_back(
      cf_meta.levels[0].files[0].name);
  compaction_input_file_names.push_back(
      cf_meta.levels[0].files[
          cf_meta.levels[0].files.size() - 1].name);
  ASSERT_OK(dbfull()->CompactFiles(
      CompactionOptions(), handles_[1],
      compaction_input_file_names, 0));

  dbfull()->GetColumnFamilyMetaData(handles_[1], &cf_meta);
  ASSERT_EQ(cf_meta.levels[0].files.size(), 1U);
}

TEST_P(DBTestUniversalCompaction, UniversalCompactionTargetLevel) {
  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.write_buffer_size = 100 << 10;     // 100KB
  options.num_levels = 7;
  options.disable_auto_compactions = true;
  DestroyAndReopen(options);

  // Generate 3 overlapping files
  Random rnd(301);
  for (int i = 0; i < 210; i++) {
    ASSERT_OK(Put(Key(i), RandomString(&rnd, 100)));
  }
  ASSERT_OK(Flush());

  for (int i = 200; i < 300; i++) {
    ASSERT_OK(Put(Key(i), RandomString(&rnd, 100)));
  }
  ASSERT_OK(Flush());

  for (int i = 250; i < 260; i++) {
    ASSERT_OK(Put(Key(i), RandomString(&rnd, 100)));
  }
  ASSERT_OK(Flush());

  ASSERT_EQ("3", FilesPerLevel(0));
  // Compact all files into 1 file and put it in L4
  CompactRangeOptions compact_options;
  compact_options.change_level = true;
  compact_options.target_level = 4;
  compact_options.exclusive_manual_compaction = exclusive_manual_compaction_;
  db_->CompactRange(compact_options, nullptr, nullptr);
  ASSERT_EQ("0,0,0,0,1", FilesPerLevel(0));
}


class DBTestUniversalCompactionMultiLevels
    : public DBTestUniversalCompactionBase {
 public:
  DBTestUniversalCompactionMultiLevels() :
      DBTestUniversalCompactionBase(
          "/db_universal_compaction_multi_levels_test") {}
};

TEST_P(DBTestUniversalCompactionMultiLevels, UniversalCompactionMultiLevels) {
  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.num_levels = num_levels_;
  options.write_buffer_size = 100 << 10;  // 100KB
  options.level0_file_num_compaction_trigger = 8;
  options.max_background_compactions = 3;
  options.target_file_size_base = 32 * 1024;
  CreateAndReopenWithCF({"pikachu"}, options);

  // Trigger compaction if size amplification exceeds 110%
  options.compaction_options_universal.max_size_amplification_percent = 110;
  options = CurrentOptions(options);
  ReopenWithColumnFamilies({"default", "pikachu"}, options);

  Random rnd(301);
  int num_keys = 100000;
  for (int i = 0; i < num_keys * 2; i++) {
    ASSERT_OK(Put(1, Key(i % num_keys), Key(i)));
  }

  dbfull()->TEST_WaitForCompact();

  for (int i = num_keys; i < num_keys * 2; i++) {
    ASSERT_EQ(Get(1, Key(i % num_keys)), Key(i));
  }
}
// Tests universal compaction with trivial move enabled
TEST_P(DBTestUniversalCompactionMultiLevels, UniversalCompactionTrivialMove) {
  int32_t trivial_move = 0;
  int32_t non_trivial_move = 0;
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBImpl::BackgroundCompaction:TrivialMove",
      [&](void* arg) { trivial_move++; });
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBImpl::BackgroundCompaction:NonTrivial", [&](void* arg) {
        non_trivial_move++;
        ASSERT_TRUE(arg != nullptr);
        int output_level = *(static_cast<int*>(arg));
        ASSERT_EQ(output_level, 0);
      });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.compaction_options_universal.allow_trivial_move = true;
  options.num_levels = 3;
  options.write_buffer_size = 100 << 10;  // 100KB
  options.level0_file_num_compaction_trigger = 3;
  options.max_background_compactions = 2;
  options.target_file_size_base = 32 * 1024;
  DestroyAndReopen(options);
  CreateAndReopenWithCF({"pikachu"}, options);

  // Trigger compaction if size amplification exceeds 110%
  options.compaction_options_universal.max_size_amplification_percent = 110;
  options = CurrentOptions(options);
  ReopenWithColumnFamilies({"default", "pikachu"}, options);

  Random rnd(301);
  int num_keys = 150000;
  for (int i = 0; i < num_keys; i++) {
    ASSERT_OK(Put(1, Key(i), Key(i)));
  }
  std::vector<std::string> values;

  ASSERT_OK(Flush(1));
  dbfull()->TEST_WaitForCompact();

  ASSERT_GT(trivial_move, 0);
  ASSERT_GT(non_trivial_move, 0);

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

INSTANTIATE_TEST_CASE_P(DBTestUniversalCompactionMultiLevels,
                        DBTestUniversalCompactionMultiLevels,
                        ::testing::Combine(::testing::Values(3, 20),
                                           ::testing::Bool()));

class DBTestUniversalCompactionParallel :
    public DBTestUniversalCompactionBase {
 public:
  DBTestUniversalCompactionParallel() :
      DBTestUniversalCompactionBase(
          "/db_universal_compaction_prallel_test") {}
};

TEST_P(DBTestUniversalCompactionParallel, UniversalCompactionParallel) {
  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.num_levels = num_levels_;
  options.write_buffer_size = 1 << 10;  // 1KB
  options.level0_file_num_compaction_trigger = 3;
  options.max_background_compactions = 3;
  options.max_background_flushes = 3;
  options.target_file_size_base = 1 * 1024;
  options.compaction_options_universal.max_size_amplification_percent = 110;
  DestroyAndReopen(options);
  CreateAndReopenWithCF({"pikachu"}, options);

  // Delay every compaction so multiple compactions will happen.
  std::atomic<int> num_compactions_running(0);
  std::atomic<bool> has_parallel(false);
  rocksdb::SyncPoint::GetInstance()->SetCallBack("CompactionJob::Run():Start",
                                                 [&](void* arg) {
    if (num_compactions_running.fetch_add(1) > 0) {
      has_parallel.store(true);
      return;
    }
    for (int nwait = 0; nwait < 20000; nwait++) {
      if (has_parallel.load() || num_compactions_running.load() > 1) {
        has_parallel.store(true);
        break;
      }
      env_->SleepForMicroseconds(1000);
    }
  });
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "CompactionJob::Run():End",
      [&](void* arg) { num_compactions_running.fetch_add(-1); });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  options = CurrentOptions(options);
  ReopenWithColumnFamilies({"default", "pikachu"}, options);

  Random rnd(301);
  int num_keys = 30000;
  for (int i = 0; i < num_keys * 2; i++) {
    ASSERT_OK(Put(1, Key(i % num_keys), Key(i)));
  }
  dbfull()->TEST_WaitForCompact();

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
  ASSERT_EQ(num_compactions_running.load(), 0);
  ASSERT_TRUE(has_parallel.load());

  for (int i = num_keys; i < num_keys * 2; i++) {
    ASSERT_EQ(Get(1, Key(i % num_keys)), Key(i));
  }

  // Reopen and check.
  ReopenWithColumnFamilies({"default", "pikachu"}, options);
  for (int i = num_keys; i < num_keys * 2; i++) {
    ASSERT_EQ(Get(1, Key(i % num_keys)), Key(i));
  }
}

INSTANTIATE_TEST_CASE_P(DBTestUniversalCompactionParallel,
                        DBTestUniversalCompactionParallel,
                        ::testing::Combine(::testing::Values(1, 10),
                                           ::testing::Bool()));

TEST_P(DBTestUniversalCompaction, UniversalCompactionOptions) {
  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.write_buffer_size = 105 << 10;    // 105KB
  options.arena_block_size = 4 << 10;       // 4KB
  options.target_file_size_base = 32 << 10;  // 32KB
  options.level0_file_num_compaction_trigger = 4;
  options.num_levels = num_levels_;
  options.compaction_options_universal.compression_size_percent = -1;
  DestroyAndReopen(options);
  CreateAndReopenWithCF({"pikachu"}, options);

  Random rnd(301);
  int key_idx = 0;

  for (int num = 0; num < options.level0_file_num_compaction_trigger; num++) {
    // Write 100KB (100 values, each 1K)
    for (int i = 0; i < 100; i++) {
      ASSERT_OK(Put(1, Key(key_idx), RandomString(&rnd, 990)));
      key_idx++;
    }
    dbfull()->TEST_WaitForFlushMemTable(handles_[1]);

    if (num < options.level0_file_num_compaction_trigger - 1) {
      ASSERT_EQ(NumSortedRuns(1), num + 1);
    }
  }

  dbfull()->TEST_WaitForCompact();
  ASSERT_EQ(NumSortedRuns(1), 1);
}

TEST_P(DBTestUniversalCompaction, UniversalCompactionStopStyleSimilarSize) {
  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.write_buffer_size = 105 << 10;    // 105KB
  options.arena_block_size = 4 << 10;       // 4KB
  options.target_file_size_base = 32 << 10;  // 32KB
  // trigger compaction if there are >= 4 files
  options.level0_file_num_compaction_trigger = 4;
  options.compaction_options_universal.size_ratio = 10;
  options.compaction_options_universal.stop_style =
      kCompactionStopStyleSimilarSize;
  options.num_levels = num_levels_;
  DestroyAndReopen(options);

  Random rnd(301);
  int key_idx = 0;

  // Stage 1:
  //   Generate a set of files at level 0, but don't trigger level-0
  //   compaction.
  for (int num = 0; num < options.level0_file_num_compaction_trigger - 1;
       num++) {
    // Write 100KB (100 values, each 1K)
    for (int i = 0; i < 100; i++) {
      ASSERT_OK(Put(Key(key_idx), RandomString(&rnd, 990)));
      key_idx++;
    }
    dbfull()->TEST_WaitForFlushMemTable();
    ASSERT_EQ(NumSortedRuns(), num + 1);
  }

  // Generate one more file at level-0, which should trigger level-0
  // compaction.
  for (int i = 0; i < 100; i++) {
    ASSERT_OK(Put(Key(key_idx), RandomString(&rnd, 990)));
    key_idx++;
  }
  dbfull()->TEST_WaitForCompact();
  // Suppose each file flushed from mem table has size 1. Now we compact
  // (level0_file_num_compaction_trigger+1)=4 files and should have a big
  // file of size 4.
  ASSERT_EQ(NumSortedRuns(), 1);

  // Stage 2:
  //   Now we have one file at level 0, with size 4. We also have some data in
  //   mem table. Let's continue generating new files at level 0, but don't
  //   trigger level-0 compaction.
  //   First, clean up memtable before inserting new data. This will generate
  //   a level-0 file, with size around 0.4 (according to previously written
  //   data amount).
  dbfull()->Flush(FlushOptions());
  for (int num = 0; num < options.level0_file_num_compaction_trigger - 3;
       num++) {
    // Write 110KB (11 values, each 10K)
    for (int i = 0; i < 100; i++) {
      ASSERT_OK(Put(Key(key_idx), RandomString(&rnd, 990)));
      key_idx++;
    }
    dbfull()->TEST_WaitForFlushMemTable();
    ASSERT_EQ(NumSortedRuns(), num + 3);
  }

  // Generate one more file at level-0, which should trigger level-0
  // compaction.
  for (int i = 0; i < 100; i++) {
    ASSERT_OK(Put(Key(key_idx), RandomString(&rnd, 990)));
    key_idx++;
  }
  dbfull()->TEST_WaitForCompact();
  // Before compaction, we have 4 files at level 0, with size 4, 0.4, 1, 1.
  // After compaction, we should have 3 files, with size 4, 0.4, 2.
  ASSERT_EQ(NumSortedRuns(), 3);
  // Stage 3:
  //   Now we have 3 files at level 0, with size 4, 0.4, 2. Generate one
  //   more file at level-0, which should trigger level-0 compaction.
  for (int i = 0; i < 100; i++) {
    ASSERT_OK(Put(Key(key_idx), RandomString(&rnd, 990)));
    key_idx++;
  }
  dbfull()->TEST_WaitForCompact();
  // Level-0 compaction is triggered, but no file will be picked up.
  ASSERT_EQ(NumSortedRuns(), 4);
}

TEST_P(DBTestUniversalCompaction, UniversalCompactionCompressRatio1) {
  if (!Snappy_Supported()) {
    return;
  }

  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.write_buffer_size = 100 << 10;     // 100KB
  options.target_file_size_base = 32 << 10;  // 32KB
  options.level0_file_num_compaction_trigger = 2;
  options.num_levels = num_levels_;
  options.compaction_options_universal.compression_size_percent = 70;
  DestroyAndReopen(options);

  Random rnd(301);
  int key_idx = 0;

  // The first compaction (2) is compressed.
  for (int num = 0; num < 2; num++) {
    // Write 110KB (11 values, each 10K)
    for (int i = 0; i < 11; i++) {
      ASSERT_OK(Put(Key(key_idx), CompressibleString(&rnd, 10000)));
      key_idx++;
    }
    dbfull()->TEST_WaitForFlushMemTable();
    dbfull()->TEST_WaitForCompact();
  }
  ASSERT_LT(TotalSize(), 110000U * 2 * 0.9);

  // The second compaction (4) is compressed
  for (int num = 0; num < 2; num++) {
    // Write 110KB (11 values, each 10K)
    for (int i = 0; i < 11; i++) {
      ASSERT_OK(Put(Key(key_idx), CompressibleString(&rnd, 10000)));
      key_idx++;
    }
    dbfull()->TEST_WaitForFlushMemTable();
    dbfull()->TEST_WaitForCompact();
  }
  ASSERT_LT(TotalSize(), 110000 * 4 * 0.9);

  // The third compaction (2 4) is compressed since this time it is
  // (1 1 3.2) and 3.2/5.2 doesn't reach ratio.
  for (int num = 0; num < 2; num++) {
    // Write 110KB (11 values, each 10K)
    for (int i = 0; i < 11; i++) {
      ASSERT_OK(Put(Key(key_idx), CompressibleString(&rnd, 10000)));
      key_idx++;
    }
    dbfull()->TEST_WaitForFlushMemTable();
    dbfull()->TEST_WaitForCompact();
  }
  ASSERT_LT(TotalSize(), 110000 * 6 * 0.9);

  // When we start for the compaction up to (2 4 8), the latest
  // compressed is not compressed.
  for (int num = 0; num < 8; num++) {
    // Write 110KB (11 values, each 10K)
    for (int i = 0; i < 11; i++) {
      ASSERT_OK(Put(Key(key_idx), CompressibleString(&rnd, 10000)));
      key_idx++;
    }
    dbfull()->TEST_WaitForFlushMemTable();
    dbfull()->TEST_WaitForCompact();
  }
  ASSERT_GT(TotalSize(), 110000 * 11 * 0.8 + 110000 * 2);
}

TEST_P(DBTestUniversalCompaction, UniversalCompactionCompressRatio2) {
  if (!Snappy_Supported()) {
    return;
  }
  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.write_buffer_size = 100 << 10;     // 100KB
  options.target_file_size_base = 32 << 10;  // 32KB
  options.level0_file_num_compaction_trigger = 2;
  options.num_levels = num_levels_;
  options.compaction_options_universal.compression_size_percent = 95;
  DestroyAndReopen(options);

  Random rnd(301);
  int key_idx = 0;

  // When we start for the compaction up to (2 4 8), the latest
  // compressed is compressed given the size ratio to compress.
  for (int num = 0; num < 14; num++) {
    // Write 120KB (12 values, each 10K)
    for (int i = 0; i < 12; i++) {
      ASSERT_OK(Put(Key(key_idx), CompressibleString(&rnd, 10000)));
      key_idx++;
    }
    dbfull()->TEST_WaitForFlushMemTable();
    dbfull()->TEST_WaitForCompact();
  }
  ASSERT_LT(TotalSize(), 120000U * 12 * 0.8 + 120000 * 2);
}

// Test that checks trivial move in universal compaction
TEST_P(DBTestUniversalCompaction, UniversalCompactionTrivialMoveTest1) {
  int32_t trivial_move = 0;
  int32_t non_trivial_move = 0;
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBImpl::BackgroundCompaction:TrivialMove",
      [&](void* arg) { trivial_move++; });
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBImpl::BackgroundCompaction:NonTrivial", [&](void* arg) {
        non_trivial_move++;
        ASSERT_TRUE(arg != nullptr);
        int output_level = *(static_cast<int*>(arg));
        ASSERT_EQ(output_level, 0);
      });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.compaction_options_universal.allow_trivial_move = true;
  options.num_levels = 2;
  options.write_buffer_size = 100 << 10;  // 100KB
  options.level0_file_num_compaction_trigger = 3;
  options.max_background_compactions = 1;
  options.target_file_size_base = 32 * 1024;
  DestroyAndReopen(options);
  CreateAndReopenWithCF({"pikachu"}, options);

  // Trigger compaction if size amplification exceeds 110%
  options.compaction_options_universal.max_size_amplification_percent = 110;
  options = CurrentOptions(options);
  ReopenWithColumnFamilies({"default", "pikachu"}, options);

  Random rnd(301);
  int num_keys = 250000;
  for (int i = 0; i < num_keys; i++) {
    ASSERT_OK(Put(1, Key(i), Key(i)));
  }
  std::vector<std::string> values;

  ASSERT_OK(Flush(1));
  dbfull()->TEST_WaitForCompact();

  ASSERT_GT(trivial_move, 0);
  ASSERT_GT(non_trivial_move, 0);

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}
// Test that checks trivial move in universal compaction
TEST_P(DBTestUniversalCompaction, UniversalCompactionTrivialMoveTest2) {
  int32_t trivial_move = 0;
  int32_t non_trivial_move = 0;
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBImpl::BackgroundCompaction:TrivialMove",
      [&](void* arg) { trivial_move++; });
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBImpl::BackgroundCompaction:NonTrivial",
      [&](void* arg) { non_trivial_move++; });

  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.compaction_options_universal.allow_trivial_move = true;
  options.num_levels = 15;
  options.write_buffer_size = 100 << 10;  // 100KB
  options.level0_file_num_compaction_trigger = 8;
  options.max_background_compactions = 4;
  options.target_file_size_base = 64 * 1024;
  DestroyAndReopen(options);
  CreateAndReopenWithCF({"pikachu"}, options);

  // Trigger compaction if size amplification exceeds 110%
  options.compaction_options_universal.max_size_amplification_percent = 110;
  options = CurrentOptions(options);
  ReopenWithColumnFamilies({"default", "pikachu"}, options);

  Random rnd(301);
  int num_keys = 500000;
  for (int i = 0; i < num_keys; i++) {
    ASSERT_OK(Put(1, Key(i), Key(i)));
  }
  std::vector<std::string> values;

  ASSERT_OK(Flush(1));
  dbfull()->TEST_WaitForCompact();

  ASSERT_GT(trivial_move, 0);
  ASSERT_EQ(non_trivial_move, 0);

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

TEST_P(DBTestUniversalCompaction, UniversalCompactionFourPaths) {
  Options options = CurrentOptions();
  options.db_paths.emplace_back(dbname_, 300 * 1024);
  options.db_paths.emplace_back(dbname_ + "_2", 300 * 1024);
  options.db_paths.emplace_back(dbname_ + "_3", 500 * 1024);
  options.db_paths.emplace_back(dbname_ + "_4", 1024 * 1024 * 1024);
  options.memtable_factory.reset(
      new SpecialSkipListFactory(KNumKeysByGenerateNewFile - 1));
  options.compaction_style = kCompactionStyleUniversal;
  options.compaction_options_universal.size_ratio = 5;
  options.write_buffer_size = 111 << 10;  // 114KB
  options.arena_block_size = 4 << 10;
  options.level0_file_num_compaction_trigger = 2;
  options.num_levels = 1;

  std::vector<std::string> filenames;
  env_->GetChildren(options.db_paths[1].path, &filenames);
  // Delete archival files.
  for (size_t i = 0; i < filenames.size(); ++i) {
    env_->DeleteFile(options.db_paths[1].path + "/" + filenames[i]);
  }
  env_->DeleteDir(options.db_paths[1].path);
  Reopen(options);

  Random rnd(301);
  int key_idx = 0;

  // First three 110KB files are not going to second path.
  // After that, (100K, 200K)
  for (int num = 0; num < 3; num++) {
    GenerateNewFile(&rnd, &key_idx);
  }

  // Another 110KB triggers a compaction to 400K file to second path
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[2].path));

  // (1, 4)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[2].path));
  ASSERT_EQ(1, GetSstFileCount(dbname_));

  // (1,1,4) -> (2, 4)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[2].path));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(0, GetSstFileCount(dbname_));

  // (1, 2, 4)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[2].path));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(1, GetSstFileCount(dbname_));

  // (1, 1, 2, 4) -> (8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[3].path));

  // (1, 8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[3].path));
  ASSERT_EQ(1, GetSstFileCount(dbname_));

  // (1, 1, 8) -> (2, 8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[3].path));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));

  // (1, 2, 8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[3].path));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(1, GetSstFileCount(dbname_));

  // (1, 1, 2, 8) -> (4, 8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[2].path));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[3].path));

  // (1, 4, 8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[3].path));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[2].path));
  ASSERT_EQ(1, GetSstFileCount(dbname_));

  for (int i = 0; i < key_idx; i++) {
    auto v = Get(Key(i));
    ASSERT_NE(v, "NOT_FOUND");
    ASSERT_TRUE(v.size() == 1 || v.size() == 990);
  }

  Reopen(options);

  for (int i = 0; i < key_idx; i++) {
    auto v = Get(Key(i));
    ASSERT_NE(v, "NOT_FOUND");
    ASSERT_TRUE(v.size() == 1 || v.size() == 990);
  }

  Destroy(options);
}

TEST_P(DBTestUniversalCompaction, IncreaseUniversalCompactionNumLevels) {
  std::function<void(int)> verify_func = [&](int num_keys_in_db) {
    std::string keys_in_db;
    Iterator* iter = dbfull()->NewIterator(ReadOptions(), handles_[1]);
    for (iter->SeekToFirst(); iter->Valid(); iter->Next()) {
      keys_in_db.append(iter->key().ToString());
      keys_in_db.push_back(',');
    }
    delete iter;

    std::string expected_keys;
    for (int i = 0; i <= num_keys_in_db; i++) {
      expected_keys.append(Key(i));
      expected_keys.push_back(',');
    }

    ASSERT_EQ(keys_in_db, expected_keys);
  };

  Random rnd(301);
  int max_key1 = 200;
  int max_key2 = 600;
  int max_key3 = 800;
  const int KNumKeysPerFile = 10;

  // Stage 1: open a DB with universal compaction, num_levels=1
  Options options = CurrentOptions();
  options.compaction_style = kCompactionStyleUniversal;
  options.num_levels = 1;
  options.write_buffer_size = 200 << 10;  // 200KB
  options.level0_file_num_compaction_trigger = 3;
  options.memtable_factory.reset(new SpecialSkipListFactory(KNumKeysPerFile));
  options = CurrentOptions(options);
  CreateAndReopenWithCF({"pikachu"}, options);

  for (int i = 0; i <= max_key1; i++) {
    // each value is 10K
    ASSERT_OK(Put(1, Key(i), RandomString(&rnd, 10000)));
    dbfull()->TEST_WaitForFlushMemTable(handles_[1]);
    dbfull()->TEST_WaitForCompact();
  }
  ASSERT_OK(Flush(1));
  dbfull()->TEST_WaitForCompact();

  // Stage 2: reopen with universal compaction, num_levels=4
  options.compaction_style = kCompactionStyleUniversal;
  options.num_levels = 4;
  options = CurrentOptions(options);
  ReopenWithColumnFamilies({"default", "pikachu"}, options);

  verify_func(max_key1);

  // Insert more keys
  for (int i = max_key1 + 1; i <= max_key2; i++) {
    // each value is 10K
    ASSERT_OK(Put(1, Key(i), RandomString(&rnd, 10000)));
    dbfull()->TEST_WaitForFlushMemTable(handles_[1]);
    dbfull()->TEST_WaitForCompact();
  }
  ASSERT_OK(Flush(1));
  dbfull()->TEST_WaitForCompact();

  verify_func(max_key2);
  // Compaction to non-L0 has happened.
  ASSERT_GT(NumTableFilesAtLevel(options.num_levels - 1, 1), 0);

  // Stage 3: Revert it back to one level and revert to num_levels=1.
  options.num_levels = 4;
  options.target_file_size_base = INT_MAX;
  ReopenWithColumnFamilies({"default", "pikachu"}, options);
  // Compact all to level 0
  CompactRangeOptions compact_options;
  compact_options.change_level = true;
  compact_options.target_level = 0;
  compact_options.exclusive_manual_compaction = exclusive_manual_compaction_;
  dbfull()->CompactRange(compact_options, handles_[1], nullptr, nullptr);
  // Need to restart it once to remove higher level records in manifest.
  ReopenWithColumnFamilies({"default", "pikachu"}, options);
  // Final reopen
  options.compaction_style = kCompactionStyleUniversal;
  options.num_levels = 1;
  options = CurrentOptions(options);
  ReopenWithColumnFamilies({"default", "pikachu"}, options);

  // Insert more keys
  for (int i = max_key2 + 1; i <= max_key3; i++) {
    // each value is 10K
    ASSERT_OK(Put(1, Key(i), RandomString(&rnd, 10000)));
    dbfull()->TEST_WaitForFlushMemTable(handles_[1]);
    dbfull()->TEST_WaitForCompact();
  }
  ASSERT_OK(Flush(1));
  dbfull()->TEST_WaitForCompact();
  verify_func(max_key3);
}


TEST_P(DBTestUniversalCompaction, UniversalCompactionSecondPathRatio) {
  if (!Snappy_Supported()) {
    return;
  }
  Options options = CurrentOptions();
  options.db_paths.emplace_back(dbname_, 500 * 1024);
  options.db_paths.emplace_back(dbname_ + "_2", 1024 * 1024 * 1024);
  options.compaction_style = kCompactionStyleUniversal;
  options.compaction_options_universal.size_ratio = 5;
  options.write_buffer_size = 111 << 10;  // 114KB
  options.arena_block_size = 4 << 10;
  options.level0_file_num_compaction_trigger = 2;
  options.num_levels = 1;
  options.memtable_factory.reset(
      new SpecialSkipListFactory(KNumKeysByGenerateNewFile - 1));

  std::vector<std::string> filenames;
  env_->GetChildren(options.db_paths[1].path, &filenames);
  // Delete archival files.
  for (size_t i = 0; i < filenames.size(); ++i) {
    env_->DeleteFile(options.db_paths[1].path + "/" + filenames[i]);
  }
  env_->DeleteDir(options.db_paths[1].path);
  Reopen(options);

  Random rnd(301);
  int key_idx = 0;

  // First three 110KB files are not going to second path.
  // After that, (100K, 200K)
  for (int num = 0; num < 3; num++) {
    GenerateNewFile(&rnd, &key_idx);
  }

  // Another 110KB triggers a compaction to 400K file to second path
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));

  // (1, 4)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(1, GetSstFileCount(dbname_));

  // (1,1,4) -> (2, 4)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(1, GetSstFileCount(dbname_));

  // (1, 2, 4)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(2, GetSstFileCount(dbname_));

  // (1, 1, 2, 4) -> (8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(0, GetSstFileCount(dbname_));

  // (1, 8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(1, GetSstFileCount(dbname_));

  // (1, 1, 8) -> (2, 8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(1, GetSstFileCount(dbname_));

  // (1, 2, 8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(2, GetSstFileCount(dbname_));

  // (1, 1, 2, 8) -> (4, 8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(2, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(0, GetSstFileCount(dbname_));

  // (1, 4, 8)
  GenerateNewFile(&rnd, &key_idx);
  ASSERT_EQ(2, GetSstFileCount(options.db_paths[1].path));
  ASSERT_EQ(1, GetSstFileCount(dbname_));

  for (int i = 0; i < key_idx; i++) {
    auto v = Get(Key(i));
    ASSERT_NE(v, "NOT_FOUND");
    ASSERT_TRUE(v.size() == 1 || v.size() == 990);
  }

  Reopen(options);

  for (int i = 0; i < key_idx; i++) {
    auto v = Get(Key(i));
    ASSERT_NE(v, "NOT_FOUND");
    ASSERT_TRUE(v.size() == 1 || v.size() == 990);
  }

  Destroy(options);
}

INSTANTIATE_TEST_CASE_P(UniversalCompactionNumLevels, DBTestUniversalCompaction,
                        ::testing::Combine(::testing::Values(1, 3, 5),
                                           ::testing::Bool()));

class DBTestUniversalManualCompactionOutputPathId
    : public DBTestUniversalCompactionBase {
 public:
  DBTestUniversalManualCompactionOutputPathId() :
      DBTestUniversalCompactionBase(
          "/db_universal_compaction_manual_pid_test") {}
};

TEST_P(DBTestUniversalManualCompactionOutputPathId,
       ManualCompactionOutputPathId) {
  Options options = CurrentOptions();
  options.create_if_missing = true;
  options.db_paths.emplace_back(dbname_, 1000000000);
  options.db_paths.emplace_back(dbname_ + "_2", 1000000000);
  options.compaction_style = kCompactionStyleUniversal;
  options.num_levels = num_levels_;
  options.target_file_size_base = 1 << 30;  // Big size
  options.level0_file_num_compaction_trigger = 10;
  Destroy(options);
  DestroyAndReopen(options);
  CreateAndReopenWithCF({"pikachu"}, options);
  MakeTables(3, "p", "q", 1);
  dbfull()->TEST_WaitForCompact();
  ASSERT_EQ(2, TotalLiveFiles(1));
  ASSERT_EQ(2, GetSstFileCount(options.db_paths[0].path));
  ASSERT_EQ(0, GetSstFileCount(options.db_paths[1].path));

  // Full compaction to DB path 0
  CompactRangeOptions compact_options;
  compact_options.target_path_id = 1;
  compact_options.exclusive_manual_compaction = exclusive_manual_compaction_;
  db_->CompactRange(compact_options, handles_[1], nullptr, nullptr);
  ASSERT_EQ(1, TotalLiveFiles(1));
  ASSERT_EQ(0, GetSstFileCount(options.db_paths[0].path));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));

  ReopenWithColumnFamilies({kDefaultColumnFamilyName, "pikachu"}, options);
  ASSERT_EQ(1, TotalLiveFiles(1));
  ASSERT_EQ(0, GetSstFileCount(options.db_paths[0].path));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));

  MakeTables(1, "p", "q", 1);
  ASSERT_EQ(2, TotalLiveFiles(1));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[0].path));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));

  ReopenWithColumnFamilies({kDefaultColumnFamilyName, "pikachu"}, options);
  ASSERT_EQ(2, TotalLiveFiles(1));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[0].path));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[1].path));

  // Full compaction to DB path 0
  compact_options.target_path_id = 0;
  compact_options.exclusive_manual_compaction = exclusive_manual_compaction_;
  db_->CompactRange(compact_options, handles_[1], nullptr, nullptr);
  ASSERT_EQ(1, TotalLiveFiles(1));
  ASSERT_EQ(1, GetSstFileCount(options.db_paths[0].path));
  ASSERT_EQ(0, GetSstFileCount(options.db_paths[1].path));

  // Fail when compacting to an invalid path ID
  compact_options.target_path_id = 2;
  compact_options.exclusive_manual_compaction = exclusive_manual_compaction_;
  ASSERT_TRUE(db_->CompactRange(compact_options, handles_[1], nullptr, nullptr)
                  .IsInvalidArgument());
}

INSTANTIATE_TEST_CASE_P(DBTestUniversalManualCompactionOutputPathId,
                        DBTestUniversalManualCompactionOutputPathId,
                        ::testing::Combine(::testing::Values(1, 8),
                                           ::testing::Bool()));

}  // namespace rocksdb

#endif  // !defined(ROCKSDB_LITE)

int main(int argc, char** argv) {
#if !defined(ROCKSDB_LITE)
  rocksdb::port::InstallStackTraceHandler();
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
#else
  return 0;
#endif
}
