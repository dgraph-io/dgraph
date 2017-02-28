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
#include "util/fault_injection_test_env.h"
#include "util/options_helper.h"
#include "util/sync_point.h"

namespace rocksdb {
class DBWALTest : public DBTestBase {
 public:
  DBWALTest() : DBTestBase("/db_wal_test") {}
};

TEST_F(DBWALTest, WAL) {
  do {
    CreateAndReopenWithCF({"pikachu"}, CurrentOptions());
    WriteOptions writeOpt = WriteOptions();
    writeOpt.disableWAL = true;
    ASSERT_OK(dbfull()->Put(writeOpt, handles_[1], "foo", "v1"));
    ASSERT_OK(dbfull()->Put(writeOpt, handles_[1], "bar", "v1"));

    ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    ASSERT_EQ("v1", Get(1, "foo"));
    ASSERT_EQ("v1", Get(1, "bar"));

    writeOpt.disableWAL = false;
    ASSERT_OK(dbfull()->Put(writeOpt, handles_[1], "bar", "v2"));
    writeOpt.disableWAL = true;
    ASSERT_OK(dbfull()->Put(writeOpt, handles_[1], "foo", "v2"));

    ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    // Both value's should be present.
    ASSERT_EQ("v2", Get(1, "bar"));
    ASSERT_EQ("v2", Get(1, "foo"));

    writeOpt.disableWAL = true;
    ASSERT_OK(dbfull()->Put(writeOpt, handles_[1], "bar", "v3"));
    writeOpt.disableWAL = false;
    ASSERT_OK(dbfull()->Put(writeOpt, handles_[1], "foo", "v3"));

    ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    // again both values should be present.
    ASSERT_EQ("v3", Get(1, "foo"));
    ASSERT_EQ("v3", Get(1, "bar"));
  } while (ChangeCompactOptions());
}

TEST_F(DBWALTest, RollLog) {
  do {
    CreateAndReopenWithCF({"pikachu"}, CurrentOptions());
    ASSERT_OK(Put(1, "foo", "v1"));
    ASSERT_OK(Put(1, "baz", "v5"));

    ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    for (int i = 0; i < 10; i++) {
      ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    }
    ASSERT_OK(Put(1, "foo", "v4"));
    for (int i = 0; i < 10; i++) {
      ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    }
  } while (ChangeOptions());
}

TEST_F(DBWALTest, SyncWALNotBlockWrite) {
  Options options = CurrentOptions();
  options.max_write_buffer_number = 4;
  DestroyAndReopen(options);

  ASSERT_OK(Put("foo1", "bar1"));
  ASSERT_OK(Put("foo5", "bar5"));

  rocksdb::SyncPoint::GetInstance()->LoadDependency({
      {"WritableFileWriter::SyncWithoutFlush:1",
       "DBWALTest::SyncWALNotBlockWrite:1"},
      {"DBWALTest::SyncWALNotBlockWrite:2",
       "WritableFileWriter::SyncWithoutFlush:2"},
  });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  std::thread thread([&]() { ASSERT_OK(db_->SyncWAL()); });

  TEST_SYNC_POINT("DBWALTest::SyncWALNotBlockWrite:1");
  ASSERT_OK(Put("foo2", "bar2"));
  ASSERT_OK(Put("foo3", "bar3"));
  FlushOptions fo;
  fo.wait = false;
  ASSERT_OK(db_->Flush(fo));
  ASSERT_OK(Put("foo4", "bar4"));

  TEST_SYNC_POINT("DBWALTest::SyncWALNotBlockWrite:2");

  thread.join();

  ASSERT_EQ(Get("foo1"), "bar1");
  ASSERT_EQ(Get("foo2"), "bar2");
  ASSERT_EQ(Get("foo3"), "bar3");
  ASSERT_EQ(Get("foo4"), "bar4");
  ASSERT_EQ(Get("foo5"), "bar5");
  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

TEST_F(DBWALTest, SyncWALNotWaitWrite) {
  ASSERT_OK(Put("foo1", "bar1"));
  ASSERT_OK(Put("foo3", "bar3"));

  rocksdb::SyncPoint::GetInstance()->LoadDependency({
      {"SpecialEnv::WalFile::Append:1", "DBWALTest::SyncWALNotWaitWrite:1"},
      {"DBWALTest::SyncWALNotWaitWrite:2", "SpecialEnv::WalFile::Append:2"},
  });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  std::thread thread([&]() { ASSERT_OK(Put("foo2", "bar2")); });
  TEST_SYNC_POINT("DBWALTest::SyncWALNotWaitWrite:1");
  ASSERT_OK(db_->SyncWAL());
  TEST_SYNC_POINT("DBWALTest::SyncWALNotWaitWrite:2");

  thread.join();

  ASSERT_EQ(Get("foo1"), "bar1");
  ASSERT_EQ(Get("foo2"), "bar2");
  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

TEST_F(DBWALTest, Recover) {
  do {
    CreateAndReopenWithCF({"pikachu"}, CurrentOptions());
    ASSERT_OK(Put(1, "foo", "v1"));
    ASSERT_OK(Put(1, "baz", "v5"));

    ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    ASSERT_EQ("v1", Get(1, "foo"));

    ASSERT_EQ("v1", Get(1, "foo"));
    ASSERT_EQ("v5", Get(1, "baz"));
    ASSERT_OK(Put(1, "bar", "v2"));
    ASSERT_OK(Put(1, "foo", "v3"));

    ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    ASSERT_EQ("v3", Get(1, "foo"));
    ASSERT_OK(Put(1, "foo", "v4"));
    ASSERT_EQ("v4", Get(1, "foo"));
    ASSERT_EQ("v2", Get(1, "bar"));
    ASSERT_EQ("v5", Get(1, "baz"));
  } while (ChangeOptions());
}

TEST_F(DBWALTest, RecoverWithTableHandle) {
  do {
    Options options = CurrentOptions();
    options.create_if_missing = true;
    options.disable_auto_compactions = true;
    options.avoid_flush_during_recovery = false;
    DestroyAndReopen(options);
    CreateAndReopenWithCF({"pikachu"}, options);

    ASSERT_OK(Put(1, "foo", "v1"));
    ASSERT_OK(Put(1, "bar", "v2"));
    ASSERT_OK(Flush(1));
    ASSERT_OK(Put(1, "foo", "v3"));
    ASSERT_OK(Put(1, "bar", "v4"));
    ASSERT_OK(Flush(1));
    ASSERT_OK(Put(1, "big", std::string(100, 'a')));
    ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());

    std::vector<std::vector<FileMetaData>> files;
    dbfull()->TEST_GetFilesMetaData(handles_[1], &files);
    size_t total_files = 0;
    for (const auto& level : files) {
      total_files += level.size();
    }
    ASSERT_EQ(total_files, 3);
    for (const auto& level : files) {
      for (const auto& file : level) {
        if (kInfiniteMaxOpenFiles == option_config_) {
          ASSERT_TRUE(file.table_reader_handle != nullptr);
        } else {
          ASSERT_TRUE(file.table_reader_handle == nullptr);
        }
      }
    }
  } while (ChangeOptions());
}

TEST_F(DBWALTest, IgnoreRecoveredLog) {
  std::string backup_logs = dbname_ + "/backup_logs";

  // delete old files in backup_logs directory
  env_->CreateDirIfMissing(backup_logs);
  std::vector<std::string> old_files;
  env_->GetChildren(backup_logs, &old_files);
  for (auto& file : old_files) {
    if (file != "." && file != "..") {
      env_->DeleteFile(backup_logs + "/" + file);
    }
  }

  do {
    Options options = CurrentOptions();
    options.create_if_missing = true;
    options.merge_operator = MergeOperators::CreateUInt64AddOperator();
    options.wal_dir = dbname_ + "/logs";
    DestroyAndReopen(options);

    // fill up the DB
    std::string one, two;
    PutFixed64(&one, 1);
    PutFixed64(&two, 2);
    ASSERT_OK(db_->Merge(WriteOptions(), Slice("foo"), Slice(one)));
    ASSERT_OK(db_->Merge(WriteOptions(), Slice("foo"), Slice(one)));
    ASSERT_OK(db_->Merge(WriteOptions(), Slice("bar"), Slice(one)));

    // copy the logs to backup
    std::vector<std::string> logs;
    env_->GetChildren(options.wal_dir, &logs);
    for (auto& log : logs) {
      if (log != ".." && log != ".") {
        CopyFile(options.wal_dir + "/" + log, backup_logs + "/" + log);
      }
    }

    // recover the DB
    Reopen(options);
    ASSERT_EQ(two, Get("foo"));
    ASSERT_EQ(one, Get("bar"));
    Close();

    // copy the logs from backup back to wal dir
    for (auto& log : logs) {
      if (log != ".." && log != ".") {
        CopyFile(backup_logs + "/" + log, options.wal_dir + "/" + log);
      }
    }
    // this should ignore the log files, recovery should not happen again
    // if the recovery happens, the same merge operator would be called twice,
    // leading to incorrect results
    Reopen(options);
    ASSERT_EQ(two, Get("foo"));
    ASSERT_EQ(one, Get("bar"));
    Close();
    Destroy(options);
    Reopen(options);
    Close();

    // copy the logs from backup back to wal dir
    env_->CreateDirIfMissing(options.wal_dir);
    for (auto& log : logs) {
      if (log != ".." && log != ".") {
        CopyFile(backup_logs + "/" + log, options.wal_dir + "/" + log);
      }
    }
    // assert that we successfully recovered only from logs, even though we
    // destroyed the DB
    Reopen(options);
    ASSERT_EQ(two, Get("foo"));
    ASSERT_EQ(one, Get("bar"));

    // Recovery will fail if DB directory doesn't exist.
    Destroy(options);
    // copy the logs from backup back to wal dir
    env_->CreateDirIfMissing(options.wal_dir);
    for (auto& log : logs) {
      if (log != ".." && log != ".") {
        CopyFile(backup_logs + "/" + log, options.wal_dir + "/" + log);
        // we won't be needing this file no more
        env_->DeleteFile(backup_logs + "/" + log);
      }
    }
    Status s = TryReopen(options);
    ASSERT_TRUE(!s.ok());
  } while (ChangeOptions(kSkipHashCuckoo));
}

TEST_F(DBWALTest, RecoveryWithEmptyLog) {
  do {
    CreateAndReopenWithCF({"pikachu"}, CurrentOptions());
    ASSERT_OK(Put(1, "foo", "v1"));
    ASSERT_OK(Put(1, "foo", "v2"));
    ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    ASSERT_OK(Put(1, "foo", "v3"));
    ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
    ASSERT_EQ("v3", Get(1, "foo"));
  } while (ChangeOptions());
}

#if !(defined NDEBUG) || !defined(OS_WIN)
TEST_F(DBWALTest, PreallocateBlock) {
  Options options = CurrentOptions();
  options.write_buffer_size = 10 * 1000 * 1000;
  options.max_total_wal_size = 0;

  size_t expected_preallocation_size = static_cast<size_t>(
      options.write_buffer_size + options.write_buffer_size / 10);

  DestroyAndReopen(options);

  std::atomic<int> called(0);
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBTestWalFile.GetPreallocationStatus", [&](void* arg) {
        ASSERT_TRUE(arg != nullptr);
        size_t preallocation_size = *(static_cast<size_t*>(arg));
        ASSERT_EQ(expected_preallocation_size, preallocation_size);
        called.fetch_add(1);
      });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();
  Put("", "");
  Flush();
  Put("", "");
  Close();
  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
  ASSERT_EQ(2, called.load());

