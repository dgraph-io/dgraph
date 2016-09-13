// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.

package org.rocksdb.util;

import org.rocksdb.BuiltinComparator;
import org.rocksdb.ComparatorOptions;
import org.rocksdb.Slice;

/**
 * This is a Java Native implementation of the C++
 * equivalent ReverseBytewiseComparatorImpl using {@link Slice}
 *
 * The performance of Comparators implemented in Java is always
 * less than their C++ counterparts due to the bridging overhead,
 * as such you likely don't want to use this apart from benchmarking
 * and you most likely instead wanted
 * {@link BuiltinComparator#REVERSE_BYTEWISE_COMPARATOR}
 */
public class ReverseBytewiseComparator extends BytewiseComparator {

  public ReverseBytewiseComparator(final ComparatorOptions copt) {
    super(copt);
  }

  @Override
  public String name() {
    return "rocksdb.java.ReverseBytewiseComparator";
  }

  @Override
  public int compare(final Slice a, final Slice b) {
    return -super.compare(a, b);
  }
}
