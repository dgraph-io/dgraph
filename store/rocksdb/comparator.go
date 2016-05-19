package rocksdb

// #cgo LDFLAGS: -lrocksdb
// #include "/usr/include/rocksdb/c.h"
import "C"

// DestroyComparator deallocates a *C.rocksdb_comparator_t.
//
// This is provided as a convienience to advanced users that have implemented
// their own comparators in C in their own code.
func DestroyComparator(cmp *C.rocksdb_comparator_t) {
	C.rocksdb_comparator_destroy(cmp)
}
