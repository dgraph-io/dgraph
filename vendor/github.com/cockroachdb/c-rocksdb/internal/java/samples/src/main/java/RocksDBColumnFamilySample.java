// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.

import org.rocksdb.*;

import java.util.ArrayList;
import java.util.List;

public class RocksDBColumnFamilySample {
  static {
    RocksDB.loadLibrary();
  }

  public static void main(String[] args) throws RocksDBException {
    if (args.length < 1) {
      System.out.println(
          "usage: RocksDBColumnFamilySample db_path");
      return;
    }
    String db_path = args[0];

    System.out.println("RocksDBColumnFamilySample");
    try(final Options options = new Options().setCreateIfMissing(true);
        final RocksDB db = RocksDB.open(options, db_path)) {

      assert(db != null);

      // create column family
      try(final ColumnFamilyHandle columnFamilyHandle = db.createColumnFamily(
          new ColumnFamilyDescriptor("new_cf".getBytes(),
          new ColumnFamilyOptions()))) {
        assert (columnFamilyHandle != null);
      }
    }

    // open DB with two column families
    final List<ColumnFamilyDescriptor> columnFamilyDescriptors =
        new ArrayList<>();
    // have to open default column family
    columnFamilyDescriptors.add(new ColumnFamilyDescriptor(
        RocksDB.DEFAULT_COLUMN_FAMILY, new ColumnFamilyOptions()));
    // open the new one, too
    columnFamilyDescriptors.add(new ColumnFamilyDescriptor(
        "new_cf".getBytes(), new ColumnFamilyOptions()));
    final List<ColumnFamilyHandle> columnFamilyHandles = new ArrayList<>();
    try(final DBOptions options = new DBOptions();
        final RocksDB db = RocksDB.open(options, db_path,
            columnFamilyDescriptors, columnFamilyHandles)) {
      assert(db != null);

      try {
        // put and get from non-default column family
        db.put(columnFamilyHandles.get(0), new WriteOptions(),
            "key".getBytes(), "value".getBytes());
        String value = new String(db.get(columnFamilyHandles.get(0),
            "key".getBytes()));

        // atomic write
        try (final WriteBatch wb = new WriteBatch()) {
          wb.put(columnFamilyHandles.get(0), "key2".getBytes(),
              "value2".getBytes());
          wb.put(columnFamilyHandles.get(1), "key3".getBytes(),
              "value3".getBytes());
          wb.remove(columnFamilyHandles.get(0), "key".getBytes());
          db.write(new WriteOptions(), wb);
        }

        // drop column family
        db.dropColumnFamily(columnFamilyHandles.get(1));
      } finally {
        for (final ColumnFamilyHandle handle : columnFamilyHandles) {
          handle.close();
        }
      }
    }
  }
}
