// This file is a subset of the C API from RocksDB. It should remain consistent.
// There will be another file which contains some extra routines that we find
// useful.
#ifndef __DGROCKSDBC__
#define __DGROCKSDBC__

#ifdef __cplusplus
extern "C" {
#endif

typedef struct rdb_t rdb_t;
typedef struct rdb_options_t rdb_options_t;
typedef struct rdb_readoptions_t rdb_readoptions_t;
typedef struct rdb_writeoptions_t rdb_writeoptions_t;
typedef struct rdb_writebatch_t rdb_writebatch_t;
typedef struct rdb_iterator_t rdb_iterator_t;
typedef struct rdb_filterpolicy_t rdb_filterpolicy_t;
typedef struct rdb_cache_t rdb_cache_t;
typedef struct rdb_block_based_table_options_t rdb_block_based_table_options_t;
typedef struct rdb_snapshot_t rdb_snapshot_t;
typedef struct rdb_checkpoint_t rdb_checkpoint_t;

//////////////////////////// rdb_t
rdb_t* rdb_open(
	const rdb_options_t* options,
	const char* name,
	char** errptr);
rdb_t* rdb_open_for_read_only(
	const rdb_options_t* options,
	const char* name,
	unsigned char error_if_log_file_exist,
	char** errptr);
void rdb_close(rdb_t* db);
char* rdb_get(
    rdb_t* db,
    const rdb_readoptions_t* options,
    const char* key, size_t keylen,
    size_t* vallen,
    char** errptr);
void rdb_put(
    rdb_t* db,
    const rdb_writeoptions_t* options,
    const char* key, size_t keylen,
    const char* val, size_t vallen,
    char** errptr);
void rdb_delete(
    rdb_t* db,
    const rdb_writeoptions_t* options,
    const char* key, size_t keylen,
    char** errptr);
char* rdb_property_value(
    rdb_t* db,
    const char* propname);

//////////////////////////// rdb_writebatch_t
rdb_writebatch_t* rdb_writebatch_create();
rdb_writebatch_t* rdb_writebatch_create_from(const char* rep, size_t size);
void rdb_writebatch_destroy(rdb_writebatch_t* b);
void rdb_writebatch_clear(rdb_writebatch_t* b);
int rdb_writebatch_count(rdb_writebatch_t* b);
void rdb_writebatch_put(
    rdb_writebatch_t* b,
    const char* key, size_t klen,
    const char* val, size_t vlen);
void rdb_writebatch_delete(
    rdb_writebatch_t* b,
    const char* key, size_t klen);
void rdb_write(
    rdb_t* db,
    const rdb_writeoptions_t* options,
    rdb_writebatch_t* batch,
    char** errptr);

//////////////////////////// rdb_options_t
rdb_options_t* rdb_options_create();
void rdb_options_set_create_if_missing(
    rdb_options_t* opt, unsigned char v);
void rdb_options_set_block_based_table_factory(
    rdb_options_t *opt,
    rdb_block_based_table_options_t* table_options);

//////////////////////////// rdb_readoptions_t
rdb_readoptions_t* rdb_readoptions_create();
void rdb_readoptions_destroy(rdb_readoptions_t* opt);
void rdb_readoptions_set_fill_cache(
    rdb_readoptions_t* opt, unsigned char v);

//////////////////////////// rdb_writeoptions_t
rdb_writeoptions_t* rdb_writeoptions_create();
void rdb_writeoptions_destroy(rdb_writeoptions_t* opt);
void rdb_writeoptions_set_sync(
    rdb_writeoptions_t* opt, unsigned char v);

//////////////////////////// rdb_iterator_t
rdb_iterator_t* rdb_create_iterator(
    rdb_t* db,
    const rdb_readoptions_t* options);
void rdb_iter_destroy(rdb_iterator_t* iter);
unsigned char rdb_iter_valid(const rdb_iterator_t* iter);
void rdb_iter_seek_to_first(rdb_iterator_t* iter);
void rdb_iter_seek_to_last(rdb_iterator_t* iter);
void rdb_iter_seek(rdb_iterator_t* iter, const char* k, size_t klen);
void rdb_iter_next(rdb_iterator_t* iter);
void rdb_iter_prev(rdb_iterator_t* iter);
const char* rdb_iter_key(const rdb_iterator_t* iter, size_t* klen);
const char* rdb_iter_value(const rdb_iterator_t* iter, size_t* vlen);
void rdb_iter_get_error(const rdb_iterator_t* iter, char** errptr);

//////////////////////////// rdb_filterpolicy_t
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
    const char* (*name)(void*));
rdb_filterpolicy_t* rdbc_filterpolicy_create(uintptr_t idx);
rdb_filterpolicy_t* rdb_filterpolicy_create_bloom(int bits_per_key);

//////////////////////////// rdb_cache_t
rdb_cache_t* rdb_cache_create_lru(size_t capacity);
void rdb_cache_destroy(rdb_cache_t* cache);
void rdb_cache_set_capacity(rdb_cache_t* cache, size_t capacity);

//////////////////////////// rdb_block_based_table_options_t
rdb_block_based_table_options_t*
rdb_block_based_options_create();
void rdb_block_based_options_destroy(
    rdb_block_based_table_options_t* options);
void rdb_block_based_options_set_block_size(
    rdb_block_based_table_options_t* options, size_t block_size);
void rdb_block_based_options_set_filter_policy(
    rdb_block_based_table_options_t* options,
    rdb_filterpolicy_t* filter_policy);
void rdb_block_based_options_set_no_block_cache(
    rdb_block_based_table_options_t* options,
    unsigned char no_block_cache);
void rdb_block_based_options_set_block_cache(
    rdb_block_based_table_options_t* options,
    rdb_cache_t* block_cache);
void rdb_block_based_options_set_block_cache_compressed(
    rdb_block_based_table_options_t* options,
    rdb_cache_t* block_cache_compressed);
void rdb_block_based_options_set_whole_key_filtering(
    rdb_block_based_table_options_t* options, unsigned char v);

//////////////////////////// rdb_snapshot_t
const rdb_snapshot_t* rdb_create_snapshot(
    rdb_t* db);
void rdb_release_snapshot(
    rdb_t* db,
    const rdb_snapshot_t* snapshot);

//////////////////////////// rdb_checkpoint_t
rdb_checkpoint_t* rdb_create_checkpoint(rdb_t* db, char** errptr);
void rdb_open_checkpoint(
	rdb_checkpoint_t* checkpoint,
  const char* checkpoint_dir,
  char** errptr);
void rdb_destroy_checkpoint(rdb_checkpoint_t* checkpoint);

#ifdef __cplusplus
}  /* end extern "C" */
#endif

#endif  // __DGROCKSDBC__
