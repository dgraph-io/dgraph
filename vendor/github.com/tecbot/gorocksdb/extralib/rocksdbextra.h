#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct rocksdb_block_based_table_options_t rocksdb_block_based_table_options_t;

size_t rocksdb_block_based_options_usage(
  rocksdb_block_based_table_options_t* options);

#ifdef __cplusplus
}
#endif