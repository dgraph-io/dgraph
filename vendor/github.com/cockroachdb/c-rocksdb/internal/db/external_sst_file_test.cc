//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.

#include <functional>

#include "db/db_test_util.h"
#include "port/port.h"
#include "port/stack_trace.h"
#include "rocksdb/sst_file_writer.h"
#include "util/testutil.h"

namespace rocksdb {

#ifndef ROCKSDB_LITE
class ExternalSSTFileTest : public DBTestBase {
 public:
  ExternalSSTFileTest() : DBTestBase("/external_sst_file_test") {
    sst_files_dir_ = dbname_ + "/sst_files/";
    DestroyAndRecreateExternalSSTFilesDir();
  }

  void DestroyAndRecreateExternalSSTFilesDir() {
    test::DestroyDir(env_, sst_files_dir_);
    env_->CreateDir(sst_files_dir_);
  }

  Status GenerateAndAddExternalFile(
      const Options options,
      std::vector<std::pair<std::string, std::string>> data, int file_id = -1,
      bool allow_global_seqno = false, bool sort_data = false,
      std::map<std::string, std::string>* true_data = nullptr,
      ColumnFamilyHandle* cfh = nullptr) {
    // Generate a file id if not provided
    if (file_id == -1) {
      file_id = last_file_id_ + 1;
      last_file_id_++;
    }

    // Sort data if asked to do so
    if (sort_data) {
      std::sort(data.begin(), data.end(),
                [&](const std::pair<std::string, std::string>& e1,
                    const std::pair<std::string, std::string>& e2) {
                  return options.comparator->Compare(e1.first, e2.first) < 0;
                });
      auto uniq_iter = std::unique(
          data.begin(), data.end(),
          [&](const std::pair<std::string, std::string>& e1,
              const std::pair<std::string, std::string>& e2) {
            return options.comparator->Compare(e1.first, e2.first) == 0;
          });
      data.resize(uniq_iter - data.begin());
    }
    std::string file_path = sst_files_dir_ + ToString(file_id);
    SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator,
                                  cfh);

    Status s = sst_file_writer.Open(file_path);
    if (!s.ok()) {
      return s;
    }
    for (auto& entry : data) {
      s = sst_file_writer.Add(entry.first, entry.second);
      if (!s.ok()) {
        sst_file_writer.Finish();
        return s;
      }
    }
    s = sst_file_writer.Finish();

    if (s.ok()) {
      IngestExternalFileOptions ifo;
      ifo.allow_global_seqno = allow_global_seqno;
      if (cfh) {
        s = db_->IngestExternalFile(cfh, {file_path}, ifo);
      } else {
        s = db_->IngestExternalFile({file_path}, ifo);
      }
    }

    if (s.ok() && true_data) {
      for (auto& entry : data) {
        (*true_data)[entry.first] = entry.second;
      }
    }

    return s;
  }

  Status GenerateAndAddExternalFile(
      const Options options, std::vector<std::pair<int, std::string>> data,
      int file_id = -1, bool allow_global_seqno = false, bool sort_data = false,
      std::map<std::string, std::string>* true_data = nullptr,
      ColumnFamilyHandle* cfh = nullptr) {
    std::vector<std::pair<std::string, std::string>> file_data;
    for (auto& entry : data) {
      file_data.emplace_back(Key(entry.first), entry.second);
    }
    return GenerateAndAddExternalFile(options, file_data, file_id,
                                      allow_global_seqno, sort_data, true_data,
                                      cfh);
  }

  Status GenerateAndAddExternalFile(
      const Options options, std::vector<int> keys, int file_id = -1,
      bool allow_global_seqno = false, bool sort_data = false,
      std::map<std::string, std::string>* true_data = nullptr,
      ColumnFamilyHandle* cfh = nullptr) {
    std::vector<std::pair<std::string, std::string>> file_data;
    for (auto& k : keys) {
      file_data.emplace_back(Key(k), Key(k) + ToString(file_id));
    }
    return GenerateAndAddExternalFile(options, file_data, file_id,
                                      allow_global_seqno, sort_data, true_data,
                                      cfh);
  }

  Status DeprecatedAddFile(const std::vector<std::string>& files,
                           bool move_files = false,
                           bool skip_snapshot_check = false) {
    IngestExternalFileOptions opts;
    opts.move_files = move_files;
    opts.snapshot_consistency = !skip_snapshot_check;
    opts.allow_global_seqno = false;
    opts.allow_blocking_flush = false;
    return db_->IngestExternalFile(files, opts);
  }

  ~ExternalSSTFileTest() { test::DestroyDir(env_, sst_files_dir_); }

 protected:
  int last_file_id_ = 0;
  std::string sst_files_dir_;
};

TEST_F(ExternalSSTFileTest, Basic) {
  do {
    Options options = CurrentOptions();

    SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);

    // Current file size should be 0 after sst_file_writer init and before open a file.
    ASSERT_EQ(sst_file_writer.FileSize(), 0);

    // file1.sst (0 => 99)
    std::string file1 = sst_files_dir_ + "file1.sst";
    ASSERT_OK(sst_file_writer.Open(file1));
    for (int k = 0; k < 100; k++) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
    }
    ExternalSstFileInfo file1_info;
    Status s = sst_file_writer.Finish(&file1_info);
    ASSERT_TRUE(s.ok()) << s.ToString();

    // Current file size should be non-zero after success write.
    ASSERT_GT(sst_file_writer.FileSize(), 0);

    ASSERT_EQ(file1_info.file_path, file1);
    ASSERT_EQ(file1_info.num_entries, 100);
    ASSERT_EQ(file1_info.smallest_key, Key(0));
    ASSERT_EQ(file1_info.largest_key, Key(99));
    // sst_file_writer already finished, cannot add this value
    s = sst_file_writer.Add(Key(100), "bad_val");
    ASSERT_FALSE(s.ok()) << s.ToString();

    // file2.sst (100 => 199)
    std::string file2 = sst_files_dir_ + "file2.sst";
    ASSERT_OK(sst_file_writer.Open(file2));
    for (int k = 100; k < 200; k++) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
    }
    // Cannot add this key because it's not after last added key
    s = sst_file_writer.Add(Key(99), "bad_val");
    ASSERT_FALSE(s.ok()) << s.ToString();
    ExternalSstFileInfo file2_info;
    s = sst_file_writer.Finish(&file2_info);
    ASSERT_TRUE(s.ok()) << s.ToString();
    ASSERT_EQ(file2_info.file_path, file2);
    ASSERT_EQ(file2_info.num_entries, 100);
    ASSERT_EQ(file2_info.smallest_key, Key(100));
    ASSERT_EQ(file2_info.largest_key, Key(199));

    // file3.sst (195 => 299)
    // This file values overlap with file2 values
    std::string file3 = sst_files_dir_ + "file3.sst";
    ASSERT_OK(sst_file_writer.Open(file3));
    for (int k = 195; k < 300; k++) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val_overlap"));
    }
    ExternalSstFileInfo file3_info;
    s = sst_file_writer.Finish(&file3_info);

    ASSERT_TRUE(s.ok()) << s.ToString();
    // Current file size should be non-zero after success finish.
    ASSERT_GT(sst_file_writer.FileSize(), 0);
    ASSERT_EQ(file3_info.file_path, file3);
    ASSERT_EQ(file3_info.num_entries, 105);
    ASSERT_EQ(file3_info.smallest_key, Key(195));
    ASSERT_EQ(file3_info.largest_key, Key(299));

    // file4.sst (30 => 39)
    // This file values overlap with file1 values
    std::string file4 = sst_files_dir_ + "file4.sst";
    ASSERT_OK(sst_file_writer.Open(file4));
    for (int k = 30; k < 40; k++) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val_overlap"));
    }
    ExternalSstFileInfo file4_info;
    s = sst_file_writer.Finish(&file4_info);
    ASSERT_TRUE(s.ok()) << s.ToString();
    ASSERT_EQ(file4_info.file_path, file4);
    ASSERT_EQ(file4_info.num_entries, 10);
    ASSERT_EQ(file4_info.smallest_key, Key(30));
    ASSERT_EQ(file4_info.largest_key, Key(39));

    // file5.sst (400 => 499)
    std::string file5 = sst_files_dir_ + "file5.sst";
    ASSERT_OK(sst_file_writer.Open(file5));
    for (int k = 400; k < 500; k++) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
    }
    ExternalSstFileInfo file5_info;
    s = sst_file_writer.Finish(&file5_info);
    ASSERT_TRUE(s.ok()) << s.ToString();
    ASSERT_EQ(file5_info.file_path, file5);
    ASSERT_EQ(file5_info.num_entries, 100);
    ASSERT_EQ(file5_info.smallest_key, Key(400));
    ASSERT_EQ(file5_info.largest_key, Key(499));

    // Cannot create an empty sst file
    std::string file_empty = sst_files_dir_ + "file_empty.sst";
    ExternalSstFileInfo file_empty_info;
    s = sst_file_writer.Finish(&file_empty_info);
    ASSERT_NOK(s);

    DestroyAndReopen(options);
    // Add file using file path
    s = DeprecatedAddFile({file1});
    ASSERT_TRUE(s.ok()) << s.ToString();
    ASSERT_EQ(db_->GetLatestSequenceNumber(), 0U);
    for (int k = 0; k < 100; k++) {
      ASSERT_EQ(Get(Key(k)), Key(k) + "_val");
    }

    // Add file while holding a snapshot will fail
    const Snapshot* s1 = db_->GetSnapshot();
    if (s1 != nullptr) {
      ASSERT_NOK(DeprecatedAddFile({file2}));
      db_->ReleaseSnapshot(s1);
    }
    // We can add the file after releaseing the snapshot
    ASSERT_OK(DeprecatedAddFile({file2}));

    ASSERT_EQ(db_->GetLatestSequenceNumber(), 0U);
    for (int k = 0; k < 200; k++) {
      ASSERT_EQ(Get(Key(k)), Key(k) + "_val");
    }

    // This file has overlapping values with the exisitng data
    s = DeprecatedAddFile({file3});
    ASSERT_FALSE(s.ok()) << s.ToString();

    // This file has overlapping values with the exisitng data
    s = DeprecatedAddFile({file4});
    ASSERT_FALSE(s.ok()) << s.ToString();

    // Overwrite values of keys divisible by 5
    for (int k = 0; k < 200; k += 5) {
      ASSERT_OK(Put(Key(k), Key(k) + "_val_new"));
    }
    ASSERT_NE(db_->GetLatestSequenceNumber(), 0U);

    // Key range of file5 (400 => 499) dont overlap with any keys in DB
    ASSERT_OK(DeprecatedAddFile({file5}));

    // Make sure values are correct before and after flush/compaction
    for (int i = 0; i < 2; i++) {
      for (int k = 0; k < 200; k++) {
        std::string value = Key(k) + "_val";
        if (k % 5 == 0) {
          value += "_new";
        }
        ASSERT_EQ(Get(Key(k)), value);
      }
      for (int k = 400; k < 500; k++) {
        std::string value = Key(k) + "_val";
        ASSERT_EQ(Get(Key(k)), value);
      }
      ASSERT_OK(Flush());
      ASSERT_OK(db_->CompactRange(CompactRangeOptions(), nullptr, nullptr));
    }

    Close();
    options.disable_auto_compactions = true;
    Reopen(options);

    // Delete keys in range (400 => 499)
    for (int k = 400; k < 500; k++) {
      ASSERT_OK(Delete(Key(k)));
    }
    // We deleted range (400 => 499) but cannot add file5 because
    // of the range tombstones
    ASSERT_NOK(DeprecatedAddFile({file5}));

    // Compacting the DB will remove the tombstones
    ASSERT_OK(db_->CompactRange(CompactRangeOptions(), nullptr, nullptr));

    // Now we can add the file
    ASSERT_OK(DeprecatedAddFile({file5}));

    // Verify values of file5 in DB
    for (int k = 400; k < 500; k++) {
      std::string value = Key(k) + "_val";
      ASSERT_EQ(Get(Key(k)), value);
    }
    DestroyAndRecreateExternalSSTFilesDir();
  } while (ChangeOptions(kSkipPlainTable | kSkipUniversalCompaction |
                         kSkipFIFOCompaction));
}
class SstFileWriterCollector : public TablePropertiesCollector {
 public:
  explicit SstFileWriterCollector(const std::string prefix) : prefix_(prefix) {
    name_ = prefix_ + "_SstFileWriterCollector";
  }

