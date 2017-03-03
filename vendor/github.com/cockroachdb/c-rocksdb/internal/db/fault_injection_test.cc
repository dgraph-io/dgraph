//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.
//
// Copyright 2014 The LevelDB Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file. See the AUTHORS file for names of contributors.

// This test uses a custom Env to keep track of the state of a filesystem as of
// the last "sync". It then checks for data loss errors by purposely dropping
// file data (or entire files) not protected by a "sync".

#include "db/db_impl.h"
#include "db/filename.h"
#include "db/log_format.h"
#include "db/version_set.h"
#include "rocksdb/cache.h"
#include "rocksdb/db.h"
#include "rocksdb/env.h"
#include "rocksdb/table.h"
#include "rocksdb/write_batch.h"
#include "util/fault_injection_test_env.h"
#include "util/logging.h"
#include "util/mock_env.h"
#include "util/mutexlock.h"
#include "util/sync_point.h"
#include "util/testharness.h"
#include "util/testutil.h"

namespace rocksdb {

static const int kValueSize = 1000;
static const int kMaxNumValues = 2000;
static const size_t kNumIterations = 3;

class FaultInjectionTest : public testing::Test,
                           public testing::WithParamInterface<bool> {
 protected:
  enum OptionConfig {
    kDefault,
    kDifferentDataDir,
    kWalDir,
    kSyncWal,
    kWalDirSyncWal,
    kMultiLevels,
    kEnd,
  };
  int option_config_;
  // When need to make sure data is persistent, sync WAL
  bool sync_use_wal_;
  // When need to make sure data is persistent, call DB::CompactRange()
  bool sync_use_compact_;

  bool sequential_order_;

 protected:
 public:
  enum ExpectedVerifResult { kValExpectFound, kValExpectNoError };
  enum ResetMethod {
    kResetDropUnsyncedData,
    kResetDropRandomUnsyncedData,
    kResetDeleteUnsyncedFiles,
    kResetDropAndDeleteUnsynced
  };

  std::unique_ptr<Env> base_env_;
  FaultInjectionTestEnv* env_;
  std::string dbname_;
  shared_ptr<Cache> tiny_cache_;
  Options options_;
  DB* db_;

  FaultInjectionTest()
      : option_config_(kDefault),
        sync_use_wal_(false),
        sync_use_compact_(true),
        base_env_(nullptr),
        env_(NULL),
        db_(NULL) {
  }

  ~FaultInjectionTest() {
    rocksdb::SyncPoint::GetInstance()->DisableProcessing();
    rocksdb::SyncPoint::GetInstance()->ClearAllCallBacks();
  }

  bool ChangeOptions() {
    option_config_++;
    if (option_config_ >= kEnd) {
      return false;
    } else {
      if (option_config_ == kMultiLevels) {
        base_env_.reset(new MockEnv(Env::Default()));
      }
      return true;
    }
  }

  // Return the current option configuration.
  Options CurrentOptions() {
    sync_use_wal_ = false;
    sync_use_compact_ = true;
    Options options;
    switch (option_config_) {
      case kWalDir:
        options.wal_dir = test::TmpDir(env_) + "/fault_test_wal";
        break;
      case kDifferentDataDir:
        options.db_paths.emplace_back(test::TmpDir(env_) + "/fault_test_data",
                                      1000000U);
        break;
      case kSyncWal:
        sync_use_wal_ = true;
        sync_use_compact_ = false;
        break;
      case kWalDirSyncWal:
        options.wal_dir = test::TmpDir(env_) + "/fault_test_wal";
        sync_use_wal_ = true;
        sync_use_compact_ = false;
        break;
      case kMultiLevels:
        options.write_buffer_size = 64 * 1024;
        options.target_file_size_base = 64 * 1024;
        options.level0_file_num_compaction_trigger = 2;
        options.level0_slowdown_writes_trigger = 2;
        options.level0_stop_writes_trigger = 4;
        options.max_bytes_for_level_base = 128 * 1024;
        options.max_write_buffer_number = 2;
        options.max_background_compactions = 8;
        options.max_background_flushes = 8;
        sync_use_wal_ = true;
        sync_use_compact_ = false;
        break;
      default:
        break;
    }
    return options;
  }

  Status NewDB() {
    assert(db_ == NULL);
    assert(tiny_cache_ == nullptr);
    assert(env_ == NULL);

    env_ =
        new FaultInjectionTestEnv(base_env_ ? base_env_.get() : Env::Default());

    options_ = CurrentOptions();
    options_.env = env_;
    options_.paranoid_checks = true;

    BlockBasedTableOptions table_options;
    tiny_cache_ = NewLRUCache(100);
    table_options.block_cache = tiny_cache_;
    options_.table_factory.reset(NewBlockBasedTableFactory(table_options));

    dbname_ = test::TmpDir() + "/fault_test";

    EXPECT_OK(DestroyDB(dbname_, options_));

    options_.create_if_missing = true;
    Status s = OpenDB();
    options_.create_if_missing = false;
    return s;
  }

  void SetUp() override {
    sequential_order_ = GetParam();
    ASSERT_OK(NewDB());
  }

  void TearDown() override {
    CloseDB();

    Status s = DestroyDB(dbname_, options_);

    delete env_;
    env_ = NULL;

    tiny_cache_.reset();

    ASSERT_OK(s);
  }

  void Build(const WriteOptions& write_options, int start_idx, int num_vals) {
    std::string key_space, value_space;
    WriteBatch batch;
    for (int i = start_idx; i < start_idx + num_vals; i++) {
      Slice key = Key(i, &key_space);
      batch.Clear();
      batch.Put(key, Value(i, &value_space));
      ASSERT_OK(db_->Write(write_options, &batch));
    }
  }

  Status ReadValue(int i, std::string* val) const {
    std::string key_space, value_space;
    Slice key = Key(i, &key_space);
    Value(i, &value_space);
    ReadOptions options;
    return db_->Get(options, key, val);
  }

  Status Verify(int start_idx, int num_vals,
                ExpectedVerifResult expected) const {
    std::string val;
    std::string value_space;
    Status s;
    for (int i = start_idx; i < start_idx + num_vals && s.ok(); i++) {
      Value(i, &value_space);
      s = ReadValue(i, &val);
      if (s.ok()) {
        EXPECT_EQ(value_space, val);
      }
      if (expected == kValExpectFound) {
        if (!s.ok()) {
          fprintf(stderr, "Error when read %dth record (expect found): %s\n", i,
                  s.ToString().c_str());
          return s;
        }
      } else if (!s.ok() && !s.IsNotFound()) {
        fprintf(stderr, "Error when read %dth record: %s\n", i,
                s.ToString().c_str());
        return s;
      }
    }
    return Status::OK();
  }

#if __clang_major__ > 3 || (__clang_major__ == 3 && __clang_minor__ >= 9)
__attribute__((__no_sanitize__("undefined")))
#elif __GNUC__ > 4 || (__GNUC__ == 4 && __GNUC_MINOR__ >= 9)
__attribute__((__no_sanitize_undefined__))
#endif
  // Return the ith key
  Slice Key(int i, std::string* storage) const {
    int num = i;
    if (!sequential_order_) {
      // random transfer
      const int m = 0x5bd1e995;
      num *= m;
      num ^= num << 24;
    }
    char buf[100];
    snprintf(buf, sizeof(buf), "%016d", num);
    storage->assign(buf, strlen(buf));
    return Slice(*storage);
  }

  // Return the value to associate with the specified key
  Slice Value(int k, std::string* storage) const {
    Random r(k);
    return test::RandomString(&r, kValueSize, storage);
  }

  void CloseDB() {
    delete db_;
    db_ = NULL;
  }

  Status OpenDB() {
    CloseDB();
    env_->ResetState();
    return DB::Open(options_, dbname_, &db_);
  }

  void DeleteAllData() {
    Iterator* iter = db_->NewIterator(ReadOptions());
    WriteOptions options;
    for (iter->SeekToFirst(); iter->Valid(); iter->Next()) {
      ASSERT_OK(db_->Delete(WriteOptions(), iter->key()));
    }

    delete iter;

    FlushOptions flush_options;
    flush_options.wait = true;
    db_->Flush(flush_options);
  }

  // rnd cannot be null for kResetDropRandomUnsyncedData
  void ResetDBState(ResetMethod reset_method, Random* rnd = nullptr) {
    env_->AssertNoOpenFile();
    switch (reset_method) {
      case kResetDropUnsyncedData:
        ASSERT_OK(env_->DropUnsyncedFileData());
        break;
      case kResetDropRandomUnsyncedData:
        ASSERT_OK(env_->DropRandomUnsyncedFileData(rnd));
        break;
      case kResetDeleteUnsyncedFiles:
        ASSERT_OK(env_->DeleteFilesCreatedAfterLastDirSync());
        break;
      case kResetDropAndDeleteUnsynced:
        ASSERT_OK(env_->DropUnsyncedFileData());
        ASSERT_OK(env_->DeleteFilesCreatedAfterLastDirSync());
        break;
      default:
        assert(false);
    }
  }

  void PartialCompactTestPreFault(int num_pre_sync, int num_post_sync) {
    DeleteAllData();

    WriteOptions write_options;
    write_options.sync = sync_use_wal_;

    Build(write_options, 0, num_pre_sync);
    if (sync_use_compact_) {
      db_->CompactRange(CompactRangeOptions(), nullptr, nullptr);
    }
    write_options.sync = false;
    Build(write_options, num_pre_sync, num_post_sync);
  }

  void PartialCompactTestReopenWithFault(ResetMethod reset_method,
                                         int num_pre_sync, int num_post_sync,
                                         Random* rnd = nullptr) {
    env_->SetFilesystemActive(false);
    CloseDB();
    ResetDBState(reset_method, rnd);
    ASSERT_OK(OpenDB());
    ASSERT_OK(Verify(0, num_pre_sync, FaultInjectionTest::kValExpectFound));
    ASSERT_OK(Verify(num_pre_sync, num_post_sync,
                     FaultInjectionTest::kValExpectNoError));
    WaitCompactionFinish();
    ASSERT_OK(Verify(0, num_pre_sync, FaultInjectionTest::kValExpectFound));
    ASSERT_OK(Verify(num_pre_sync, num_post_sync,
                     FaultInjectionTest::kValExpectNoError));
  }

  void NoWriteTestPreFault() {
  }

  void NoWriteTestReopenWithFault(ResetMethod reset_method) {
    CloseDB();
    ResetDBState(reset_method);
    ASSERT_OK(OpenDB());
  }

  void WaitCompactionFinish() {
    static_cast<DBImpl*>(db_->GetRootDB())->TEST_WaitForCompact();
    ASSERT_OK(db_->Put(WriteOptions(), "", ""));
  }
};

TEST_P(FaultInjectionTest, FaultTest) {
  do {
    Random rnd(301);

    for (size_t idx = 0; idx < kNumIterations; idx++) {
      int num_pre_sync = rnd.Uniform(kMaxNumValues);
      int num_post_sync = rnd.Uniform(kMaxNumValues);

      PartialCompactTestPreFault(num_pre_sync, num_post_sync);
      PartialCompactTestReopenWithFault(kResetDropUnsyncedData, num_pre_sync,
                                        num_post_sync);
      NoWriteTestPreFault();
      NoWriteTestReopenWithFault(kResetDropUnsyncedData);

      PartialCompactTestPreFault(num_pre_sync, num_post_sync);
      PartialCompactTestReopenWithFault(kResetDropRandomUnsyncedData,
                                        num_pre_sync, num_post_sync, &rnd);
      NoWriteTestPreFault();
      NoWriteTestReopenWithFault(kResetDropUnsyncedData);

      // Setting a separate data path won't pass the test as we don't sync
      // it after creating new files,
      PartialCompactTestPreFault(num_pre_sync, num_post_sync);
      PartialCompactTestReopenWithFault(kResetDropAndDeleteUnsynced,
                                        num_pre_sync, num_post_sync);
      NoWriteTestPreFault();
      NoWriteTestReopenWithFault(kResetDropAndDeleteUnsynced);

      PartialCompactTestPreFault(num_pre_sync, num_post_sync);
      // No new files created so we expect all values since no files will be
      // dropped.
      PartialCompactTestReopenWithFault(kResetDeleteUnsyncedFiles, num_pre_sync,
                                        num_post_sync);
      NoWriteTestPreFault();
      NoWriteTestReopenWithFault(kResetDeleteUnsyncedFiles);
    }
  } while (ChangeOptions());
}

// Previous log file is not fsynced if sync is forced after log rolling.
TEST_P(FaultInjectionTest, WriteOptionSyncTest) {
  test::SleepingBackgroundTask sleeping_task_low;
  env_->SetBackgroundThreads(1, Env::HIGH);
  // Block the job queue to prevent flush job from running.
  env_->Schedule(&test::SleepingBackgroundTask::DoSleepTask, &sleeping_task_low,
                 Env::Priority::HIGH);
  sleeping_task_low.WaitUntilSleeping();

  WriteOptions write_options;
  write_options.sync = false;

  std::string key_space, value_space;
  ASSERT_OK(
      db_->Put(write_options, Key(1, &key_space), Value(1, &value_space)));
  FlushOptions flush_options;
  flush_options.wait = false;
  ASSERT_OK(db_->Flush(flush_options));
  write_options.sync = true;
  ASSERT_OK(
      db_->Put(write_options, Key(2, &key_space), Value(2, &value_space)));

  env_->SetFilesystemActive(false);
  NoWriteTestReopenWithFault(kResetDropAndDeleteUnsynced);
  sleeping_task_low.WakeUp();
  sleeping_task_low.WaitUntilDone();

  ASSERT_OK(OpenDB());
  std::string val;
  Value(2, &value_space);
  ASSERT_OK(ReadValue(2, &val));
  ASSERT_EQ(value_space, val);

  Value(1, &value_space);
  ASSERT_OK(ReadValue(1, &val));
  ASSERT_EQ(value_space, val);
}

TEST_P(FaultInjectionTest, UninstalledCompaction) {
  options_.target_file_size_base = 32 * 1024;
  options_.write_buffer_size = 100 << 10;  // 100KB
  options_.level0_file_num_compaction_trigger = 6;
  options_.level0_stop_writes_trigger = 1 << 10;
  options_.level0_slowdown_writes_trigger = 1 << 10;
  options_.max_background_compactions = 1;
  OpenDB();

  if (!sequential_order_) {
    rocksdb::SyncPoint::GetInstance()->LoadDependency({
        {"FaultInjectionTest::FaultTest:0", "DBImpl::BGWorkCompaction"},
        {"CompactionJob::Run():End", "FaultInjectionTest::FaultTest:1"},
        {"FaultInjectionTest::FaultTest:2",
         "DBImpl::BackgroundCompaction:NonTrivial:AfterRun"},
    });
  }
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  int kNumKeys = 1000;
  Build(WriteOptions(), 0, kNumKeys);
  FlushOptions flush_options;
  flush_options.wait = true;
  db_->Flush(flush_options);
  ASSERT_OK(db_->Put(WriteOptions(), "", ""));
  TEST_SYNC_POINT("FaultInjectionTest::FaultTest:0");
  TEST_SYNC_POINT("FaultInjectionTest::FaultTest:1");
  env_->SetFilesystemActive(false);
  TEST_SYNC_POINT("FaultInjectionTest::FaultTest:2");
  CloseDB();
  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
  ResetDBState(kResetDropUnsyncedData);

  std::atomic<bool> opened(false);
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBImpl::Open:Opened", [&](void* arg) { opened.store(true); });
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBImpl::BGWorkCompaction",
      [&](void* arg) { ASSERT_TRUE(opened.load()); });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();
  ASSERT_OK(OpenDB());
  ASSERT_OK(Verify(0, kNumKeys, FaultInjectionTest::kValExpectFound));
  WaitCompactionFinish();
  ASSERT_OK(Verify(0, kNumKeys, FaultInjectionTest::kValExpectFound));
  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
  rocksdb::SyncPoint::GetInstance()->ClearAllCallBacks();
}

