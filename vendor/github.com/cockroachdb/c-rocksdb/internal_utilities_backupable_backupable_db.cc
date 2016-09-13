//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.
//
// Copyright (c) 2011 The LevelDB Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file. See the AUTHORS file for names of contributors.

#ifndef ROCKSDB_LITE

#include "rocksdb/utilities/backupable_db.h"
#include "db/filename.h"
#include "util/channel.h"
#include "util/coding.h"
#include "util/crc32c.h"
#include "util/file_reader_writer.h"
#include "util/logging.h"
#include "util/string_util.h"
#include "rocksdb/rate_limiter.h"
#include "rocksdb/transaction_log.h"
#include "port/port.h"
#include "util/sync_point.h"

#ifndef __STDC_FORMAT_MACROS
#define __STDC_FORMAT_MACROS
#endif  // __STDC_FORMAT_MACROS

#include <inttypes.h>
#include <stdlib.h>
#include <algorithm>
#include <vector>
#include <map>
#include <mutex>
#include <sstream>
#include <string>
#include <limits>
#include <atomic>
#include <future>
#include <thread>
#include <unordered_map>
#include <unordered_set>

namespace rocksdb {

void BackupStatistics::IncrementNumberSuccessBackup() {
  number_success_backup++;
}
void BackupStatistics::IncrementNumberFailBackup() {
  number_fail_backup++;
}

uint32_t BackupStatistics::GetNumberSuccessBackup() const {
  return number_success_backup;
}
uint32_t BackupStatistics::GetNumberFailBackup() const {
  return number_fail_backup;
}

std::string BackupStatistics::ToString() const {
  char result[50];
  snprintf(result, sizeof(result), "# success backup: %u, # fail backup: %u",
           GetNumberSuccessBackup(), GetNumberFailBackup());
  return result;
}

void BackupableDBOptions::Dump(Logger* logger) const {
  Log(logger, "               Options.backup_dir: %s", backup_dir.c_str());
  Log(logger, "               Options.backup_env: %p", backup_env);
  Log(logger, "        Options.share_table_files: %d",
      static_cast<int>(share_table_files));
  Log(logger, "                 Options.info_log: %p", info_log);
  Log(logger, "                     Options.sync: %d", static_cast<int>(sync));
  Log(logger, "         Options.destroy_old_data: %d",
      static_cast<int>(destroy_old_data));
  Log(logger, "         Options.backup_log_files: %d",
      static_cast<int>(backup_log_files));
  Log(logger, "        Options.backup_rate_limit: %" PRIu64, backup_rate_limit);
  Log(logger, "       Options.restore_rate_limit: %" PRIu64,
      restore_rate_limit);
  Log(logger, "Options.max_background_operations: %d",
      max_background_operations);
}

// -------- BackupEngineImpl class ---------
class BackupEngineImpl : public BackupEngine {
 public:
  BackupEngineImpl(Env* db_env, const BackupableDBOptions& options,
                   bool read_only = false);
  ~BackupEngineImpl();
  Status CreateNewBackupWithMetadata(DB* db, const std::string& app_metadata,
                                     bool flush_before_backup = false,
                                     std::function<void()> progress_callback =
                                         []() {}) override;
  Status PurgeOldBackups(uint32_t num_backups_to_keep) override;
  Status DeleteBackup(BackupID backup_id) override;
  void StopBackup() override {
    stop_backup_.store(true, std::memory_order_release);
  }
  Status GarbageCollect() override;

  void GetBackupInfo(std::vector<BackupInfo>* backup_info) override;
  void GetCorruptedBackups(std::vector<BackupID>* corrupt_backup_ids) override;
  Status RestoreDBFromBackup(
      BackupID backup_id, const std::string& db_dir, const std::string& wal_dir,
      const RestoreOptions& restore_options = RestoreOptions()) override;
  Status RestoreDBFromLatestBackup(
      const std::string& db_dir, const std::string& wal_dir,
      const RestoreOptions& restore_options = RestoreOptions()) override {
    return RestoreDBFromBackup(latest_backup_id_, db_dir, wal_dir,
                               restore_options);
  }

  virtual Status VerifyBackup(BackupID backup_id) override;

  Status Initialize();

 private:
  void DeleteChildren(const std::string& dir, uint32_t file_type_filter = 0);

  // Extends the "result" map with pathname->size mappings for the contents of
  // "dir" in "env". Pathnames are prefixed with "dir".
  Status InsertPathnameToSizeBytes(
      const std::string& dir, Env* env,
      std::unordered_map<std::string, uint64_t>* result);

  struct FileInfo {
    FileInfo(const std::string& fname, uint64_t sz, uint32_t checksum)
      : refs(0), filename(fname), size(sz), checksum_value(checksum) {}

    FileInfo(const FileInfo&) = delete;
    FileInfo& operator=(const FileInfo&) = delete;

    int refs;
    const std::string filename;
    const uint64_t size;
    const uint32_t checksum_value;
  };

  class BackupMeta {
   public:
    BackupMeta(const std::string& meta_filename,
        std::unordered_map<std::string, std::shared_ptr<FileInfo>>* file_infos,
        Env* env)
      : timestamp_(0), size_(0), meta_filename_(meta_filename),
        file_infos_(file_infos), env_(env) {}

    BackupMeta(const BackupMeta&) = delete;
    BackupMeta& operator=(const BackupMeta&) = delete;

    ~BackupMeta() {}

    void RecordTimestamp() {
      env_->GetCurrentTime(&timestamp_);
    }
    int64_t GetTimestamp() const {
      return timestamp_;
    }
    uint64_t GetSize() const {
      return size_;
    }
    uint32_t GetNumberFiles() { return static_cast<uint32_t>(files_.size()); }
    void SetSequenceNumber(uint64_t sequence_number) {
      sequence_number_ = sequence_number;
    }
    uint64_t GetSequenceNumber() {
      return sequence_number_;
    }

    const std::string& GetAppMetadata() const { return app_metadata_; }

    void SetAppMetadata(const std::string& app_metadata) {
      app_metadata_ = app_metadata;
    }

    Status AddFile(std::shared_ptr<FileInfo> file_info);

    Status Delete(bool delete_meta = true);

    bool Empty() {
      return files_.empty();
    }

    std::shared_ptr<FileInfo> GetFile(const std::string& filename) const {
      auto it = file_infos_->find(filename);
      if (it == file_infos_->end())
        return nullptr;
      return it->second;
    }

    const std::vector<std::shared_ptr<FileInfo>>& GetFiles() {
      return files_;
    }

    // @param abs_path_to_size Pre-fetched file sizes (bytes).
    Status LoadFromFile(
        const std::string& backup_dir,
        const std::unordered_map<std::string, uint64_t>& abs_path_to_size);
    Status StoreToFile(bool sync);

    std::string GetInfoString() {
      std::ostringstream ss;
      ss << "Timestamp: " << timestamp_ << std::endl;
      char human_size[16];
      AppendHumanBytes(size_, human_size, sizeof(human_size));
      ss << "Size: " << human_size << std::endl;
      ss << "Files:" << std::endl;
      for (const auto& file : files_) {
        AppendHumanBytes(file->size, human_size, sizeof(human_size));
        ss << file->filename << ", size " << human_size << ", refs "
           << file->refs << std::endl;
      }
      return ss.str();
    }

   private:
    int64_t timestamp_;
    // sequence number is only approximate, should not be used
    // by clients
    uint64_t sequence_number_;
    uint64_t size_;
    std::string app_metadata_;
    std::string const meta_filename_;
    // files with relative paths (without "/" prefix!!)
    std::vector<std::shared_ptr<FileInfo>> files_;
    std::unordered_map<std::string, std::shared_ptr<FileInfo>>* file_infos_;
    Env* env_;

    static const size_t max_backup_meta_file_size_ = 10 * 1024 * 1024;  // 10MB
  };  // BackupMeta

