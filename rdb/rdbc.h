// This file is a subset of the C API from RocksDB. It should remain consistent.
// There will be another file which contains some extra routines that we find
// useful.
#ifndef __DGROCKSDBC__
#define __DGROCKSDBC__

#ifdef __cplusplus
extern "C" {
#endif

typedef struct rocksdb_t rocksdb_t;
typedef struct rocksdb_options_t rocksdb_options_t;
typedef struct rocksdb_readoptions_t rocksdb_readoptions_t;
typedef struct rocksdb_writeoptions_t rocksdb_writeoptions_t;
typedef struct rocksdb_writebatch_t rocksdb_writebatch_t;
typedef struct rocksdb_iterator_t rocksdb_iterator_t;
typedef struct rocksdb_filterpolicy_t rocksdb_filterpolicy_t;
typedef struct rocksdb_cache_t rocksdb_cache_t;
typedef struct rocksdb_block_based_table_options_t rocksdb_block_based_table_options_t;

//////////////////////////// rocksdb_t
rocksdb_t* rocksdb_open(
	const rocksdb_options_t* options,
	const char* name,
	char** errptr);
rocksdb_t* rocksdb_open_for_read_only(
	const rocksdb_options_t* options,
	const char* name,
	unsigned char error_if_log_file_exist,
	char** errptr);
void rocksdb_close(rocksdb_t* db);
char* rocksdb_get(
    rocksdb_t* db,
    const rocksdb_readoptions_t* options,
    const char* key, size_t keylen,
    size_t* vallen,
    char** errptr);
void rocksdb_put(
    rocksdb_t* db,
    const rocksdb_writeoptions_t* options,
    const char* key, size_t keylen,
    const char* val, size_t vallen,
    char** errptr);
void rocksdb_delete(
    rocksdb_t* db,
    const rocksdb_writeoptions_t* options,
    const char* key, size_t keylen,
    char** errptr);
char* rocksdb_property_value(
    rocksdb_t* db,
    const char* propname);

//////////////////////////// rocksdb_writebatch_t
rocksdb_writebatch_t* rocksdb_writebatch_create();
rocksdb_writebatch_t* rocksdb_writebatch_create_from(const char* rep,
                                                     size_t size);
void rocksdb_writebatch_destroy(rocksdb_writebatch_t* b);
void rocksdb_writebatch_clear(rocksdb_writebatch_t* b);
int rocksdb_writebatch_count(rocksdb_writebatch_t* b);
void rocksdb_writebatch_put(
    rocksdb_writebatch_t* b,
    const char* key, size_t klen,
    const char* val, size_t vlen);
void rocksdb_writebatch_delete(
    rocksdb_writebatch_t* b,
    const char* key, size_t klen);
void rocksdb_write(
    rocksdb_t* db,
    const rocksdb_writeoptions_t* options,
    rocksdb_writebatch_t* batch,
    char** errptr);

//////////////////////////// rocksdb_options_t
rocksdb_options_t* rocksdb_options_create();
void rocksdb_options_set_create_if_missing(
    rocksdb_options_t* opt, unsigned char v);
void rocksdb_options_set_block_based_table_factory(
    rocksdb_options_t *opt,
    rocksdb_block_based_table_options_t* table_options);

//////////////////////////// rocksdb_readoptions_t
rocksdb_readoptions_t* rocksdb_readoptions_create();
void rocksdb_readoptions_destroy(rocksdb_readoptions_t* opt);
void rocksdb_readoptions_set_fill_cache(
    rocksdb_readoptions_t* opt, unsigned char v);

//////////////////////////// rocksdb_writeoptions_t
rocksdb_writeoptions_t* rocksdb_writeoptions_create();
void rocksdb_writeoptions_destroy(rocksdb_writeoptions_t* opt);
void rocksdb_writeoptions_set_sync(
    rocksdb_writeoptions_t* opt, unsigned char v);

//////////////////////////// rocksdb_iterator_t
rocksdb_iterator_t* rocksdb_create_iterator(
    rocksdb_t* db,
    const rocksdb_readoptions_t* options);
void rocksdb_iter_destroy(rocksdb_iterator_t* iter);
unsigned char rocksdb_iter_valid(const rocksdb_iterator_t* iter);
void rocksdb_iter_seek_to_first(rocksdb_iterator_t* iter);
void rocksdb_iter_seek_to_last(rocksdb_iterator_t* iter);
void rocksdb_iter_seek(rocksdb_iterator_t* iter, const char* k, size_t klen);
void rocksdb_iter_next(rocksdb_iterator_t* iter);
void rocksdb_iter_prev(rocksdb_iterator_t* iter);
const char* rocksdb_iter_key(const rocksdb_iterator_t* iter, size_t* klen);
const char* rocksdb_iter_value(const rocksdb_iterator_t* iter, size_t* vlen);
void rocksdb_iter_get_error(const rocksdb_iterator_t* iter, char** errptr);

//////////////////////////// rocksdb_filterpolicy_t
rocksdb_filterpolicy_t* rocksdb_filterpolicy_create(
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
rocksdb_filterpolicy_t* rdbc_filterpolicy_create(uintptr_t idx);
rocksdb_filterpolicy_t* rocksdb_filterpolicy_create_bloom(int bits_per_key);

//////////////////////////// rocksdb_cache_t
rocksdb_cache_t* rocksdb_cache_create_lru(size_t capacity);
void rocksdb_cache_destroy(rocksdb_cache_t* cache);
void rocksdb_cache_set_capacity(rocksdb_cache_t* cache, size_t capacity);

//////////////////////////// rocksdb_block_based_table_options_t
rocksdb_block_based_table_options_t*
rocksdb_block_based_options_create();
void rocksdb_block_based_options_destroy(
    rocksdb_block_based_table_options_t* options);
void rocksdb_block_based_options_set_block_size(
    rocksdb_block_based_table_options_t* options, size_t block_size);
void rocksdb_block_based_options_set_filter_policy(
    rocksdb_block_based_table_options_t* options,
    rocksdb_filterpolicy_t* filter_policy);
void rocksdb_block_based_options_set_no_block_cache(
    rocksdb_block_based_table_options_t* options,
    unsigned char no_block_cache);
void rocksdb_block_based_options_set_block_cache(
    rocksdb_block_based_table_options_t* options,
    rocksdb_cache_t* block_cache);
void rocksdb_block_based_options_set_block_cache_compressed(
    rocksdb_block_based_table_options_t* options,
    rocksdb_cache_t* block_cache_compressed);
void rocksdb_block_based_options_set_whole_key_filtering(
    rocksdb_block_based_table_options_t* options, unsigned char v);

#ifdef __cplusplus
}  /* end extern "C" */
#endif

#endif  // __DGROCKSDBC__