  const char* Name() const override { return name_.c_str(); }

  Status Finish(UserCollectedProperties* properties) override {
    *properties = UserCollectedProperties{
        {prefix_ + "_SstFileWriterCollector", "YES"},
        {prefix_ + "_Count", std::to_string(count_)},
    };
    return Status::OK();
  }

  Status AddUserKey(const Slice& user_key, const Slice& value, EntryType type,
                    SequenceNumber seq, uint64_t file_size) override {
    ++count_;
    return Status::OK();
  }

  virtual UserCollectedProperties GetReadableProperties() const override {
    return UserCollectedProperties{};
  }

 private:
  uint32_t count_ = 0;
  std::string prefix_;
  std::string name_;
};

class SstFileWriterCollectorFactory : public TablePropertiesCollectorFactory {
 public:
  explicit SstFileWriterCollectorFactory(std::string prefix)
      : prefix_(prefix), num_created_(0) {}
  virtual TablePropertiesCollector* CreateTablePropertiesCollector(
      TablePropertiesCollectorFactory::Context context) override {
    num_created_++;
    return new SstFileWriterCollector(prefix_);
  }
  const char* Name() const override { return "SstFileWriterCollectorFactory"; }

  std::string prefix_;
  uint32_t num_created_;
};

TEST_F(ExternalSSTFileTest, AddList) {
  do {
    Options options = CurrentOptions();

    auto abc_collector = std::make_shared<SstFileWriterCollectorFactory>("abc");
    auto xyz_collector = std::make_shared<SstFileWriterCollectorFactory>("xyz");

    options.table_properties_collector_factories.emplace_back(abc_collector);
    options.table_properties_collector_factories.emplace_back(xyz_collector);

    SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);

    // file1.sst (0 => 99)
    std::string file1 = sst_files_dir_ + "file1.sst";
    ASSERT_OK(sst_file_writer.Open(file1));
    for (int k = 0; k < 100; k++) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
    }
    ExternalSstFileInfo file1_info;
    Status s = sst_file_writer.Finish(&file1_info);
    ASSERT_TRUE(s.ok()) << s.ToString();
    ASSERT_EQ(file1_info.file_path, file1);
    ASSERT_EQ(file1_info.num_entries, 100);
    ASSERT_EQ(file1_info.smallest_key, Key(0));
    ASSERT_EQ(file1_info.largest_key, Key(99));
    // sst_file_writer already finished, cannot add this value
    s = sst_file_writer.Add(Key(100), "bad_val");
    ASSERT_FALSE(s.ok()) << s.ToString();

    // file2.sst (100 => 199)
    std::string file2 = sst_files_dir_ + "file2.sst";
    ASSERT_OK(sst_file_writer.Open(file2));
    for (int k = 100; k < 200; k++) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
    }
    // Cannot add this key because it's not after last added key
    s = sst_file_writer.Add(Key(99), "bad_val");
    ASSERT_FALSE(s.ok()) << s.ToString();
    ExternalSstFileInfo file2_info;
    s = sst_file_writer.Finish(&file2_info);
    ASSERT_TRUE(s.ok()) << s.ToString();
    ASSERT_EQ(file2_info.file_path, file2);
    ASSERT_EQ(file2_info.num_entries, 100);
    ASSERT_EQ(file2_info.smallest_key, Key(100));
    ASSERT_EQ(file2_info.largest_key, Key(199));

    // file3.sst (195 => 199)
    // This file values overlap with file2 values
    std::string file3 = sst_files_dir_ + "file3.sst";
    ASSERT_OK(sst_file_writer.Open(file3));
    for (int k = 195; k < 200; k++) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val_overlap"));
    }
    ExternalSstFileInfo file3_info;
    s = sst_file_writer.Finish(&file3_info);
    ASSERT_TRUE(s.ok()) << s.ToString();
    ASSERT_EQ(file3_info.file_path, file3);
    ASSERT_EQ(file3_info.num_entries, 5);
    ASSERT_EQ(file3_info.smallest_key, Key(195));
    ASSERT_EQ(file3_info.largest_key, Key(199));

    // file4.sst (30 => 39)
    // This file values overlap with file1 values
    std::string file4 = sst_files_dir_ + "file4.sst";
    ASSERT_OK(sst_file_writer.Open(file4));
    for (int k = 30; k < 40; k++) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val_overlap"));
    }
    ExternalSstFileInfo file4_info;
    s = sst_file_writer.Finish(&file4_info);
    ASSERT_TRUE(s.ok()) << s.ToString();
    ASSERT_EQ(file4_info.file_path, file4);
    ASSERT_EQ(file4_info.num_entries, 10);
    ASSERT_EQ(file4_info.smallest_key, Key(30));
    ASSERT_EQ(file4_info.largest_key, Key(39));

    // file5.sst (200 => 299)
    std::string file5 = sst_files_dir_ + "file5.sst";
    ASSERT_OK(sst_file_writer.Open(file5));
    for (int k = 200; k < 300; k++) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
    }
    ExternalSstFileInfo file5_info;
    s = sst_file_writer.Finish(&file5_info);
    ASSERT_TRUE(s.ok()) << s.ToString();
    ASSERT_EQ(file5_info.file_path, file5);
    ASSERT_EQ(file5_info.num_entries, 100);
    ASSERT_EQ(file5_info.smallest_key, Key(200));
    ASSERT_EQ(file5_info.largest_key, Key(299));

    // list 1 has internal key range conflict
    std::vector<std::string> file_list0({file1, file2});
    std::vector<std::string> file_list1({file3, file2, file1});
    std::vector<std::string> file_list2({file5});
    std::vector<std::string> file_list3({file3, file4});

    DestroyAndReopen(options);

    // This list of files have key ranges are overlapping with each other
    s = DeprecatedAddFile(file_list1);
    ASSERT_FALSE(s.ok()) << s.ToString();

    // Add files using file path list
    s = DeprecatedAddFile(file_list0);
    ASSERT_TRUE(s.ok()) << s.ToString();
    ASSERT_EQ(db_->GetLatestSequenceNumber(), 0U);
    for (int k = 0; k < 200; k++) {
      ASSERT_EQ(Get(Key(k)), Key(k) + "_val");
    }

    TablePropertiesCollection props;
    ASSERT_OK(db_->GetPropertiesOfAllTables(&props));
    ASSERT_EQ(props.size(), 2);
    for (auto file_props : props) {
      auto user_props = file_props.second->user_collected_properties;
      ASSERT_EQ(user_props["abc_SstFileWriterCollector"], "YES");
      ASSERT_EQ(user_props["xyz_SstFileWriterCollector"], "YES");
      ASSERT_EQ(user_props["abc_Count"], "100");
      ASSERT_EQ(user_props["xyz_Count"], "100");
    }

    // Add file while holding a snapshot will fail
    const Snapshot* s1 = db_->GetSnapshot();
    if (s1 != nullptr) {
      ASSERT_NOK(DeprecatedAddFile(file_list2));
      db_->ReleaseSnapshot(s1);
    }
    // We can add the file after releaseing the snapshot
    ASSERT_OK(DeprecatedAddFile(file_list2));
    ASSERT_EQ(db_->GetLatestSequenceNumber(), 0U);
    for (int k = 0; k < 300; k++) {
      ASSERT_EQ(Get(Key(k)), Key(k) + "_val");
    }

    ASSERT_OK(db_->GetPropertiesOfAllTables(&props));
    ASSERT_EQ(props.size(), 3);
    for (auto file_props : props) {
      auto user_props = file_props.second->user_collected_properties;
      ASSERT_EQ(user_props["abc_SstFileWriterCollector"], "YES");
      ASSERT_EQ(user_props["xyz_SstFileWriterCollector"], "YES");
      ASSERT_EQ(user_props["abc_Count"], "100");
      ASSERT_EQ(user_props["xyz_Count"], "100");
    }

    // This file list has overlapping values with the exisitng data
    s = DeprecatedAddFile(file_list3);
    ASSERT_FALSE(s.ok()) << s.ToString();

    // Overwrite values of keys divisible by 5
    for (int k = 0; k < 200; k += 5) {
      ASSERT_OK(Put(Key(k), Key(k) + "_val_new"));
    }
    ASSERT_NE(db_->GetLatestSequenceNumber(), 0U);

    // Make sure values are correct before and after flush/compaction
    for (int i = 0; i < 2; i++) {
      for (int k = 0; k < 200; k++) {
        std::string value = Key(k) + "_val";
        if (k % 5 == 0) {
          value += "_new";
        }
        ASSERT_EQ(Get(Key(k)), value);
      }
      for (int k = 200; k < 300; k++) {
        std::string value = Key(k) + "_val";
        ASSERT_EQ(Get(Key(k)), value);
      }
      ASSERT_OK(Flush());
      ASSERT_OK(db_->CompactRange(CompactRangeOptions(), nullptr, nullptr));
    }

    // Delete keys in range (200 => 299)
    for (int k = 200; k < 300; k++) {
      ASSERT_OK(Delete(Key(k)));
    }
    // We deleted range (200 => 299) but cannot add file5 because
    // of the range tombstones
    ASSERT_NOK(DeprecatedAddFile(file_list2));

    // Compacting the DB will remove the tombstones
    ASSERT_OK(db_->CompactRange(CompactRangeOptions(), nullptr, nullptr));

    // Now we can add the file
    ASSERT_OK(DeprecatedAddFile(file_list2));

    // Verify values of file5 in DB
    for (int k = 200; k < 300; k++) {
      std::string value = Key(k) + "_val";
      ASSERT_EQ(Get(Key(k)), value);
    }
    DestroyAndRecreateExternalSSTFilesDir();
  } while (ChangeOptions(kSkipPlainTable | kSkipUniversalCompaction |
                         kSkipFIFOCompaction));
}

