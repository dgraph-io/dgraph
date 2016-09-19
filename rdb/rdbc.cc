// This file is a subset of the C API from RocksDB. It should remain consistent.
// There will be another file which contains some extra routines that we find
// useful.
#include <cstddef>
#include <cstdlib>
#include <memory>
#include <string>

#include "rocksdb/cache.h"
#include "rocksdb/db.h"
#include "rocksdb/filter_policy.h"
#include "rocksdb/iterator.h"
#include "rocksdb/options.h"
#include "rocksdb/snapshot.h"
#include "rocksdb/status.h"
#include "rocksdb/table.h"
#include "rocksdb/write_batch.h"
#include "rocksdb/utilities/checkpoint.h"

#include "rdbc.h"
#include "_cgo_export.h"

using rocksdb::DB;
using rocksdb::Options;
using rocksdb::Status;
using rocksdb::ReadOptions;
using rocksdb::Slice;
using rocksdb::WriteOptions;
using rocksdb::WriteBatch;
using rocksdb::Iterator;
using rocksdb::FilterPolicy;
using rocksdb::NewBloomFilterPolicy;
using rocksdb::Cache;
using rocksdb::NewLRUCache;
using rocksdb::BlockBasedTableOptions;
using rocksdb::Snapshot;
using rocksdb::Checkpoint;

struct rdb_t { DB* rep; };
struct rdb_options_t { Options rep; };
struct rdb_readoptions_t {
  ReadOptions rep;
  Slice upper_bound; // stack variable to set pointer to in ReadOptions
};
struct rdb_writeoptions_t { WriteOptions rep; };
struct rdb_writebatch_t { WriteBatch rep; };
struct rdb_iterator_t { Iterator* rep; };
struct rdb_cache_t { std::shared_ptr<Cache> rep; };
struct rdb_block_based_table_options_t { BlockBasedTableOptions rep; };
struct rdb_snapshot_t { const Snapshot* rep; };
struct rdb_checkpoint_t { Checkpoint* rep; };

bool SaveError(char** errptr, const Status& s) {
  assert(errptr != nullptr);
  if (s.ok()) {
    return false;
  } else if (*errptr == nullptr) {
    *errptr = strdup(s.ToString().c_str());
  } else {
    // TODO(sanjay): Merge with existing error?
    // This is a bug if *errptr is not created by malloc()
    free(*errptr);
    *errptr = strdup(s.ToString().c_str());
  }
  return true;
}

static char* CopyString(const std::string& str) {
  char* result = reinterpret_cast<char*>(malloc(sizeof(char) * str.size()));
  memcpy(result, str.data(), sizeof(char) * str.size());
  return result;
}

//////////////////////////// rdb_t
rdb_t* rdb_open(
  const rdb_options_t* options,
  const char* name,
  char** errptr) {
  DB* db;
  if (SaveError(errptr, DB::Open(options->rep, std::string(name), &db))) {
    return nullptr;
  }
  rdb_t* result = new rdb_t;
  result->rep = db;
  return result;
}

rdb_t* rdb_open_for_read_only(
    const rdb_options_t* options,
    const char* name,
    unsigned char error_if_log_file_exist,
    char** errptr) {
  DB* db;
  if (SaveError(errptr, DB::OpenForReadOnly(options->rep, std::string(name), &db, error_if_log_file_exist))) {
    return nullptr;
  }
  rdb_t* result = new rdb_t;
  result->rep = db;
  return result;
}

void rdb_close(rdb_t* db) {
  delete db->rep;
  delete db;
}

char* rdb_get(
    rdb_t* db,
    const rdb_readoptions_t* options,
    const char* key, size_t keylen,
    size_t* vallen,
    char** errptr) {
  char* result = nullptr;
  std::string tmp;
  Status s = db->rep->Get(options->rep, Slice(key, keylen), &tmp);
  if (s.ok()) {
    *vallen = tmp.size();
    result = CopyString(tmp);
  } else {
    *vallen = 0;
    if (!s.IsNotFound()) {
      SaveError(errptr, s);
    }
  }
  return result;
}

void rdb_put(
    rdb_t* db,
    const rdb_writeoptions_t* options,
    const char* key, size_t keylen,
    const char* val, size_t vallen,
    char** errptr) {
  SaveError(errptr,
            db->rep->Put(options->rep, Slice(key, keylen), Slice(val, vallen)));
}