  options.max_total_wal_size = 1000 * 1000;
  expected_preallocation_size = static_cast<size_t>(options.max_total_wal_size);
  Reopen(options);
  called.store(0);
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBTestWalFile.GetPreallocationStatus", [&](void* arg) {
        ASSERT_TRUE(arg != nullptr);
        size_t preallocation_size = *(static_cast<size_t*>(arg));
        ASSERT_EQ(expected_preallocation_size, preallocation_size);
        called.fetch_add(1);
      });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();
  Put("", "");
  Flush();
  Put("", "");
  Close();
  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
  ASSERT_EQ(2, called.load());

  options.db_write_buffer_size = 800 * 1000;
  expected_preallocation_size =
      static_cast<size_t>(options.db_write_buffer_size);
  Reopen(options);
  called.store(0);
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBTestWalFile.GetPreallocationStatus", [&](void* arg) {
        ASSERT_TRUE(arg != nullptr);
        size_t preallocation_size = *(static_cast<size_t*>(arg));
        ASSERT_EQ(expected_preallocation_size, preallocation_size);
        called.fetch_add(1);
      });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();
  Put("", "");
  Flush();
  Put("", "");
  Close();
  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
  ASSERT_EQ(2, called.load());

  expected_preallocation_size = 700 * 1000;
  std::shared_ptr<WriteBufferManager> write_buffer_manager =
      std::make_shared<WriteBufferManager>(static_cast<uint64_t>(700 * 1000));
  options.write_buffer_manager = write_buffer_manager;
  Reopen(options);
  called.store(0);
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBTestWalFile.GetPreallocationStatus", [&](void* arg) {
        ASSERT_TRUE(arg != nullptr);
        size_t preallocation_size = *(static_cast<size_t*>(arg));
        ASSERT_EQ(expected_preallocation_size, preallocation_size);
        called.fetch_add(1);
      });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();
  Put("", "");
  Flush();
  Put("", "");
  Close();
  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
  ASSERT_EQ(2, called.load());
}
#endif  // !(defined NDEBUG) || !defined(OS_WIN)

