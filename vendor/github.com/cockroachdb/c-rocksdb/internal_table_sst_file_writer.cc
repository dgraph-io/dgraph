//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.

#include "rocksdb/sst_file_writer.h"

#include <vector>
#include "db/dbformat.h"
#include "rocksdb/table.h"
#include "table/block_based_table_builder.h"
#include "table/sst_file_writer_collectors.h"
#include "util/file_reader_writer.h"
#include "util/sync_point.h"

namespace rocksdb {

const std::string ExternalSstFilePropertyNames::kVersion =
    "rocksdb.external_sst_file.version";
const std::string ExternalSstFilePropertyNames::kGlobalSeqno =
    "rocksdb.external_sst_file.global_seqno";
const size_t kFadviseTrigger = 1024 * 1024; // 1MB

struct SstFileWriter::Rep {
  Rep(const EnvOptions& _env_options, const Options& options,
      const Comparator* _user_comparator, ColumnFamilyHandle* _cfh,
      bool _invalidate_page_cache)
      : env_options(_env_options),
        ioptions(options),
        mutable_cf_options(options),
        internal_comparator(_user_comparator),
        cfh(_cfh),
        invalidate_page_cache(_invalidate_page_cache),
        last_fadvise_size(0) {}

  std::unique_ptr<WritableFileWriter> file_writer;
  std::unique_ptr<TableBuilder> builder;
  EnvOptions env_options;
  ImmutableCFOptions ioptions;
  MutableCFOptions mutable_cf_options;
  InternalKeyComparator internal_comparator;
  ExternalSstFileInfo file_info;
  InternalKey ikey;
  std::string column_family_name;
  ColumnFamilyHandle* cfh;
  // If true, We will give the OS a hint that this file pages is not needed
  // everytime we write 1MB to the file
  bool invalidate_page_cache;
  // the size of the file during the last time we called Fadvise to remove
  // cached pages from page cache.
  uint64_t last_fadvise_size;
};

SstFileWriter::SstFileWriter(const EnvOptions& env_options,
                             const Options& options,
                             const Comparator* user_comparator,
                             ColumnFamilyHandle* column_family,
                             bool invalidate_page_cache)
    : rep_(new Rep(env_options, options, user_comparator, column_family,
                   invalidate_page_cache)) {
  rep_->file_info.file_size = 0;
}

SstFileWriter::~SstFileWriter() {
  if (rep_->builder) {
    // User did not call Finish() or Finish() failed, we need to
    // abandon the builder.
    rep_->builder->Abandon();
  }

  delete rep_;
}

Status SstFileWriter::Open(const std::string& file_path) {
  Rep* r = rep_;
  Status s;
  std::unique_ptr<WritableFile> sst_file;
  s = r->ioptions.env->NewWritableFile(file_path, &sst_file, r->env_options);
  if (!s.ok()) {
    return s;
  }

  CompressionType compression_type;
  if (r->ioptions.bottommost_compression != kDisableCompressionOption) {
    compression_type = r->ioptions.bottommost_compression;
  } else if (!r->ioptions.compression_per_level.empty()) {
    // Use the compression of the last level if we have per level compression
    compression_type = *(r->ioptions.compression_per_level.rbegin());
  } else {
    compression_type = r->mutable_cf_options.compression;
  }

  std::vector<std::unique_ptr<IntTblPropCollectorFactory>>
      int_tbl_prop_collector_factories;

  // SstFileWriter properties collector to add SstFileWriter version.
  int_tbl_prop_collector_factories.emplace_back(
      new SstFileWriterPropertiesCollectorFactory(2 /* version */,
                                                  0 /* global_seqno*/));

  // User collector factories
  auto user_collector_factories =
      r->ioptions.table_properties_collector_factories;
  for (size_t i = 0; i < user_collector_factories.size(); i++) {
    int_tbl_prop_collector_factories.emplace_back(
        new UserKeyTablePropertiesCollectorFactory(
            user_collector_factories[i]));
  }
  int unknown_level = -1;
  uint32_t cf_id;

  if (r->cfh != nullptr) {
    // user explicitly specified that this file will be ingested into cfh,
    // we can persist this information in the file.
    cf_id = r->cfh->GetID();
    r->column_family_name = r->cfh->GetName();
  } else {
    r->column_family_name = "";
    cf_id = TablePropertiesCollectorFactory::Context::kUnknownColumnFamily;
  }

  TableBuilderOptions table_builder_options(
      r->ioptions, r->internal_comparator, &int_tbl_prop_collector_factories,
      compression_type, r->ioptions.compression_opts,
      nullptr /* compression_dict */, false /* skip_filters */,
      r->column_family_name, unknown_level);
  r->file_writer.reset(
      new WritableFileWriter(std::move(sst_file), r->env_options));

  // TODO(tec) : If table_factory is using compressed block cache, we will
  // be adding the external sst file blocks into it, which is wasteful.
  r->builder.reset(r->ioptions.table_factory->NewTableBuilder(
      table_builder_options, cf_id, r->file_writer.get()));

  r->file_info.file_path = file_path;
  r->file_info.file_size = 0;
  r->file_info.num_entries = 0;
  r->file_info.sequence_number = 0;
  r->file_info.version = 2;
  return s;
}

Status SstFileWriter::Add(const Slice& user_key, const Slice& value) {
  Rep* r = rep_;
  if (!r->builder) {
    return Status::InvalidArgument("File is not opened");
  }

  if (r->file_info.num_entries == 0) {
    r->file_info.smallest_key.assign(user_key.data(), user_key.size());
  } else {
    if (r->internal_comparator.user_comparator()->Compare(
            user_key, r->file_info.largest_key) <= 0) {
      // Make sure that keys are added in order
      return Status::InvalidArgument("Keys must be added in order");
    }
  }

  // TODO(tec) : For external SST files we could omit the seqno and type.
  r->ikey.Set(user_key, 0 /* Sequence Number */,
              ValueType::kTypeValue /* Put */);
  r->builder->Add(r->ikey.Encode(), value);

  // update file info
  r->file_info.num_entries++;
  r->file_info.largest_key.assign(user_key.data(), user_key.size());
  r->file_info.file_size = r->builder->FileSize();

  InvalidatePageCache(false /* closing */);

  return Status::OK();
}

Status SstFileWriter::Finish(ExternalSstFileInfo* file_info) {
  Rep* r = rep_;
  if (!r->builder) {
    return Status::InvalidArgument("File is not opened");
  }
  if (r->file_info.num_entries == 0) {
    return Status::InvalidArgument("Cannot create sst file with no entries");
  }

  Status s = r->builder->Finish();
  r->file_info.file_size = r->builder->FileSize();

  if (s.ok()) {
    if (!r->ioptions.disable_data_sync) {
      s = r->file_writer->Sync(r->ioptions.use_fsync);
    }
    InvalidatePageCache(true /* closing */);
    if (s.ok()) {
      s = r->file_writer->Close();
    }
  } else {
    r->builder->Abandon();
  }

  if (!s.ok()) {
    r->ioptions.env->DeleteFile(r->file_info.file_path);
  }

  if (file_info != nullptr) {
    *file_info = r->file_info;
  }

  r->builder.reset();
  return s;
}

void SstFileWriter::InvalidatePageCache(bool closing) {
  Rep* r = rep_;
  if (r->invalidate_page_cache == false) {
    // Fadvise disabled
    return;
  }

  uint64_t bytes_since_last_fadvise =
      r->builder->FileSize() - r->last_fadvise_size;
  if (bytes_since_last_fadvise > kFadviseTrigger || closing) {
    TEST_SYNC_POINT_CALLBACK("SstFileWriter::InvalidatePageCache",
                             &(bytes_since_last_fadvise));
    // Tell the OS that we dont need this file in page cache
    r->file_writer->InvalidateCache(0, 0);
    r->last_fadvise_size = r->builder->FileSize();
  }
}

uint64_t SstFileWriter::FileSize() {
  return rep_->file_info.file_size;
}
}  // namespace rocksdb