TEST_F(ExternalSSTFileTest, AddListAtomicity) {
  do {
    Options options = CurrentOptions();

    SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);

    // files[0].sst (0 => 99)
    // files[1].sst (100 => 199)
    // ...
    // file[8].sst (800 => 899)
    int n = 9;
    std::vector<std::string> files(n);
    std::vector<ExternalSstFileInfo> files_info(n);
    for (int i = 0; i < n; i++) {
      files[i] = sst_files_dir_ + "file" + std::to_string(i) + ".sst";
      ASSERT_OK(sst_file_writer.Open(files[i]));
      for (int k = i * 100; k < (i + 1) * 100; k++) {
        ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
      }
      Status s = sst_file_writer.Finish(&files_info[i]);
      ASSERT_TRUE(s.ok()) << s.ToString();
      ASSERT_EQ(files_info[i].file_path, files[i]);
      ASSERT_EQ(files_info[i].num_entries, 100);
      ASSERT_EQ(files_info[i].smallest_key, Key(i * 100));
      ASSERT_EQ(files_info[i].largest_key, Key((i + 1) * 100 - 1));
    }
    files.push_back(sst_files_dir_ + "file" + std::to_string(n) + ".sst");
    auto s = DeprecatedAddFile(files);
    ASSERT_NOK(s) << s.ToString();
    for (int k = 0; k < n * 100; k++) {
      ASSERT_EQ("NOT_FOUND", Get(Key(k)));
    }
    files.pop_back();
    ASSERT_OK(DeprecatedAddFile(files));
    for (int k = 0; k < n * 100; k++) {
      std::string value = Key(k) + "_val";
      ASSERT_EQ(Get(Key(k)), value);
    }
    DestroyAndRecreateExternalSSTFilesDir();
  } while (ChangeOptions(kSkipPlainTable | kSkipUniversalCompaction |
                         kSkipFIFOCompaction));
}
// This test reporduce a bug that can happen in some cases if the DB started
// purging obsolete files when we are adding an external sst file.
// This situation may result in deleting the file while it's being added.
TEST_F(ExternalSSTFileTest, PurgeObsoleteFilesBug) {
  Options options = CurrentOptions();
  SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);

  // file1.sst (0 => 500)
  std::string sst_file_path = sst_files_dir_ + "file1.sst";
  Status s = sst_file_writer.Open(sst_file_path);
  ASSERT_OK(s);
  for (int i = 0; i < 500; i++) {
    std::string k = Key(i);
    s = sst_file_writer.Add(k, k + "_val");
    ASSERT_OK(s);
  }

  ExternalSstFileInfo sst_file_info;
  s = sst_file_writer.Finish(&sst_file_info);
  ASSERT_OK(s);

  options.delete_obsolete_files_period_micros = 0;
  options.disable_auto_compactions = true;
  DestroyAndReopen(options);

  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBImpl::AddFile:FileCopied", [&](void* arg) {
        ASSERT_OK(Put("aaa", "bbb"));
        ASSERT_OK(Flush());
        ASSERT_OK(Put("aaa", "xxx"));
        ASSERT_OK(Flush());
        db_->CompactRange(CompactRangeOptions(), nullptr, nullptr);
      });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  s = DeprecatedAddFile({sst_file_path});
  ASSERT_OK(s);

  for (int i = 0; i < 500; i++) {
    std::string k = Key(i);
    std::string v = k + "_val";
    ASSERT_EQ(Get(k), v);
  }

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

TEST_F(ExternalSSTFileTest, NoCopy) {
  Options options = CurrentOptions();
  const ImmutableCFOptions ioptions(options);

  SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);

  // file1.sst (0 => 99)
  std::string file1 = sst_files_dir_ + "file1.sst";
  ASSERT_OK(sst_file_writer.Open(file1));
  for (int k = 0; k < 100; k++) {
    ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
  }
  ExternalSstFileInfo file1_info;
  Status s = sst_file_writer.Finish(&file1_info);
  ASSERT_TRUE(s.ok()) << s.ToString();
  ASSERT_EQ(file1_info.file_path, file1);
  ASSERT_EQ(file1_info.num_entries, 100);
  ASSERT_EQ(file1_info.smallest_key, Key(0));
  ASSERT_EQ(file1_info.largest_key, Key(99));

  // file2.sst (100 => 299)
  std::string file2 = sst_files_dir_ + "file2.sst";
  ASSERT_OK(sst_file_writer.Open(file2));
  for (int k = 100; k < 300; k++) {
    ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
  }
  ExternalSstFileInfo file2_info;
  s = sst_file_writer.Finish(&file2_info);
  ASSERT_TRUE(s.ok()) << s.ToString();
  ASSERT_EQ(file2_info.file_path, file2);
  ASSERT_EQ(file2_info.num_entries, 200);
  ASSERT_EQ(file2_info.smallest_key, Key(100));
  ASSERT_EQ(file2_info.largest_key, Key(299));

  // file3.sst (110 => 124) .. overlap with file2.sst
  std::string file3 = sst_files_dir_ + "file3.sst";
  ASSERT_OK(sst_file_writer.Open(file3));
  for (int k = 110; k < 125; k++) {
    ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val_overlap"));
  }
  ExternalSstFileInfo file3_info;
  s = sst_file_writer.Finish(&file3_info);
  ASSERT_TRUE(s.ok()) << s.ToString();
  ASSERT_EQ(file3_info.file_path, file3);
  ASSERT_EQ(file3_info.num_entries, 15);
  ASSERT_EQ(file3_info.smallest_key, Key(110));
  ASSERT_EQ(file3_info.largest_key, Key(124));
  s = DeprecatedAddFile({file1}, true /* move file */);
  ASSERT_TRUE(s.ok()) << s.ToString();
  ASSERT_EQ(Status::NotFound(), env_->FileExists(file1));

  s = DeprecatedAddFile({file2}, false /* copy file */);
  ASSERT_TRUE(s.ok()) << s.ToString();
  ASSERT_OK(env_->FileExists(file2));

  // This file have overlapping values with the exisitng data
  s = DeprecatedAddFile({file2}, true /* move file */);
  ASSERT_FALSE(s.ok()) << s.ToString();
  ASSERT_OK(env_->FileExists(file3));

  for (int k = 0; k < 300; k++) {
    ASSERT_EQ(Get(Key(k)), Key(k) + "_val");
  }
}

