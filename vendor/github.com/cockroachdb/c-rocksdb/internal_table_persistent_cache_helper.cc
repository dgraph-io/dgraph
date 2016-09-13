//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.
#include "table/persistent_cache_helper.h"
#include "table/format.h"

namespace rocksdb {

void PersistentCacheHelper::InsertRawPage(
    const PersistentCacheOptions& cache_options, const BlockHandle& handle,
    const char* data, const size_t size) {
  assert(cache_options.persistent_cache);
  assert(cache_options.persistent_cache->IsCompressed());

  // construct the page key
  char cache_key[BlockBasedTable::kMaxCacheKeyPrefixSize + kMaxVarint64Length];
  auto key = BlockBasedTable::GetCacheKey(cache_options.key_prefix.c_str(),
                                          cache_options.key_prefix.size(),
                                          handle, cache_key);
  // insert content to cache
  cache_options.persistent_cache->Insert(key, data, size);
}

void PersistentCacheHelper::InsertUncompressedPage(
    const PersistentCacheOptions& cache_options, const BlockHandle& handle,
    const BlockContents& contents) {
  assert(cache_options.persistent_cache);
  assert(!cache_options.persistent_cache->IsCompressed());
  if (!contents.cachable || contents.compression_type != kNoCompression) {
    // We shouldn't cache this. Either
    // (1) content is not cacheable
    // (2) content is compressed
    return;
  }

  // construct the page key
  char cache_key[BlockBasedTable::kMaxCacheKeyPrefixSize + kMaxVarint64Length];
  auto key = BlockBasedTable::GetCacheKey(cache_options.key_prefix.c_str(),
                                          cache_options.key_prefix.size(),
                                          handle, cache_key);
  // insert block contents to page cache
  cache_options.persistent_cache->Insert(key, contents.data.data(),
                                         contents.data.size());
}

Status PersistentCacheHelper::LookupRawPage(
    const PersistentCacheOptions& cache_options, const BlockHandle& handle,
    std::unique_ptr<char[]>* raw_data, const size_t raw_data_size) {
  assert(cache_options.persistent_cache);
  assert(cache_options.persistent_cache->IsCompressed());

  // construct the page key
  char cache_key[BlockBasedTable::kMaxCacheKeyPrefixSize + kMaxVarint64Length];
  auto key = BlockBasedTable::GetCacheKey(cache_options.key_prefix.c_str(),
                                          cache_options.key_prefix.size(),
                                          handle, cache_key);
  // Lookup page
  size_t size;
  Status s = cache_options.persistent_cache->Lookup(key, raw_data, &size);
  if (!s.ok()) {
    // cache miss
    RecordTick(cache_options.statistics, PERSISTENT_CACHE_MISS);
    return s;
  }

  // cache hit
  assert(raw_data_size == handle.size() + kBlockTrailerSize);
  assert(size == raw_data_size);
  RecordTick(cache_options.statistics, PERSISTENT_CACHE_HIT);
  return Status::OK();
}

Status PersistentCacheHelper::LookupUncompressedPage(
    const PersistentCacheOptions& cache_options, const BlockHandle& handle,
    BlockContents* contents) {
  assert(cache_options.persistent_cache);
  assert(!cache_options.persistent_cache->IsCompressed());
  if (!contents) {
    // We shouldn't lookup in the cache. Either
    // (1) Nowhere to store
    return Status::NotFound();
  }

  // construct the page key
  char cache_key[BlockBasedTable::kMaxCacheKeyPrefixSize + kMaxVarint64Length];
  auto key = BlockBasedTable::GetCacheKey(cache_options.key_prefix.c_str(),
                                          cache_options.key_prefix.size(),
                                          handle, cache_key);
  // Lookup page
  std::unique_ptr<char[]> data;
  size_t size;
  Status s = cache_options.persistent_cache->Lookup(key, &data, &size);
  if (!s.ok()) {
    // cache miss
    RecordTick(cache_options.statistics, PERSISTENT_CACHE_MISS);
    return s;
  }

  // please note we are potentially comparing compressed data size with
  // uncompressed data size
  assert(handle.size() <= size);

  // update stats
  RecordTick(cache_options.statistics, PERSISTENT_CACHE_HIT);
  // construct result and return
  *contents =
      BlockContents(std::move(data), size, false /*cacheable*/, kNoCompression);
  return Status::OK();
}

}  // namespace rocksdb
