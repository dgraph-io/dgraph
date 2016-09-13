//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.
//
// Copyright (c) 2011 The LevelDB Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file. See the AUTHORS file for names of contributors.

#include "db/version_edit.h"

#include "db/version_set.h"
#include "util/coding.h"
#include "util/event_logger.h"
#include "util/sync_point.h"
#include "rocksdb/slice.h"

namespace rocksdb {

// Tag numbers for serialized VersionEdit.  These numbers are written to
// disk and should not be changed.
enum Tag {
  kComparator = 1,
  kLogNumber = 2,
  kNextFileNumber = 3,
  kLastSequence = 4,
  kCompactPointer = 5,
  kDeletedFile = 6,
  kNewFile = 7,
  // 8 was used for large value refs
  kPrevLogNumber = 9,

  // these are new formats divergent from open source leveldb
  kNewFile2 = 100,
  kNewFile3 = 102,
  kNewFile4 = 103,      // 4th (the latest) format version of adding files
  kColumnFamily = 200,  // specify column family for version edit
  kColumnFamilyAdd = 201,
  kColumnFamilyDrop = 202,
  kMaxColumnFamily = 203,
};

enum CustomTag {
  kTerminate = 1,  // The end of customized fields
  kNeedCompaction = 2,
  kPathId = 65,
};
// If this bit for the custom tag is set, opening DB should fail if
// we don't know this field.
uint32_t kCustomTagNonSafeIgnoreMask = 1 << 6;

uint64_t PackFileNumberAndPathId(uint64_t number, uint64_t path_id) {
  assert(number <= kFileNumberMask);
  return number | (path_id * (kFileNumberMask + 1));
}

void VersionEdit::Clear() {
  comparator_.clear();
  max_level_ = 0;
  log_number_ = 0;
  prev_log_number_ = 0;
  last_sequence_ = 0;
  next_file_number_ = 0;
  max_column_family_ = 0;
  has_comparator_ = false;
  has_log_number_ = false;
  has_prev_log_number_ = false;
  has_next_file_number_ = false;
  has_last_sequence_ = false;
  has_max_column_family_ = false;
  deleted_files_.clear();
  new_files_.clear();
  column_family_ = 0;
  is_column_family_add_ = 0;
  is_column_family_drop_ = 0;
  column_family_name_.clear();
}

bool VersionEdit::EncodeTo(std::string* dst) const {
  if (has_comparator_) {
    PutVarint32(dst, kComparator);
    PutLengthPrefixedSlice(dst, comparator_);
  }
  if (has_log_number_) {
    PutVarint32(dst, kLogNumber);
    PutVarint64(dst, log_number_);
  }
  if (has_prev_log_number_) {
    PutVarint32(dst, kPrevLogNumber);
    PutVarint64(dst, prev_log_number_);
  }
  if (has_next_file_number_) {
    PutVarint32(dst, kNextFileNumber);
    PutVarint64(dst, next_file_number_);
  }
  if (has_last_sequence_) {
    PutVarint32(dst, kLastSequence);
    PutVarint64(dst, last_sequence_);
  }
  if (has_max_column_family_) {
    PutVarint32(dst, kMaxColumnFamily);
    PutVarint32(dst, max_column_family_);
  }

  for (const auto& deleted : deleted_files_) {
    PutVarint32(dst, kDeletedFile);
    PutVarint32(dst, deleted.first /* level */);
    PutVarint64(dst, deleted.second /* file number */);
  }

  for (size_t i = 0; i < new_files_.size(); i++) {
    const FileMetaData& f = new_files_[i].second;
    if (!f.smallest.Valid() || !f.largest.Valid()) {
      return false;
    }
    bool has_customized_fields = false;
    if (f.marked_for_compaction) {
      PutVarint32(dst, kNewFile4);
      has_customized_fields = true;
    } else if (f.fd.GetPathId() == 0) {
      // Use older format to make sure user can roll back the build if they
      // don't config multiple DB paths.
      PutVarint32(dst, kNewFile2);
    } else {
      PutVarint32(dst, kNewFile3);
    }
    PutVarint32(dst, new_files_[i].first);  // level
    PutVarint64(dst, f.fd.GetNumber());
    if (f.fd.GetPathId() != 0 && !has_customized_fields) {
      // kNewFile3
      PutVarint32(dst, f.fd.GetPathId());
    }
    PutVarint64(dst, f.fd.GetFileSize());
    PutLengthPrefixedSlice(dst, f.smallest.Encode());
    PutLengthPrefixedSlice(dst, f.largest.Encode());
    PutVarint64(dst, f.smallest_seqno);
    PutVarint64(dst, f.largest_seqno);
    if (has_customized_fields) {
      // Customized fields' format:
      // +-----------------------------+
      // | 1st field's tag (varint32)  |
      // +-----------------------------+
      // | 1st field's size (varint32) |
      // +-----------------------------+
      // |    bytes for 1st field      |
      // |  (based on size decoded)    |
      // +-----------------------------+
      // |                             |
      // |          ......             |
      // |                             |
      // +-----------------------------+
      // | last field's size (varint32)|
      // +-----------------------------+
      // |    bytes for last field     |
      // |  (based on size decoded)    |
      // +-----------------------------+
      // | terminating tag (varint32)  |
      // +-----------------------------+
      //
      // Customized encoding for fields:
      //   tag kPathId: 1 byte as path_id
      //   tag kNeedCompaction:
      //        now only can take one char value 1 indicating need-compaction
      //
      if (f.fd.GetPathId() != 0) {
        PutVarint32(dst, CustomTag::kPathId);
        char p = static_cast<char>(f.fd.GetPathId());
        PutLengthPrefixedSlice(dst, Slice(&p, 1));
      }
      if (f.marked_for_compaction) {
        PutVarint32(dst, CustomTag::kNeedCompaction);
        char p = static_cast<char>(1);
        PutLengthPrefixedSlice(dst, Slice(&p, 1));
      }
      TEST_SYNC_POINT_CALLBACK("VersionEdit::EncodeTo:NewFile4:CustomizeFields",
                               dst);

      PutVarint32(dst, CustomTag::kTerminate);
    }
  }

  // 0 is default and does not need to be explicitly written
  if (column_family_ != 0) {
    PutVarint32(dst, kColumnFamily);
    PutVarint32(dst, column_family_);
  }

  if (is_column_family_add_) {
    PutVarint32(dst, kColumnFamilyAdd);
    PutLengthPrefixedSlice(dst, Slice(column_family_name_));
  }

  if (is_column_family_drop_) {
    PutVarint32(dst, kColumnFamilyDrop);
  }
  return true;
}

static bool GetInternalKey(Slice* input, InternalKey* dst) {
  Slice str;
  if (GetLengthPrefixedSlice(input, &str)) {
    dst->DecodeFrom(str);
    return dst->Valid();
  } else {
    return false;
  }
}

bool VersionEdit::GetLevel(Slice* input, int* level, const char** msg) {
  uint32_t v;
  if (GetVarint32(input, &v)) {
    *level = v;
    if (max_level_ < *level) {
      max_level_ = *level;
    }
    return true;
  } else {
    return false;
  }
}

const char* VersionEdit::DecodeNewFile4From(Slice* input) {
  const char* msg = nullptr;
  int level;
  FileMetaData f;
  uint64_t number;
  uint32_t path_id = 0;
  uint64_t file_size;
  if (GetLevel(input, &level, &msg) && GetVarint64(input, &number) &&
      GetVarint64(input, &file_size) && GetInternalKey(input, &f.smallest) &&
      GetInternalKey(input, &f.largest) &&
      GetVarint64(input, &f.smallest_seqno) &&
      GetVarint64(input, &f.largest_seqno)) {
    // See comments in VersionEdit::EncodeTo() for format of customized fields
    while (true) {
      uint32_t custom_tag;
      Slice field;
      if (!GetVarint32(input, &custom_tag)) {
        return "new-file4 custom field";
      }
      if (custom_tag == kTerminate) {
        break;
      }
      if (!GetLengthPrefixedSlice(input, &field)) {
        return "new-file4 custom field lenth prefixed slice error";
      }
      switch (custom_tag) {
        case kPathId:
          if (field.size() != 1) {
            return "path_id field wrong size";
          }
          path_id = field[0];
          if (path_id > 3) {
            return "path_id wrong vaue";
          }
          break;
        case kNeedCompaction:
          if (field.size() != 1) {
            return "need_compaction field wrong size";
          }
          f.marked_for_compaction = (field[0] == 1);
          break;
        default:
          if ((custom_tag & kCustomTagNonSafeIgnoreMask) != 0) {
            // Should not proceed if cannot understand it
            return "new-file4 custom field not supported";
          }
          break;
      }
    }
  } else {
    return "new-file4 entry";
  }
  f.fd = FileDescriptor(number, path_id, file_size);
  new_files_.push_back(std::make_pair(level, f));
  return nullptr;
}

Status VersionEdit::DecodeFrom(const Slice& src) {
  Clear();
  Slice input = src;
  const char* msg = nullptr;
  uint32_t tag;

  // Temporary storage for parsing
  int level;
  FileMetaData f;
  Slice str;
  InternalKey key;

  while (msg == nullptr && GetVarint32(&input, &tag)) {
    switch (tag) {
      case kComparator:
        if (GetLengthPrefixedSlice(&input, &str)) {
          comparator_ = str.ToString();
          has_comparator_ = true;
        } else {
          msg = "comparator name";
        }
        break;

      case kLogNumber:
        if (GetVarint64(&input, &log_number_)) {
          has_log_number_ = true;
        } else {
          msg = "log number";
        }
        break;

      case kPrevLogNumber:
        if (GetVarint64(&input, &prev_log_number_)) {
          has_prev_log_number_ = true;
        } else {
          msg = "previous log number";
        }
        break;

      case kNextFileNumber:
        if (GetVarint64(&input, &next_file_number_)) {
          has_next_file_number_ = true;
        } else {
          msg = "next file number";
        }
        break;

      case kLastSequence:
        if (GetVarint64(&input, &last_sequence_)) {
          has_last_sequence_ = true;
        } else {
          msg = "last sequence number";
        }
        break;

      case kMaxColumnFamily:
        if (GetVarint32(&input, &max_column_family_)) {
          has_max_column_family_ = true;
        } else {
          msg = "max column family";
        }
        break;

      case kCompactPointer:
        if (GetLevel(&input, &level, &msg) &&
            GetInternalKey(&input, &key)) {
          // we don't use compact pointers anymore,
          // but we should not fail if they are still
          // in manifest
        } else {
          if (!msg) {
            msg = "compaction pointer";
          }
        }
        break;

      case kDeletedFile: {
        uint64_t number;
        if (GetLevel(&input, &level, &msg) && GetVarint64(&input, &number)) {
          deleted_files_.insert(std::make_pair(level, number));
        } else {
          if (!msg) {
            msg = "deleted file";
          }
        }
        break;
      }

      case kNewFile: {
        uint64_t number;
        uint64_t file_size;
        if (GetLevel(&input, &level, &msg) && GetVarint64(&input, &number) &&
            GetVarint64(&input, &file_size) &&
            GetInternalKey(&input, &f.smallest) &&
            GetInternalKey(&input, &f.largest)) {
          f.fd = FileDescriptor(number, 0, file_size);
          new_files_.push_back(std::make_pair(level, f));
        } else {
          if (!msg) {
            msg = "new-file entry";
          }
        }
        break;
      }
      case kNewFile2: {
        uint64_t number;
        uint64_t file_size;
        if (GetLevel(&input, &level, &msg) && GetVarint64(&input, &number) &&
            GetVarint64(&input, &file_size) &&
            GetInternalKey(&input, &f.smallest) &&
            GetInternalKey(&input, &f.largest) &&
            GetVarint64(&input, &f.smallest_seqno) &&
            GetVarint64(&input, &f.largest_seqno)) {
          f.fd = FileDescriptor(number, 0, file_size);
          new_files_.push_back(std::make_pair(level, f));
        } else {
          if (!msg) {
            msg = "new-file2 entry";
          }
        }
        break;
      }

      case kNewFile3: {
        uint64_t number;
        uint32_t path_id;
        uint64_t file_size;
        if (GetLevel(&input, &level, &msg) && GetVarint64(&input, &number) &&
            GetVarint32(&input, &path_id) && GetVarint64(&input, &file_size) &&
            GetInternalKey(&input, &f.smallest) &&
            GetInternalKey(&input, &f.largest) &&
            GetVarint64(&input, &f.smallest_seqno) &&
            GetVarint64(&input, &f.largest_seqno)) {
          f.fd = FileDescriptor(number, path_id, file_size);
          new_files_.push_back(std::make_pair(level, f));
        } else {
          if (!msg) {
            msg = "new-file3 entry";
          }
        }
        break;
      }

      case kNewFile4: {
        msg = DecodeNewFile4From(&input);
        break;
      }

      case kColumnFamily:
        if (!GetVarint32(&input, &column_family_)) {
          if (!msg) {
            msg = "set column family id";
          }
        }
        break;

      case kColumnFamilyAdd:
        if (GetLengthPrefixedSlice(&input, &str)) {
          is_column_family_add_ = true;
          column_family_name_ = str.ToString();
        } else {
          if (!msg) {
            msg = "column family add";
          }
        }
        break;

      case kColumnFamilyDrop:
        is_column_family_drop_ = true;
        break;

      default:
        msg = "unknown tag";
        break;
    }
  }

  if (msg == nullptr && !input.empty()) {
    msg = "invalid tag";
  }

  Status result;
  if (msg != nullptr) {
    result = Status::Corruption("VersionEdit", msg);
  }
  return result;
}

std::string VersionEdit::DebugString(bool hex_key) const {
  std::string r;
  r.append("VersionEdit {");
  if (has_comparator_) {
    r.append("\n  Comparator: ");
    r.append(comparator_);
  }
  if (has_log_number_) {
    r.append("\n  LogNumber: ");
    AppendNumberTo(&r, log_number_);
  }
  if (has_prev_log_number_) {
    r.append("\n  PrevLogNumber: ");
    AppendNumberTo(&r, prev_log_number_);
  }
  if (has_next_file_number_) {
    r.append("\n  NextFileNumber: ");
    AppendNumberTo(&r, next_file_number_);
  }
  if (has_last_sequence_) {
    r.append("\n  LastSeq: ");
    AppendNumberTo(&r, last_sequence_);
  }
  for (DeletedFileSet::const_iterator iter = deleted_files_.begin();
       iter != deleted_files_.end();
       ++iter) {
    r.append("\n  DeleteFile: ");
    AppendNumberTo(&r, iter->first);
    r.append(" ");
    AppendNumberTo(&r, iter->second);
  }
  for (size_t i = 0; i < new_files_.size(); i++) {
    const FileMetaData& f = new_files_[i].second;
    r.append("\n  AddFile: ");
    AppendNumberTo(&r, new_files_[i].first);
    r.append(" ");
    AppendNumberTo(&r, f.fd.GetNumber());
    r.append(" ");
    AppendNumberTo(&r, f.fd.GetFileSize());
    r.append(" ");
    r.append(f.smallest.DebugString(hex_key));
    r.append(" .. ");
    r.append(f.largest.DebugString(hex_key));
  }
  r.append("\n  ColumnFamily: ");
  AppendNumberTo(&r, column_family_);
  if (is_column_family_add_) {
    r.append("\n  ColumnFamilyAdd: ");
    r.append(column_family_name_);
  }
  if (is_column_family_drop_) {
    r.append("\n  ColumnFamilyDrop");
  }
  if (has_max_column_family_) {
    r.append("\n  MaxColumnFamily: ");
    AppendNumberTo(&r, max_column_family_);
  }
  r.append("\n}\n");
  return r;
}

std::string VersionEdit::DebugJSON(int edit_num, bool hex_key) const {
  JSONWriter jw;
  jw << "EditNumber" << edit_num;

  if (has_comparator_) {
    jw << "Comparator" << comparator_;
  }
  if (has_log_number_) {
    jw << "LogNumber" << log_number_;
  }
  if (has_prev_log_number_) {
    jw << "PrevLogNumber" << prev_log_number_;
  }
  if (has_next_file_number_) {
    jw << "NextFileNumber" << next_file_number_;
  }
  if (has_last_sequence_) {
    jw << "LastSeq" << last_sequence_;
  }

  if (!deleted_files_.empty()) {
    jw << "DeletedFiles";
    jw.StartArray();

    for (DeletedFileSet::const_iterator iter = deleted_files_.begin();
         iter != deleted_files_.end();
         ++iter) {
      jw.StartArrayedObject();
      jw << "Level" << iter->first;
      jw << "FileNumber" << iter->second;
      jw.EndArrayedObject();
    }

    jw.EndArray();
  }

  if (!new_files_.empty()) {
    jw << "AddedFiles";
    jw.StartArray();

    for (size_t i = 0; i < new_files_.size(); i++) {
      jw.StartArrayedObject();
      jw << "Level" << new_files_[i].first;
      const FileMetaData& f = new_files_[i].second;
      jw << "FileNumber" << f.fd.GetNumber();
      jw << "FileSize" << f.fd.GetFileSize();
      jw << "SmallestIKey" << f.smallest.DebugString(hex_key);
      jw << "LargestIKey" << f.largest.DebugString(hex_key);
      jw.EndArrayedObject();
    }

    jw.EndArray();
  }

  jw << "ColumnFamily" << column_family_;

  if (is_column_family_add_) {
    jw << "ColumnFamilyAdd" << column_family_name_;
  }
  if (is_column_family_drop_) {
    jw << "ColumnFamilyDrop" << column_family_name_;
  }
  if (has_max_column_family_) {
    jw << "MaxColumnFamily" << max_column_family_;
  }

  jw.EndObject();

  return jw.Get();
}

}  // namespace rocksdb