TEST_F(ExternalSSTFileTest, SkipSnapshot) {
  Options options = CurrentOptions();

  SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);

  // file1.sst (0 => 99)
  std::string file1 = sst_files_dir_ + "file1.sst";
  ASSERT_OK(sst_file_writer.Open(file1));
  for (int k = 0; k < 100; k++) {
    ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
  }
  ExternalSstFileInfo file1_info;
  Status s = sst_file_writer.Finish(&file1_info);
  ASSERT_TRUE(s.ok()) << s.ToString();
  ASSERT_EQ(file1_info.file_path, file1);
  ASSERT_EQ(file1_info.num_entries, 100);
  ASSERT_EQ(file1_info.smallest_key, Key(0));
  ASSERT_EQ(file1_info.largest_key, Key(99));

  // file2.sst (100 => 299)
  std::string file2 = sst_files_dir_ + "file2.sst";
  ASSERT_OK(sst_file_writer.Open(file2));
  for (int k = 100; k < 300; k++) {
    ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
  }
  ExternalSstFileInfo file2_info;
  s = sst_file_writer.Finish(&file2_info);
  ASSERT_TRUE(s.ok()) << s.ToString();
  ASSERT_EQ(file2_info.file_path, file2);
  ASSERT_EQ(file2_info.num_entries, 200);
  ASSERT_EQ(file2_info.smallest_key, Key(100));
  ASSERT_EQ(file2_info.largest_key, Key(299));

  ASSERT_OK(DeprecatedAddFile({file1}));

  // Add file will fail when holding snapshot and use the default
  // skip_snapshot_check to false
  const Snapshot* s1 = db_->GetSnapshot();
  if (s1 != nullptr) {
    ASSERT_NOK(DeprecatedAddFile({file2}));
  }

  // Add file will success when set skip_snapshot_check to true even db holding
  // snapshot
  if (s1 != nullptr) {
    ASSERT_OK(DeprecatedAddFile({file2}, false, true));
    db_->ReleaseSnapshot(s1);
  }

  // file3.sst (300 => 399)
  std::string file3 = sst_files_dir_ + "file3.sst";
  ASSERT_OK(sst_file_writer.Open(file3));
  for (int k = 300; k < 400; k++) {
    ASSERT_OK(sst_file_writer.Add(Key(k), Key(k) + "_val"));
  }
  ExternalSstFileInfo file3_info;
  s = sst_file_writer.Finish(&file3_info);
  ASSERT_TRUE(s.ok()) << s.ToString();
  ASSERT_EQ(file3_info.file_path, file3);
  ASSERT_EQ(file3_info.num_entries, 100);
  ASSERT_EQ(file3_info.smallest_key, Key(300));
  ASSERT_EQ(file3_info.largest_key, Key(399));

  // check that we have change the old key
  ASSERT_EQ(Get(Key(300)), "NOT_FOUND");
  const Snapshot* s2 = db_->GetSnapshot();
  ASSERT_OK(DeprecatedAddFile({file3}, false, true));
  ASSERT_EQ(Get(Key(300)), Key(300) + ("_val"));
  ASSERT_EQ(Get(Key(300), s2), Key(300) + ("_val"));

  db_->ReleaseSnapshot(s2);
}

TEST_F(ExternalSSTFileTest, MultiThreaded) {
  // Bulk load 10 files every file contain 1000 keys
  int num_files = 10;
  int keys_per_file = 1000;

  // Generate file names
  std::vector<std::string> file_names;
  for (int i = 0; i < num_files; i++) {
    std::string file_name = "file_" + ToString(i) + ".sst";
    file_names.push_back(sst_files_dir_ + file_name);
  }

  do {
    Options options = CurrentOptions();

    std::atomic<int> thread_num(0);
    std::function<void()> write_file_func = [&]() {
      int file_idx = thread_num.fetch_add(1);
      int range_start = file_idx * keys_per_file;
      int range_end = range_start + keys_per_file;

      SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);

      ASSERT_OK(sst_file_writer.Open(file_names[file_idx]));

      for (int k = range_start; k < range_end; k++) {
        ASSERT_OK(sst_file_writer.Add(Key(k), Key(k)));
      }

      Status s = sst_file_writer.Finish();
      ASSERT_TRUE(s.ok()) << s.ToString();
    };
    // Write num_files files in parallel
    std::vector<std::thread> sst_writer_threads;
    for (int i = 0; i < num_files; ++i) {
      sst_writer_threads.emplace_back(write_file_func);
    }

    for (auto& t : sst_writer_threads) {
      t.join();
    }

    fprintf(stderr, "Wrote %d files (%d keys)\n", num_files,
            num_files * keys_per_file);

    thread_num.store(0);
    std::atomic<int> files_added(0);
    // Thread 0 -> Load {f0,f1}
    // Thread 1 -> Load {f0,f1}
    // Thread 2 -> Load {f2,f3}
    // Thread 3 -> Load {f2,f3}
    // Thread 4 -> Load {f4,f5}
    // Thread 5 -> Load {f4,f5}
    // ...
    std::function<void()> load_file_func = [&]() {
      // We intentionally add every file twice, and assert that it was added
      // only once and the other add failed
      int thread_id = thread_num.fetch_add(1);
      int file_idx = (thread_id / 2) * 2;
      // sometimes we use copy, sometimes link .. the result should be the same
      bool move_file = (thread_id % 3 == 0);

      std::vector<std::string> files_to_add;

      files_to_add = {file_names[file_idx]};
      if (static_cast<size_t>(file_idx + 1) < file_names.size()) {
        files_to_add.push_back(file_names[file_idx + 1]);
      }

      Status s = DeprecatedAddFile(files_to_add, move_file);
      if (s.ok()) {
        files_added += static_cast<int>(files_to_add.size());
      }
    };

    // Bulk load num_files files in parallel
    std::vector<std::thread> add_file_threads;
    DestroyAndReopen(options);
    for (int i = 0; i < num_files; ++i) {
      add_file_threads.emplace_back(load_file_func);
    }

    for (auto& t : add_file_threads) {
      t.join();
    }
    ASSERT_EQ(files_added.load(), num_files);
    fprintf(stderr, "Loaded %d files (%d keys)\n", num_files,
            num_files * keys_per_file);

    // Overwrite values of keys divisible by 100
    for (int k = 0; k < num_files * keys_per_file; k += 100) {
      std::string key = Key(k);
      Status s = Put(key, key + "_new");
      ASSERT_TRUE(s.ok());
    }

    for (int i = 0; i < 2; i++) {
      // Make sure the values are correct before and after flush/compaction
      for (int k = 0; k < num_files * keys_per_file; ++k) {
        std::string key = Key(k);
        std::string value = (k % 100 == 0) ? (key + "_new") : key;
        ASSERT_EQ(Get(key), value);
      }
      ASSERT_OK(Flush());
      ASSERT_OK(db_->CompactRange(CompactRangeOptions(), nullptr, nullptr));
    }

    fprintf(stderr, "Verified %d values\n", num_files * keys_per_file);
    DestroyAndRecreateExternalSSTFilesDir();
  } while (ChangeOptions(kSkipPlainTable | kSkipUniversalCompaction |
                         kSkipFIFOCompaction));
}

TEST_F(ExternalSSTFileTest, OverlappingRanges) {
  Random rnd(301);
  do {
    Options options = CurrentOptions();
    DestroyAndReopen(options);

    SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);

    printf("Option config = %d\n", option_config_);
    std::vector<std::pair<int, int>> key_ranges;
    for (int i = 0; i < 500; i++) {
      int range_start = rnd.Uniform(20000);
      int keys_per_range = 10 + rnd.Uniform(41);

      key_ranges.emplace_back(range_start, range_start + keys_per_range);
    }

    int memtable_add = 0;
    int success_add_file = 0;
    int failed_add_file = 0;
    std::map<std::string, std::string> true_data;
    for (size_t i = 0; i < key_ranges.size(); i++) {
      int range_start = key_ranges[i].first;
      int range_end = key_ranges[i].second;

      Status s;
      std::string range_val = "range_" + ToString(i);

      // For 20% of ranges we use DB::Put, for 80% we use DB::AddFile
      if (i && i % 5 == 0) {
        // Use DB::Put to insert range (insert into memtable)
        range_val += "_put";
        for (int k = range_start; k <= range_end; k++) {
          s = Put(Key(k), range_val);
          ASSERT_OK(s);
        }
        memtable_add++;
      } else {
        // Use DB::AddFile to insert range
        range_val += "_add_file";

        // Generate the file containing the range
        std::string file_name = sst_files_dir_ + env_->GenerateUniqueId();
        ASSERT_OK(sst_file_writer.Open(file_name));
        for (int k = range_start; k <= range_end; k++) {
          s = sst_file_writer.Add(Key(k), range_val);
          ASSERT_OK(s);
        }
        ExternalSstFileInfo file_info;
        s = sst_file_writer.Finish(&file_info);
        ASSERT_OK(s);

        // Insert the generated file
        s = DeprecatedAddFile({file_name});

        auto it = true_data.lower_bound(Key(range_start));
        if (it != true_data.end() && it->first <= Key(range_end)) {
          // This range overlap with data already exist in DB
          ASSERT_NOK(s);
          failed_add_file++;
        } else {
          ASSERT_OK(s);
          success_add_file++;
        }
      }

      if (s.ok()) {
        // Update true_data map to include the new inserted data
        for (int k = range_start; k <= range_end; k++) {
          true_data[Key(k)] = range_val;
        }
      }

      // Flush / Compact the DB
      if (i && i % 50 == 0) {
        Flush();
      }
      if (i && i % 75 == 0) {
        db_->CompactRange(CompactRangeOptions(), nullptr, nullptr);
      }
    }

    printf("Total: %" ROCKSDB_PRIszt
           " ranges\n"
           "AddFile()|Success: %d ranges\n"
           "AddFile()|RangeConflict: %d ranges\n"
           "Put(): %d ranges\n",
           key_ranges.size(), success_add_file, failed_add_file, memtable_add);

    // Verify the correctness of the data
    for (const auto& kv : true_data) {
      ASSERT_EQ(Get(kv.first), kv.second);
    }
    printf("keys/values verified\n");
    DestroyAndRecreateExternalSSTFilesDir();
  } while (ChangeOptions(kSkipPlainTable | kSkipUniversalCompaction |
                         kSkipFIFOCompaction));
}