  inline std::string GetAbsolutePath(
      const std::string &relative_path = "") const {
    assert(relative_path.size() == 0 || relative_path[0] != '/');
    return options_.backup_dir + "/" + relative_path;
  }
  inline std::string GetPrivateDirRel() const {
    return "private";
  }
  inline std::string GetSharedChecksumDirRel() const {
    return "shared_checksum";
  }
  inline std::string GetPrivateFileRel(BackupID backup_id,
                                       bool tmp = false,
                                       const std::string& file = "") const {
    assert(file.size() == 0 || file[0] != '/');
    return GetPrivateDirRel() + "/" + rocksdb::ToString(backup_id) +
           (tmp ? ".tmp" : "") + "/" + file;
  }
  inline std::string GetSharedFileRel(const std::string& file = "",
                                      bool tmp = false) const {
    assert(file.size() == 0 || file[0] != '/');
    return "shared/" + file + (tmp ? ".tmp" : "");
  }
  inline std::string GetSharedFileWithChecksumRel(const std::string& file = "",
                                                  bool tmp = false) const {
    assert(file.size() == 0 || file[0] != '/');
    return GetSharedChecksumDirRel() + "/" + file + (tmp ? ".tmp" : "");
  }
  inline std::string GetSharedFileWithChecksum(const std::string& file,
                                               const uint32_t checksum_value,
                                               const uint64_t file_size) const {
    assert(file.size() == 0 || file[0] != '/');
    std::string file_copy = file;
    return file_copy.insert(file_copy.find_last_of('.'),
                            "_" + rocksdb::ToString(checksum_value) + "_" +
                                rocksdb::ToString(file_size));
  }
  inline std::string GetFileFromChecksumFile(const std::string& file) const {
    assert(file.size() == 0 || file[0] != '/');
    std::string file_copy = file;
    size_t first_underscore = file_copy.find_first_of('_');
    return file_copy.erase(first_underscore,
                           file_copy.find_last_of('.') - first_underscore);
  }
  inline std::string GetLatestBackupFile(bool tmp = false) const {
    return GetAbsolutePath(std::string("LATEST_BACKUP") + (tmp ? ".tmp" : ""));
  }
  inline std::string GetBackupMetaDir() const {
    return GetAbsolutePath("meta");
  }
  inline std::string GetBackupMetaFile(BackupID backup_id) const {
    return GetBackupMetaDir() + "/" + rocksdb::ToString(backup_id);
  }

  Status PutLatestBackupFileContents(uint32_t latest_backup);

  // If size_limit == 0, there is no size limit, copy everything.
  //
  // Exactly one of src and contents must be non-empty.
  //
  // @param src If non-empty, the file is copied from this pathname.
  // @param contents If non-empty, the file will be created with these contents.
  Status CopyOrCreateFile(const std::string& src, const std::string& dst,
                          const std::string& contents, Env* src_env,
                          Env* dst_env, bool sync, RateLimiter* rate_limiter,
                          uint64_t* size = nullptr,
                          uint32_t* checksum_value = nullptr,
                          uint64_t size_limit = 0,
                          std::function<void()> progress_callback = []() {});

  Status CalculateChecksum(const std::string& src,
                           Env* src_env,
                           uint64_t size_limit,
                           uint32_t* checksum_value);

  struct CopyOrCreateResult {
    uint64_t size;
    uint32_t checksum_value;
    Status status;
  };

  // Exactly one of src_path and contents must be non-empty. If src_path is
  // non-empty, the file is copied from this pathname. Otherwise, if contents is
  // non-empty, the file will be created at dst_path with these contents.
  struct CopyOrCreateWorkItem {
    std::string src_path;
    std::string dst_path;
    std::string contents;
    Env* src_env;
    Env* dst_env;
    bool sync;
    RateLimiter* rate_limiter;
    uint64_t size_limit;
    std::promise<CopyOrCreateResult> result;
    std::function<void()> progress_callback;

    CopyOrCreateWorkItem() {}
    CopyOrCreateWorkItem(const CopyOrCreateWorkItem&) = delete;
    CopyOrCreateWorkItem& operator=(const CopyOrCreateWorkItem&) = delete;

    CopyOrCreateWorkItem(CopyOrCreateWorkItem&& o) ROCKSDB_NOEXCEPT {
      *this = std::move(o);
    }

    CopyOrCreateWorkItem& operator=(CopyOrCreateWorkItem&& o) ROCKSDB_NOEXCEPT {
      src_path = std::move(o.src_path);
      dst_path = std::move(o.dst_path);
      contents = std::move(o.contents);
      src_env = o.src_env;
      dst_env = o.dst_env;
      sync = o.sync;
      rate_limiter = o.rate_limiter;
      size_limit = o.size_limit;
      result = std::move(o.result);
      progress_callback = std::move(o.progress_callback);
      return *this;
    }

    CopyOrCreateWorkItem(std::string _src_path, std::string _dst_path,
                         std::string _contents, Env* _src_env, Env* _dst_env,
                         bool _sync, RateLimiter* _rate_limiter,
                         uint64_t _size_limit,
                         std::function<void()> _progress_callback = []() {})
        : src_path(std::move(_src_path)),
          dst_path(std::move(_dst_path)),
          contents(std::move(_contents)),
          src_env(_src_env),
          dst_env(_dst_env),
          sync(_sync),
          rate_limiter(_rate_limiter),
          size_limit(_size_limit),
          progress_callback(_progress_callback) {}
  };

  struct BackupAfterCopyOrCreateWorkItem {
    std::future<CopyOrCreateResult> result;
    bool shared;
    bool needed_to_copy;
    Env* backup_env;
    std::string dst_path_tmp;
    std::string dst_path;
    std::string dst_relative;
    BackupAfterCopyOrCreateWorkItem() {}

    BackupAfterCopyOrCreateWorkItem(BackupAfterCopyOrCreateWorkItem&& o)
        ROCKSDB_NOEXCEPT {
      *this = std::move(o);
    }

    BackupAfterCopyOrCreateWorkItem& operator=(
        BackupAfterCopyOrCreateWorkItem&& o) ROCKSDB_NOEXCEPT {
      result = std::move(o.result);
      shared = o.shared;
      needed_to_copy = o.needed_to_copy;
      backup_env = o.backup_env;
      dst_path_tmp = std::move(o.dst_path_tmp);
      dst_path = std::move(o.dst_path);
      dst_relative = std::move(o.dst_relative);
      return *this;
    }

    BackupAfterCopyOrCreateWorkItem(std::future<CopyOrCreateResult>&& _result,
                                    bool _shared, bool _needed_to_copy,
                                    Env* _backup_env, std::string _dst_path_tmp,
                                    std::string _dst_path,
                                    std::string _dst_relative)
        : result(std::move(_result)),
          shared(_shared),
          needed_to_copy(_needed_to_copy),
          backup_env(_backup_env),
          dst_path_tmp(std::move(_dst_path_tmp)),
          dst_path(std::move(_dst_path)),
          dst_relative(std::move(_dst_relative)) {}
  };

  struct RestoreAfterCopyOrCreateWorkItem {
    std::future<CopyOrCreateResult> result;
    uint32_t checksum_value;
    RestoreAfterCopyOrCreateWorkItem() {}
    RestoreAfterCopyOrCreateWorkItem(std::future<CopyOrCreateResult>&& _result,
                                     uint32_t _checksum_value)
        : result(std::move(_result)), checksum_value(_checksum_value) {}
    RestoreAfterCopyOrCreateWorkItem(RestoreAfterCopyOrCreateWorkItem&& o)
        ROCKSDB_NOEXCEPT {
      *this = std::move(o);
    }

    RestoreAfterCopyOrCreateWorkItem& operator=(
        RestoreAfterCopyOrCreateWorkItem&& o) ROCKSDB_NOEXCEPT {
      result = std::move(o.result);
      checksum_value = o.checksum_value;
      return *this;
    }
  };

  bool initialized_;
  std::mutex byte_report_mutex_;
  channel<CopyOrCreateWorkItem> files_to_copy_or_create_;
  std::vector<std::thread> threads_;

  // Adds a file to the backup work queue to be copied or created if it doesn't
  // already exist.
  //
  // Exactly one of src_dir and contents must be non-empty.
  //
  // @param src_dir If non-empty, the file in this directory named fname will be
  //    copied.
  // @param fname Name of destination file and, in case of copy, source file.
  // @param contents If non-empty, the file will be created with these contents.
  Status AddBackupFileWorkItem(
      std::unordered_set<std::string>& live_dst_paths,
      std::vector<BackupAfterCopyOrCreateWorkItem>& backup_items_to_finish,
      BackupID backup_id, bool shared, const std::string& src_dir,
      const std::string& fname,  // starts with "/"
      RateLimiter* rate_limiter, uint64_t size_bytes, uint64_t size_limit = 0,
      bool shared_checksum = false,
      std::function<void()> progress_callback = []() {},
      const std::string& contents = std::string());