#ifndef ROCKSDB_LITE
TEST_F(DBWALTest, FullPurgePreservesRecycledLog) {
  // For github issue #1303
  for (int i = 0; i < 2; ++i) {
    Options options = CurrentOptions();
    options.create_if_missing = true;
    options.recycle_log_file_num = 2;
    if (i != 0) {
      options.wal_dir = alternative_wal_dir_;
    }

    DestroyAndReopen(options);
    ASSERT_OK(Put("foo", "v1"));
    VectorLogPtr log_files;
    ASSERT_OK(dbfull()->GetSortedWalFiles(log_files));
    ASSERT_GT(log_files.size(), 0);
    ASSERT_OK(Flush());

    // Now the original WAL is in log_files[0] and should be marked for
    // recycling.
    // Verify full purge cannot remove this file.
    JobContext job_context(0);
    dbfull()->TEST_LockMutex();
    dbfull()->FindObsoleteFiles(&job_context, true /* force */);
    dbfull()->TEST_UnlockMutex();
    dbfull()->PurgeObsoleteFiles(job_context);

    if (i == 0) {
      ASSERT_OK(
          env_->FileExists(LogFileName(dbname_, log_files[0]->LogNumber())));
    } else {
      ASSERT_OK(env_->FileExists(
          LogFileName(alternative_wal_dir_, log_files[0]->LogNumber())));
    }
  }
}

TEST_F(DBWALTest, GetSortedWalFiles) {
  do {
    CreateAndReopenWithCF({"pikachu"}, CurrentOptions());
    VectorLogPtr log_files;
    ASSERT_OK(dbfull()->GetSortedWalFiles(log_files));
    ASSERT_EQ(0, log_files.size());

    ASSERT_OK(Put(1, "foo", "v1"));
    ASSERT_OK(dbfull()->GetSortedWalFiles(log_files));
    ASSERT_EQ(1, log_files.size());
  } while (ChangeOptions());
}

TEST_F(DBWALTest, RecoveryWithLogDataForSomeCFs) {
  // Test for regression of WAL cleanup missing files that don't contain data
  // for every column family.
  do {
    CreateAndReopenWithCF({"pikachu"}, CurrentOptions());
    ASSERT_OK(Put(1, "foo", "v1"));
    ASSERT_OK(Put(1, "foo", "v2"));
    uint64_t earliest_log_nums[2];
    for (int i = 0; i < 2; ++i) {
      if (i > 0) {
        ReopenWithColumnFamilies({"default", "pikachu"}, CurrentOptions());
      }
      VectorLogPtr log_files;
      ASSERT_OK(dbfull()->GetSortedWalFiles(log_files));
      if (log_files.size() > 0) {
        earliest_log_nums[i] = log_files[0]->LogNumber();
      } else {
        earliest_log_nums[i] = port::kMaxUint64;
      }
    }
    // Check at least the first WAL was cleaned up during the recovery.
    ASSERT_LT(earliest_log_nums[0], earliest_log_nums[1]);
  } while (ChangeOptions());
}

TEST_F(DBWALTest, RecoverWithLargeLog) {
  do {
    {
      Options options = CurrentOptions();
      CreateAndReopenWithCF({"pikachu"}, options);
      ASSERT_OK(Put(1, "big1", std::string(200000, '1')));
      ASSERT_OK(Put(1, "big2", std::string(200000, '2')));
      ASSERT_OK(Put(1, "small3", std::string(10, '3')));
      ASSERT_OK(Put(1, "small4", std::string(10, '4')));
      ASSERT_EQ(NumTableFilesAtLevel(0, 1), 0);
    }

    // Make sure that if we re-open with a small write buffer size that
    // we flush table files in the middle of a large log file.
    Options options;
    options.write_buffer_size = 100000;
    options = CurrentOptions(options);
    ReopenWithColumnFamilies({"default", "pikachu"}, options);
    ASSERT_EQ(NumTableFilesAtLevel(0, 1), 3);
    ASSERT_EQ(std::string(200000, '1'), Get(1, "big1"));
    ASSERT_EQ(std::string(200000, '2'), Get(1, "big2"));
    ASSERT_EQ(std::string(10, '3'), Get(1, "small3"));
    ASSERT_EQ(std::string(10, '4'), Get(1, "small4"));
    ASSERT_GT(NumTableFilesAtLevel(0, 1), 1);
  } while (ChangeCompactOptions());
}