TEST_F(ExternalSSTFileTest, PickedLevel) {
  Options options = CurrentOptions();
  options.disable_auto_compactions = false;
  options.level0_file_num_compaction_trigger = 4;
  options.num_levels = 4;
  DestroyAndReopen(options);

  std::map<std::string, std::string> true_data;

  // File 0 will go to last level (L3)
  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 10}, -1, false, false,
                                       &true_data));
  EXPECT_EQ(FilesPerLevel(), "0,0,0,1");

  // File 1 will go to level L2 (since it overlap with file 0 in L3)
  ASSERT_OK(GenerateAndAddExternalFile(options, {2, 9}, -1, false, false,
                                       &true_data));
  EXPECT_EQ(FilesPerLevel(), "0,0,1,1");

  rocksdb::SyncPoint::GetInstance()->LoadDependency({
      {"ExternalSSTFileTest::PickedLevel:0", "BackgroundCallCompaction:0"},
      {"DBImpl::BackgroundCompaction:Start",
       "ExternalSSTFileTest::PickedLevel:1"},
      {"ExternalSSTFileTest::PickedLevel:2",
       "DBImpl::BackgroundCompaction:NonTrivial:AfterRun"},
  });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  // Flush 4 files containing the same keys
  for (int i = 0; i < 4; i++) {
    ASSERT_OK(Put(Key(3), Key(3) + "put"));
    ASSERT_OK(Put(Key(8), Key(8) + "put"));
    true_data[Key(3)] = Key(3) + "put";
    true_data[Key(8)] = Key(8) + "put";
    ASSERT_OK(Flush());
  }

  // Wait for BackgroundCompaction() to be called
  TEST_SYNC_POINT("ExternalSSTFileTest::PickedLevel:0");
  TEST_SYNC_POINT("ExternalSSTFileTest::PickedLevel:1");

  EXPECT_EQ(FilesPerLevel(), "4,0,1,1");

  // This file overlaps with file 0 (L3), file 1 (L2) and the
  // output of compaction going to L1
  ASSERT_OK(GenerateAndAddExternalFile(options, {4, 7}, -1, false, false,
                                       &true_data));
  EXPECT_EQ(FilesPerLevel(), "5,0,1,1");

  // This file does not overlap with any file or with the running compaction
  ASSERT_OK(GenerateAndAddExternalFile(options, {9000, 9001}, -1, false, false,
                                       &true_data));
  EXPECT_EQ(FilesPerLevel(), "5,0,1,2");

  // Hold compaction from finishing
  TEST_SYNC_POINT("ExternalSSTFileTest::PickedLevel:2");

  dbfull()->TEST_WaitForCompact();
  EXPECT_EQ(FilesPerLevel(), "1,1,1,2");

  size_t kcnt = 0;
  VerifyDBFromMap(true_data, &kcnt, false);

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

TEST_F(ExternalSSTFileTest, PickedLevelBug) {
  Options options = CurrentOptions();
  options.disable_auto_compactions = false;
  options.level0_file_num_compaction_trigger = 3;
  options.num_levels = 2;
  DestroyAndReopen(options);

  std::vector<int> file_keys;

  // file #1 in L0
  file_keys = {0, 5, 7};
  for (int k : file_keys) {
    ASSERT_OK(Put(Key(k), Key(k)));
  }
  ASSERT_OK(Flush());

  // file #2 in L0
  file_keys = {4, 6, 8, 9};
  for (int k : file_keys) {
    ASSERT_OK(Put(Key(k), Key(k)));
  }
  ASSERT_OK(Flush());

  // We have 2 overlapping files in L0
  EXPECT_EQ(FilesPerLevel(), "2");

  rocksdb::SyncPoint::GetInstance()->LoadDependency(
      {{"DBImpl::AddFile:MutexLock", "ExternalSSTFileTest::PickedLevelBug:0"},
       {"ExternalSSTFileTest::PickedLevelBug:1", "DBImpl::AddFile:MutexUnlock"},
       {"ExternalSSTFileTest::PickedLevelBug:2",
        "DBImpl::RunManualCompaction:0"},
       {"ExternalSSTFileTest::PickedLevelBug:3",
        "DBImpl::RunManualCompaction:1"}});

  std::atomic<bool> bg_compact_started(false);
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "DBImpl::BackgroundCompaction:Start",
      [&](void* arg) { bg_compact_started.store(true); });

  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  // While writing the MANIFEST start a thread that will ask for compaction
  std::thread bg_compact([&]() {
    ASSERT_OK(db_->CompactRange(CompactRangeOptions(), nullptr, nullptr));
  });
  TEST_SYNC_POINT("ExternalSSTFileTest::PickedLevelBug:2");

  // Start a thread that will ingest a new file
  std::thread bg_addfile([&]() {
    file_keys = {1, 2, 3};
    ASSERT_OK(GenerateAndAddExternalFile(options, file_keys, 1));
  });

  // Wait for AddFile to start picking levels and writing MANIFEST
  TEST_SYNC_POINT("ExternalSSTFileTest::PickedLevelBug:0");

  TEST_SYNC_POINT("ExternalSSTFileTest::PickedLevelBug:3");

  // We need to verify that no compactions can run while AddFile is
  // ingesting the files into the levels it find suitable. So we will
  // wait for 2 seconds to give a chance for compactions to run during
  // this period, and then make sure that no compactions where able to run
  env_->SleepForMicroseconds(1000000 * 2);
  ASSERT_FALSE(bg_compact_started.load());

  // Hold AddFile from finishing writing the MANIFEST
  TEST_SYNC_POINT("ExternalSSTFileTest::PickedLevelBug:1");

  bg_addfile.join();
  bg_compact.join();

  dbfull()->TEST_WaitForCompact();

  int total_keys = 0;
  Iterator* iter = db_->NewIterator(ReadOptions());
  for (iter->SeekToFirst(); iter->Valid(); iter->Next()) {
    ASSERT_OK(iter->status());
    total_keys++;
  }
  ASSERT_EQ(total_keys, 10);

  delete iter;

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

TEST_F(ExternalSSTFileTest, CompactDuringAddFileRandom) {
  Options options = CurrentOptions();
  options.disable_auto_compactions = false;
  options.level0_file_num_compaction_trigger = 2;
  options.num_levels = 2;
  DestroyAndReopen(options);

  std::function<void()> bg_compact = [&]() {
    ASSERT_OK(db_->CompactRange(CompactRangeOptions(), nullptr, nullptr));
  };

  int range_id = 0;
  std::vector<int> file_keys;
  std::function<void()> bg_addfile = [&]() {
    ASSERT_OK(GenerateAndAddExternalFile(options, file_keys, range_id));
  };

  std::vector<std::thread> threads;
  while (range_id < 5000) {
    int range_start = range_id * 10;
    int range_end = range_start + 10;

    file_keys.clear();
    for (int k = range_start + 1; k < range_end; k++) {
      file_keys.push_back(k);
    }
    ASSERT_OK(Put(Key(range_start), Key(range_start)));
    ASSERT_OK(Put(Key(range_end), Key(range_end)));
    ASSERT_OK(Flush());

    if (range_id % 10 == 0) {
      threads.emplace_back(bg_compact);
    }
    threads.emplace_back(bg_addfile);

    for (auto& t : threads) {
      t.join();
    }
    threads.clear();

    range_id++;
  }

  for (int rid = 0; rid < 5000; rid++) {
    int range_start = rid * 10;
    int range_end = range_start + 10;

    ASSERT_EQ(Get(Key(range_start)), Key(range_start)) << rid;
    ASSERT_EQ(Get(Key(range_end)), Key(range_end)) << rid;
    for (int k = range_start + 1; k < range_end; k++) {
      std::string v = Key(k) + ToString(rid);
      ASSERT_EQ(Get(Key(k)), v) << rid;
    }
  }
}

TEST_F(ExternalSSTFileTest, PickedLevelDynamic) {
  Options options = CurrentOptions();
  options.disable_auto_compactions = false;
  options.level0_file_num_compaction_trigger = 4;
  options.level_compaction_dynamic_level_bytes = true;
  options.num_levels = 4;
  DestroyAndReopen(options);
  std::map<std::string, std::string> true_data;

  rocksdb::SyncPoint::GetInstance()->LoadDependency({
      {"ExternalSSTFileTest::PickedLevelDynamic:0",
       "BackgroundCallCompaction:0"},
      {"DBImpl::BackgroundCompaction:Start",
       "ExternalSSTFileTest::PickedLevelDynamic:1"},
      {"ExternalSSTFileTest::PickedLevelDynamic:2",
       "DBImpl::BackgroundCompaction:NonTrivial:AfterRun"},
  });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  // Flush 4 files containing the same keys
  for (int i = 0; i < 4; i++) {
    for (int k = 20; k <= 30; k++) {
      ASSERT_OK(Put(Key(k), Key(k) + "put"));
      true_data[Key(k)] = Key(k) + "put";
    }
    for (int k = 50; k <= 60; k++) {
      ASSERT_OK(Put(Key(k), Key(k) + "put"));
      true_data[Key(k)] = Key(k) + "put";
    }
    ASSERT_OK(Flush());
  }

  // Wait for BackgroundCompaction() to be called
  TEST_SYNC_POINT("ExternalSSTFileTest::PickedLevelDynamic:0");
  TEST_SYNC_POINT("ExternalSSTFileTest::PickedLevelDynamic:1");

  // This file overlaps with the output of the compaction (going to L3)
  // so the file will be added to L0 since L3 is the base level
  ASSERT_OK(GenerateAndAddExternalFile(options, {31, 32, 33, 34}, -1, false,
                                       false, &true_data));
  EXPECT_EQ(FilesPerLevel(), "5");

  // This file does not overlap with the current running compactiong
  ASSERT_OK(GenerateAndAddExternalFile(options, {9000, 9001}, -1, false, false,
                                       &true_data));
  EXPECT_EQ(FilesPerLevel(), "5,0,0,1");

  // Hold compaction from finishing
  TEST_SYNC_POINT("ExternalSSTFileTest::PickedLevelDynamic:2");

  // Output of the compaction will go to L3
  dbfull()->TEST_WaitForCompact();
  EXPECT_EQ(FilesPerLevel(), "1,0,0,2");

  Close();
  options.disable_auto_compactions = true;
  Reopen(options);

  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 15, 19}, -1, false, false,
                                       &true_data));
  ASSERT_EQ(FilesPerLevel(), "1,0,0,3");

  ASSERT_OK(GenerateAndAddExternalFile(options, {1000, 1001, 1002}, -1, false,
                                       false, &true_data));
  ASSERT_EQ(FilesPerLevel(), "1,0,0,4");

  ASSERT_OK(GenerateAndAddExternalFile(options, {500, 600, 700}, -1, false,
                                       false, &true_data));
  ASSERT_EQ(FilesPerLevel(), "1,0,0,5");

  // File 5 overlaps with file 2 (L3 / base level)
  ASSERT_OK(GenerateAndAddExternalFile(options, {2, 10}, -1, false, false,
                                       &true_data));
  ASSERT_EQ(FilesPerLevel(), "2,0,0,5");

  // File 6 overlaps with file 2 (L3 / base level) and file 5 (L0)
  ASSERT_OK(GenerateAndAddExternalFile(options, {3, 9}, -1, false, false,
                                       &true_data));
  ASSERT_EQ(FilesPerLevel(), "3,0,0,5");

  // Verify data in files
  size_t kcnt = 0;
  VerifyDBFromMap(true_data, &kcnt, false);

  // Write range [5 => 10] to L0
  for (int i = 5; i <= 10; i++) {
    std::string k = Key(i);
    std::string v = k + "put";
    ASSERT_OK(Put(k, v));
    true_data[k] = v;
  }
  ASSERT_OK(Flush());
  ASSERT_EQ(FilesPerLevel(), "4,0,0,5");

  // File 7 overlaps with file 4 (L3)
  ASSERT_OK(GenerateAndAddExternalFile(options, {650, 651, 652}, -1, false,
                                       false, &true_data));
  ASSERT_EQ(FilesPerLevel(), "5,0,0,5");

  VerifyDBFromMap(true_data, &kcnt, false);

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

