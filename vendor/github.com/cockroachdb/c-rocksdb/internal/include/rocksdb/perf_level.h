// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.

#ifndef INCLUDE_ROCKSDB_PERF_LEVEL_H_
#define INCLUDE_ROCKSDB_PERF_LEVEL_H_

#include <stdint.h>
#include <string>

namespace rocksdb {

// How much perf stats to collect. Affects perf_context and iostats_context.

enum PerfLevel : char {
  kUninitialized = -1,            // unknown setting
  kDisable = 0,                   // disable perf stats
  kEnableCount = 1,               // enable only count stats
  kEnableTimeExceptForMutex = 2,  // Other than count stats, also enable time
                                  // stats except for mutexes
  kEnableTime = 3,                // enable count and time stats
  kOutOfBounds = 4                // N.B. Must always be the last value!
};

// set the perf stats level for current thread
void SetPerfLevel(PerfLevel level);

// get current perf stats level for current thread
PerfLevel GetPerfLevel();

}  // namespace rocksdb

#endif  // INCLUDE_ROCKSDB_PERF_LEVEL_H_