// In https://reviews.facebook.net/D20661 we change
// recovery behavior: previously for each log file each column family
// memtable was flushed, even it was empty. Now it's changed:
// we try to create the smallest number of table files by merging
// updates from multiple logs
TEST_F(DBWALTest, RecoverCheckFileAmountWithSmallWriteBuffer) {
  Options options = CurrentOptions();
  options.write_buffer_size = 5000000;
  CreateAndReopenWithCF({"pikachu", "dobrynia", "nikitich"}, options);

  // Since we will reopen DB with smaller write_buffer_size,
  // each key will go to new SST file
  ASSERT_OK(Put(1, Key(10), DummyString(1000000)));
  ASSERT_OK(Put(1, Key(10), DummyString(1000000)));
  ASSERT_OK(Put(1, Key(10), DummyString(1000000)));
  ASSERT_OK(Put(1, Key(10), DummyString(1000000)));

  ASSERT_OK(Put(3, Key(10), DummyString(1)));
  // Make 'dobrynia' to be flushed and new WAL file to be created
  ASSERT_OK(Put(2, Key(10), DummyString(7500000)));
  ASSERT_OK(Put(2, Key(1), DummyString(1)));
  dbfull()->TEST_WaitForFlushMemTable(handles_[2]);
  {
    auto tables = ListTableFiles(env_, dbname_);
    ASSERT_EQ(tables.size(), static_cast<size_t>(1));
    // Make sure 'dobrynia' was flushed: check sst files amount
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "dobrynia"),
              static_cast<uint64_t>(1));
  }
  // New WAL file
  ASSERT_OK(Put(1, Key(1), DummyString(1)));
  ASSERT_OK(Put(1, Key(1), DummyString(1)));
  ASSERT_OK(Put(3, Key(10), DummyString(1)));
  ASSERT_OK(Put(3, Key(10), DummyString(1)));
  ASSERT_OK(Put(3, Key(10), DummyString(1)));

  options.write_buffer_size = 4096;
  options.arena_block_size = 4096;
  ReopenWithColumnFamilies({"default", "pikachu", "dobrynia", "nikitich"},
                           options);
  {
    // No inserts => default is empty
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "default"),
              static_cast<uint64_t>(0));
    // First 4 keys goes to separate SSTs + 1 more SST for 2 smaller keys
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "pikachu"),
              static_cast<uint64_t>(5));
    // 1 SST for big key + 1 SST for small one
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "dobrynia"),
              static_cast<uint64_t>(2));
    // 1 SST for all keys
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "nikitich"),
              static_cast<uint64_t>(1));
  }
}

// In https://reviews.facebook.net/D20661 we change
// recovery behavior: previously for each log file each column family
// memtable was flushed, even it wasn't empty. Now it's changed:
// we try to create the smallest number of table files by merging
// updates from multiple logs
TEST_F(DBWALTest, RecoverCheckFileAmount) {
  Options options = CurrentOptions();
  options.write_buffer_size = 100000;
  options.arena_block_size = 4 * 1024;
  options.avoid_flush_during_recovery = false;
  CreateAndReopenWithCF({"pikachu", "dobrynia", "nikitich"}, options);

  ASSERT_OK(Put(0, Key(1), DummyString(1)));
  ASSERT_OK(Put(1, Key(1), DummyString(1)));
  ASSERT_OK(Put(2, Key(1), DummyString(1)));

  // Make 'nikitich' memtable to be flushed
  ASSERT_OK(Put(3, Key(10), DummyString(1002400)));
  ASSERT_OK(Put(3, Key(1), DummyString(1)));
  dbfull()->TEST_WaitForFlushMemTable(handles_[3]);
  // 4 memtable are not flushed, 1 sst file
  {
    auto tables = ListTableFiles(env_, dbname_);
    ASSERT_EQ(tables.size(), static_cast<size_t>(1));
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "nikitich"),
              static_cast<uint64_t>(1));
  }
  // Memtable for 'nikitich' has flushed, new WAL file has opened
  // 4 memtable still not flushed

  // Write to new WAL file
  ASSERT_OK(Put(0, Key(1), DummyString(1)));
  ASSERT_OK(Put(1, Key(1), DummyString(1)));
  ASSERT_OK(Put(2, Key(1), DummyString(1)));

  // Fill up 'nikitich' one more time
  ASSERT_OK(Put(3, Key(10), DummyString(1002400)));
  // make it flush
  ASSERT_OK(Put(3, Key(1), DummyString(1)));
  dbfull()->TEST_WaitForFlushMemTable(handles_[3]);
  // There are still 4 memtable not flushed, and 2 sst tables
  ASSERT_OK(Put(0, Key(1), DummyString(1)));
  ASSERT_OK(Put(1, Key(1), DummyString(1)));
  ASSERT_OK(Put(2, Key(1), DummyString(1)));

  {
    auto tables = ListTableFiles(env_, dbname_);
    ASSERT_EQ(tables.size(), static_cast<size_t>(2));
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "nikitich"),
              static_cast<uint64_t>(2));
  }

  ReopenWithColumnFamilies({"default", "pikachu", "dobrynia", "nikitich"},
                           options);
  {
    std::vector<uint64_t> table_files = ListTableFiles(env_, dbname_);
    // Check, that records for 'default', 'dobrynia' and 'pikachu' from
    // first, second and third WALs  went to the same SST.
    // So, there is 6 SSTs: three  for 'nikitich', one for 'default', one for
    // 'dobrynia', one for 'pikachu'
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "default"),
              static_cast<uint64_t>(1));
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "nikitich"),
              static_cast<uint64_t>(3));
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "dobrynia"),
              static_cast<uint64_t>(1));
    ASSERT_EQ(GetNumberOfSstFilesForColumnFamily(db_, "pikachu"),
              static_cast<uint64_t>(1));
  }
}

