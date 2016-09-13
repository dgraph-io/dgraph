// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.

package org.rocksdb;

import java.util.Random;

import org.junit.ClassRule;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.ExpectedException;

import static org.assertj.core.api.Assertions.assertThat;

public class ReadOptionsTest {

  @ClassRule
  public static final RocksMemoryResource rocksMemoryResource =
      new RocksMemoryResource();

  @Rule
  public ExpectedException exception = ExpectedException.none();

  @Test
  public void verifyChecksum() {
    try (final ReadOptions opt = new ReadOptions()) {
      final Random rand = new Random();
      final boolean boolValue = rand.nextBoolean();
      opt.setVerifyChecksums(boolValue);
      assertThat(opt.verifyChecksums()).isEqualTo(boolValue);
    }
  }

  @Test
  public void fillCache() {
    try (final ReadOptions opt = new ReadOptions()) {
      final Random rand = new Random();
      final boolean boolValue = rand.nextBoolean();
      opt.setFillCache(boolValue);
      assertThat(opt.fillCache()).isEqualTo(boolValue);
    }
  }

  @Test
  public void tailing() {
    try (final ReadOptions opt = new ReadOptions()) {
      final Random rand = new Random();
      final boolean boolValue = rand.nextBoolean();
      opt.setTailing(boolValue);
      assertThat(opt.tailing()).isEqualTo(boolValue);
    }
  }

  @Test
  public void snapshot() {
    try (final ReadOptions opt = new ReadOptions()) {
      opt.setSnapshot(null);
      assertThat(opt.snapshot()).isNull();
    }
  }

  @Test
  public void failSetVerifyChecksumUninitialized() {
    try (final ReadOptions readOptions =
             setupUninitializedReadOptions(exception)) {
      readOptions.setVerifyChecksums(true);
    }
  }

  @Test
  public void failVerifyChecksumUninitialized() {
    try (final ReadOptions readOptions =
             setupUninitializedReadOptions(exception)) {
      readOptions.verifyChecksums();
    }
  }

  @Test
  public void failSetFillCacheUninitialized() {
    try (final ReadOptions readOptions =
             setupUninitializedReadOptions(exception)) {
      readOptions.setFillCache(true);
    }
  }

  @Test
  public void failFillCacheUninitialized() {
    try (final ReadOptions readOptions =
             setupUninitializedReadOptions(exception)) {
      readOptions.fillCache();
    }
  }

  @Test
  public void failSetTailingUninitialized() {
    try (final ReadOptions readOptions =
             setupUninitializedReadOptions(exception)) {
      readOptions.setTailing(true);
    }
  }

  @Test
  public void failTailingUninitialized() {
    try (final ReadOptions readOptions =
             setupUninitializedReadOptions(exception)) {
      readOptions.tailing();
    }
  }

  @Test
  public void failSetSnapshotUninitialized() {
    try (final ReadOptions readOptions =
             setupUninitializedReadOptions(exception)) {
      readOptions.setSnapshot(null);
    }
  }

  @Test
  public void failSnapshotUninitialized() {
    try (final ReadOptions readOptions =
             setupUninitializedReadOptions(exception)) {
      readOptions.snapshot();
    }
  }

  private ReadOptions setupUninitializedReadOptions(
      ExpectedException exception) {
    final ReadOptions readOptions = new ReadOptions();
    readOptions.close();
    exception.expect(AssertionError.class);
    return readOptions;
  }
}