  // backup state data
  BackupID latest_backup_id_;
  std::map<BackupID, unique_ptr<BackupMeta>> backups_;
  std::map<BackupID,
           std::pair<Status, unique_ptr<BackupMeta>>> corrupt_backups_;
  std::unordered_map<std::string,
                     std::shared_ptr<FileInfo>> backuped_file_infos_;
  std::atomic<bool> stop_backup_;

  // options data
  BackupableDBOptions options_;
  Env* db_env_;
  Env* backup_env_;

  // directories
  unique_ptr<Directory> backup_directory_;
  unique_ptr<Directory> shared_directory_;
  unique_ptr<Directory> meta_directory_;
  unique_ptr<Directory> private_directory_;

  static const size_t kDefaultCopyFileBufferSize = 5 * 1024 * 1024LL;  // 5MB
  size_t copy_file_buffer_size_;
  bool read_only_;
  BackupStatistics backup_statistics_;
  static const size_t kMaxAppMetaSize = 1024 * 1024;  // 1MB
};

Status BackupEngine::Open(Env* env, const BackupableDBOptions& options,
                          BackupEngine** backup_engine_ptr) {
  std::unique_ptr<BackupEngineImpl> backup_engine(
      new BackupEngineImpl(env, options));
  auto s = backup_engine->Initialize();
  if (!s.ok()) {
    *backup_engine_ptr = nullptr;
    return s;
  }
  *backup_engine_ptr = backup_engine.release();
  return Status::OK();
}

BackupEngineImpl::BackupEngineImpl(Env* db_env,
                                   const BackupableDBOptions& options,
                                   bool read_only)
    : initialized_(false),
      stop_backup_(false),
      options_(options),
      db_env_(db_env),
      backup_env_(options.backup_env != nullptr ? options.backup_env : db_env_),
      copy_file_buffer_size_(kDefaultCopyFileBufferSize),
      read_only_(read_only) {
  if (options_.backup_rate_limiter == nullptr &&
      options_.backup_rate_limit > 0) {
    options_.backup_rate_limiter.reset(
        NewGenericRateLimiter(options_.backup_rate_limit));
  }
  if (options_.restore_rate_limiter == nullptr &&
      options_.restore_rate_limit > 0) {
    options_.restore_rate_limiter.reset(
        NewGenericRateLimiter(options_.restore_rate_limit));
  }
}

BackupEngineImpl::~BackupEngineImpl() {
  files_to_copy_or_create_.sendEof();
  for (auto& t : threads_) {
    t.join();
  }
  LogFlush(options_.info_log);
}

Status BackupEngineImpl::Initialize() {
  assert(!initialized_);
  initialized_ = true;
  if (read_only_) {
    Log(options_.info_log, "Starting read_only backup engine");
  }
  options_.Dump(options_.info_log);

  if (!read_only_) {
    // gather the list of directories that we need to create
    std::vector<std::pair<std::string, std::unique_ptr<Directory>*>>
        directories;
    directories.emplace_back(GetAbsolutePath(), &backup_directory_);
    if (options_.share_table_files) {
      if (options_.share_files_with_checksum) {
        directories.emplace_back(
            GetAbsolutePath(GetSharedFileWithChecksumRel()),
            &shared_directory_);
      } else {
        directories.emplace_back(GetAbsolutePath(GetSharedFileRel()),
                                 &shared_directory_);
      }
    }
    directories.emplace_back(GetAbsolutePath(GetPrivateDirRel()),
                             &private_directory_);
    directories.emplace_back(GetBackupMetaDir(), &meta_directory_);
    // create all the dirs we need
    for (const auto& d : directories) {
      auto s = backup_env_->CreateDirIfMissing(d.first);
      if (s.ok()) {
        s = backup_env_->NewDirectory(d.first, d.second);
      }
      if (!s.ok()) {
        return s;
      }
    }
  }

  std::vector<std::string> backup_meta_files;
  {
    auto s = backup_env_->GetChildren(GetBackupMetaDir(), &backup_meta_files);
    if (!s.ok()) {
      return s;
    }
  }
  // create backups_ structure
  for (auto& file : backup_meta_files) {
    if (file == "." || file == "..") {
      continue;
    }
    Log(options_.info_log, "Detected backup %s", file.c_str());
    BackupID backup_id = 0;
    sscanf(file.c_str(), "%u", &backup_id);
    if (backup_id == 0 || file != rocksdb::ToString(backup_id)) {
      if (!read_only_) {
        // invalid file name, delete that
        auto s = backup_env_->DeleteFile(GetBackupMetaDir() + "/" + file);
        Log(options_.info_log, "Unrecognized meta file %s, deleting -- %s",
            file.c_str(), s.ToString().c_str());
      }
      continue;
    }
    assert(backups_.find(backup_id) == backups_.end());
    backups_.insert(
        std::make_pair(backup_id, unique_ptr<BackupMeta>(new BackupMeta(
                                      GetBackupMetaFile(backup_id),
                                      &backuped_file_infos_, backup_env_))));
  }

  latest_backup_id_ = 0;
  if (options_.destroy_old_data) {  // Destroy old data
    assert(!read_only_);
    Log(options_.info_log,
        "Backup Engine started with destroy_old_data == true, deleting all "
        "backups");
    auto s = PurgeOldBackups(0);
    if (s.ok()) {
      s = GarbageCollect();
    }
    if (!s.ok()) {
      return s;
    }
  } else {  // Load data from storage
    std::unordered_map<std::string, uint64_t> abs_path_to_size;
    for (const auto& rel_dir :
         {GetSharedFileRel(), GetSharedFileWithChecksumRel()}) {
      const auto abs_dir = GetAbsolutePath(rel_dir);
      InsertPathnameToSizeBytes(abs_dir, backup_env_, &abs_path_to_size);
    }
    // load the backups if any
    for (auto& backup : backups_) {
      InsertPathnameToSizeBytes(
          GetAbsolutePath(GetPrivateFileRel(backup.first)), backup_env_,
          &abs_path_to_size);
      Status s =
          backup.second->LoadFromFile(options_.backup_dir, abs_path_to_size);
      if (!s.ok()) {
        Log(options_.info_log, "Backup %u corrupted -- %s", backup.first,
            s.ToString().c_str());
        corrupt_backups_.insert(std::make_pair(
              backup.first, std::make_pair(s, std::move(backup.second))));
      } else {
        Log(options_.info_log, "Loading backup %" PRIu32 " OK:\n%s",
            backup.first, backup.second->GetInfoString().c_str());
        latest_backup_id_ = std::max(latest_backup_id_, backup.first);
      }
    }

    for (const auto& corrupt : corrupt_backups_) {
      backups_.erase(backups_.find(corrupt.first));
    }
  }

  Log(options_.info_log, "Latest backup is %u", latest_backup_id_);

  if (!read_only_) {
    auto s = PutLatestBackupFileContents(latest_backup_id_);
    if (!s.ok()) {
      return s;
    }
  }

  // set up threads perform copies from files_to_copy_or_create_ in the
  // background
  for (int t = 0; t < options_.max_background_operations; t++) {
    threads_.emplace_back([this]() {
      CopyOrCreateWorkItem work_item;
      while (files_to_copy_or_create_.read(work_item)) {
        CopyOrCreateResult result;
        result.status = CopyOrCreateFile(
            work_item.src_path, work_item.dst_path, work_item.contents,
            work_item.src_env, work_item.dst_env, work_item.sync,
            work_item.rate_limiter, &result.size, &result.checksum_value,
            work_item.size_limit, work_item.progress_callback);
        work_item.result.set_value(std::move(result));
      }
    });
  }

  Log(options_.info_log, "Initialized BackupEngine");

  return Status::OK();
}

Status BackupEngineImpl::CreateNewBackupWithMetadata(
    DB* db, const std::string& app_metadata, bool flush_before_backup,
    std::function<void()> progress_callback) {
  assert(initialized_);
  assert(!read_only_);
  if (app_metadata.size() > kMaxAppMetaSize) {
    return Status::InvalidArgument("App metadata too large");
  }
  Status s;
  std::vector<std::string> live_files;
  VectorLogPtr live_wal_files;
  uint64_t manifest_file_size = 0;
  uint64_t sequence_number = db->GetLatestSequenceNumber();

  s = db->DisableFileDeletions();
  if (s.ok()) {
    // this will return live_files prefixed with "/"
    s = db->GetLiveFiles(live_files, &manifest_file_size, flush_before_backup);
  }
  // if we didn't flush before backup, we need to also get WAL files
  if (s.ok() && !flush_before_backup && options_.backup_log_files) {
    // returns file names prefixed with "/"
    s = db->GetSortedWalFiles(live_wal_files);
  }
  if (!s.ok()) {
    db->EnableFileDeletions(false);
    return s;
  }
  TEST_SYNC_POINT("BackupEngineImpl::CreateNewBackup:SavedLiveFiles1");
  TEST_SYNC_POINT("BackupEngineImpl::CreateNewBackup:SavedLiveFiles2");

  BackupID new_backup_id = latest_backup_id_ + 1;
  assert(backups_.find(new_backup_id) == backups_.end());
  auto ret = backups_.insert(
      std::make_pair(new_backup_id, unique_ptr<BackupMeta>(new BackupMeta(
                                        GetBackupMetaFile(new_backup_id),
                                        &backuped_file_infos_, backup_env_))));
  assert(ret.second == true);
  auto& new_backup = ret.first->second;
  new_backup->RecordTimestamp();
  new_backup->SetSequenceNumber(sequence_number);
  new_backup->SetAppMetadata(app_metadata);

  auto start_backup = backup_env_-> NowMicros();

  Log(options_.info_log, "Started the backup process -- creating backup %u",
      new_backup_id);

  // create temporary private dir
  s = backup_env_->CreateDir(
      GetAbsolutePath(GetPrivateFileRel(new_backup_id, true)));

  RateLimiter* rate_limiter = options_.backup_rate_limiter.get();
  if (rate_limiter) {
    copy_file_buffer_size_ = rate_limiter->GetSingleBurstBytes();
  }

  // A set into which we will insert the dst_paths that are calculated for live
  // files and live WAL files.
  // This is used to check whether a live files shares a dst_path with another
  // live file.
  std::unordered_set<std::string> live_dst_paths;
  live_dst_paths.reserve(live_files.size() + live_wal_files.size());

  // Pre-fetch sizes for data files
  std::unordered_map<std::string, uint64_t> data_path_to_size;
  if (s.ok()) {
    s = InsertPathnameToSizeBytes(db->GetName(), db_env_, &data_path_to_size);
  }

  std::vector<BackupAfterCopyOrCreateWorkItem> backup_items_to_finish;
  // Add a CopyOrCreateWorkItem to the channel for each live file
  std::string manifest_fname, current_fname;
  for (size_t i = 0; s.ok() && i < live_files.size(); ++i) {
    uint64_t number;
    FileType type;
    bool ok = ParseFileName(live_files[i], &number, &type);
    if (!ok) {
      assert(false);
      return Status::Corruption("Can't parse file name. This is very bad");
    }
    // we should only get sst, manifest and current files here
    assert(type == kTableFile || type == kDescriptorFile ||
           type == kCurrentFile);
    if (type == kCurrentFile) {
      // We will craft the current file manually to ensure it's consistent with
      // the manifest number. This is necessary because current's file contents
      // can change during backup.
      current_fname = live_files[i];
      continue;
    } else if (type == kDescriptorFile) {
      manifest_fname = live_files[i];
    }

    auto data_path_to_size_iter =
        data_path_to_size.find(db->GetName() + live_files[i]);
    uint64_t size_bytes = data_path_to_size_iter == data_path_to_size.end()
                              ? port::kMaxUint64
                              : data_path_to_size_iter->second;

    // rules:
    // * if it's kTableFile, then it's shared
    // * if it's kDescriptorFile, limit the size to manifest_file_size
    s = AddBackupFileWorkItem(
        live_dst_paths, backup_items_to_finish, new_backup_id,
        options_.share_table_files && type == kTableFile, db->GetName(),
        live_files[i], rate_limiter, size_bytes,
        (type == kDescriptorFile) ? manifest_file_size : 0,
        options_.share_files_with_checksum && type == kTableFile,
        progress_callback);
  }
  if (s.ok() && !current_fname.empty() && !manifest_fname.empty()) {
    // Write the current file with the manifest filename as its contents.
    s = AddBackupFileWorkItem(
        live_dst_paths, backup_items_to_finish, new_backup_id,
        false /* shared */, "" /* src_dir */, CurrentFileName(""), rate_limiter,
        manifest_fname.size(), 0 /* size_limit */, false /* shared_checksum */,
        progress_callback, manifest_fname.substr(1) + "\n");
  }

  // Pre-fetch sizes for WAL files
  std::unordered_map<std::string, uint64_t> wal_path_to_size;
  if (s.ok()) {
    if (db->GetOptions().wal_dir != "") {
      s = InsertPathnameToSizeBytes(db->GetOptions().wal_dir, db_env_,
                                    &wal_path_to_size);
    } else {
      wal_path_to_size = std::move(data_path_to_size);
    }
  }

  // Add a CopyOrCreateWorkItem to the channel for each WAL file
  for (size_t i = 0; s.ok() && i < live_wal_files.size(); ++i) {
    auto wal_path_to_size_iter =
        wal_path_to_size.find(live_wal_files[i]->PathName());
    uint64_t size_bytes = wal_path_to_size_iter == wal_path_to_size.end()
                              ? port::kMaxUint64
                              : wal_path_to_size_iter->second;
    if (live_wal_files[i]->Type() == kAliveLogFile) {
      // we only care about live log files
      // copy the file into backup_dir/files/<new backup>/
      s = AddBackupFileWorkItem(live_dst_paths, backup_items_to_finish,
                                new_backup_id, false, /* not shared */
                                db->GetOptions().wal_dir,
                                live_wal_files[i]->PathName(), rate_limiter,
                                size_bytes);
    }
  }

  Status item_status;
  for (auto& item : backup_items_to_finish) {
    item.result.wait();
    auto result = item.result.get();
    item_status = result.status;
    if (item_status.ok() && item.shared && item.needed_to_copy) {
      item_status = item.backup_env->RenameFile(item.dst_path_tmp,
                                                item.dst_path);
    }
    if (item_status.ok()) {
      item_status = new_backup.get()->AddFile(
              std::make_shared<FileInfo>(item.dst_relative,
                                         result.size,
                                         result.checksum_value));
    }
    if (!item_status.ok()) {
      s = item_status;
    }
  }

  // we copied all the files, enable file deletions
  db->EnableFileDeletions(false);

  if (s.ok()) {
    // move tmp private backup to real backup folder
    Log(options_.info_log,
        "Moving tmp backup directory to the real one: %s -> %s\n",
        GetAbsolutePath(GetPrivateFileRel(new_backup_id, true)).c_str(),
        GetAbsolutePath(GetPrivateFileRel(new_backup_id, false)).c_str());
    s = backup_env_->RenameFile(
        GetAbsolutePath(GetPrivateFileRel(new_backup_id, true)),  // tmp
        GetAbsolutePath(GetPrivateFileRel(new_backup_id, false)));
  }

  auto backup_time = backup_env_->NowMicros() - start_backup;

  if (s.ok()) {
    // persist the backup metadata on the disk
    s = new_backup->StoreToFile(options_.sync);
  }
  if (s.ok()) {
    // install the newly created backup meta! (atomic)
    s = PutLatestBackupFileContents(new_backup_id);
  }
  if (s.ok() && options_.sync) {
    unique_ptr<Directory> backup_private_directory;
    backup_env_->NewDirectory(
        GetAbsolutePath(GetPrivateFileRel(new_backup_id, false)),
        &backup_private_directory);
    if (backup_private_directory != nullptr) {
      backup_private_directory->Fsync();
    }
    if (private_directory_ != nullptr) {
      private_directory_->Fsync();
    }
    if (meta_directory_ != nullptr) {
      meta_directory_->Fsync();
    }
    if (shared_directory_ != nullptr) {
      shared_directory_->Fsync();
    }
    if (backup_directory_ != nullptr) {
      backup_directory_->Fsync();
    }
  }

  if (s.ok()) {
    backup_statistics_.IncrementNumberSuccessBackup();
  }
  if (!s.ok()) {
    backup_statistics_.IncrementNumberFailBackup();
    // clean all the files we might have created
    Log(options_.info_log, "Backup failed -- %s", s.ToString().c_str());
    Log(options_.info_log, "Backup Statistics %s\n",
        backup_statistics_.ToString().c_str());
    // delete files that we might have already written
    DeleteBackup(new_backup_id);
    GarbageCollect();
    return s;
  }

  // here we know that we succeeded and installed the new backup
  // in the LATEST_BACKUP file
  latest_backup_id_ = new_backup_id;
  Log(options_.info_log, "Backup DONE. All is good");

  // backup_speed is in byte/second
  double backup_speed = new_backup->GetSize() / (1.048576 * backup_time);
  Log(options_.info_log, "Backup number of files: %u",
      new_backup->GetNumberFiles());
  char human_size[16];
  AppendHumanBytes(new_backup->GetSize(), human_size, sizeof(human_size));
  Log(options_.info_log, "Backup size: %s", human_size);
  Log(options_.info_log, "Backup time: %" PRIu64 " microseconds", backup_time);
  Log(options_.info_log, "Backup speed: %.3f MB/s", backup_speed);
  Log(options_.info_log, "Backup Statistics %s",
      backup_statistics_.ToString().c_str());
  return s;
}

Status BackupEngineImpl::PurgeOldBackups(uint32_t num_backups_to_keep) {
  assert(initialized_);
  assert(!read_only_);
  Log(options_.info_log, "Purging old backups, keeping %u",
      num_backups_to_keep);
  std::vector<BackupID> to_delete;
  auto itr = backups_.begin();
  while ((backups_.size() - to_delete.size()) > num_backups_to_keep) {
    to_delete.push_back(itr->first);
    itr++;
  }
  for (auto backup_id : to_delete) {
    auto s = DeleteBackup(backup_id);
    if (!s.ok()) {
      return s;
    }
  }
  return Status::OK();
}

Status BackupEngineImpl::DeleteBackup(BackupID backup_id) {
  assert(initialized_);
  assert(!read_only_);
  Log(options_.info_log, "Deleting backup %u", backup_id);
  auto backup = backups_.find(backup_id);
  if (backup != backups_.end()) {
    auto s = backup->second->Delete();
    if (!s.ok()) {
      return s;
    }
    backups_.erase(backup);
  } else {
    auto corrupt = corrupt_backups_.find(backup_id);
    if (corrupt == corrupt_backups_.end()) {
      return Status::NotFound("Backup not found");
    }
    auto s = corrupt->second.second->Delete();
    if (!s.ok()) {
      return s;
    }
    corrupt_backups_.erase(corrupt);
  }

  std::vector<std::string> to_delete;
  for (auto& itr : backuped_file_infos_) {
    if (itr.second->refs == 0) {
      Status s = backup_env_->DeleteFile(GetAbsolutePath(itr.first));
      Log(options_.info_log, "Deleting %s -- %s", itr.first.c_str(),
          s.ToString().c_str());
      to_delete.push_back(itr.first);
    }
  }
  for (auto& td : to_delete) {
    backuped_file_infos_.erase(td);
  }

  // take care of private dirs -- GarbageCollect() will take care of them
  // if they are not empty
  std::string private_dir = GetPrivateFileRel(backup_id);
  Status s = backup_env_->DeleteDir(GetAbsolutePath(private_dir));
  Log(options_.info_log, "Deleting private dir %s -- %s",
      private_dir.c_str(), s.ToString().c_str());
  return Status::OK();
}

void BackupEngineImpl::GetBackupInfo(std::vector<BackupInfo>* backup_info) {
  assert(initialized_);
  backup_info->reserve(backups_.size());
  for (auto& backup : backups_) {
    if (!backup.second->Empty()) {
      backup_info->push_back(BackupInfo(
          backup.first, backup.second->GetTimestamp(), backup.second->GetSize(),
          backup.second->GetNumberFiles(), backup.second->GetAppMetadata()));
    }
  }
}

void
BackupEngineImpl::GetCorruptedBackups(
    std::vector<BackupID>* corrupt_backup_ids) {
  assert(initialized_);
  corrupt_backup_ids->reserve(corrupt_backups_.size());
  for (auto& backup : corrupt_backups_) {
    corrupt_backup_ids->push_back(backup.first);
  }
}

Status BackupEngineImpl::RestoreDBFromBackup(
    BackupID backup_id, const std::string& db_dir, const std::string& wal_dir,
    const RestoreOptions& restore_options) {
  assert(initialized_);
  auto corrupt_itr = corrupt_backups_.find(backup_id);
  if (corrupt_itr != corrupt_backups_.end()) {
    return corrupt_itr->second.first;
  }
  auto backup_itr = backups_.find(backup_id);
  if (backup_itr == backups_.end()) {
    return Status::NotFound("Backup not found");
  }
  auto& backup = backup_itr->second;
  if (backup->Empty()) {
    return Status::NotFound("Backup not found");
  }

  Log(options_.info_log, "Restoring backup id %u\n", backup_id);
  Log(options_.info_log, "keep_log_files: %d\n",
      static_cast<int>(restore_options.keep_log_files));

  // just in case. Ignore errors
  db_env_->CreateDirIfMissing(db_dir);
  db_env_->CreateDirIfMissing(wal_dir);

  if (restore_options.keep_log_files) {
    // delete files in db_dir, but keep all the log files
    DeleteChildren(db_dir, 1 << kLogFile);
    // move all the files from archive dir to wal_dir
    std::string archive_dir = ArchivalDirectory(wal_dir);
    std::vector<std::string> archive_files;
    db_env_->GetChildren(archive_dir, &archive_files);  // ignore errors
    for (const auto& f : archive_files) {
      uint64_t number;
      FileType type;
      bool ok = ParseFileName(f, &number, &type);
      if (ok && type == kLogFile) {
        Log(options_.info_log, "Moving log file from archive/ to wal_dir: %s",
            f.c_str());
        Status s =
            db_env_->RenameFile(archive_dir + "/" + f, wal_dir + "/" + f);
        if (!s.ok()) {
          // if we can't move log file from archive_dir to wal_dir,
          // we should fail, since it might mean data loss
          return s;
        }
      }
    }
  } else {
    DeleteChildren(wal_dir);
    DeleteChildren(ArchivalDirectory(wal_dir));
    DeleteChildren(db_dir);
  }

  RateLimiter* rate_limiter = options_.restore_rate_limiter.get();
  if (rate_limiter) {
    copy_file_buffer_size_ = rate_limiter->GetSingleBurstBytes();
  }
  Status s;
  std::vector<RestoreAfterCopyOrCreateWorkItem> restore_items_to_finish;
  for (const auto& file_info : backup->GetFiles()) {
    const std::string &file = file_info->filename;
    std::string dst;
    // 1. extract the filename
    size_t slash = file.find_last_of('/');
    // file will either be shared/<file>, shared_checksum/<file_crc32_size>
    // or private/<number>/<file>
    assert(slash != std::string::npos);
    dst = file.substr(slash + 1);

    // if the file was in shared_checksum, extract the real file name
    // in this case the file is <number>_<checksum>_<size>.<type>
    if (file.substr(0, slash) == GetSharedChecksumDirRel()) {
      dst = GetFileFromChecksumFile(dst);
    }

    // 2. find the filetype
    uint64_t number;
    FileType type;
    bool ok = ParseFileName(dst, &number, &type);
    if (!ok) {
      return Status::Corruption("Backup corrupted");
    }
    // 3. Construct the final path
    // kLogFile lives in wal_dir and all the rest live in db_dir
    dst = ((type == kLogFile) ? wal_dir : db_dir) +
      "/" + dst;

    Log(options_.info_log, "Restoring %s to %s\n", file.c_str(), dst.c_str());
    CopyOrCreateWorkItem copy_or_create_work_item(
        GetAbsolutePath(file), dst, "" /* contents */, backup_env_, db_env_,
        false, rate_limiter, 0 /* size_limit */);
    RestoreAfterCopyOrCreateWorkItem after_copy_or_create_work_item(
        copy_or_create_work_item.result.get_future(),
        file_info->checksum_value);
    files_to_copy_or_create_.write(std::move(copy_or_create_work_item));
    restore_items_to_finish.push_back(
        std::move(after_copy_or_create_work_item));
  }
  Status item_status;
  for (auto& item : restore_items_to_finish) {
    item.result.wait();
    auto result = item.result.get();
    item_status = result.status;
    // Note: It is possible that both of the following bad-status cases occur
    // during copying. But, we only return one status.
    if (!item_status.ok()) {
      s = item_status;
      break;
    } else if (item.checksum_value != result.checksum_value) {
      s = Status::Corruption("Checksum check failed");
      break;
    }
  }

  Log(options_.info_log, "Restoring done -- %s\n", s.ToString().c_str());
  return s;
}

Status BackupEngineImpl::VerifyBackup(BackupID backup_id) {
  assert(initialized_);
  auto corrupt_itr = corrupt_backups_.find(backup_id);
  if (corrupt_itr != corrupt_backups_.end()) {
    return corrupt_itr->second.first;
  }

  auto backup_itr = backups_.find(backup_id);
  if (backup_itr == backups_.end()) {
    return Status::NotFound();
  }

  auto& backup = backup_itr->second;
  if (backup->Empty()) {
    return Status::NotFound();
  }

  Log(options_.info_log, "Verifying backup id %u\n", backup_id);

  std::unordered_map<std::string, uint64_t> curr_abs_path_to_size;
  for (const auto& rel_dir : {GetPrivateFileRel(backup_id), GetSharedFileRel(),
                              GetSharedFileWithChecksumRel()}) {
    const auto abs_dir = GetAbsolutePath(rel_dir);
    InsertPathnameToSizeBytes(abs_dir, backup_env_, &curr_abs_path_to_size);
  }

  for (const auto& file_info : backup->GetFiles()) {
    const auto abs_path = GetAbsolutePath(file_info->filename);
    if (curr_abs_path_to_size.find(abs_path) == curr_abs_path_to_size.end()) {
      return Status::NotFound("File missing: " + abs_path);
    }
    if (file_info->size != curr_abs_path_to_size[abs_path]) {
      return Status::Corruption("File corrupted: " + abs_path);
    }
  }
  return Status::OK();
}

// this operation HAS to be atomic
// writing 4 bytes to the file is atomic alright, but we should *never*
// do something like 1. delete file, 2. write new file
// We write to a tmp file and then atomically rename
Status BackupEngineImpl::PutLatestBackupFileContents(uint32_t latest_backup) {
  assert(!read_only_);
  Status s;
  unique_ptr<WritableFile> file;
  EnvOptions env_options;
  env_options.use_mmap_writes = false;
  s = backup_env_->NewWritableFile(GetLatestBackupFile(true),
                                   &file,
                                   env_options);
  if (!s.ok()) {
    backup_env_->DeleteFile(GetLatestBackupFile(true));
    return s;
  }

  unique_ptr<WritableFileWriter> file_writer(
      new WritableFileWriter(std::move(file), env_options));
  char file_contents[10];
  int len =
      snprintf(file_contents, sizeof(file_contents), "%u\n", latest_backup);
  s = file_writer->Append(Slice(file_contents, len));
  if (s.ok() && options_.sync) {
    file_writer->Sync(false);
  }
  if (s.ok()) {
    s = file_writer->Close();
  }
  if (s.ok()) {
    // atomically replace real file with new tmp
    s = backup_env_->RenameFile(GetLatestBackupFile(true),
                                GetLatestBackupFile(false));
  }
  return s;
}

Status BackupEngineImpl::CopyOrCreateFile(
    const std::string& src, const std::string& dst, const std::string& contents,
    Env* src_env, Env* dst_env, bool sync, RateLimiter* rate_limiter,
    uint64_t* size, uint32_t* checksum_value, uint64_t size_limit,
    std::function<void()> progress_callback) {
  assert(src.empty() != contents.empty());
  Status s;
  unique_ptr<WritableFile> dst_file;
  unique_ptr<SequentialFile> src_file;
  EnvOptions env_options;
  env_options.use_mmap_writes = false;
  env_options.use_os_buffer = false;
  if (size != nullptr) {
    *size = 0;
  }
  if (checksum_value != nullptr) {
    *checksum_value = 0;
  }

  // Check if size limit is set. if not, set it to very big number
  if (size_limit == 0) {
    size_limit = std::numeric_limits<uint64_t>::max();
  }

  s = dst_env->NewWritableFile(dst, &dst_file, env_options);
  if (s.ok() && !src.empty()) {
    s = src_env->NewSequentialFile(src, &src_file, env_options);
  }
  if (!s.ok()) {
    return s;
  }

  unique_ptr<WritableFileWriter> dest_writer(
      new WritableFileWriter(std::move(dst_file), env_options));
  unique_ptr<SequentialFileReader> src_reader;
  unique_ptr<char[]> buf;
  if (!src.empty()) {
    src_reader.reset(new SequentialFileReader(std::move(src_file)));
    buf.reset(new char[copy_file_buffer_size_]);
  }

  Slice data;
  uint64_t processed_buffer_size = 0;
  do {
    if (stop_backup_.load(std::memory_order_acquire)) {
      return Status::Incomplete("Backup stopped");
    }
    if (!src.empty()) {
      size_t buffer_to_read = (copy_file_buffer_size_ < size_limit)
                                  ? copy_file_buffer_size_
                                  : size_limit;
      s = src_reader->Read(buffer_to_read, &data, buf.get());
      processed_buffer_size += buffer_to_read;
    } else {
      data = contents;
    }
    size_limit -= data.size();

    if (!s.ok()) {
      return s;
    }

    if (size != nullptr) {
      *size += data.size();
    }
    if (checksum_value != nullptr) {
      *checksum_value =
          crc32c::Extend(*checksum_value, data.data(), data.size());
    }
    s = dest_writer->Append(data);
    if (rate_limiter != nullptr) {
      rate_limiter->Request(data.size(), Env::IO_LOW);
    }
    if (processed_buffer_size > options_.callback_trigger_interval_size) {
      processed_buffer_size -= options_.callback_trigger_interval_size;
      std::lock_guard<std::mutex> lock(byte_report_mutex_);
      progress_callback();
    }
  } while (s.ok() && contents.empty() && data.size() > 0 && size_limit > 0);

  if (s.ok() && sync) {
    s = dest_writer->Sync(false);
  }
  return s;
}

// fname will always start with "/"
Status BackupEngineImpl::AddBackupFileWorkItem(
    std::unordered_set<std::string>& live_dst_paths,
    std::vector<BackupAfterCopyOrCreateWorkItem>& backup_items_to_finish,
    BackupID backup_id, bool shared, const std::string& src_dir,
    const std::string& fname, RateLimiter* rate_limiter, uint64_t size_bytes,
    uint64_t size_limit, bool shared_checksum,
    std::function<void()> progress_callback, const std::string& contents) {
  assert(!fname.empty() && fname[0] == '/');
  assert(contents.empty() != src_dir.empty());

  std::string dst_relative = fname.substr(1);
  std::string dst_relative_tmp;
  Status s;
  uint32_t checksum_value = 0;

  if (shared && shared_checksum) {
    // add checksum and file length to the file name
    s = CalculateChecksum(src_dir + fname, db_env_, size_limit,
                          &checksum_value);
    if (!s.ok()) {
      return s;
    }
    if (size_bytes == port::kMaxUint64) {
      return Status::NotFound("File missing: " + src_dir + fname);
    }
    dst_relative =
        GetSharedFileWithChecksum(dst_relative, checksum_value, size_bytes);
    dst_relative_tmp = GetSharedFileWithChecksumRel(dst_relative, true);
    dst_relative = GetSharedFileWithChecksumRel(dst_relative, false);
  } else if (shared) {
    dst_relative_tmp = GetSharedFileRel(dst_relative, true);
    dst_relative = GetSharedFileRel(dst_relative, false);
  } else {
    dst_relative_tmp = GetPrivateFileRel(backup_id, true, dst_relative);
    dst_relative = GetPrivateFileRel(backup_id, false, dst_relative);
  }
  std::string dst_path = GetAbsolutePath(dst_relative);
  std::string dst_path_tmp = GetAbsolutePath(dst_relative_tmp);

  // if it's shared, we also need to check if it exists -- if it does, no need
  // to copy it again.
  bool need_to_copy = true;
  // true if dst_path is the same path as another live file
  const bool same_path =
      live_dst_paths.find(dst_path) != live_dst_paths.end();

  bool file_exists = false;
  if (shared && !same_path) {
    Status exist = backup_env_->FileExists(dst_path);
    if (exist.ok()) {
      file_exists = true;
    } else if (exist.IsNotFound()) {
      file_exists = false;
    } else {
      assert(s.IsIOError());
      return exist;
    }
  }

  if (!contents.empty()) {
    need_to_copy = false;
  } else if (shared && (same_path || file_exists)) {
    need_to_copy = false;
    if (shared_checksum) {
      Log(options_.info_log,
          "%s already present, with checksum %u and size %" PRIu64,
          fname.c_str(), checksum_value, size_bytes);
    } else if (backuped_file_infos_.find(dst_relative) ==
               backuped_file_infos_.end() && !same_path) {
      // file already exists, but it's not referenced by any backup. overwrite
      // the file
      Log(options_.info_log,
          "%s already present, but not referenced by any backup. We will "
          "overwrite the file.",
          fname.c_str());
      need_to_copy = true;
      backup_env_->DeleteFile(dst_path);
    } else {
      // the file is present and referenced by a backup
      Log(options_.info_log, "%s already present, calculate checksum",
          fname.c_str());
      s = CalculateChecksum(src_dir + fname, db_env_, size_limit,
                            &checksum_value);
    }
  }
  live_dst_paths.insert(dst_path);

  if (!contents.empty() || need_to_copy) {
    Log(options_.info_log, "Copying %s to %s", fname.c_str(),
        dst_path_tmp.c_str());
    CopyOrCreateWorkItem copy_or_create_work_item(
        src_dir.empty() ? "" : src_dir + fname, dst_path_tmp, contents, db_env_,
        backup_env_, options_.sync, rate_limiter, size_limit,
        progress_callback);
    BackupAfterCopyOrCreateWorkItem after_copy_or_create_work_item(
        copy_or_create_work_item.result.get_future(), shared, need_to_copy,
        backup_env_, dst_path_tmp, dst_path, dst_relative);
    files_to_copy_or_create_.write(std::move(copy_or_create_work_item));
    backup_items_to_finish.push_back(std::move(after_copy_or_create_work_item));
  } else {
    std::promise<CopyOrCreateResult> promise_result;
    BackupAfterCopyOrCreateWorkItem after_copy_or_create_work_item(
        promise_result.get_future(), shared, need_to_copy, backup_env_,
        dst_path_tmp, dst_path, dst_relative);
    backup_items_to_finish.push_back(std::move(after_copy_or_create_work_item));
    CopyOrCreateResult result;
    result.status = s;
    result.size = size_bytes;
    result.checksum_value = checksum_value;
    promise_result.set_value(std::move(result));
  }
  return s;
}

Status BackupEngineImpl::CalculateChecksum(const std::string& src, Env* src_env,
                                           uint64_t size_limit,
                                           uint32_t* checksum_value) {
  *checksum_value = 0;
  if (size_limit == 0) {
    size_limit = std::numeric_limits<uint64_t>::max();
  }

  EnvOptions env_options;
  env_options.use_mmap_writes = false;
  env_options.use_os_buffer = false;

  std::unique_ptr<SequentialFile> src_file;
  Status s = src_env->NewSequentialFile(src, &src_file, env_options);
  if (!s.ok()) {
    return s;
  }

  unique_ptr<SequentialFileReader> src_reader(
      new SequentialFileReader(std::move(src_file)));
  std::unique_ptr<char[]> buf(new char[copy_file_buffer_size_]);
  Slice data;

  do {
    if (stop_backup_.load(std::memory_order_acquire)) {
      return Status::Incomplete("Backup stopped");
    }
    size_t buffer_to_read = (copy_file_buffer_size_ < size_limit) ?
      copy_file_buffer_size_ : size_limit;
    s = src_reader->Read(buffer_to_read, &data, buf.get());

    if (!s.ok()) {
      return s;
    }

    size_limit -= data.size();
    *checksum_value = crc32c::Extend(*checksum_value, data.data(), data.size());
  } while (data.size() > 0 && size_limit > 0);

  return s;
}

void BackupEngineImpl::DeleteChildren(const std::string& dir,
                                      uint32_t file_type_filter) {
  std::vector<std::string> children;
  db_env_->GetChildren(dir, &children);  // ignore errors

  for (const auto& f : children) {
    uint64_t number;
    FileType type;
    bool ok = ParseFileName(f, &number, &type);
    if (ok && (file_type_filter & (1 << type))) {
      // don't delete this file
      continue;
    }
    db_env_->DeleteFile(dir + "/" + f);  // ignore errors
  }
}

Status BackupEngineImpl::InsertPathnameToSizeBytes(
    const std::string& dir, Env* env,
    std::unordered_map<std::string, uint64_t>* result) {
  assert(result != nullptr);
  std::vector<Env::FileAttributes> files_attrs;
  Status status = env->GetChildrenFileAttributes(dir, &files_attrs);
  if (!status.ok()) {
    return status;
  }
  const bool slash_needed = dir.empty() || dir.back() != '/';
  for (const auto& file_attrs : files_attrs) {
    result->emplace(dir + (slash_needed ? "/" : "") + file_attrs.name,
                    file_attrs.size_bytes);
  }
  return Status::OK();
}

Status BackupEngineImpl::GarbageCollect() {
  assert(!read_only_);
  Log(options_.info_log, "Starting garbage collection");

  if (options_.share_table_files) {
    // delete obsolete shared files
    std::vector<std::string> shared_children;
    {
      auto shared_path = GetAbsolutePath(GetSharedFileRel());
      auto s = backup_env_->FileExists(shared_path);
      if (s.ok()) {
        s = backup_env_->GetChildren(shared_path, &shared_children);
      } else if (s.IsNotFound()) {
        s = Status::OK();
      }
      if (!s.ok()) {
        return s;
      }
    }
    for (auto& child : shared_children) {
      std::string rel_fname = GetSharedFileRel(child);
      auto child_itr = backuped_file_infos_.find(rel_fname);
      // if it's not refcounted, delete it
      if (child_itr == backuped_file_infos_.end() ||
          child_itr->second->refs == 0) {
        // this might be a directory, but DeleteFile will just fail in that
        // case, so we're good
        Status s = backup_env_->DeleteFile(GetAbsolutePath(rel_fname));
        Log(options_.info_log, "Deleting %s -- %s", rel_fname.c_str(),
            s.ToString().c_str());
        backuped_file_infos_.erase(rel_fname);
      }
    }
  }

  // delete obsolete private files
  std::vector<std::string> private_children;
  {
    auto s = backup_env_->GetChildren(GetAbsolutePath(GetPrivateDirRel()),
                                      &private_children);
    if (!s.ok()) {
      return s;
    }
  }
  for (auto& child : private_children) {
    BackupID backup_id = 0;
    bool tmp_dir = child.find(".tmp") != std::string::npos;
    sscanf(child.c_str(), "%u", &backup_id);
    if (!tmp_dir &&  // if it's tmp_dir, delete it
        (backup_id == 0 || backups_.find(backup_id) != backups_.end())) {
      // it's either not a number or it's still alive. continue
      continue;
    }
    // here we have to delete the dir and all its children
    std::string full_private_path =
        GetAbsolutePath(GetPrivateFileRel(backup_id, tmp_dir));
    std::vector<std::string> subchildren;
    backup_env_->GetChildren(full_private_path, &subchildren);
    for (auto& subchild : subchildren) {
      Status s = backup_env_->DeleteFile(full_private_path + subchild);
      Log(options_.info_log, "Deleting %s -- %s",
          (full_private_path + subchild).c_str(), s.ToString().c_str());
    }
    // finally delete the private dir
    Status s = backup_env_->DeleteDir(full_private_path);
    Log(options_.info_log, "Deleting dir %s -- %s", full_private_path.c_str(),
        s.ToString().c_str());
  }

  return Status::OK();
}

// ------- BackupMeta class --------

Status BackupEngineImpl::BackupMeta::AddFile(
    std::shared_ptr<FileInfo> file_info) {
  auto itr = file_infos_->find(file_info->filename);
  if (itr == file_infos_->end()) {
    auto ret = file_infos_->insert({file_info->filename, file_info});
    if (ret.second) {
      itr = ret.first;
      itr->second->refs = 1;
    } else {
      // if this happens, something is seriously wrong
      return Status::Corruption("In memory metadata insertion error");
    }
  } else {
    if (itr->second->checksum_value != file_info->checksum_value) {
      return Status::Corruption(
          "Checksum mismatch for existing backup file. Delete old backups and "
          "try again.");
    }
    ++itr->second->refs;  // increase refcount if already present
  }

  size_ += file_info->size;
  files_.push_back(itr->second);

  return Status::OK();
}

Status BackupEngineImpl::BackupMeta::Delete(bool delete_meta) {
  Status s;
  for (const auto& file : files_) {
    --file->refs;  // decrease refcount
  }
  files_.clear();
  // delete meta file
  if (delete_meta) {
    s = env_->FileExists(meta_filename_);
    if (s.ok()) {
      s = env_->DeleteFile(meta_filename_);
    } else if (s.IsNotFound()) {
      s = Status::OK();  // nothing to delete
    }
  }
  timestamp_ = 0;
  return s;
}

Slice kMetaDataPrefix("metadata ");

// each backup meta file is of the format:
// <timestamp>
// <seq number>
// <metadata(literal string)> <metadata> (optional)
// <number of files>
// <file1> <crc32(literal string)> <crc32_value>
// <file2> <crc32(literal string)> <crc32_value>
// ...
Status BackupEngineImpl::BackupMeta::LoadFromFile(
    const std::string& backup_dir,
    const std::unordered_map<std::string, uint64_t>& abs_path_to_size) {
  assert(Empty());
  Status s;
  unique_ptr<SequentialFile> backup_meta_file;
  s = env_->NewSequentialFile(meta_filename_, &backup_meta_file, EnvOptions());
  if (!s.ok()) {
    return s;
  }

  unique_ptr<SequentialFileReader> backup_meta_reader(
      new SequentialFileReader(std::move(backup_meta_file)));
  unique_ptr<char[]> buf(new char[max_backup_meta_file_size_ + 1]);
  Slice data;
  s = backup_meta_reader->Read(max_backup_meta_file_size_, &data, buf.get());

  if (!s.ok() || data.size() == max_backup_meta_file_size_) {
    return s.ok() ? Status::Corruption("File size too big") : s;
  }
  buf[data.size()] = 0;

  uint32_t num_files = 0;
  char *next;
  timestamp_ = strtoull(data.data(), &next, 10);
  data.remove_prefix(next - data.data() + 1); // +1 for '\n'
  sequence_number_ = strtoull(data.data(), &next, 10);
  data.remove_prefix(next - data.data() + 1); // +1 for '\n'

  if (data.starts_with(kMetaDataPrefix)) {
    // app metadata present
    data.remove_prefix(kMetaDataPrefix.size());
    Slice hex_encoded_metadata = GetSliceUntil(&data, '\n');
    bool decode_success = hex_encoded_metadata.DecodeHex(&app_metadata_);
    if (!decode_success) {
      return Status::Corruption(
          "Failed to decode stored hex encoded app metadata");
    }
  }

  num_files = static_cast<uint32_t>(strtoul(data.data(), &next, 10));
  data.remove_prefix(next - data.data() + 1); // +1 for '\n'

  std::vector<std::shared_ptr<FileInfo>> files;

  Slice checksum_prefix("crc32 ");

  for (uint32_t i = 0; s.ok() && i < num_files; ++i) {
    auto line = GetSliceUntil(&data, '\n');
    std::string filename = GetSliceUntil(&line, ' ').ToString();

    uint64_t size;
    const std::shared_ptr<FileInfo> file_info = GetFile(filename);
    if (file_info) {
      size = file_info->size;
    } else {
      std::string abs_path = backup_dir + "/" + filename;
      try {
        size = abs_path_to_size.at(abs_path);
      } catch (std::out_of_range&) {
        return Status::NotFound("Size missing for pathname: " + abs_path);
      }
    }

    if (line.empty()) {
      return Status::Corruption("File checksum is missing for " + filename +
                                " in " + meta_filename_);
    }

    uint32_t checksum_value = 0;
    if (line.starts_with(checksum_prefix)) {
      line.remove_prefix(checksum_prefix.size());
      checksum_value = static_cast<uint32_t>(
          strtoul(line.data(), nullptr, 10));
      if (line != rocksdb::ToString(checksum_value)) {
        return Status::Corruption("Invalid checksum value for " + filename +
                                  " in " + meta_filename_);
      }
    } else {
      return Status::Corruption("Unknown checksum type for " + filename +
                                " in " + meta_filename_);
    }

    files.emplace_back(new FileInfo(filename, size, checksum_value));
  }

  if (s.ok() && data.size() > 0) {
    // file has to be read completely. if not, we count it as corruption
    s = Status::Corruption("Tailing data in backup meta file in " +
                           meta_filename_);
  }

  if (s.ok()) {
    files_.reserve(files.size());
    for (const auto& file_info : files) {
      s = AddFile(file_info);
      if (!s.ok()) {
        break;
      }
    }
  }

  return s;
}

Status BackupEngineImpl::BackupMeta::StoreToFile(bool sync) {
  Status s;
  unique_ptr<WritableFile> backup_meta_file;
  EnvOptions env_options;
  env_options.use_mmap_writes = false;
  s = env_->NewWritableFile(meta_filename_ + ".tmp", &backup_meta_file,
                            env_options);
  if (!s.ok()) {
    return s;
  }

  unique_ptr<char[]> buf(new char[max_backup_meta_file_size_]);
  size_t len = 0, buf_size = max_backup_meta_file_size_;
  len += snprintf(buf.get(), buf_size, "%" PRId64 "\n", timestamp_);
  len += snprintf(buf.get() + len, buf_size - len, "%" PRIu64 "\n",
                  sequence_number_);
  if (!app_metadata_.empty()) {
    std::string hex_encoded_metadata =
        Slice(app_metadata_).ToString(/* hex */ true);
    if (hex_encoded_metadata.size() + kMetaDataPrefix.size() + 1 >
        buf_size - len) {
      return Status::Corruption("Buffer too small to fit backup metadata");
    }
    memcpy(buf.get() + len, kMetaDataPrefix.data(), kMetaDataPrefix.size());
    len += kMetaDataPrefix.size();
    memcpy(buf.get() + len, hex_encoded_metadata.data(),
           hex_encoded_metadata.size());
    len += hex_encoded_metadata.size();
    buf[len++] = '\n';
  }
  len += snprintf(buf.get() + len, buf_size - len, "%" ROCKSDB_PRIszt "\n",
                  files_.size());
  for (const auto& file : files_) {
    // use crc32 for now, switch to something else if needed
    len += snprintf(buf.get() + len, buf_size - len, "%s crc32 %u\n",
                    file->filename.c_str(), file->checksum_value);
  }

  s = backup_meta_file->Append(Slice(buf.get(), len));
  if (s.ok() && sync) {
    s = backup_meta_file->Sync();
  }
  if (s.ok()) {
    s = backup_meta_file->Close();
  }
  if (s.ok()) {
    s = env_->RenameFile(meta_filename_ + ".tmp", meta_filename_);
  }
  return s;
}

// -------- BackupEngineReadOnlyImpl ---------
class BackupEngineReadOnlyImpl : public BackupEngineReadOnly {
 public:
  BackupEngineReadOnlyImpl(Env* db_env, const BackupableDBOptions& options)
      : backup_engine_(new BackupEngineImpl(db_env, options, true)) {}