void rdb_delete(
    rdb_t* db,
    const rdb_writeoptions_t* options,
    const char* key, size_t keylen,
    char** errptr) {
  SaveError(errptr, db->rep->Delete(options->rep, Slice(key, keylen)));
}

char* rdb_property_value(
    rdb_t* db,
    const char* propname) {
  std::string tmp;
  if (db->rep->GetProperty(Slice(propname), &tmp)) {
    // We use strdup() since we expect human readable output.
    return strdup(tmp.c_str());
  } else {
    return nullptr;
  }
}

//////////////////////////// rdb_writebatch_t
rdb_writebatch_t* rdb_writebatch_create() {
  return new rdb_writebatch_t;
}

rdb_writebatch_t* rdb_writebatch_create_from(const char* rep,
                                                     size_t size) {
  rdb_writebatch_t* b = new rdb_writebatch_t;
  b->rep = WriteBatch(std::string(rep, size));
  return b;
}

void rdb_writebatch_destroy(rdb_writebatch_t* b) {
  delete b;
}

void rdb_writebatch_clear(rdb_writebatch_t* b) {
  b->rep.Clear();
}

int rdb_writebatch_count(rdb_writebatch_t* b) {
  return b->rep.Count();
}

void rdb_writebatch_put(
    rdb_writebatch_t* b,
    const char* key, size_t klen,
    const char* val, size_t vlen) {
  b->rep.Put(Slice(key, klen), Slice(val, vlen));
}

void rdb_writebatch_delete(
    rdb_writebatch_t* b,
    const char* key, size_t klen) {
  b->rep.Delete(Slice(key, klen));
}

void rdb_write(
    rdb_t* db,
    const rdb_writeoptions_t* options,
    rdb_writebatch_t* batch,
    char** errptr) {
  SaveError(errptr, db->rep->Write(options->rep, &batch->rep));
}

//////////////////////////// rdb_options_t
rdb_options_t* rdb_options_create() {
  return new rdb_options_t;
}

void rdb_options_set_create_if_missing(
    rdb_options_t* opt, unsigned char v) {
  opt->rep.create_if_missing = v;
}

void rdb_options_set_block_based_table_factory(
    rdb_options_t *opt,
    rdb_block_based_table_options_t* table_options) {
  if (table_options) {
    opt->rep.table_factory.reset(
        rocksdb::NewBlockBasedTableFactory(table_options->rep));
  }
}

//////////////////////////// rdb_readoptions_t
rdb_readoptions_t* rdb_readoptions_create() {
  return new rdb_readoptions_t;
}

void rdb_readoptions_destroy(rdb_readoptions_t* opt) {
  delete opt;
}

void rdb_readoptions_set_fill_cache(
    rdb_readoptions_t* opt, unsigned char v) {
  opt->rep.fill_cache = v;
}

void rdb_readoptions_set_snapshot(
    rdb_readoptions_t* opt,
    const rdb_snapshot_t* snap) {
  opt->rep.snapshot = (snap ? snap->rep : nullptr);
}

//////////////////////////// rdb_writeoptions_t
rdb_writeoptions_t* rdb_writeoptions_create() {
  return new rdb_writeoptions_t;
}

void rdb_writeoptions_destroy(rdb_writeoptions_t* opt) {
  delete opt;
}

void rdb_writeoptions_set_sync(
    rdb_writeoptions_t* opt, unsigned char v) {
  opt->rep.sync = v;
}

//////////////////////////// rdb_iterator_t
rdb_iterator_t* rdb_create_iterator(
    rdb_t* db,
    const rdb_readoptions_t* options) {
  rdb_iterator_t* result = new rdb_iterator_t;
  result->rep = db->rep->NewIterator(options->rep);
  return result;
}

void rdb_iter_destroy(rdb_iterator_t* iter) {
  delete iter->rep;
  delete iter;
}

unsigned char rdb_iter_valid(const rdb_iterator_t* iter) {
  return iter->rep->Valid();
}

void rdb_iter_seek_to_first(rdb_iterator_t* iter) {
  iter->rep->SeekToFirst();
}

void rdb_iter_seek_to_last(rdb_iterator_t* iter) {
  iter->rep->SeekToLast();
}

void rdb_iter_seek(rdb_iterator_t* iter, const char* k, size_t klen) {
  iter->rep->Seek(Slice(k, klen));
}