TEST_F(DBWALTest, SyncMultipleLogs) {
  const uint64_t kNumBatches = 2;
  const int kBatchSize = 1000;

  Options options = CurrentOptions();
  options.create_if_missing = true;
  options.write_buffer_size = 4096;
  Reopen(options);

  WriteBatch batch;
  WriteOptions wo;
  wo.sync = true;

  for (uint64_t b = 0; b < kNumBatches; b++) {
    batch.Clear();
    for (int i = 0; i < kBatchSize; i++) {
      batch.Put(Key(i), DummyString(128));
    }

    dbfull()->Write(wo, &batch);
  }

  ASSERT_OK(dbfull()->SyncWAL());
}

// Github issue 1339. Prior the fix we read sequence id from the first log to
// a local variable, then keep increase the variable as we replay logs,
// ignoring actual sequence id of the records. This is incorrect if some writes
// come with WAL disabled.
TEST_F(DBWALTest, PartOfWritesWithWALDisabled) {
  std::unique_ptr<FaultInjectionTestEnv> fault_env(
      new FaultInjectionTestEnv(env_));
  Options options = CurrentOptions();
  options.env = fault_env.get();
  options.disable_auto_compactions = true;
  WriteOptions wal_on, wal_off;
  wal_on.sync = true;
  wal_on.disableWAL = false;
  wal_off.disableWAL = true;
  CreateAndReopenWithCF({"dummy"}, options);
  ASSERT_OK(Put(1, "dummy", "d1", wal_on));  // seq id 1
  ASSERT_OK(Put(1, "dummy", "d2", wal_off));
  ASSERT_OK(Put(1, "dummy", "d3", wal_off));
  ASSERT_OK(Put(0, "key", "v4", wal_on));  // seq id 4
  ASSERT_OK(Flush(0));
  ASSERT_OK(Put(0, "key", "v5", wal_on));  // seq id 5
  ASSERT_EQ("v5", Get(0, "key"));
  // Simulate a crash.
  fault_env->SetFilesystemActive(false);
  Close();
  fault_env->ResetState();
  ReopenWithColumnFamilies({"default", "dummy"}, options);
  // Prior to the fix, we may incorrectly recover "v5" with sequence id = 3.
  ASSERT_EQ("v5", Get(0, "key"));
  // Destroy DB before destruct fault_env.
  Destroy(options);
}

//
// Test WAL recovery for the various modes available
//
class RecoveryTestHelper {
 public:
  // Number of WAL files to generate
  static const int kWALFilesCount = 10;
  // Starting number for the WAL file name like 00010.log
  static const int kWALFileOffset = 10;
  // Keys to be written per WAL file
  static const int kKeysPerWALFile = 1024;
  // Size of the value
  static const int kValueSize = 10;

  // Create WAL files with values filled in
  static void FillData(DBWALTest* test, const Options& options,
                       const size_t wal_count, size_t* count) {
    const ImmutableDBOptions db_options(options);

    *count = 0;

    shared_ptr<Cache> table_cache = NewLRUCache(50000, 16);
    EnvOptions env_options;
    WriteBufferManager write_buffer_manager(db_options.db_write_buffer_size);

    unique_ptr<VersionSet> versions;
    unique_ptr<WalManager> wal_manager;
    WriteController write_controller;

    versions.reset(new VersionSet(test->dbname_, &db_options, env_options,
                                  table_cache.get(), &write_buffer_manager,
                                  &write_controller));

    wal_manager.reset(new WalManager(db_options, env_options));

    std::unique_ptr<log::Writer> current_log_writer;

    for (size_t j = kWALFileOffset; j < wal_count + kWALFileOffset; j++) {
      uint64_t current_log_number = j;
      std::string fname = LogFileName(test->dbname_, current_log_number);
      unique_ptr<WritableFile> file;
      ASSERT_OK(db_options.env->NewWritableFile(fname, &file, env_options));
      unique_ptr<WritableFileWriter> file_writer(
          new WritableFileWriter(std::move(file), env_options));
      current_log_writer.reset(
          new log::Writer(std::move(file_writer), current_log_number,
                          db_options.recycle_log_file_num > 0));

      for (int i = 0; i < kKeysPerWALFile; i++) {
        std::string key = "key" + ToString((*count)++);
        std::string value = test->DummyString(kValueSize);
        assert(current_log_writer.get() != nullptr);
        uint64_t seq = versions->LastSequence() + 1;
        WriteBatch batch;
        batch.Put(key, value);
        WriteBatchInternal::SetSequence(&batch, seq);
        current_log_writer->AddRecord(WriteBatchInternal::Contents(&batch));
        versions->SetLastSequence(seq);
      }
    }
  }

  // Recreate and fill the store with some data
  static size_t FillData(DBWALTest* test, Options* options) {
    options->create_if_missing = true;
    test->DestroyAndReopen(*options);
    test->Close();

    size_t count = 0;
    FillData(test, *options, kWALFilesCount, &count);
    return count;
  }

  // Read back all the keys we wrote and return the number of keys found
  static size_t GetData(DBWALTest* test) {
    size_t count = 0;
    for (size_t i = 0; i < kWALFilesCount * kKeysPerWALFile; i++) {
      if (test->Get("key" + ToString(i)) != "NOT_FOUND") {
        ++count;
      }
    }
    return count;
  }

  // Manuall corrupt the specified WAL
  static void CorruptWAL(DBWALTest* test, const Options& options,
                         const double off, const double len,
                         const int wal_file_id, const bool trunc = false) {
    Env* env = options.env;
    std::string fname = LogFileName(test->dbname_, wal_file_id);
    uint64_t size;
    ASSERT_OK(env->GetFileSize(fname, &size));
    ASSERT_GT(size, 0);
#ifdef OS_WIN
    // Windows disk cache behaves differently. When we truncate
    // the original content is still in the cache due to the original
    // handle is still open. Generally, in Windows, one prohibits
    // shared access to files and it is not needed for WAL but we allow
    // it to induce corruption at various tests.
    test->Close();
#endif
    if (trunc) {
      ASSERT_EQ(0, truncate(fname.c_str(), static_cast<int64_t>(size * off)));
    } else {
      InduceCorruption(fname, static_cast<size_t>(size * off),
                       static_cast<size_t>(size * len));
    }
  }

