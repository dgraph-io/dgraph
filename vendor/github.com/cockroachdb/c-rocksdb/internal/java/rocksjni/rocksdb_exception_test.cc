// Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
// This source code is licensed under the BSD-style license found in the
// LICENSE file in the root directory of this source tree. An additional grant
// of patent rights can be found in the PATENTS file in the same directory.

#include <jni.h>

#include "include/org_rocksdb_RocksDBExceptionTest.h"

#include "rocksdb/slice.h"
#include "rocksdb/status.h"
#include "rocksjni/portal.h"

/*
 * Class:     org_rocksdb_RocksDBExceptionTest
 * Method:    raiseException
 * Signature: ()V
 */
void Java_org_rocksdb_RocksDBExceptionTest_raiseException(JNIEnv* env,
                                                          jobject jobj) {
  rocksdb::RocksDBExceptionJni::ThrowNew(env, std::string("test message"));
}

/*
 * Class:     org_rocksdb_RocksDBExceptionTest
 * Method:    raiseExceptionWithStatusCode
 * Signature: ()V
 */
void Java_org_rocksdb_RocksDBExceptionTest_raiseExceptionWithStatusCode(
    JNIEnv* env, jobject jobj) {
  rocksdb::RocksDBExceptionJni::ThrowNew(env, "test message",
                                         rocksdb::Status::NotSupported());
}

/*
 * Class:     org_rocksdb_RocksDBExceptionTest
 * Method:    raiseExceptionNoMsgWithStatusCode
 * Signature: ()V
 */
void Java_org_rocksdb_RocksDBExceptionTest_raiseExceptionNoMsgWithStatusCode(
    JNIEnv* env, jobject jobj) {
  rocksdb::RocksDBExceptionJni::ThrowNew(env, rocksdb::Status::NotSupported());
}

/*
 * Class:     org_rocksdb_RocksDBExceptionTest
 * Method:    raiseExceptionWithStatusCodeSubCode
 * Signature: ()V
 */
void Java_org_rocksdb_RocksDBExceptionTest_raiseExceptionWithStatusCodeSubCode(
    JNIEnv* env, jobject jobj) {
  rocksdb::RocksDBExceptionJni::ThrowNew(
      env, "test message",
      rocksdb::Status::TimedOut(rocksdb::Status::SubCode::kLockTimeout));
}

/*
 * Class:     org_rocksdb_RocksDBExceptionTest
 * Method:    raiseExceptionNoMsgWithStatusCodeSubCode
 * Signature: ()V
 */
void Java_org_rocksdb_RocksDBExceptionTest_raiseExceptionNoMsgWithStatusCodeSubCode(
    JNIEnv* env, jobject jobj) {
  rocksdb::RocksDBExceptionJni::ThrowNew(
      env, rocksdb::Status::TimedOut(rocksdb::Status::SubCode::kLockTimeout));
}

/*
 * Class:     org_rocksdb_RocksDBExceptionTest
 * Method:    raiseExceptionWithStatusCodeState
 * Signature: ()V
 */
void Java_org_rocksdb_RocksDBExceptionTest_raiseExceptionWithStatusCodeState(
    JNIEnv* env, jobject jobj) {
  rocksdb::Slice state("test state");
  rocksdb::RocksDBExceptionJni::ThrowNew(env, "test message",
                                         rocksdb::Status::NotSupported(state));
}
