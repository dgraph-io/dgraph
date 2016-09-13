//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.

#pragma once

#include "rocksdb/types.h"

namespace rocksdb {

class DB;

// Abstract handle to particular state of a DB.
// A Snapshot is an immutable object and can therefore be safely
// accessed from multiple threads without any external synchronization.
//
// To Create a Snapshot, call DB::GetSnapshot().
// To Destroy a Snapshot, call DB::ReleaseSnapshot(snapshot).
class Snapshot {
 public:
  // returns Snapshot's sequence number
  virtual SequenceNumber GetSequenceNumber() const = 0;

 protected:
  virtual ~Snapshot();
};

// Simple RAII wrapper class for Snapshot.
// Constructing this object will create a snapshot.  Destructing will
// release the snapshot.
class ManagedSnapshot {
 public:
  explicit ManagedSnapshot(DB* db);

  // Instead of creating a snapshot, take ownership of the input snapshot.
  ManagedSnapshot(DB* db, const Snapshot* _snapshot);

  ~ManagedSnapshot();

  const Snapshot* snapshot();

 private:
  DB* db_;
  const Snapshot* snapshot_;
};

}  // namespace rocksdb