TEST_F(ExternalSSTFileTest, AddExternalSstFileWithCustomCompartor) {
  Options options = CurrentOptions();
  options.comparator = ReverseBytewiseComparator();
  DestroyAndReopen(options);

  SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);

  // Generate files with these key ranges
  // {14  -> 0}
  // {24 -> 10}
  // {34 -> 20}
  // {44 -> 30}
  // ..
  std::vector<std::string> generated_files;
  for (int i = 0; i < 10; i++) {
    std::string file_name = sst_files_dir_ + env_->GenerateUniqueId();
    ASSERT_OK(sst_file_writer.Open(file_name));

    int range_end = i * 10;
    int range_start = range_end + 15;
    for (int k = (range_start - 1); k >= range_end; k--) {
      ASSERT_OK(sst_file_writer.Add(Key(k), Key(k)));
    }
    ExternalSstFileInfo file_info;
    ASSERT_OK(sst_file_writer.Finish(&file_info));
    generated_files.push_back(file_name);
  }

  std::vector<std::string> in_files;

  // These 2nd and 3rd files overlap with each other
  in_files = {generated_files[0], generated_files[4], generated_files[5],
              generated_files[7]};
  ASSERT_NOK(DeprecatedAddFile(in_files));

  // These 2 files dont overlap with each other
  in_files = {generated_files[0], generated_files[2]};
  ASSERT_OK(DeprecatedAddFile(in_files));

  // These 2 files dont overlap with each other but overlap with keys in DB
  in_files = {generated_files[3], generated_files[7]};
  ASSERT_NOK(DeprecatedAddFile(in_files));

  // Files dont overlap and dont overlap with DB key range
  in_files = {generated_files[4], generated_files[6], generated_files[8]};
  ASSERT_OK(DeprecatedAddFile(in_files));

  for (int i = 0; i < 100; i++) {
    if (i % 20 <= 14) {
      ASSERT_EQ(Get(Key(i)), Key(i));
    } else {
      ASSERT_EQ(Get(Key(i)), "NOT_FOUND");
    }
  }
}

TEST_F(ExternalSSTFileTest, AddFileTrivialMoveBug) {
  Options options = CurrentOptions();
  options.num_levels = 3;
  options.IncreaseParallelism(20);
  DestroyAndReopen(options);

  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 4}, 1));  // L3
  ASSERT_OK(GenerateAndAddExternalFile(options, {2, 3}, 2));  // L2

  ASSERT_OK(GenerateAndAddExternalFile(options, {10, 14}, 3));  // L3
  ASSERT_OK(GenerateAndAddExternalFile(options, {12, 13}, 4));  // L2

  ASSERT_OK(GenerateAndAddExternalFile(options, {20, 24}, 5));  // L3
  ASSERT_OK(GenerateAndAddExternalFile(options, {22, 23}, 6));  // L2

  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "CompactionJob::Run():Start", [&](void* arg) {
        // fit in L3 but will overlap with compaction so will be added
        // to L2 but a compaction will trivially move it to L3
        // and break LSM consistency
        ASSERT_OK(dbfull()->SetOptions({{"max_bytes_for_level_base", "1"}}));
        ASSERT_OK(GenerateAndAddExternalFile(options, {15, 16}, 7));
      });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  CompactRangeOptions cro;
  cro.exclusive_manual_compaction = false;
  ASSERT_OK(db_->CompactRange(cro, nullptr, nullptr));

  dbfull()->TEST_WaitForCompact();

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

TEST_F(ExternalSSTFileTest, CompactAddedFiles) {
  Options options = CurrentOptions();
  options.num_levels = 3;
  DestroyAndReopen(options);

  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 10}, 1));  // L3
  ASSERT_OK(GenerateAndAddExternalFile(options, {2, 9}, 2));   // L2
  ASSERT_OK(GenerateAndAddExternalFile(options, {3, 8}, 3));   // L1
  ASSERT_OK(GenerateAndAddExternalFile(options, {4, 7}, 4));   // L0

  ASSERT_OK(db_->CompactRange(CompactRangeOptions(), nullptr, nullptr));
}

TEST_F(ExternalSSTFileTest, SstFileWriterNonSharedKeys) {
  Options options = CurrentOptions();
  DestroyAndReopen(options);
  std::string file_path = sst_files_dir_ + "/not_shared";
  SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);

  std::string suffix(100, 'X');
  ASSERT_OK(sst_file_writer.Open(file_path));
  ASSERT_OK(sst_file_writer.Add("A" + suffix, "VAL"));
  ASSERT_OK(sst_file_writer.Add("BB" + suffix, "VAL"));
  ASSERT_OK(sst_file_writer.Add("CC" + suffix, "VAL"));
  ASSERT_OK(sst_file_writer.Add("CXD" + suffix, "VAL"));
  ASSERT_OK(sst_file_writer.Add("CZZZ" + suffix, "VAL"));
  ASSERT_OK(sst_file_writer.Add("ZAAAX" + suffix, "VAL"));

  ASSERT_OK(sst_file_writer.Finish());
  ASSERT_OK(DeprecatedAddFile({file_path}));
}

TEST_F(ExternalSSTFileTest, IngestFileWithGlobalSeqnoRandomized) {
  Options options = CurrentOptions();
  options.IncreaseParallelism(20);
  options.level0_slowdown_writes_trigger = 256;
  options.level0_stop_writes_trigger = 256;

  for (int iter = 0; iter < 2; iter++) {
    bool write_to_memtable = (iter == 0);
    DestroyAndReopen(options);

    Random rnd(301);
    std::map<std::string, std::string> true_data;
    for (int i = 0; i < 2000; i++) {
      std::vector<std::pair<std::string, std::string>> random_data;
      for (int j = 0; j < 100; j++) {
        std::string k;
        std::string v;
        test::RandomString(&rnd, rnd.Next() % 20, &k);
        test::RandomString(&rnd, rnd.Next() % 50, &v);
        random_data.emplace_back(k, v);
      }

      if (write_to_memtable && rnd.OneIn(4)) {
        // 25% of writes go through memtable
        for (auto& entry : random_data) {
          ASSERT_OK(Put(entry.first, entry.second));
          true_data[entry.first] = entry.second;
        }
      } else {
        ASSERT_OK(GenerateAndAddExternalFile(options, random_data, -1, true,
                                             true, &true_data));
      }
    }
    size_t kcnt = 0;
    VerifyDBFromMap(true_data, &kcnt, false);
    db_->CompactRange(CompactRangeOptions(), nullptr, nullptr);
    VerifyDBFromMap(true_data, &kcnt, false);
  }
}

TEST_F(ExternalSSTFileTest, IngestFileWithGlobalSeqnoAssignedLevel) {
  Options options = CurrentOptions();
  options.num_levels = 5;
  options.disable_auto_compactions = true;
  DestroyAndReopen(options);
  std::vector<std::pair<std::string, std::string>> file_data;
  std::map<std::string, std::string> true_data;

  // Insert 100 -> 200 into the memtable
  for (int i = 100; i <= 200; i++) {
    ASSERT_OK(Put(Key(i), "memtable"));
    true_data[Key(i)] = "memtable";
  }

  // Insert 0 -> 20 using AddFile
  file_data.clear();
  for (int i = 0; i <= 20; i++) {
    file_data.emplace_back(Key(i), "L4");
  }
  ASSERT_OK(GenerateAndAddExternalFile(options, file_data, -1, true, false,
                                       &true_data));

  // This file dont overlap with anything in the DB, will go to L4
  ASSERT_EQ("0,0,0,0,1", FilesPerLevel());

  // Insert 80 -> 130 using AddFile
  file_data.clear();
  for (int i = 80; i <= 130; i++) {
    file_data.emplace_back(Key(i), "L0");
  }
  ASSERT_OK(GenerateAndAddExternalFile(options, file_data, -1, true, false,
                                       &true_data));

  // This file overlap with the memtable, so it will flush it and add
  // it self to L0
  ASSERT_EQ("2,0,0,0,1", FilesPerLevel());

  // Insert 30 -> 50 using AddFile
  file_data.clear();
  for (int i = 30; i <= 50; i++) {
    file_data.emplace_back(Key(i), "L4");
  }
  ASSERT_OK(GenerateAndAddExternalFile(options, file_data, -1, true, false,
                                       &true_data));

  // This file dont overlap with anything in the DB and fit in L4 as well
  ASSERT_EQ("2,0,0,0,2", FilesPerLevel());

  // Insert 10 -> 40 using AddFile
  file_data.clear();
  for (int i = 10; i <= 40; i++) {
    file_data.emplace_back(Key(i), "L3");
  }
  ASSERT_OK(GenerateAndAddExternalFile(options, file_data, -1, true, false,
                                       &true_data));

  // This file overlap with files in L4, we will ingest it in L3
  ASSERT_EQ("2,0,0,1,2", FilesPerLevel());

  size_t kcnt = 0;
  VerifyDBFromMap(true_data, &kcnt, false);
}