  // Overwrite data with 'a' from offset for length len
  static void InduceCorruption(const std::string& filename, size_t offset,
                               size_t len) {
    ASSERT_GT(len, 0U);

    int fd = open(filename.c_str(), O_RDWR);

    // On windows long is 32-bit
    ASSERT_LE(offset, std::numeric_limits<long>::max());

    ASSERT_GT(fd, 0);
    ASSERT_EQ(offset, lseek(fd, static_cast<long>(offset), SEEK_SET));

    void* buf = alloca(len);
    memset(buf, 'a', len);
    ASSERT_EQ(len, write(fd, buf, static_cast<unsigned int>(len)));

    close(fd);
  }
};

// Test scope:
// - We expect to open the data store when there is incomplete trailing writes
// at the end of any of the logs
// - We do not expect to open the data store for corruption
TEST_F(DBWALTest, kTolerateCorruptedTailRecords) {
  const int jstart = RecoveryTestHelper::kWALFileOffset;
  const int jend = jstart + RecoveryTestHelper::kWALFilesCount;

  for (auto trunc : {true, false}) {        /* Corruption style */
    for (int i = 0; i < 4; i++) {           /* Corruption offset position */
      for (int j = jstart; j < jend; j++) { /* WAL file */
        // Fill data for testing
        Options options = CurrentOptions();
        const size_t row_count = RecoveryTestHelper::FillData(this, &options);
        // test checksum failure or parsing
        RecoveryTestHelper::CorruptWAL(this, options, /*off=*/i * .3,
                                       /*len%=*/.1, /*wal=*/j, trunc);

        if (trunc) {
          options.wal_recovery_mode =
              WALRecoveryMode::kTolerateCorruptedTailRecords;
          options.create_if_missing = false;
          ASSERT_OK(TryReopen(options));
          const size_t recovered_row_count = RecoveryTestHelper::GetData(this);
          ASSERT_TRUE(i == 0 || recovered_row_count > 0);
          ASSERT_LT(recovered_row_count, row_count);
        } else {
          options.wal_recovery_mode =
              WALRecoveryMode::kTolerateCorruptedTailRecords;
          ASSERT_NOK(TryReopen(options));
        }
      }
    }
  }
}

// Test scope:
// We don't expect the data store to be opened if there is any corruption
// (leading, middle or trailing -- incomplete writes or corruption)
TEST_F(DBWALTest, kAbsoluteConsistency) {
  const int jstart = RecoveryTestHelper::kWALFileOffset;
  const int jend = jstart + RecoveryTestHelper::kWALFilesCount;

  // Verify clean slate behavior
  Options options = CurrentOptions();
  const size_t row_count = RecoveryTestHelper::FillData(this, &options);
  options.wal_recovery_mode = WALRecoveryMode::kAbsoluteConsistency;
  options.create_if_missing = false;
  ASSERT_OK(TryReopen(options));
  ASSERT_EQ(RecoveryTestHelper::GetData(this), row_count);

  for (auto trunc : {true, false}) { /* Corruption style */
    for (int i = 0; i < 4; i++) {    /* Corruption offset position */
      if (trunc && i == 0) {
        continue;
      }

      for (int j = jstart; j < jend; j++) { /* wal files */
        // fill with new date
        RecoveryTestHelper::FillData(this, &options);
        // corrupt the wal
        RecoveryTestHelper::CorruptWAL(this, options, /*off=*/i * .3,
                                       /*len%=*/.1, j, trunc);
        // verify
        options.wal_recovery_mode = WALRecoveryMode::kAbsoluteConsistency;
        options.create_if_missing = false;
        ASSERT_NOK(TryReopen(options));
      }
    }
  }
}

// Test scope:
// - We expect to open data store under all circumstances
// - We expect only data upto the point where the first error was encountered
TEST_F(DBWALTest, kPointInTimeRecovery) {
  const int jstart = RecoveryTestHelper::kWALFileOffset;
  const int jend = jstart + RecoveryTestHelper::kWALFilesCount;
  const int maxkeys =
      RecoveryTestHelper::kWALFilesCount * RecoveryTestHelper::kKeysPerWALFile;

  for (auto trunc : {true, false}) {        /* Corruption style */
    for (int i = 0; i < 4; i++) {           /* Offset of corruption */
      for (int j = jstart; j < jend; j++) { /* WAL file */
        // Fill data for testing
        Options options = CurrentOptions();
        const size_t row_count = RecoveryTestHelper::FillData(this, &options);

        // Corrupt the wal
        RecoveryTestHelper::CorruptWAL(this, options, /*off=*/i * .3,
                                       /*len%=*/.1, j, trunc);

        // Verify
        options.wal_recovery_mode = WALRecoveryMode::kPointInTimeRecovery;
        options.create_if_missing = false;
        ASSERT_OK(TryReopen(options));

        // Probe data for invariants
        size_t recovered_row_count = RecoveryTestHelper::GetData(this);
        ASSERT_LT(recovered_row_count, row_count);

        bool expect_data = true;
        for (size_t k = 0; k < maxkeys; ++k) {
          bool found = Get("key" + ToString(i)) != "NOT_FOUND";
          if (expect_data && !found) {
            expect_data = false;
          }
          ASSERT_EQ(found, expect_data);
        }

        const size_t min = RecoveryTestHelper::kKeysPerWALFile *
                           (j - RecoveryTestHelper::kWALFileOffset);
        ASSERT_GE(recovered_row_count, min);
        if (!trunc && i != 0) {
          const size_t max = RecoveryTestHelper::kKeysPerWALFile *
                             (j - RecoveryTestHelper::kWALFileOffset + 1);
          ASSERT_LE(recovered_row_count, max);
        }
      }
    }
  }
}

