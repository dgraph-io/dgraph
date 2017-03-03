//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.

#include "db/external_sst_file_ingestion_job.h"

#define __STDC_FORMAT_MACROS

#include <inttypes.h>
#include <algorithm>
#include <string>
#include <vector>

#include "db/version_edit.h"
#include "table/merger.h"
#include "table/scoped_arena_iterator.h"
#include "table/sst_file_writer_collectors.h"
#include "table/table_builder.h"
#include "util/file_reader_writer.h"
#include "util/file_util.h"
#include "util/stop_watch.h"
#include "util/sync_point.h"

namespace rocksdb {

Status ExternalSstFileIngestionJob::Prepare(
    const std::vector<std::string>& external_files_paths) {
  Status status;

  // Read the information of files we are ingesting
  for (const std::string& file_path : external_files_paths) {
    IngestedFileInfo file_to_ingest;
    status = GetIngestedFileInfo(file_path, &file_to_ingest);
    if (!status.ok()) {
      return status;
    }
    files_to_ingest_.push_back(file_to_ingest);
  }

  for (const IngestedFileInfo& f : files_to_ingest_) {
    if (f.cf_id !=
            TablePropertiesCollectorFactory::Context::kUnknownColumnFamily &&
        f.cf_id != cfd_->GetID()) {
      return Status::InvalidArgument(
          "External file column family id dont match");
    }
  }

  const Comparator* ucmp = cfd_->internal_comparator().user_comparator();
  auto num_files = files_to_ingest_.size();
  if (num_files == 0) {
    return Status::InvalidArgument("The list of files is empty");
  } else if (num_files > 1) {
    // Verify that passed files dont have overlapping ranges
    autovector<const IngestedFileInfo*> sorted_files;
    for (size_t i = 0; i < num_files; i++) {
      sorted_files.push_back(&files_to_ingest_[i]);
    }

    std::sort(
        sorted_files.begin(), sorted_files.end(),
        [&ucmp](const IngestedFileInfo* info1, const IngestedFileInfo* info2) {
          return ucmp->Compare(info1->smallest_user_key,
                               info2->smallest_user_key) < 0;
        });

    for (size_t i = 0; i < num_files - 1; i++) {
      if (ucmp->Compare(sorted_files[i]->largest_user_key,
                        sorted_files[i + 1]->smallest_user_key) >= 0) {
        return Status::NotSupported("Files have overlapping ranges");
      }
    }
  }

  for (IngestedFileInfo& f : files_to_ingest_) {
    if (f.num_entries == 0) {
      return Status::InvalidArgument("File contain no entries");
    }

    if (!f.smallest_internal_key().Valid() ||
        !f.largest_internal_key().Valid()) {
      return Status::Corruption("Generated table have corrupted keys");
    }
  }

  // Copy/Move external files into DB
  for (IngestedFileInfo& f : files_to_ingest_) {
    f.fd = FileDescriptor(versions_->NewFileNumber(), 0, f.file_size);

    const std::string path_outside_db = f.external_file_path;
    const std::string path_inside_db =
        TableFileName(db_options_.db_paths, f.fd.GetNumber(), f.fd.GetPathId());

    if (ingestion_options_.move_files) {
      status = env_->LinkFile(path_outside_db, path_inside_db);
      if (status.IsNotSupported()) {
        // Original file is on a different FS, use copy instead of hard linking
        status = CopyFile(env_, path_outside_db, path_inside_db, 0,
                          db_options_.use_fsync);
      }
    } else {
      status = CopyFile(env_, path_outside_db, path_inside_db, 0,
                        db_options_.use_fsync);
    }
    TEST_SYNC_POINT("DBImpl::AddFile:FileCopied");
    if (!status.ok()) {
      break;
    }
    f.internal_file_path = path_inside_db;
  }

  if (!status.ok()) {
    // We failed, remove all files that we copied into the db
    for (IngestedFileInfo& f : files_to_ingest_) {
      if (f.internal_file_path == "") {
        break;
      }
      Status s = env_->DeleteFile(f.internal_file_path);
      if (!s.ok()) {
        Log(InfoLogLevel::WARN_LEVEL, db_options_.info_log,
            "AddFile() clean up for file %s failed : %s",
            f.internal_file_path.c_str(), s.ToString().c_str());
      }
    }
  }

  return status;
}

Status ExternalSstFileIngestionJob::NeedsFlush(bool* flush_needed) {
  SuperVersion* super_version = cfd_->GetSuperVersion();
  Status status =
      IngestedFilesOverlapWithMemtables(super_version, flush_needed);

  if (status.ok() && *flush_needed &&
      !ingestion_options_.allow_blocking_flush) {
    status = Status::InvalidArgument("External file requires flush");
  }
  return status;
}

Status ExternalSstFileIngestionJob::Run() {
  Status status;
#ifndef NDEBUG
  // We should never run the job with a memtable that is overlapping
  // with the files we are ingesting
  bool need_flush = false;
  status = NeedsFlush(&need_flush);
  assert(status.ok() && need_flush == false);
#endif

  bool consumed_seqno = false;
  bool force_global_seqno = false;
  const SequenceNumber last_seqno = versions_->LastSequence();
  if (ingestion_options_.snapshot_consistency && !db_snapshots_->empty()) {
    // We need to assign a global sequence number to all the files even
    // if the dont overlap with any ranges since we have snapshots
    force_global_seqno = true;
  }

  SuperVersion* super_version = cfd_->GetSuperVersion();
  edit_.SetColumnFamily(cfd_->GetID());
  // The levels that the files will be ingested into
  for (IngestedFileInfo& f : files_to_ingest_) {
    bool overlap_with_db = false;
    status = AssignLevelForIngestedFile(super_version, &f, &overlap_with_db);
    if (!status.ok()) {
      return status;
    }

    if (overlap_with_db || force_global_seqno) {
      status = AssignGlobalSeqnoForIngestedFile(&f, last_seqno + 1);
      consumed_seqno = true;
    } else {
      status = AssignGlobalSeqnoForIngestedFile(&f, 0);
    }

    if (!status.ok()) {
      return status;
    }

    edit_.AddFile(f.picked_level, f.fd.GetNumber(), f.fd.GetPathId(),
                  f.fd.GetFileSize(), f.smallest_internal_key(),
                  f.largest_internal_key(), f.assigned_seqno, f.assigned_seqno,
                  false);
  }

  if (consumed_seqno) {
    versions_->SetLastSequence(last_seqno + 1);
  }

  return status;
}

void ExternalSstFileIngestionJob::UpdateStats() {
  // Update internal stats for new ingested files
  uint64_t total_keys = 0;
  uint64_t total_l0_files = 0;
  uint64_t total_time = env_->NowMicros() - job_start_time_;
  for (IngestedFileInfo& f : files_to_ingest_) {
    InternalStats::CompactionStats stats(1);
    stats.micros = total_time;
    stats.bytes_written = f.fd.GetFileSize();
    stats.num_output_files = 1;
    cfd_->internal_stats()->AddCompactionStats(f.picked_level, stats);
    cfd_->internal_stats()->AddCFStats(InternalStats::BYTES_INGESTED_ADD_FILE,
                                       f.fd.GetFileSize());
    total_keys += f.num_entries;
    if (f.picked_level == 0) {
      total_l0_files += 1;
    }
    Log(InfoLogLevel::INFO_LEVEL, db_options_.info_log,
        "[AddFile] External SST file %s was ingested in L%d with path %s "
        "(global_seqno=%" PRIu64 ")\n",
        f.external_file_path.c_str(), f.picked_level,
        f.internal_file_path.c_str(), f.assigned_seqno);
  }
  cfd_->internal_stats()->AddCFStats(InternalStats::INGESTED_NUM_KEYS_TOTAL,
                                     total_keys);
  cfd_->internal_stats()->AddCFStats(InternalStats::INGESTED_NUM_FILES_TOTAL,
                                     files_to_ingest_.size());
  cfd_->internal_stats()->AddCFStats(
      InternalStats::INGESTED_LEVEL0_NUM_FILES_TOTAL, total_l0_files);
}

void ExternalSstFileIngestionJob::Cleanup(const Status& status) {
  if (!status.ok()) {
    // We failed to add the files to the database
    // remove all the files we copied
    for (IngestedFileInfo& f : files_to_ingest_) {
      Status s = env_->DeleteFile(f.internal_file_path);
      if (!s.ok()) {
        Log(InfoLogLevel::WARN_LEVEL, db_options_.info_log,
            "AddFile() clean up for file %s failed : %s",
            f.internal_file_path.c_str(), s.ToString().c_str());
      }
    }
  } else if (status.ok() && ingestion_options_.move_files) {
    // The files were moved and added successfully, remove original file links
    for (IngestedFileInfo& f : files_to_ingest_) {
      Status s = env_->DeleteFile(f.external_file_path);
      if (!s.ok()) {
        Log(InfoLogLevel::WARN_LEVEL, db_options_.info_log,
            "%s was added to DB successfully but failed to remove original "
            "file link : %s",
            f.external_file_path.c_str(), s.ToString().c_str());
      }
    }
  }
}

Status ExternalSstFileIngestionJob::GetIngestedFileInfo(
    const std::string& external_file, IngestedFileInfo* file_to_ingest) {
  file_to_ingest->external_file_path = external_file;

  // Get external file size
  Status status = env_->GetFileSize(external_file, &file_to_ingest->file_size);
  if (!status.ok()) {
    return status;
  }

  // Create TableReader for external file
  std::unique_ptr<TableReader> table_reader;
  std::unique_ptr<RandomAccessFile> sst_file;
  std::unique_ptr<RandomAccessFileReader> sst_file_reader;

  status = env_->NewRandomAccessFile(external_file, &sst_file, env_options_);
  if (!status.ok()) {
    return status;
  }
  sst_file_reader.reset(new RandomAccessFileReader(std::move(sst_file)));

  status = cfd_->ioptions()->table_factory->NewTableReader(
      TableReaderOptions(*cfd_->ioptions(), env_options_,
                         cfd_->internal_comparator()),
      std::move(sst_file_reader), file_to_ingest->file_size, &table_reader);
  if (!status.ok()) {
    return status;
  }

  // Get the external file properties
  auto props = table_reader->GetTableProperties();
  const auto& uprops = props->user_collected_properties;

  // Get table version
  auto version_iter = uprops.find(ExternalSstFilePropertyNames::kVersion);
  if (version_iter == uprops.end()) {
    return Status::Corruption("External file version not found");
  }
  file_to_ingest->version = DecodeFixed32(version_iter->second.c_str());

  auto seqno_iter = uprops.find(ExternalSstFilePropertyNames::kGlobalSeqno);
  if (file_to_ingest->version == 2) {
    // version 2 imply that we have global sequence number
    if (seqno_iter == uprops.end()) {
      return Status::Corruption(
          "External file global sequence number not found");
    }

    // Set the global sequence number
    file_to_ingest->original_seqno = DecodeFixed64(seqno_iter->second.c_str());
    file_to_ingest->global_seqno_offset = props->properties_offsets.at(
        ExternalSstFilePropertyNames::kGlobalSeqno);

    if (file_to_ingest->global_seqno_offset == 0) {
      return Status::Corruption("Was not able to find file global seqno field");
    }
  } else {
    return Status::InvalidArgument("external file version is not supported");
  }
  // Get number of entries in table
  file_to_ingest->num_entries = props->num_entries;

  ParsedInternalKey key;
  ReadOptions ro;
  // During reading the external file we can cache blocks that we read into
  // the block cache, if we later change the global seqno of this file, we will
  // have block in cache that will include keys with wrong seqno.
  // We need to disable fill_cache so that we read from the file without
  // updating the block cache.
  ro.fill_cache = false;
  std::unique_ptr<InternalIterator> iter(table_reader->NewIterator(ro));

  // Get first (smallest) key from file
  iter->SeekToFirst();
  if (!ParseInternalKey(iter->key(), &key)) {
    return Status::Corruption("external file have corrupted keys");
  }
  if (key.sequence != 0) {
    return Status::Corruption("external file have non zero sequence number");
  }
  file_to_ingest->smallest_user_key = key.user_key.ToString();

  // Get last (largest) key from file
  iter->SeekToLast();
  if (!ParseInternalKey(iter->key(), &key)) {
    return Status::Corruption("external file have corrupted keys");
  }
  if (key.sequence != 0) {
    return Status::Corruption("external file have non zero sequence number");
  }
  file_to_ingest->largest_user_key = key.user_key.ToString();

  file_to_ingest->cf_id = static_cast<uint32_t>(props->column_family_id);

  file_to_ingest->table_properties = *props;

  return status;
}

Status ExternalSstFileIngestionJob::IngestedFilesOverlapWithMemtables(
    SuperVersion* sv, bool* overlap) {
  // Create an InternalIterator over all memtables
  Arena arena;
  ReadOptions ro;
  ro.total_order_seek = true;
  MergeIteratorBuilder merge_iter_builder(&cfd_->internal_comparator(), &arena);
  merge_iter_builder.AddIterator(sv->mem->NewIterator(ro, &arena));
  sv->imm->AddIterators(ro, &merge_iter_builder);
  ScopedArenaIterator memtable_iter(merge_iter_builder.Finish());

  Status status;
  *overlap = false;
  for (IngestedFileInfo& f : files_to_ingest_) {
    status =
        IngestedFileOverlapWithIteratorRange(&f, memtable_iter.get(), overlap);
    if (!status.ok() || *overlap == true) {
      break;
    }
  }

  return status;
}

Status ExternalSstFileIngestionJob::AssignLevelForIngestedFile(
    SuperVersion* sv, IngestedFileInfo* file_to_ingest, bool* overlap_with_db) {
  *overlap_with_db = false;

  Arena arena;
  ReadOptions ro;
  ro.total_order_seek = true;

  Status status;
  int target_level = 0;
  auto* vstorage = cfd_->current()->storage_info();
  for (int lvl = 0; lvl < cfd_->NumberLevels(); lvl++) {
    if (lvl > 0 && lvl < vstorage->base_level()) {
      continue;
    }

    if (vstorage->NumLevelFiles(lvl) > 0) {
      bool overlap_with_level = false;
      MergeIteratorBuilder merge_iter_builder(&cfd_->internal_comparator(),
                                              &arena);
      RangeDelAggregator range_del_agg(cfd_->internal_comparator(),
                                       {} /* snapshots */);
      sv->current->AddIteratorsForLevel(ro, env_options_, &merge_iter_builder,
                                        lvl, &range_del_agg);
      if (!range_del_agg.IsEmpty()) {
        return Status::NotSupported(
            "file ingestion with range tombstones is currently unsupported");
      }
      ScopedArenaIterator level_iter(merge_iter_builder.Finish());

      status = IngestedFileOverlapWithIteratorRange(
          file_to_ingest, level_iter.get(), &overlap_with_level);
      if (!status.ok()) {
        return status;
      }

      if (overlap_with_level) {
        // We must use L0 or any level higher than `lvl` to be able to overwrite
        // the keys that we overlap with in this level, We also need to assign
        // this file a seqno to overwrite the existing keys in level `lvl`
        *overlap_with_db = true;
        break;
      }
    }

    // We dont overlap with any keys in this level, but we still need to check
    // if our file can fit in it

    if (IngestedFileFitInLevel(file_to_ingest, lvl)) {
      target_level = lvl;
    }
  }
  file_to_ingest->picked_level = target_level;
  return status;
}

Status ExternalSstFileIngestionJob::AssignGlobalSeqnoForIngestedFile(
    IngestedFileInfo* file_to_ingest, SequenceNumber seqno) {
  if (file_to_ingest->original_seqno == seqno) {
    // This file already have the correct global seqno
    return Status::OK();
  } else if (!ingestion_options_.allow_global_seqno) {
    return Status::InvalidArgument("Global seqno is required, but disabled");
  } else if (file_to_ingest->global_seqno_offset == 0) {
    return Status::InvalidArgument(
        "Trying to set global seqno for a file that dont have a global seqno "
        "field");
  }

  std::unique_ptr<RandomRWFile> rwfile;
  Status status = env_->NewRandomRWFile(file_to_ingest->internal_file_path,
                                        &rwfile, env_options_);
  if (!status.ok()) {
    return status;
  }

  // Write the new seqno in the global sequence number field in the file
  std::string seqno_val;
  PutFixed64(&seqno_val, seqno);
  status = rwfile->Write(file_to_ingest->global_seqno_offset, seqno_val);
  if (status.ok()) {
    file_to_ingest->assigned_seqno = seqno;
  }
  return status;
}

Status ExternalSstFileIngestionJob::IngestedFileOverlapWithIteratorRange(
    const IngestedFileInfo* file_to_ingest, InternalIterator* iter,
    bool* overlap) {
  auto* vstorage = cfd_->current()->storage_info();
  auto* ucmp = vstorage->InternalComparator()->user_comparator();
  InternalKey range_start(file_to_ingest->smallest_user_key, kMaxSequenceNumber,
                          kValueTypeForSeek);
  iter->Seek(range_start.Encode());
  if (!iter->status().ok()) {
    return iter->status();
  }

  *overlap = false;
  if (iter->Valid()) {
    ParsedInternalKey seek_result;
    if (!ParseInternalKey(iter->key(), &seek_result)) {
      return Status::Corruption("DB have corrupted keys");
    }

    if (ucmp->Compare(seek_result.user_key, file_to_ingest->largest_user_key) <=
        0) {
      *overlap = true;
    }
  }

  return iter->status();
}

bool ExternalSstFileIngestionJob::IngestedFileFitInLevel(
    const IngestedFileInfo* file_to_ingest, int level) {
  if (level == 0) {
    // Files can always fit in L0
    return true;
  }

  auto* vstorage = cfd_->current()->storage_info();
  Slice file_smallest_user_key(file_to_ingest->smallest_user_key);
  Slice file_largest_user_key(file_to_ingest->largest_user_key);

  if (vstorage->OverlapInLevel(level, &file_smallest_user_key,
                               &file_largest_user_key)) {
    // File overlap with another files in this level, we cannot
    // add it to this level
    return false;
  }
  if (cfd_->RangeOverlapWithCompaction(file_smallest_user_key,
                                       file_largest_user_key, level)) {
    // File overlap with a running compaction output that will be stored
    // in this level, we cannot add this file to this level
    return false;
  }

  // File did not overlap with level files, our compaction output
  return true;
}

}  // namespace rocksdb