  virtual ~BackupEngineReadOnlyImpl() {}

  virtual void GetBackupInfo(std::vector<BackupInfo>* backup_info) override {
    backup_engine_->GetBackupInfo(backup_info);
  }

  virtual void GetCorruptedBackups(
      std::vector<BackupID>* corrupt_backup_ids) override {
    backup_engine_->GetCorruptedBackups(corrupt_backup_ids);
  }

  virtual Status RestoreDBFromBackup(
      BackupID backup_id, const std::string& db_dir, const std::string& wal_dir,
      const RestoreOptions& restore_options = RestoreOptions()) override {
    return backup_engine_->RestoreDBFromBackup(backup_id, db_dir, wal_dir,
                                               restore_options);
  }

  virtual Status RestoreDBFromLatestBackup(
      const std::string& db_dir, const std::string& wal_dir,
      const RestoreOptions& restore_options = RestoreOptions()) override {
    return backup_engine_->RestoreDBFromLatestBackup(db_dir, wal_dir,
                                                     restore_options);
  }

  virtual Status VerifyBackup(BackupID backup_id) override {
    return backup_engine_->VerifyBackup(backup_id);
  }

  Status Initialize() { return backup_engine_->Initialize(); }

 private:
  std::unique_ptr<BackupEngineImpl> backup_engine_;
};

Status BackupEngineReadOnly::Open(Env* env, const BackupableDBOptions& options,
                                  BackupEngineReadOnly** backup_engine_ptr) {
  if (options.destroy_old_data) {
    return Status::InvalidArgument(
        "Can't destroy old data with ReadOnly BackupEngine");
  }
  std::unique_ptr<BackupEngineReadOnlyImpl> backup_engine(
      new BackupEngineReadOnlyImpl(env, options));
  auto s = backup_engine->Initialize();
  if (!s.ok()) {
    *backup_engine_ptr = nullptr;
    return s;
  }
  *backup_engine_ptr = backup_engine.release();
  return Status::OK();
}

}  // namespace rocksdb

#endif  // ROCKSDB_LITE