TEST_P(FaultInjectionTest, ManualLogSyncTest) {
  test::SleepingBackgroundTask sleeping_task_low;
  env_->SetBackgroundThreads(1, Env::HIGH);
  // Block the job queue to prevent flush job from running.
  env_->Schedule(&test::SleepingBackgroundTask::DoSleepTask, &sleeping_task_low,
                 Env::Priority::HIGH);
  sleeping_task_low.WaitUntilSleeping();

  WriteOptions write_options;
  write_options.sync = false;

  std::string key_space, value_space;
  ASSERT_OK(
      db_->Put(write_options, Key(1, &key_space), Value(1, &value_space)));
  FlushOptions flush_options;
  flush_options.wait = false;
  ASSERT_OK(db_->Flush(flush_options));
  ASSERT_OK(
      db_->Put(write_options, Key(2, &key_space), Value(2, &value_space)));
  ASSERT_OK(db_->SyncWAL());

  env_->SetFilesystemActive(false);
  NoWriteTestReopenWithFault(kResetDropAndDeleteUnsynced);
  sleeping_task_low.WakeUp();
  sleeping_task_low.WaitUntilDone();

  ASSERT_OK(OpenDB());
  std::string val;
  Value(2, &value_space);
  ASSERT_OK(ReadValue(2, &val));
  ASSERT_EQ(value_space, val);

  Value(1, &value_space);
  ASSERT_OK(ReadValue(1, &val));
  ASSERT_EQ(value_space, val);
}

TEST_P(FaultInjectionTest, WriteBatchWalTerminationTest) {
  ReadOptions ro;
  Options options = CurrentOptions();
  options.env = env_;

  WriteOptions wo;
  wo.sync = true;
  wo.disableWAL = false;
  WriteBatch batch;
  batch.Put("cats", "dogs");
  batch.MarkWalTerminationPoint();
  batch.Put("boys", "girls");
  ASSERT_OK(db_->Write(wo, &batch));

  env_->SetFilesystemActive(false);
  NoWriteTestReopenWithFault(kResetDropAndDeleteUnsynced);
  ASSERT_OK(OpenDB());

  std::string val;
  ASSERT_OK(db_->Get(ro, "cats", &val));
  ASSERT_EQ("dogs", val);
  ASSERT_EQ(db_->Get(ro, "boys", &val), Status::NotFound());
}

INSTANTIATE_TEST_CASE_P(FaultTest, FaultInjectionTest, ::testing::Bool());

}  // namespace rocksdb

int main(int argc, char** argv) {
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}