// Test scope:
// - We expect to open the data store under all scenarios
// - We expect to have recovered records past the corruption zone
TEST_F(DBWALTest, kSkipAnyCorruptedRecords) {
  const int jstart = RecoveryTestHelper::kWALFileOffset;
  const int jend = jstart + RecoveryTestHelper::kWALFilesCount;

  for (auto trunc : {true, false}) {        /* Corruption style */
    for (int i = 0; i < 4; i++) {           /* Corruption offset */
      for (int j = jstart; j < jend; j++) { /* wal files */
        // Fill data for testing
        Options options = CurrentOptions();
        const size_t row_count = RecoveryTestHelper::FillData(this, &options);

        // Corrupt the WAL
        RecoveryTestHelper::CorruptWAL(this, options, /*off=*/i * .3,
                                       /*len%=*/.1, j, trunc);

        // Verify behavior
        options.wal_recovery_mode = WALRecoveryMode::kSkipAnyCorruptedRecords;
        options.create_if_missing = false;
        ASSERT_OK(TryReopen(options));

        // Probe data for invariants
        size_t recovered_row_count = RecoveryTestHelper::GetData(this);
        ASSERT_LT(recovered_row_count, row_count);

        if (!trunc) {
          ASSERT_TRUE(i != 0 || recovered_row_count > 0);
        }
      }
    }
  }
}

TEST_F(DBWALTest, AvoidFlushDuringRecovery) {
  Options options = CurrentOptions();
  options.disable_auto_compactions = true;
  options.avoid_flush_during_recovery = false;

  // Test with flush after recovery.
  Reopen(options);
  ASSERT_OK(Put("foo", "v1"));
  ASSERT_OK(Put("bar", "v2"));
  ASSERT_OK(Flush());
  ASSERT_OK(Put("foo", "v3"));
  ASSERT_OK(Put("bar", "v4"));
  ASSERT_EQ(1, TotalTableFiles());
  // Reopen DB. Check if WAL logs flushed.
  Reopen(options);
  ASSERT_EQ("v3", Get("foo"));
  ASSERT_EQ("v4", Get("bar"));
  ASSERT_EQ(2, TotalTableFiles());

  // Test without flush after recovery.
  options.avoid_flush_during_recovery = true;
  DestroyAndReopen(options);
  ASSERT_OK(Put("foo", "v5"));
  ASSERT_OK(Put("bar", "v6"));
  ASSERT_OK(Flush());
  ASSERT_OK(Put("foo", "v7"));
  ASSERT_OK(Put("bar", "v8"));
  ASSERT_EQ(1, TotalTableFiles());
  // Reopen DB. WAL logs should not be flushed this time.
  Reopen(options);
  ASSERT_EQ("v7", Get("foo"));
  ASSERT_EQ("v8", Get("bar"));
  ASSERT_EQ(1, TotalTableFiles());

  // Force flush with allow_2pc.
  options.avoid_flush_during_recovery = true;
  options.allow_2pc = true;
  ASSERT_OK(Put("foo", "v9"));
  ASSERT_OK(Put("bar", "v10"));
  ASSERT_OK(Flush());
  ASSERT_OK(Put("foo", "v11"));
  ASSERT_OK(Put("bar", "v12"));
  Reopen(options);
  ASSERT_EQ("v11", Get("foo"));
  ASSERT_EQ("v12", Get("bar"));
  ASSERT_EQ(2, TotalTableFiles());
}

TEST_F(DBWALTest, WalCleanupAfterAvoidFlushDuringRecovery) {
  // Verifies WAL files that were present during recovery, but not flushed due
  // to avoid_flush_during_recovery, will be considered for deletion at a later
  // stage. We check at least one such file is deleted during Flush().
  Options options = CurrentOptions();
  options.disable_auto_compactions = true;
  options.avoid_flush_during_recovery = true;
  Reopen(options);

  ASSERT_OK(Put("foo", "v1"));
  Reopen(options);
  for (int i = 0; i < 2; ++i) {
    if (i > 0) {
      // Flush() triggers deletion of obsolete tracked files
      Flush();
    }
    VectorLogPtr log_files;
    ASSERT_OK(dbfull()->GetSortedWalFiles(log_files));
    if (i == 0) {
      ASSERT_GT(log_files.size(), 0);
    } else {
      ASSERT_EQ(0, log_files.size());
    }
  }
}

TEST_F(DBWALTest, RecoverWithoutFlush) {
  Options options = CurrentOptions();
  options.avoid_flush_during_recovery = true;
  options.create_if_missing = false;
  options.disable_auto_compactions = true;
  options.write_buffer_size = 64 * 1024 * 1024;

  size_t count = RecoveryTestHelper::FillData(this, &options);
  auto validateData = [this, count]() {
    for (size_t i = 0; i < count; i++) {
      ASSERT_NE(Get("key" + ToString(i)), "NOT_FOUND");
    }
  };
  Reopen(options);
  validateData();
  // Insert some data without flush
  ASSERT_OK(Put("foo", "foo_v1"));
  ASSERT_OK(Put("bar", "bar_v1"));
  Reopen(options);
  validateData();
  ASSERT_EQ(Get("foo"), "foo_v1");
  ASSERT_EQ(Get("bar"), "bar_v1");
  // Insert again and reopen
  ASSERT_OK(Put("foo", "foo_v2"));
  ASSERT_OK(Put("bar", "bar_v2"));
  Reopen(options);
  validateData();
  ASSERT_EQ(Get("foo"), "foo_v2");
  ASSERT_EQ(Get("bar"), "bar_v2");
  // manual flush and insert again
  Flush();
  ASSERT_EQ(Get("foo"), "foo_v2");
  ASSERT_EQ(Get("bar"), "bar_v2");
  ASSERT_OK(Put("foo", "foo_v3"));
  ASSERT_OK(Put("bar", "bar_v3"));
  Reopen(options);
  validateData();
  ASSERT_EQ(Get("foo"), "foo_v3");
  ASSERT_EQ(Get("bar"), "bar_v3");
}