void rdb_iter_next(rdb_iterator_t* iter) {
  iter->rep->Next();
}

void rdb_iter_prev(rdb_iterator_t* iter) {
  iter->rep->Prev();
}

const char* rdb_iter_key(const rdb_iterator_t* iter, size_t* klen) {
  Slice s = iter->rep->key();
  *klen = s.size();
  return s.data();
}

const char* rdb_iter_value(const rdb_iterator_t* iter, size_t* vlen) {
  Slice s = iter->rep->value();
  *vlen = s.size();
  return s.data();
}

void rdb_iter_get_error(const rdb_iterator_t* iter, char** errptr) {
  SaveError(errptr, iter->rep->status());
}

//////////////////////////// rdb_filterpolicy_t
struct rdb_filterpolicy_t : public FilterPolicy {
  void* state_;
  void (*destructor_)(void*);
  const char* (*name_)(void*);
  char* (*create_)(
      void*,
      const char* const* key_array, const size_t* key_length_array,
      int num_keys,
      size_t* filter_length);
  unsigned char (*key_match_)(
      void*,
      const char* key, size_t length,
      const char* filter, size_t filter_length);
  void (*delete_filter_)(
      void*,
      const char* filter, size_t filter_length);

  virtual ~rdb_filterpolicy_t() {
    (*destructor_)(state_);
  }

  virtual const char* Name() const override { return (*name_)(state_); }

  virtual void CreateFilter(const Slice* keys, int n,
                            std::string* dst) const override {
    std::vector<const char*> key_pointers(n);
    std::vector<size_t> key_sizes(n);
    for (int i = 0; i < n; i++) {
      key_pointers[i] = keys[i].data();
      key_sizes[i] = keys[i].size();
    }
    size_t len;
    char* filter = (*create_)(state_, &key_pointers[0], &key_sizes[0], n, &len);
    dst->append(filter, len);

    if (delete_filter_ != nullptr) {
      (*delete_filter_)(state_, filter, len);
    } else {
      free(filter);
    }
  }

  virtual bool KeyMayMatch(const Slice& key,
                           const Slice& filter) const override {
    return (*key_match_)(state_, key.data(), key.size(),
                         filter.data(), filter.size());
  }
};

rdb_filterpolicy_t* rdb_filterpolicy_create(
    void* state,
    void (*destructor)(void*),
    char* (*create_filter)(
        void*,
        const char* const* key_array, const size_t* key_length_array,
        int num_keys,
        size_t* filter_length),
    unsigned char (*key_may_match)(
        void*,
        const char* key, size_t length,
        const char* filter, size_t filter_length),
    void (*delete_filter)(
        void*,
        const char* filter, size_t filter_length),
    const char* (*name)(void*)) {
  rdb_filterpolicy_t* result = new rdb_filterpolicy_t;
  result->state_ = state;
  result->destructor_ = destructor;
  result->create_ = create_filter;
  result->key_match_ = key_may_match;
  result->delete_filter_ = delete_filter;
  result->name_ = name;
  return result;
}

void rdb_filterpolicy_destroy(rdb_filterpolicy_t* filter) {
  delete filter;
}

void rdbc_destruct_handler(void* state) { }
void rdbc_filterpolicy_delete_filter(void* state, const char* v, size_t s) { }

rdb_filterpolicy_t* rdbc_filterpolicy_create(uintptr_t idx) {
  return rdb_filterpolicy_create(
    (void*)idx,
    rdbc_destruct_handler,
    (char* (*)(void*, const char* const*, const size_t*, int, size_t*))(rdbc_filterpolicy_create_filter),
    (unsigned char (*)(void*, const char*, size_t, const char*, size_t))(rdbc_filterpolicy_key_may_match),
    rdbc_filterpolicy_delete_filter,
    (const char *(*)(void*))(rdbc_filterpolicy_name));
}