TEST_F(ExternalSSTFileTest, IngestFileWithGlobalSeqnoPickedSeqno) {
  Options options = CurrentOptions();
  DestroyAndReopen(options);
  std::map<std::string, std::string> true_data;

  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 2, 3, 4, 5, 6}, -1, true,
                                       false, &true_data));
  // File dont overwrite any keys, No seqno needed
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), 0);

  ASSERT_OK(GenerateAndAddExternalFile(options, {10, 11, 12, 13}, -1, true,
                                       false, &true_data));
  // File dont overwrite any keys, No seqno needed
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), 0);

  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 4, 6}, -1, true, false,
                                       &true_data));
  // File overwrite some keys, a seqno will be assigned
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), 1);

  ASSERT_OK(GenerateAndAddExternalFile(options, {11, 15, 19}, -1, true, false,
                                       &true_data));
  // File overwrite some keys, a seqno will be assigned
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), 2);

  ASSERT_OK(GenerateAndAddExternalFile(options, {120, 130}, -1, true, false,
                                       &true_data));
  // File dont overwrite any keys, No seqno needed
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), 2);

  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 130}, -1, true, false,
                                       &true_data));
  // File overwrite some keys, a seqno will be assigned
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), 3);

  // Write some keys through normal write path
  for (int i = 0; i < 50; i++) {
    ASSERT_OK(Put(Key(i), "memtable"));
    true_data[Key(i)] = "memtable";
  }
  SequenceNumber last_seqno = dbfull()->GetLatestSequenceNumber();

  ASSERT_OK(GenerateAndAddExternalFile(options, {60, 61, 62}, -1, true, false,
                                       &true_data));
  // File dont overwrite any keys, No seqno needed
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), last_seqno);

  ASSERT_OK(GenerateAndAddExternalFile(options, {40, 41, 42}, -1, true, false,
                                       &true_data));
  // File overwrite some keys, a seqno will be assigned
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), last_seqno + 1);

  ASSERT_OK(GenerateAndAddExternalFile(options, {20, 30, 40}, -1, true, false,
                                       &true_data));
  // File overwrite some keys, a seqno will be assigned
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), last_seqno + 2);

  const Snapshot* snapshot = db_->GetSnapshot();

  // We will need a seqno for the file regardless if the file overwrite
  // keys in the DB or not because we have a snapshot
  ASSERT_OK(GenerateAndAddExternalFile(options, {1000, 1002}, -1, true, false,
                                       &true_data));
  // A global seqno will be assigned anyway because of the snapshot
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), last_seqno + 3);

  ASSERT_OK(GenerateAndAddExternalFile(options, {2000, 3002}, -1, true, false,
                                       &true_data));
  // A global seqno will be assigned anyway because of the snapshot
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), last_seqno + 4);

  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 20, 40, 100, 150}, -1, true,
                                       false, &true_data));
  // A global seqno will be assigned anyway because of the snapshot
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), last_seqno + 5);

  db_->ReleaseSnapshot(snapshot);

  ASSERT_OK(GenerateAndAddExternalFile(options, {5000, 5001}, -1, true, false,
                                       &true_data));
  // No snapshot anymore, no need to assign a seqno
  ASSERT_EQ(dbfull()->GetLatestSequenceNumber(), last_seqno + 5);

  size_t kcnt = 0;
  VerifyDBFromMap(true_data, &kcnt, false);
}

TEST_F(ExternalSSTFileTest, IngestFileWithGlobalSeqnoMemtableFlush) {
  Options options = CurrentOptions();
  DestroyAndReopen(options);
  uint64_t entries_in_memtable;
  std::map<std::string, std::string> true_data;

  for (int k : {10, 20, 40, 80}) {
    ASSERT_OK(Put(Key(k), "memtable"));
    true_data[Key(k)] = "memtable";
  }
  db_->GetIntProperty(DB::Properties::kNumEntriesActiveMemTable,
                      &entries_in_memtable);
  ASSERT_GE(entries_in_memtable, 1);

  // No need for flush
  ASSERT_OK(GenerateAndAddExternalFile(options, {90, 100, 110}, -1, true, false,
                                       &true_data));
  db_->GetIntProperty(DB::Properties::kNumEntriesActiveMemTable,
                      &entries_in_memtable);
  ASSERT_GE(entries_in_memtable, 1);

  // This file will flush the memtable
  ASSERT_OK(GenerateAndAddExternalFile(options, {19, 20, 21}, -1, true, false,
                                       &true_data));
  db_->GetIntProperty(DB::Properties::kNumEntriesActiveMemTable,
                      &entries_in_memtable);
  ASSERT_EQ(entries_in_memtable, 0);

  for (int k : {200, 201, 205, 206}) {
    ASSERT_OK(Put(Key(k), "memtable"));
    true_data[Key(k)] = "memtable";
  }
  db_->GetIntProperty(DB::Properties::kNumEntriesActiveMemTable,
                      &entries_in_memtable);
  ASSERT_GE(entries_in_memtable, 1);

  // No need for flush, this file keys fit between the memtable keys
  ASSERT_OK(GenerateAndAddExternalFile(options, {202, 203, 204}, -1, true,
                                       false, &true_data));
  db_->GetIntProperty(DB::Properties::kNumEntriesActiveMemTable,
                      &entries_in_memtable);
  ASSERT_GE(entries_in_memtable, 1);

  // This file will flush the memtable
  ASSERT_OK(GenerateAndAddExternalFile(options, {206, 207}, -1, true, false,
                                       &true_data));
  db_->GetIntProperty(DB::Properties::kNumEntriesActiveMemTable,
                      &entries_in_memtable);
  ASSERT_EQ(entries_in_memtable, 0);

  size_t kcnt = 0;
  VerifyDBFromMap(true_data, &kcnt, false);
}

TEST_F(ExternalSSTFileTest, L0SortingIssue) {
  Options options = CurrentOptions();
  options.num_levels = 2;
  DestroyAndReopen(options);
  std::map<std::string, std::string> true_data;

  ASSERT_OK(Put(Key(1), "memtable"));
  ASSERT_OK(Put(Key(10), "memtable"));

  // No Flush needed, No global seqno needed, Ingest in L1
  ASSERT_OK(GenerateAndAddExternalFile(options, {7, 8}, -1, true, false));
  // No Flush needed, but need a global seqno, Ingest in L0
  ASSERT_OK(GenerateAndAddExternalFile(options, {7, 8}, -1, true, false));
  printf("%s\n", FilesPerLevel().c_str());

  // Overwrite what we added using external files
  ASSERT_OK(Put(Key(7), "memtable"));
  ASSERT_OK(Put(Key(8), "memtable"));

  // Read values from memtable
  ASSERT_EQ(Get(Key(7)), "memtable");
  ASSERT_EQ(Get(Key(8)), "memtable");

  // Flush and read from L0
  ASSERT_OK(Flush());
  printf("%s\n", FilesPerLevel().c_str());
  ASSERT_EQ(Get(Key(7)), "memtable");
  ASSERT_EQ(Get(Key(8)), "memtable");
}

TEST_F(ExternalSSTFileTest, CompactionDeadlock) {
  Options options = CurrentOptions();
  options.num_levels = 2;
  options.level0_file_num_compaction_trigger = 4;
  options.level0_slowdown_writes_trigger = 4;
  options.level0_stop_writes_trigger = 4;
  DestroyAndReopen(options);

  // atomic conter of currently running bg threads
  std::atomic<int> running_threads(0);

  rocksdb::SyncPoint::GetInstance()->LoadDependency({
      {"DBImpl::DelayWrite:Wait", "ExternalSSTFileTest::DeadLock:0"},
      {"ExternalSSTFileTest::DeadLock:1", "DBImpl::AddFile:Start"},
      {"DBImpl::AddFile:MutexLock", "ExternalSSTFileTest::DeadLock:2"},
      {"ExternalSSTFileTest::DeadLock:3", "BackgroundCallCompaction:0"},
  });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  // Start ingesting and extrnal file in the background
  std::thread bg_ingest_file([&]() {
    running_threads += 1;
    ASSERT_OK(GenerateAndAddExternalFile(options, {5, 6}));
    running_threads -= 1;
  });

  ASSERT_OK(Put(Key(1), "memtable"));
  ASSERT_OK(Flush());

  ASSERT_OK(Put(Key(2), "memtable"));
  ASSERT_OK(Flush());

  ASSERT_OK(Put(Key(3), "memtable"));
  ASSERT_OK(Flush());

  ASSERT_OK(Put(Key(4), "memtable"));
  ASSERT_OK(Flush());

  // This thread will try to insert into the memtable but since we have 4 L0
  // files this thread will be blocked and hold the writer thread
  std::thread bg_block_put([&]() {
    running_threads += 1;
    ASSERT_OK(Put(Key(10), "memtable"));
    running_threads -= 1;
  });

  // Make sure DelayWrite is called first
  TEST_SYNC_POINT("ExternalSSTFileTest::DeadLock:0");

  // `DBImpl::AddFile:Start` will wait until we be here
  TEST_SYNC_POINT("ExternalSSTFileTest::DeadLock:1");

  // Wait for IngestExternalFile() to start and aquire mutex
  TEST_SYNC_POINT("ExternalSSTFileTest::DeadLock:2");

  // Now let compaction start
  TEST_SYNC_POINT("ExternalSSTFileTest::DeadLock:3");

  // Wait for max 5 seconds, if we did not finish all bg threads
  // then we hit the deadlock bug
  for (int i = 0; i < 10; i++) {
    if (running_threads.load() == 0) {
      break;
    }
    env_->SleepForMicroseconds(500000);
  }

  ASSERT_EQ(running_threads.load(), 0);

  bg_ingest_file.join();
  bg_block_put.join();
}

