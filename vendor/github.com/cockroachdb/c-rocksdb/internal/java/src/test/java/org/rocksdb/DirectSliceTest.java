// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.
package org.rocksdb;

import org.junit.ClassRule;
import org.junit.Test;

import java.nio.ByteBuffer;

import static org.assertj.core.api.Assertions.assertThat;

public class DirectSliceTest {
  @ClassRule
  public static final RocksMemoryResource rocksMemoryResource =
      new RocksMemoryResource();

  @Test
  public void directSlice() {
    try(final DirectSlice directSlice = new DirectSlice("abc");
        final DirectSlice otherSlice = new DirectSlice("abc")) {
      assertThat(directSlice.toString()).isEqualTo("abc");
      // clear first slice
      directSlice.clear();
      assertThat(directSlice.toString()).isEmpty();
      // get first char in otherslice
      assertThat(otherSlice.get(0)).isEqualTo("a".getBytes()[0]);
      // remove prefix
      otherSlice.removePrefix(1);
      assertThat(otherSlice.toString()).isEqualTo("bc");
    }
  }

  @Test
  public void directSliceWithByteBuffer() {
    final byte[] data = "Some text".getBytes();
    final ByteBuffer buffer = ByteBuffer.allocateDirect(data.length + 1);
    buffer.put(data);
    buffer.put(data.length, (byte)0);

    try(final DirectSlice directSlice = new DirectSlice(buffer)) {
      assertThat(directSlice.toString()).isEqualTo("Some text");
    }
  }

  @Test
  public void directSliceWithByteBufferAndLength() {
    final byte[] data = "Some text".getBytes();
    final ByteBuffer buffer = ByteBuffer.allocateDirect(data.length);
    buffer.put(data);
    try(final DirectSlice directSlice = new DirectSlice(buffer, 4)) {
      assertThat(directSlice.toString()).isEqualTo("Some");
    }
  }

  @Test(expected = AssertionError.class)
  public void directSliceInitWithoutDirectAllocation() {
    final byte[] data = "Some text".getBytes();
    final ByteBuffer buffer = ByteBuffer.wrap(data);
    try(final DirectSlice directSlice = new DirectSlice(buffer)) {
      //no-op
    }
  }

  @Test(expected = AssertionError.class)
  public void directSlicePrefixInitWithoutDirectAllocation() {
    final byte[] data = "Some text".getBytes();
    final ByteBuffer buffer = ByteBuffer.wrap(data);
    try(final DirectSlice directSlice = new DirectSlice(buffer, 4)) {
      //no-op
    }
  }
}