rdb_filterpolicy_t* rdb_filterpolicy_create_bloom_format(int bits_per_key, bool original_format) {
  // Make a rdb_filterpolicy_t, but override all of its methods so
  // they delegate to a NewBloomFilterPolicy() instead of user
  // supplied C functions.
  struct Wrapper : public rdb_filterpolicy_t {
    const FilterPolicy* rep_;
    ~Wrapper() { delete rep_; }
    const char* Name() const override { return rep_->Name(); }
    void CreateFilter(const Slice* keys, int n,
                      std::string* dst) const override {
      return rep_->CreateFilter(keys, n, dst);
    }
    bool KeyMayMatch(const Slice& key, const Slice& filter) const override {
      return rep_->KeyMayMatch(key, filter);
    }
    static void DoNothing(void*) { }
  };
  Wrapper* wrapper = new Wrapper;
  wrapper->rep_ = NewBloomFilterPolicy(bits_per_key, original_format);
  wrapper->state_ = nullptr;
  wrapper->delete_filter_ = nullptr;
  wrapper->destructor_ = &Wrapper::DoNothing;
  return wrapper;
}

rdb_filterpolicy_t* rdb_filterpolicy_create_bloom_full(int bits_per_key) {
  return rdb_filterpolicy_create_bloom_format(bits_per_key, false);
}

rdb_filterpolicy_t* rdb_filterpolicy_create_bloom(int bits_per_key) {
  return rdb_filterpolicy_create_bloom_format(bits_per_key, true);
}

//////////////////////////// rdb_cache_t
rdb_cache_t* rdb_cache_create_lru(size_t capacity) {
  rdb_cache_t* c = new rdb_cache_t;
  c->rep = NewLRUCache(capacity);
  return c;
}

void rdb_cache_destroy(rdb_cache_t* cache) {
  delete cache;
}

void rdb_cache_set_capacity(rdb_cache_t* cache, size_t capacity) {
  cache->rep->SetCapacity(capacity);
}

//////////////////////////// rdb_block_based_table_options_t
rdb_block_based_table_options_t*
rdb_block_based_options_create() {
  return new rdb_block_based_table_options_t;
}

void rdb_block_based_options_destroy(
    rdb_block_based_table_options_t* options) {
  delete options;
}

void rdb_block_based_options_set_block_size(
    rdb_block_based_table_options_t* options, size_t block_size) {
  options->rep.block_size = block_size;
}

void rdb_block_based_options_set_filter_policy(
    rdb_block_based_table_options_t* options,
    rdb_filterpolicy_t* filter_policy) {
  options->rep.filter_policy.reset(filter_policy);
}

void rdb_block_based_options_set_no_block_cache(
    rdb_block_based_table_options_t* options,
    unsigned char no_block_cache) {
  options->rep.no_block_cache = no_block_cache;
}

void rdb_block_based_options_set_block_cache(
    rdb_block_based_table_options_t* options,
    rdb_cache_t* block_cache) {
  if (block_cache) {
    options->rep.block_cache = block_cache->rep;
  }
}

void rdb_block_based_options_set_block_cache_compressed(
    rdb_block_based_table_options_t* options,
    rdb_cache_t* block_cache_compressed) {
  if (block_cache_compressed) {
    options->rep.block_cache_compressed = block_cache_compressed->rep;
  }
}

void rdb_block_based_options_set_whole_key_filtering(
    rdb_block_based_table_options_t* options, unsigned char v) {
  options->rep.whole_key_filtering = v;
}

//////////////////////////// rdb_snapshot_t
const rdb_snapshot_t* rdb_create_snapshot(rdb_t* db) {
  rdb_snapshot_t* result = new rdb_snapshot_t;
  result->rep = db->rep->GetSnapshot();
  return result;
}

void rdb_release_snapshot(
    rdb_t* db,
    const rdb_snapshot_t* snapshot) {
  db->rep->ReleaseSnapshot(snapshot->rep);
  delete snapshot;
}

//////////////////////////// rdb_checkpoint_t
rdb_checkpoint_t* rdb_create_checkpoint(rdb_t* db, char** errptr) {
  Checkpoint* checkpoint;
  if (SaveError(errptr, Checkpoint::Create(db->rep, &checkpoint))) {
    return nullptr;
  }
  rdb_checkpoint_t* result = new rdb_checkpoint_t;
  result->rep = checkpoint;
  return result;
}

void rdb_open_checkpoint(
	rdb_checkpoint_t* checkpoint,
  const char* checkpoint_dir,
  char** errptr) {
  SaveError(errptr, checkpoint->rep->CreateCheckpoint(std::string(checkpoint_dir)));
}

void rdb_destroy_checkpoint(rdb_checkpoint_t* checkpoint) {
	delete checkpoint->rep;
}