TEST_F(ExternalSSTFileTest, DirtyExit) {
  Options options = CurrentOptions();
  DestroyAndReopen(options);
  std::string file_path = sst_files_dir_ + "/dirty_exit";
  std::unique_ptr<SstFileWriter> sst_file_writer;

  // Destruct SstFileWriter without calling Finish()
  sst_file_writer.reset(
      new SstFileWriter(EnvOptions(), options, options.comparator));
  ASSERT_OK(sst_file_writer->Open(file_path));
  sst_file_writer.reset();

  // Destruct SstFileWriter with a failing Finish
  sst_file_writer.reset(
      new SstFileWriter(EnvOptions(), options, options.comparator));
  ASSERT_OK(sst_file_writer->Open(file_path));
  ASSERT_NOK(sst_file_writer->Finish());
}

TEST_F(ExternalSSTFileTest, FileWithCFInfo) {
  Options options = CurrentOptions();
  CreateAndReopenWithCF({"koko", "toto"}, options);

  SstFileWriter sfw_default(EnvOptions(), options, options.comparator,
                            handles_[0]);
  SstFileWriter sfw_cf1(EnvOptions(), options, options.comparator, handles_[1]);
  SstFileWriter sfw_cf2(EnvOptions(), options, options.comparator, handles_[2]);
  SstFileWriter sfw_unknown(EnvOptions(), options, options.comparator);

  // default_cf.sst
  const std::string cf_default_sst = sst_files_dir_ + "/default_cf.sst";
  ASSERT_OK(sfw_default.Open(cf_default_sst));
  ASSERT_OK(sfw_default.Add("K1", "V1"));
  ASSERT_OK(sfw_default.Add("K2", "V2"));
  ASSERT_OK(sfw_default.Finish());

  // cf1.sst
  const std::string cf1_sst = sst_files_dir_ + "/cf1.sst";
  ASSERT_OK(sfw_cf1.Open(cf1_sst));
  ASSERT_OK(sfw_cf1.Add("K3", "V1"));
  ASSERT_OK(sfw_cf1.Add("K4", "V2"));
  ASSERT_OK(sfw_cf1.Finish());

  // cf_unknown.sst
  const std::string unknown_sst = sst_files_dir_ + "/cf_unknown.sst";
  ASSERT_OK(sfw_unknown.Open(unknown_sst));
  ASSERT_OK(sfw_unknown.Add("K5", "V1"));
  ASSERT_OK(sfw_unknown.Add("K6", "V2"));
  ASSERT_OK(sfw_unknown.Finish());

  IngestExternalFileOptions ifo;

  // SST CF dont match
  ASSERT_NOK(db_->IngestExternalFile(handles_[0], {cf1_sst}, ifo));
  // SST CF dont match
  ASSERT_NOK(db_->IngestExternalFile(handles_[2], {cf1_sst}, ifo));
  // SST CF match
  ASSERT_OK(db_->IngestExternalFile(handles_[1], {cf1_sst}, ifo));

  // SST CF dont match
  ASSERT_NOK(db_->IngestExternalFile(handles_[1], {cf_default_sst}, ifo));
  // SST CF dont match
  ASSERT_NOK(db_->IngestExternalFile(handles_[2], {cf_default_sst}, ifo));
  // SST CF match
  ASSERT_OK(db_->IngestExternalFile(handles_[0], {cf_default_sst}, ifo));

  // SST CF unknown
  ASSERT_OK(db_->IngestExternalFile(handles_[1], {unknown_sst}, ifo));
  // SST CF unknown
  ASSERT_OK(db_->IngestExternalFile(handles_[2], {unknown_sst}, ifo));
  // SST CF unknown
  ASSERT_OK(db_->IngestExternalFile(handles_[0], {unknown_sst}, ifo));

  // Cannot ingest a file into a dropped CF
  ASSERT_OK(db_->DropColumnFamily(handles_[1]));
  ASSERT_NOK(db_->IngestExternalFile(handles_[1], {unknown_sst}, ifo));

  // CF was not dropped, ok to Ingest
  ASSERT_OK(db_->IngestExternalFile(handles_[2], {unknown_sst}, ifo));
}

class TestIngestExternalFileListener : public EventListener {
 public:
  void OnExternalFileIngested(DB* db,
                              const ExternalFileIngestionInfo& info) override {
    ingested_files.push_back(info);
  }

  std::vector<ExternalFileIngestionInfo> ingested_files;
};

TEST_F(ExternalSSTFileTest, IngestionListener) {
  Options options = CurrentOptions();
  TestIngestExternalFileListener* listener =
      new TestIngestExternalFileListener();
  options.listeners.emplace_back(listener);
  CreateAndReopenWithCF({"koko", "toto"}, options);

  // Ingest into default cf
  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 2}, -1, true, true, nullptr,
                                       handles_[0]));
  ASSERT_EQ(listener->ingested_files.size(), 1);
  ASSERT_EQ(listener->ingested_files.back().cf_name, "default");
  ASSERT_EQ(listener->ingested_files.back().global_seqno, 0);
  ASSERT_EQ(listener->ingested_files.back().table_properties.column_family_id,
            0);
  ASSERT_EQ(listener->ingested_files.back().table_properties.column_family_name,
            "default");

  // Ingest into cf1
  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 2}, -1, true, true, nullptr,
                                       handles_[1]));
  ASSERT_EQ(listener->ingested_files.size(), 2);
  ASSERT_EQ(listener->ingested_files.back().cf_name, "koko");
  ASSERT_EQ(listener->ingested_files.back().global_seqno, 0);
  ASSERT_EQ(listener->ingested_files.back().table_properties.column_family_id,
            1);
  ASSERT_EQ(listener->ingested_files.back().table_properties.column_family_name,
            "koko");

  // Ingest into cf2
  ASSERT_OK(GenerateAndAddExternalFile(options, {1, 2}, -1, true, true, nullptr,
                                       handles_[2]));
  ASSERT_EQ(listener->ingested_files.size(), 3);
  ASSERT_EQ(listener->ingested_files.back().cf_name, "toto");
  ASSERT_EQ(listener->ingested_files.back().global_seqno, 0);
  ASSERT_EQ(listener->ingested_files.back().table_properties.column_family_id,
            2);
  ASSERT_EQ(listener->ingested_files.back().table_properties.column_family_name,
            "toto");
}

TEST_F(ExternalSSTFileTest, SnapshotInconsistencyBug) {
  Options options = CurrentOptions();
  DestroyAndReopen(options);
  const int kNumKeys = 10000;

  // Insert keys using normal path and take a snapshot
  for (int i = 0; i < kNumKeys; i++) {
    ASSERT_OK(Put(Key(i), Key(i) + "_V1"));
  }
  const Snapshot* snap = db_->GetSnapshot();

  // Overwrite all keys using IngestExternalFile
  std::string sst_file_path = sst_files_dir_ + "file1.sst";
  SstFileWriter sst_file_writer(EnvOptions(), options, options.comparator);
  ASSERT_OK(sst_file_writer.Open(sst_file_path));
  for (int i = 0; i < kNumKeys; i++) {
    ASSERT_OK(sst_file_writer.Add(Key(i), Key(i) + "_V2"));
  }
  ASSERT_OK(sst_file_writer.Finish());

  IngestExternalFileOptions ifo;
  ifo.move_files = true;
  ASSERT_OK(db_->IngestExternalFile({sst_file_path}, ifo));

  for (int i = 0; i < kNumKeys; i++) {
    ASSERT_EQ(Get(Key(i), snap), Key(i) + "_V1");
    ASSERT_EQ(Get(Key(i)), Key(i) + "_V2");
  }

  db_->ReleaseSnapshot(snap);
}

TEST_F(ExternalSSTFileTest, FadviseTrigger) {
  Options options = CurrentOptions();
  const int kNumKeys = 10000;

  size_t total_fadvised_bytes = 0;
  rocksdb::SyncPoint::GetInstance()->SetCallBack(
      "SstFileWriter::InvalidatePageCache", [&](void* arg) {
        size_t fadvise_size = *(reinterpret_cast<size_t*>(arg));
        total_fadvised_bytes += fadvise_size;
      });
  rocksdb::SyncPoint::GetInstance()->EnableProcessing();

  std::unique_ptr<SstFileWriter> sst_file_writer;

  std::string sst_file_path = sst_files_dir_ + "file_fadvise_disable.sst";
  sst_file_writer.reset(new SstFileWriter(EnvOptions(), options,
                                          options.comparator, nullptr, false));
  ASSERT_OK(sst_file_writer->Open(sst_file_path));
  for (int i = 0; i < kNumKeys; i++) {
    ASSERT_OK(sst_file_writer->Add(Key(i), Key(i)));
  }
  ASSERT_OK(sst_file_writer->Finish());
  // fadvise disabled
  ASSERT_EQ(total_fadvised_bytes, 0);


  sst_file_path = sst_files_dir_ + "file_fadvise_enable.sst";
  sst_file_writer.reset(new SstFileWriter(EnvOptions(), options,
                                          options.comparator, nullptr, true));
  ASSERT_OK(sst_file_writer->Open(sst_file_path));
  for (int i = 0; i < kNumKeys; i++) {
    ASSERT_OK(sst_file_writer->Add(Key(i), Key(i)));
  }
  ASSERT_OK(sst_file_writer->Finish());
  // fadvise enabled
  ASSERT_EQ(total_fadvised_bytes, sst_file_writer->FileSize());
  ASSERT_GT(total_fadvised_bytes, 0);

  rocksdb::SyncPoint::GetInstance()->DisableProcessing();
}

#endif  // ROCKSDB_LITE

}  // namespace rocksdb

int main(int argc, char** argv) {
  rocksdb::port::InstallStackTraceHandler();
  ::testing::InitGoogleTest(&argc, argv);
  return RUN_ALL_TESTS();
}
