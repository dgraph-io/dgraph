// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.

package org.rocksdb;

public abstract class AbstractWriteBatch extends RocksObject
    implements WriteBatchInterface {

  protected AbstractWriteBatch(final long nativeHandle) {
    super(nativeHandle);
  }

  @Override
  public int count() {
    assert (isOwningHandle());
    return count0(nativeHandle_);
  }

  @Override
  public void put(byte[] key, byte[] value) {
    assert (isOwningHandle());
    put(nativeHandle_, key, key.length, value, value.length);
  }

  @Override
  public void put(ColumnFamilyHandle columnFamilyHandle, byte[] key,
      byte[] value) {
    assert (isOwningHandle());
    put(nativeHandle_, key, key.length, value, value.length,
        columnFamilyHandle.nativeHandle_);
  }

  @Override
  public void merge(byte[] key, byte[] value) {
    assert (isOwningHandle());
    merge(nativeHandle_, key, key.length, value, value.length);
  }

  @Override
  public void merge(ColumnFamilyHandle columnFamilyHandle, byte[] key,
      byte[] value) {
    assert (isOwningHandle());
    merge(nativeHandle_, key, key.length, value, value.length,
        columnFamilyHandle.nativeHandle_);
  }

  @Override
  public void remove(byte[] key) {
    assert (isOwningHandle());
    remove(nativeHandle_, key, key.length);
  }

  @Override
  public void remove(ColumnFamilyHandle columnFamilyHandle, byte[] key) {
    assert (isOwningHandle());
    remove(nativeHandle_, key, key.length, columnFamilyHandle.nativeHandle_);
  }

  @Override
  public void putLogData(byte[] blob) {
    assert (isOwningHandle());
    putLogData(nativeHandle_, blob, blob.length);
  }

  @Override
  public void clear() {
    assert (isOwningHandle());
    clear0(nativeHandle_);
  }

  abstract int count0(final long handle);

  abstract void put(final long handle, final byte[] key, final int keyLen,
      final byte[] value, final int valueLen);

  abstract void put(final long handle, final byte[] key, final int keyLen,
      final byte[] value, final int valueLen, final long cfHandle);

  abstract void merge(final long handle, final byte[] key, final int keyLen,
      final byte[] value, final int valueLen);

  abstract void merge(final long handle, final byte[] key, final int keyLen,
      final byte[] value, final int valueLen, final long cfHandle);

  abstract void remove(final long handle, final byte[] key,
      final int keyLen);

  abstract void remove(final long handle, final byte[] key,
      final int keyLen, final long cfHandle);

  abstract void putLogData(final long handle, final byte[] blob,
      final int blobLen);

  abstract void clear0(final long handle);
}
