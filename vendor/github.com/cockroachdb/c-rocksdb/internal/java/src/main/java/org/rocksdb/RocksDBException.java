// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.

package org.rocksdb;

/**
 * A RocksDBException encapsulates the error of an operation.  This exception
 * type is used to describe an internal error from the c++ rocksdb library.
 */
public class RocksDBException extends Exception {

  /* @Nullable */ private final Status status;

  /**
   * The private construct used by a set of public static factory method.
   *
   * @param msg the specified error message.
   */
  public RocksDBException(final String msg) {
    this(msg, null);
  }

  public RocksDBException(final String msg, final Status status) {
    super(msg);
    this.status = status;
  }

  public RocksDBException(final Status status) {
    super(status.getState() != null ? status.getState()
        : status.getCodeString());
    this.status = status;
  }

  /**
   * Get the status returned from RocksDB
   *
   * @return The status reported by RocksDB, or null if no status is available
   */
  public Status getStatus() {
    return status;
  }
}
