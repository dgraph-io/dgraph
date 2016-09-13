// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.

#ifndef ROCKSDB_LITE

#include "util/options_parser.h"

#include <cmath>
#include <map>
#include <string>
#include <utility>
#include <vector>

#include "rocksdb/convenience.h"
#include "rocksdb/db.h"
#include "util/options_helper.h"
#include "util/string_util.h"
#include "util/sync_point.h"

#include "port/port.h"

namespace rocksdb {

static const std::string option_file_header =
    "# This is a RocksDB option file.\n"
    "#\n"
    "# For detailed file format spec, please refer to the example file\n"
    "# in examples/rocksdb_option_file_example.ini\n"
    "#\n"
    "\n";

Status PersistRocksDBOptions(const DBOptions& db_opt,
                             const std::vector<std::string>& cf_names,
                             const std::vector<ColumnFamilyOptions>& cf_opts,
                             const std::string& file_name, Env* env) {
  TEST_SYNC_POINT("PersistRocksDBOptions:start");
  if (cf_names.size() != cf_opts.size()) {
    return Status::InvalidArgument(
        "cf_names.size() and cf_opts.size() must be the same");
  }
  std::unique_ptr<WritableFile> writable;

  Status s = env->NewWritableFile(file_name, &writable, EnvOptions());
  if (!s.ok()) {
    return s;
  }
  std::string options_file_content;

  writable->Append(option_file_header + "[" +
                   opt_section_titles[kOptionSectionVersion] +
                   "]\n"
                   "  rocksdb_version=" +
                   ToString(ROCKSDB_MAJOR) + "." + ToString(ROCKSDB_MINOR) +
                   "." + ToString(ROCKSDB_PATCH) + "\n");
  writable->Append("  options_file_version=" +
                   ToString(ROCKSDB_OPTION_FILE_MAJOR) + "." +
                   ToString(ROCKSDB_OPTION_FILE_MINOR) + "\n");
  writable->Append("\n[" + opt_section_titles[kOptionSectionDBOptions] +
                   "]\n  ");

  s = GetStringFromDBOptions(&options_file_content, db_opt, "\n  ");
  if (!s.ok()) {
    writable->Close();
    return s;
  }
  writable->Append(options_file_content + "\n");

  for (size_t i = 0; i < cf_opts.size(); ++i) {
    // CFOptions section
    writable->Append("\n[" + opt_section_titles[kOptionSectionCFOptions] +
                     " \"" + EscapeOptionString(cf_names[i]) + "\"]\n  ");
    s = GetStringFromColumnFamilyOptions(&options_file_content, cf_opts[i],
                                         "\n  ");
    if (!s.ok()) {
      writable->Close();
      return s;
    }
    writable->Append(options_file_content + "\n");
    // TableOptions section
    auto* tf = cf_opts[i].table_factory.get();
    if (tf != nullptr) {
      writable->Append("[" + opt_section_titles[kOptionSectionTableOptions] +
                       tf->Name() + " \"" + EscapeOptionString(cf_names[i]) +
                       "\"]\n  ");
      s = GetStringFromTableFactory(&options_file_content, tf, "\n  ");
      if (!s.ok()) {
        return s;
      }
      writable->Append(options_file_content + "\n");
    }
  }
  writable->Flush();
  writable->Fsync();
  writable->Close();

  return RocksDBOptionsParser::VerifyRocksDBOptionsFromFile(
      db_opt, cf_names, cf_opts, file_name, env);
}

RocksDBOptionsParser::RocksDBOptionsParser() { Reset(); }

void RocksDBOptionsParser::Reset() {
  db_opt_ = DBOptions();
  db_opt_map_.clear();
  cf_names_.clear();
  cf_opts_.clear();
  cf_opt_maps_.clear();
  has_version_section_ = false;
  has_db_options_ = false;
  has_default_cf_options_ = false;
  for (int i = 0; i < 3; ++i) {
    db_version[i] = 0;
    opt_file_version[i] = 0;
  }
}

bool RocksDBOptionsParser::IsSection(const std::string& line) {
  if (line.size() < 2) {
    return false;
  }
  if (line[0] != '[' || line[line.size() - 1] != ']') {
    return false;
  }
  return true;
}

Status RocksDBOptionsParser::ParseSection(OptionSection* section,
                                          std::string* title,
                                          std::string* argument,
                                          const std::string& line,
                                          const int line_num) {
  *section = kOptionSectionUnknown;
  // A section is of the form [<SectionName> "<SectionArg>"], where
  // "<SectionArg>" is optional.
  size_t arg_start_pos = line.find("\"");
  size_t arg_end_pos = line.rfind("\"");
  // The following if-then check tries to identify whether the input
  // section has the optional section argument.
  if (arg_start_pos != std::string::npos && arg_start_pos != arg_end_pos) {
    *title = TrimAndRemoveComment(line.substr(1, arg_start_pos - 1), true);
    *argument = UnescapeOptionString(
        line.substr(arg_start_pos + 1, arg_end_pos - arg_start_pos - 1));
  } else {
    *title = TrimAndRemoveComment(line.substr(1, line.size() - 2), true);
    *argument = "";
  }
  for (int i = 0; i < kOptionSectionUnknown; ++i) {
    if (title->find(opt_section_titles[i]) == 0) {
      if (i == kOptionSectionVersion || i == kOptionSectionDBOptions ||
          i == kOptionSectionCFOptions) {
        if (title->size() == opt_section_titles[i].size()) {
          // if true, then it indicats equal
          *section = static_cast<OptionSection>(i);
          return CheckSection(*section, *argument, line_num);
        }
      } else if (i == kOptionSectionTableOptions) {
        // This type of sections has a sufffix at the end of the
        // section title
        if (title->size() > opt_section_titles[i].size()) {
          *section = static_cast<OptionSection>(i);
          return CheckSection(*section, *argument, line_num);
        }
      }
    }
  }
  return Status::InvalidArgument(std::string("Unknown section ") + line);
}

Status RocksDBOptionsParser::InvalidArgument(const int line_num,
                                             const std::string& message) {
  return Status::InvalidArgument(
      "[RocksDBOptionsParser Error] ",
      message + " (at line " + ToString(line_num) + ")");
}

Status RocksDBOptionsParser::ParseStatement(std::string* name,
                                            std::string* value,
                                            const std::string& line,
                                            const int line_num) {
  size_t eq_pos = line.find("=");
  if (eq_pos == std::string::npos) {
    return InvalidArgument(line_num, "A valid statement must have a '='.");
  }

  *name = TrimAndRemoveComment(line.substr(0, eq_pos), true);
  *value =
      TrimAndRemoveComment(line.substr(eq_pos + 1, line.size() - eq_pos - 1));
  if (name->empty()) {
    return InvalidArgument(line_num,
                           "A valid statement must have a variable name.");
  }
  return Status::OK();
}

namespace {
bool ReadOneLine(std::istringstream* iss, SequentialFile* seq_file,
                 std::string* output, bool* has_data, Status* result) {
  const int kBufferSize = 4096;
  char buffer[kBufferSize + 1];
  Slice input_slice;

  std::string line;
  bool has_complete_line = false;
  while (!has_complete_line) {
    if (std::getline(*iss, line)) {
      has_complete_line = !iss->eof();
    } else {
      has_complete_line = false;
    }
    if (!has_complete_line) {
      // if we're not sure whether we have a complete line,
      // further read from the file.
      if (*has_data) {
        *result = seq_file->Read(kBufferSize, &input_slice, buffer);
      }
      if (input_slice.size() == 0) {
        // meaning we have read all the data
        *has_data = false;
        break;
      } else {
        iss->str(line + input_slice.ToString());
        // reset the internal state of iss so that we can keep reading it.
        iss->clear();
        *has_data = (input_slice.size() == kBufferSize);
        continue;
      }
    }
  }
  *output = line;
  return *has_data || has_complete_line;
}
}  // namespace

Status RocksDBOptionsParser::Parse(const std::string& file_name, Env* env) {
  Reset();

  std::unique_ptr<SequentialFile> seq_file;
  Status s = env->NewSequentialFile(file_name, &seq_file, EnvOptions());
  if (!s.ok()) {
    return s;
  }

  OptionSection section = kOptionSectionUnknown;
  std::string title;
  std::string argument;
  std::unordered_map<std::string, std::string> opt_map;
  std::istringstream iss;
  std::string line;
  bool has_data = true;
  // we only support single-lined statement.
  for (int line_num = 1;
       ReadOneLine(&iss, seq_file.get(), &line, &has_data, &s); ++line_num) {
    if (!s.ok()) {
      return s;
    }
    line = TrimAndRemoveComment(line);
    if (line.empty()) {
      continue;
    }
    if (IsSection(line)) {
      s = EndSection(section, title, argument, opt_map);
      opt_map.clear();
      if (!s.ok()) {
        return s;
      }
      s = ParseSection(&section, &title, &argument, line, line_num);
      if (!s.ok()) {
        return s;
      }
    } else {
      std::string name;
      std::string value;
      s = ParseStatement(&name, &value, line, line_num);
      if (!s.ok()) {
        return s;
      }
      opt_map.insert({name, value});
    }
  }

  s = EndSection(section, title, argument, opt_map);
  opt_map.clear();
  if (!s.ok()) {
    return s;
  }
  return ValidityCheck();
}

Status RocksDBOptionsParser::CheckSection(const OptionSection section,
                                          const std::string& section_arg,
                                          const int line_num) {
  if (section == kOptionSectionDBOptions) {
    if (has_db_options_) {
      return InvalidArgument(
          line_num,
          "More than one DBOption section found in the option config file");
    }
    has_db_options_ = true;
  } else if (section == kOptionSectionCFOptions) {
    bool is_default_cf = (section_arg == kDefaultColumnFamilyName);
    if (cf_opts_.size() == 0 && !is_default_cf) {
      return InvalidArgument(
          line_num,
          "Default column family must be the first CFOptions section "
          "in the option config file");
    } else if (cf_opts_.size() != 0 && is_default_cf) {
      return InvalidArgument(
          line_num,
          "Default column family must be the first CFOptions section "
          "in the optio/n config file");
    } else if (GetCFOptions(section_arg) != nullptr) {
      return InvalidArgument(
          line_num,
          "Two identical column families found in option config file");
    }
    has_default_cf_options_ |= is_default_cf;
  } else if (section == kOptionSectionTableOptions) {
    if (GetCFOptions(section_arg) == nullptr) {
      return InvalidArgument(
          line_num, std::string(
                        "Does not find a matched column family name in "
                        "TableOptions section.  Column Family Name:") +
                        section_arg);
    }
  } else if (section == kOptionSectionVersion) {
    if (has_version_section_) {
      return InvalidArgument(
          line_num,
          "More than one Version section found in the option config file.");
    }
    has_version_section_ = true;
  }
  return Status::OK();
}

Status RocksDBOptionsParser::ParseVersionNumber(const std::string& ver_name,
                                                const std::string& ver_string,
                                                const int max_count,
                                                int* version) {
  int version_index = 0;
  int current_number = 0;
  int current_digit_count = 0;
  bool has_dot = false;
  for (int i = 0; i < max_count; ++i) {
    version[i] = 0;
  }
  const int kBufferSize = 200;
  char buffer[kBufferSize];
  for (size_t i = 0; i < ver_string.size(); ++i) {
    if (ver_string[i] == '.') {
      if (version_index >= max_count - 1) {
        snprintf(buffer, sizeof(buffer) - 1,
                 "A valid %s can only contains at most %d dots.",
                 ver_name.c_str(), max_count - 1);
        return Status::InvalidArgument(buffer);
      }
      if (current_digit_count == 0) {
        snprintf(buffer, sizeof(buffer) - 1,
                 "A valid %s must have at least one digit before each dot.",
                 ver_name.c_str());
        return Status::InvalidArgument(buffer);
      }
      version[version_index++] = current_number;
      current_number = 0;
      current_digit_count = 0;
      has_dot = true;
    } else if (isdigit(ver_string[i])) {
      current_number = current_number * 10 + (ver_string[i] - '0');
      current_digit_count++;
    } else {
      snprintf(buffer, sizeof(buffer) - 1,
               "A valid %s can only contains dots and numbers.",
               ver_name.c_str());
      return Status::InvalidArgument(buffer);
    }
  }
  version[version_index] = current_number;
  if (has_dot && current_digit_count == 0) {
    snprintf(buffer, sizeof(buffer) - 1,
             "A valid %s must have at least one digit after each dot.",
             ver_name.c_str());
    return Status::InvalidArgument(buffer);
  }
  return Status::OK();
}

Status RocksDBOptionsParser::EndSection(
    const OptionSection section, const std::string& section_title,
    const std::string& section_arg,
    const std::unordered_map<std::string, std::string>& opt_map) {
  Status s;
  if (section == kOptionSectionDBOptions) {
    s = GetDBOptionsFromMap(DBOptions(), opt_map, &db_opt_, true);
    if (!s.ok()) {
      return s;
    }
    db_opt_map_ = opt_map;
  } else if (section == kOptionSectionCFOptions) {
    // This condition should be ensured earlier in ParseSection
    // so we make an assertion here.
    assert(GetCFOptions(section_arg) == nullptr);
    cf_names_.emplace_back(section_arg);
    cf_opts_.emplace_back();
    s = GetColumnFamilyOptionsFromMap(ColumnFamilyOptions(), opt_map,
                                      &cf_opts_.back(), true);
    if (!s.ok()) {
      return s;
    }
    // keep the parsed string.
    cf_opt_maps_.emplace_back(opt_map);
  } else if (section == kOptionSectionTableOptions) {
    assert(GetCFOptions(section_arg) != nullptr);
    auto* cf_opt = GetCFOptionsImpl(section_arg);
    if (cf_opt == nullptr) {
      return Status::InvalidArgument(
          "The specified column family must be defined before the "
          "TableOptions section:",
          section_arg);
    }
    // Ignore error as table factory deserialization is optional
    s = GetTableFactoryFromMap(
        section_title.substr(
            opt_section_titles[kOptionSectionTableOptions].size()),
        opt_map, &(cf_opt->table_factory));
    if (!s.ok()) {
      return s;
    }
  } else if (section == kOptionSectionVersion) {
    for (const auto pair : opt_map) {
      if (pair.first == "rocksdb_version") {
        s = ParseVersionNumber(pair.first, pair.second, 3, db_version);
        if (!s.ok()) {
          return s;
        }
      } else if (pair.first == "options_file_version") {
        s = ParseVersionNumber(pair.first, pair.second, 2, opt_file_version);
        if (!s.ok()) {
          return s;
        }
        if (opt_file_version[0] < 1) {
          return Status::InvalidArgument(
              "A valid options_file_version must be at least 1.");
        }
      }
    }
  }
  return Status::OK();
}

Status RocksDBOptionsParser::ValidityCheck() {
  if (!has_db_options_) {
    return Status::Corruption(
        "A RocksDB Option file must have a single DBOptions section");
  }
  if (!has_default_cf_options_) {
    return Status::Corruption(
        "A RocksDB Option file must have a single CFOptions:default section");
  }

  return Status::OK();
}

std::string RocksDBOptionsParser::TrimAndRemoveComment(const std::string& line,
                                                       bool trim_only) {
  size_t start = 0;
  size_t end = line.size();

  // we only support "#" style comment
  if (!trim_only) {
    size_t search_pos = 0;
    while (search_pos < line.size()) {
      size_t comment_pos = line.find('#', search_pos);
      if (comment_pos == std::string::npos) {
        break;
      }
      if (comment_pos == 0 || line[comment_pos - 1] != '\\') {
        end = comment_pos;
        break;
      }
      search_pos = comment_pos + 1;
    }
  }

  while (start < end && isspace(line[start]) != 0) {
    ++start;
  }

  // start < end implies end > 0.
  while (start < end && isspace(line[end - 1]) != 0) {
    --end;
  }

  if (start < end) {
    return line.substr(start, end - start);
  }

  return "";
}

namespace {
bool AreEqualDoubles(const double a, const double b) {
  return (fabs(a - b) < 0.00001);
}

bool AreEqualOptions(
    const char* opt1, const char* opt2, const OptionTypeInfo& type_info,
    const std::string& opt_name,
    const std::unordered_map<std::string, std::string>* opt_map) {
  const char* offset1 = opt1 + type_info.offset;
  const char* offset2 = opt2 + type_info.offset;
  static const std::string kNullptrString = "nullptr";
  switch (type_info.type) {
    case OptionType::kBoolean:
      return (*reinterpret_cast<const bool*>(offset1) ==
              *reinterpret_cast<const bool*>(offset2));
    case OptionType::kInt:
      return (*reinterpret_cast<const int*>(offset1) ==
              *reinterpret_cast<const int*>(offset2));
    case OptionType::kUInt:
      return (*reinterpret_cast<const unsigned int*>(offset1) ==
              *reinterpret_cast<const unsigned int*>(offset2));
    case OptionType::kUInt32T:
      return (*reinterpret_cast<const uint32_t*>(offset1) ==
              *reinterpret_cast<const uint32_t*>(offset2));
    case OptionType::kUInt64T:
      return (*reinterpret_cast<const uint64_t*>(offset1) ==
              *reinterpret_cast<const uint64_t*>(offset2));
    case OptionType::kSizeT:
      return (*reinterpret_cast<const size_t*>(offset1) ==
              *reinterpret_cast<const size_t*>(offset2));
    case OptionType::kString:
      return (*reinterpret_cast<const std::string*>(offset1) ==
              *reinterpret_cast<const std::string*>(offset2));
    case OptionType::kDouble:
      return AreEqualDoubles(*reinterpret_cast<const double*>(offset1),
                             *reinterpret_cast<const double*>(offset2));
    case OptionType::kCompactionStyle:
      return (*reinterpret_cast<const CompactionStyle*>(offset1) ==
              *reinterpret_cast<const CompactionStyle*>(offset2));
    case OptionType::kCompressionType:
      return (*reinterpret_cast<const CompressionType*>(offset1) ==
              *reinterpret_cast<const CompressionType*>(offset2));
    case OptionType::kVectorCompressionType: {
      const auto* vec1 =
          reinterpret_cast<const std::vector<CompressionType>*>(offset1);
      const auto* vec2 =
          reinterpret_cast<const std::vector<CompressionType>*>(offset2);
      return (*vec1 == *vec2);
    }
    case OptionType::kChecksumType:
      return (*reinterpret_cast<const ChecksumType*>(offset1) ==
              *reinterpret_cast<const ChecksumType*>(offset2));
    case OptionType::kBlockBasedTableIndexType:
      return (
          *reinterpret_cast<const BlockBasedTableOptions::IndexType*>(
              offset1) ==
          *reinterpret_cast<const BlockBasedTableOptions::IndexType*>(offset2));
    case OptionType::kWALRecoveryMode:
      return (*reinterpret_cast<const WALRecoveryMode*>(offset1) ==
              *reinterpret_cast<const WALRecoveryMode*>(offset2));
    case OptionType::kAccessHint:
      return (*reinterpret_cast<const DBOptions::AccessHint*>(offset1) ==
              *reinterpret_cast<const DBOptions::AccessHint*>(offset2));
    case OptionType::kInfoLogLevel:
      return (*reinterpret_cast<const InfoLogLevel*>(offset1) ==
              *reinterpret_cast<const InfoLogLevel*>(offset2));
    default:
      if (type_info.verification == OptionVerificationType::kByName ||
          type_info.verification == OptionVerificationType::kByNameAllowNull) {
        std::string value1;
        bool result =
            SerializeSingleOptionHelper(offset1, type_info.type, &value1);
        if (result == false) {
          return false;
        }
        if (opt_map == nullptr) {
          return true;
        }
        auto iter = opt_map->find(opt_name);
        if (iter == opt_map->end()) {
          return true;
        } else {
          if (type_info.verification ==
              OptionVerificationType::kByNameAllowNull) {
            if (iter->second == kNullptrString || value1 == kNullptrString) {
              return true;
            }
          }
          return (value1 == iter->second);
        }
      }
      return false;
  }
}

}  // namespace

Status RocksDBOptionsParser::VerifyRocksDBOptionsFromFile(
    const DBOptions& db_opt, const std::vector<std::string>& cf_names,
    const std::vector<ColumnFamilyOptions>& cf_opts,
    const std::string& file_name, Env* env,
    OptionsSanityCheckLevel sanity_check_level) {
  RocksDBOptionsParser parser;
  std::unique_ptr<SequentialFile> seq_file;
  Status s = parser.Parse(file_name, env);
  if (!s.ok()) {
    return s;
  }

  // Verify DBOptions
  s = VerifyDBOptions(db_opt, *parser.db_opt(), parser.db_opt_map(),
                      sanity_check_level);
  if (!s.ok()) {
    return s;
  }

  // Verify ColumnFamily Name
  if (cf_names.size() != parser.cf_names()->size()) {
    if (sanity_check_level >= kSanityLevelLooselyCompatible) {
      return Status::InvalidArgument(
          "[RocksDBOptionParser Error] The persisted options does not have "
          "the same number of column family names as the db instance.");
    } else if (cf_opts.size() > parser.cf_opts()->size()) {
      return Status::InvalidArgument(
          "[RocksDBOptionsParser Error]",
          "The persisted options file has less number of column family "
          "names than that of the specified one.");
    }
  }
  for (size_t i = 0; i < cf_names.size(); ++i) {
    if (cf_names[i] != parser.cf_names()->at(i)) {
      return Status::InvalidArgument(
          "[RocksDBOptionParser Error] The persisted options and the db"
          "instance does not have the same name for column family ",
          ToString(i));
    }
  }

  // Verify Column Family Options
  if (cf_opts.size() != parser.cf_opts()->size()) {
    if (sanity_check_level >= kSanityLevelLooselyCompatible) {
      return Status::InvalidArgument(
          "[RocksDBOptionsParser Error]",
          "The persisted options does not have the same number of "
          "column families as the db instance.");
    } else if (cf_opts.size() > parser.cf_opts()->size()) {
      return Status::InvalidArgument(
          "[RocksDBOptionsParser Error]",
          "The persisted options file has less number of column families "
          "than that of the specified number.");
    }
  }
  for (size_t i = 0; i < cf_opts.size(); ++i) {
    s = VerifyCFOptions(cf_opts[i], parser.cf_opts()->at(i),
                        &(parser.cf_opt_maps()->at(i)), sanity_check_level);
    if (!s.ok()) {
      return s;
    }
    s = VerifyTableFactory(cf_opts[i].table_factory.get(),
                           parser.cf_opts()->at(i).table_factory.get(),
                           sanity_check_level);
    if (!s.ok()) {
      return s;
    }
  }

  return Status::OK();
}

Status RocksDBOptionsParser::VerifyDBOptions(
    const DBOptions& base_opt, const DBOptions& persisted_opt,
    const std::unordered_map<std::string, std::string>* opt_map,
    OptionsSanityCheckLevel sanity_check_level) {
  for (auto pair : db_options_type_info) {
    if (pair.second.verification == OptionVerificationType::kDeprecated) {
      // We skip checking deprecated variables as they might
      // contain random values since they might not be initialized
      continue;
    }
    if (DBOptionSanityCheckLevel(pair.first) <= sanity_check_level) {
      if (!AreEqualOptions(reinterpret_cast<const char*>(&base_opt),
                           reinterpret_cast<const char*>(&persisted_opt),
                           pair.second, pair.first, nullptr)) {
        const size_t kBufferSize = 2048;
        char buffer[kBufferSize];
        std::string base_value;
        std::string persisted_value;
        SerializeSingleOptionHelper(
            reinterpret_cast<const char*>(&base_opt) + pair.second.offset,
            pair.second.type, &base_value);
        SerializeSingleOptionHelper(
            reinterpret_cast<const char*>(&persisted_opt) + pair.second.offset,
            pair.second.type, &persisted_value);
        snprintf(buffer, sizeof(buffer),
                 "[RocksDBOptionsParser]: "
                 "failed the verification on DBOptions::%s --- "
                 "The specified one is %s while the persisted one is %s.\n",
                 pair.first.c_str(), base_value.c_str(),
                 persisted_value.c_str());
        return Status::InvalidArgument(Slice(buffer, strlen(buffer)));
      }
    }
  }
  return Status::OK();
}

Status RocksDBOptionsParser::VerifyCFOptions(
    const ColumnFamilyOptions& base_opt,
    const ColumnFamilyOptions& persisted_opt,
    const std::unordered_map<std::string, std::string>* persisted_opt_map,
    OptionsSanityCheckLevel sanity_check_level) {
  for (auto& pair : cf_options_type_info) {
    if (pair.second.verification == OptionVerificationType::kDeprecated) {
      // We skip checking deprecated variables as they might
      // contain random values since they might not be initialized
      continue;
    }
    if (CFOptionSanityCheckLevel(pair.first) <= sanity_check_level) {
      if (!AreEqualOptions(reinterpret_cast<const char*>(&base_opt),
                           reinterpret_cast<const char*>(&persisted_opt),
                           pair.second, pair.first, persisted_opt_map)) {
        const size_t kBufferSize = 2048;
        char buffer[kBufferSize];
        std::string base_value;
        std::string persisted_value;
        SerializeSingleOptionHelper(
            reinterpret_cast<const char*>(&base_opt) + pair.second.offset,
            pair.second.type, &base_value);
        SerializeSingleOptionHelper(
            reinterpret_cast<const char*>(&persisted_opt) + pair.second.offset,
            pair.second.type, &persisted_value);
        snprintf(buffer, sizeof(buffer),
                 "[RocksDBOptionsParser]: "
                 "failed the verification on ColumnFamilyOptions::%s --- "
                 "The specified one is %s while the persisted one is %s.\n",
                 pair.first.c_str(), base_value.c_str(),
                 persisted_value.c_str());
        return Status::InvalidArgument(Slice(buffer, sizeof(buffer)));
      }
    }
  }
  return Status::OK();
}

Status RocksDBOptionsParser::VerifyBlockBasedTableFactory(
    const BlockBasedTableFactory* base_tf,
    const BlockBasedTableFactory* file_tf,
    OptionsSanityCheckLevel sanity_check_level) {
  if ((base_tf != nullptr) != (file_tf != nullptr) &&
      sanity_check_level > kSanityLevelNone) {
    return Status::Corruption(
        "[RocksDBOptionsParser]: Inconsistent TableFactory class type");
  }
  if (base_tf == nullptr) {
    return Status::OK();
  }

  const auto& base_opt = base_tf->table_options();
  const auto& file_opt = file_tf->table_options();

  for (auto& pair : block_based_table_type_info) {
    if (pair.second.verification == OptionVerificationType::kDeprecated) {
      // We skip checking deprecated variables as they might
      // contain random values since they might not be initialized
      continue;
    }
    if (BBTOptionSanityCheckLevel(pair.first) <= sanity_check_level) {
      if (!AreEqualOptions(reinterpret_cast<const char*>(&base_opt),
                           reinterpret_cast<const char*>(&file_opt),
                           pair.second, pair.first, nullptr)) {
        return Status::Corruption(
            "[RocksDBOptionsParser]: "
            "failed the verification on BlockBasedTableOptions::",
            pair.first);
      }
    }
  }
  return Status::OK();
}

Status RocksDBOptionsParser::VerifyTableFactory(
    const TableFactory* base_tf, const TableFactory* file_tf,
    OptionsSanityCheckLevel sanity_check_level) {
  if (base_tf && file_tf) {
    if (sanity_check_level > kSanityLevelNone &&
        base_tf->Name() != file_tf->Name()) {
      return Status::Corruption(
          "[RocksDBOptionsParser]: "
          "failed the verification on TableFactory->Name()");
    }
    auto s = VerifyBlockBasedTableFactory(
        dynamic_cast<const BlockBasedTableFactory*>(base_tf),
        dynamic_cast<const BlockBasedTableFactory*>(file_tf),
        sanity_check_level);
    if (!s.ok()) {
      return s;
    }
    // TODO(yhchiang): add checks for other table factory types
  } else {
    // TODO(yhchiang): further support sanity check here
  }
  return Status::OK();
}
}  // namespace rocksdb

#endif  // !ROCKSDB_LITE