TEST_F(DBWALTest, RecoverWithoutFlushMultipleCF) {
  const std::string kSmallValue = "v";
  const std::string kLargeValue = DummyString(1024);
  Options options = CurrentOptions();
  options.avoid_flush_during_recovery = true;
  options.create_if_missing = false;
  options.disable_auto_compactions = true;

  auto countWalFiles = [this]() {
    VectorLogPtr log_files;
    dbfull()->GetSortedWalFiles(log_files);
    return log_files.size();
  };

  // Create DB with multiple column families and multiple log files.
  CreateAndReopenWithCF({"one", "two"}, options);
  ASSERT_OK(Put(0, "key1", kSmallValue));
  ASSERT_OK(Put(1, "key2", kLargeValue));
  Flush(1);
  ASSERT_EQ(1, countWalFiles());
  ASSERT_OK(Put(0, "key3", kSmallValue));
  ASSERT_OK(Put(2, "key4", kLargeValue));
  Flush(2);
  ASSERT_EQ(2, countWalFiles());

  // Reopen, insert and flush.
  options.db_write_buffer_size = 64 * 1024 * 1024;
  ReopenWithColumnFamilies({"default", "one", "two"}, options);
  ASSERT_EQ(Get(0, "key1"), kSmallValue);
  ASSERT_EQ(Get(1, "key2"), kLargeValue);
  ASSERT_EQ(Get(0, "key3"), kSmallValue);
  ASSERT_EQ(Get(2, "key4"), kLargeValue);
  // Insert more data.
  ASSERT_OK(Put(0, "key5", kLargeValue));
  ASSERT_OK(Put(1, "key6", kLargeValue));
  ASSERT_EQ(3, countWalFiles());
  Flush(1);
  ASSERT_OK(Put(2, "key7", kLargeValue));
  ASSERT_EQ(4, countWalFiles());

  // Reopen twice and validate.
  for (int i = 0; i < 2; i++) {
    ReopenWithColumnFamilies({"default", "one", "two"}, options);
    ASSERT_EQ(Get(0, "key1"), kSmallValue);
    ASSERT_EQ(Get(1, "key2"), kLargeValue);
    ASSERT_EQ(Get(0, "key3"), kSmallValue);
    ASSERT_EQ(Get(2, "key4"), kLargeValue);
    ASSERT_EQ(Get(0, "key5"), kLargeValue);
    ASSERT_EQ(Get(1, "key6"), kLargeValue);
    ASSERT_EQ(Get(2, "key7"), kLargeValue);
    ASSERT_EQ(4, countWalFiles());
  }
}

// In this test we are trying to do the following:
//   1. Create a DB with corrupted WAL log;
//   2. Open with avoid_flush_during_recovery = true;
//   3. Append more data without flushing, which creates new WAL log.
//   4. Open again. See if it can correctly handle previous corruption.
TEST_F(DBWALTest, RecoverFromCorruptedWALWithoutFlush) {
  const int jstart = RecoveryTestHelper::kWALFileOffset;
  const int jend = jstart + RecoveryTestHelper::kWALFilesCount;
  const int kAppendKeys = 100;
  Options options = CurrentOptions();
  options.avoid_flush_during_recovery = true;
  options.create_if_missing = false;
  options.disable_auto_compactions = true;
  options.write_buffer_size = 64 * 1024 * 1024;

  auto getAll = [this]() {
    std::vector<std::pair<std::string, std::string>> data;
    ReadOptions ropt;
    Iterator* iter = dbfull()->NewIterator(ropt);
    for (iter->SeekToFirst(); iter->Valid(); iter->Next()) {
      data.push_back(
          std::make_pair(iter->key().ToString(), iter->value().ToString()));
    }
    delete iter;
    return data;
  };
  for (auto& mode : wal_recovery_mode_string_map) {
    options.wal_recovery_mode = mode.second;
    for (auto trunc : {true, false}) {
      for (int i = 0; i < 4; i++) {
        for (int j = jstart; j < jend; j++) {
          // Create corrupted WAL
          RecoveryTestHelper::FillData(this, &options);
          RecoveryTestHelper::CorruptWAL(this, options, /*off=*/i * .3,
                                         /*len%=*/.1, /*wal=*/j, trunc);
          // Skip the test if DB won't open.
          if (!TryReopen(options).ok()) {
            ASSERT_TRUE(options.wal_recovery_mode ==
                            WALRecoveryMode::kAbsoluteConsistency ||
                        (!trunc &&
                         options.wal_recovery_mode ==
                             WALRecoveryMode::kTolerateCorruptedTailRecords));
            continue;
          }
          ASSERT_OK(TryReopen(options));
          // Append some more data.
          for (int k = 0; k < kAppendKeys; k++) {
            std::string key = "extra_key" + ToString(k);
            std::string value = DummyString(RecoveryTestHelper::kValueSize);
            ASSERT_OK(Put(key, value));
          }
          // Save data for comparison.
          auto data = getAll();
          // Reopen. Verify data.
          ASSERT_OK(TryReopen(options));
          auto actual_data = getAll();
          ASSERT_EQ(data, actual_data);
        }
      }
    }
  }
}

#endif  // ROCKSDB_LITE

TEST_F(DBWALTest, WalTermTest) {
  Options options = CurrentOptions();
  options.env = env_;
  CreateAndReopenWithCF({"pikachu"}, options);

  ASSERT_OK(Put(1, "foo", "bar"));

  WriteOptions wo;
  wo.sync = true;
  wo.disableWAL = false;

  WriteBatch batch;
  batch.Put("foo", "bar");
  batch.MarkWalTerminationPoint();
  batch.Put("foo2", "bar2");

  ASSERT_OK(dbfull()->Write(wo, &batch));

  // make sure we can re-open it.
  ASSERT_OK(TryReopenWithColumnFamilies({"default", "pikachu"}, options));
  ASSERT_EQ("bar", Get(1, "foo"));
  ASSERT_EQ("NOT_FOUND", Get(1, "foo2"));
}
}  // namespace rocksdb

int main(int argc, char** argv) {
  rocksdb::port::InstallStackTraceHandler();
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}
