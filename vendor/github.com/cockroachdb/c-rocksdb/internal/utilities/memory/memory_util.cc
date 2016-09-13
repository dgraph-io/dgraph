// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.

#ifndef ROCKSDB_LITE

#include "rocksdb/utilities/memory_util.h"

#include "db/db_impl.h"

namespace rocksdb {

Status MemoryUtil::GetApproximateMemoryUsageByType(
    const std::vector<DB*>& dbs,
    const std::unordered_set<const Cache*> cache_set,
    std::map<MemoryUtil::UsageType, uint64_t>* usage_by_type) {
  usage_by_type->clear();

  // MemTable
  for (auto* db : dbs) {
    uint64_t usage = 0;
    if (db->GetAggregatedIntProperty(DB::Properties::kSizeAllMemTables,
                                     &usage)) {
      (*usage_by_type)[MemoryUtil::kMemTableTotal] += usage;
    }
    if (db->GetAggregatedIntProperty(DB::Properties::kCurSizeAllMemTables,
                                     &usage)) {
      (*usage_by_type)[MemoryUtil::kMemTableUnFlushed] += usage;
    }
  }

  // Table Readers
  for (auto* db : dbs) {
    uint64_t usage = 0;
    if (db->GetAggregatedIntProperty(DB::Properties::kEstimateTableReadersMem,
                                     &usage)) {
      (*usage_by_type)[MemoryUtil::kTableReadersTotal] += usage;
    }
  }

  // Cache
  for (const auto* cache : cache_set) {
    if (cache != nullptr) {
      (*usage_by_type)[MemoryUtil::kCacheTotal] += cache->GetUsage();
    }
  }

  return Status::OK();
}
}  // namespace rocksdb
#endif  // !ROCKSDB_LITE
