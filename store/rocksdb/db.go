package rocksdb

/*
#cgo LDFLAGS: -lrocksdb
#include <stdlib.h>
#include "rocksdb/c.h"

// This function exists only to clean up lack-of-const warnings when
// rocksdb_approximate_sizes is called from Go-land.
void rocksdb_rocksdb_approximate_sizes(
    rocksdb_t* db,
    int num_ranges,
    char** range_start_key, const size_t* range_start_key_len,
    char** range_limit_key, const size_t* range_limit_key_len,
    uint64_t* sizes) {
  rocksdb_approximate_sizes(db,
                            num_ranges,
                            (const char* const*)range_start_key,
                            range_start_key_len,
                            (const char* const*)range_limit_key,
                            range_limit_key_len,
                            sizes);
}
*/
import "C"

import (
	"unsafe"
)

type DatabaseError string

func (e DatabaseError) Error() string {
	return string(e)
}

// DB is a reusable handle to a LevelDB database on disk, created by Open.
//
// To avoid memory and file descriptor leaks, call Close when the process no
// longer needs the handle. Calls to any DB method made after Close will
// panic.
//
// The DB instance may be shared between goroutines. The usual data race
// conditions will occur if the same key is written to from more than one, of
// course.
type DB struct {
	Ldb *C.rocksdb_t
}

// Range is a range of keys in the database. GetApproximateSizes calls with it
// begin at the key Start and end right before the key Limit.
type Range struct {
	Start []byte
	Limit []byte
}

// Snapshot provides a consistent view of read operations in a DB. It is set
// on to a ReadOptions and passed in. It is only created by DB.NewSnapshot.
//
// To prevent memory leaks and resource strain in the database, the snapshot
// returned must be released with DB.ReleaseSnapshot method on the DB that
// created it.
type Snapshot struct {
	snap *C.rocksdb_snapshot_t
}

// Open opens a database.
//
// Creating a new database is done by calling SetCreateIfMissing(true) on the
// Options passed to Open.
//
// It is usually wise to set a Cache object on the Options with SetCache to
// keep recently used data from that database in memory.
func Open(dbname string, o *Options) (*DB, error) {
	var errStr *C.char
	ldbname := C.CString(dbname)
	defer C.free(unsafe.Pointer(ldbname))

	rocksdb := C.rocksdb_open(o.Opt, ldbname, &errStr)
	if errStr != nil {
		gs := C.GoString(errStr)
		C.free(unsafe.Pointer(errStr))
		return nil, DatabaseError(gs)
	}
	return &DB{rocksdb}, nil
}

// DestroyDatabase removes a database entirely, removing everything from the
// filesystem.
func DestroyDatabase(dbname string, o *Options) error {
	var errStr *C.char
	ldbname := C.CString(dbname)
	defer C.free(unsafe.Pointer(ldbname))

	C.rocksdb_destroy_db(o.Opt, ldbname, &errStr)
	if errStr != nil {
		gs := C.GoString(errStr)
		C.free(unsafe.Pointer(errStr))
		return DatabaseError(gs)
	}
	return nil
}

// RepairDatabase attempts to repair a database.
//
// If the database is unrepairable, an error is returned.
func RepairDatabase(dbname string, o *Options) error {
	var errStr *C.char
	ldbname := C.CString(dbname)
	defer C.free(unsafe.Pointer(ldbname))

	C.rocksdb_repair_db(o.Opt, ldbname, &errStr)
	if errStr != nil {
		gs := C.GoString(errStr)
		C.free(unsafe.Pointer(errStr))
		return DatabaseError(gs)
	}
	return nil
}

// Put writes data associated with a key to the database.
//
// If a nil []byte is passed in as value, it will be returned by Get as an
// zero-length slice.
//
// The key and value byte slices may be reused safely. Put takes a copy of
// them before returning.
func (db *DB) Put(wo *WriteOptions, key, value []byte) error {
	var errStr *C.char
	// rocksdb_put, _get, and _delete call memcpy() (by way of Memtable::Add)
	// when called, so we do not need to worry about these []byte being
	// reclaimed by GC.
	var k, v *C.char
	if len(key) != 0 {
		k = (*C.char)(unsafe.Pointer(&key[0]))
	}
	if len(value) != 0 {
		v = (*C.char)(unsafe.Pointer(&value[0]))
	}

	lenk := len(key)
	lenv := len(value)
	C.rocksdb_put(
		db.Ldb, wo.Opt, k, C.size_t(lenk), v, C.size_t(lenv), &errStr)

	if errStr != nil {
		gs := C.GoString(errStr)
		C.free(unsafe.Pointer(errStr))
		return DatabaseError(gs)
	}
	return nil
}

// Get returns the data associated with the key from the database.
//
// If the key does not exist in the database, a nil []byte is returned. If the
// key does exist, but the data is zero-length in the database, a zero-length
// []byte will be returned.
//
// The key byte slice may be reused safely. Get takes a copy of
// them before returning.
func (db *DB) Get(ro *ReadOptions, key []byte) ([]byte, error) {
	var errStr *C.char
	var vallen C.size_t
	var k *C.char
	if len(key) != 0 {
		k = (*C.char)(unsafe.Pointer(&key[0]))
	}

	value := C.rocksdb_get(
		db.Ldb, ro.Opt, k, C.size_t(len(key)), &vallen, &errStr)

	if errStr != nil {
		gs := C.GoString(errStr)
		C.free(unsafe.Pointer(errStr))
		return nil, DatabaseError(gs)
	}

	if value == nil {
		return nil, nil
	}

	defer C.free(unsafe.Pointer(value))
	return C.GoBytes(unsafe.Pointer(value), C.int(vallen)), nil
}

// Delete removes the data associated with the key from the database.
//
// The key byte slice may be reused safely. Delete takes a copy of
// them before returning.
func (db *DB) Delete(wo *WriteOptions, key []byte) error {
	var errStr *C.char
	var k *C.char
	if len(key) != 0 {
		k = (*C.char)(unsafe.Pointer(&key[0]))
	}

	C.rocksdb_delete(
		db.Ldb, wo.Opt, k, C.size_t(len(key)), &errStr)

	if errStr != nil {
		gs := C.GoString(errStr)
		C.free(unsafe.Pointer(errStr))
		return DatabaseError(gs)
	}
	return nil
}

// Write atomically writes a WriteBatch to disk.
func (db *DB) Write(wo *WriteOptions, w *WriteBatch) error {
	var errStr *C.char
	C.rocksdb_write(db.Ldb, wo.Opt, w.wbatch, &errStr)
	if errStr != nil {
		gs := C.GoString(errStr)
		C.free(unsafe.Pointer(errStr))
		return DatabaseError(gs)
	}
	return nil
}

// NewIterator returns an Iterator over the the database that uses the
// ReadOptions given.
//
// Often, this is used for large, offline bulk reads while serving live
// traffic. In that case, it may be wise to disable caching so that the data
// processed by the returned Iterator does not displace the already cached
// data. This can be done by calling SetFillCache(false) on the ReadOptions
// before passing it here.
//
// Similiarly, ReadOptions.SetSnapshot is also useful.
func (db *DB) NewIterator(ro *ReadOptions) *Iterator {
	it := C.rocksdb_create_iterator(db.Ldb, ro.Opt)
	return &Iterator{Iter: it}
}

// GetApproximateSizes returns the approximate number of bytes of file system
// space used by one or more key ranges.
//
// The keys counted will begin at Range.Start and end on the key before
// Range.Limit.
func (db *DB) GetApproximateSizes(ranges []Range) []uint64 {
	starts := make([]*C.char, len(ranges))
	limits := make([]*C.char, len(ranges))
	startLens := make([]C.size_t, len(ranges))
	limitLens := make([]C.size_t, len(ranges))
	for i, r := range ranges {
		starts[i] = C.CString(string(r.Start))
		startLens[i] = C.size_t(len(r.Start))
		limits[i] = C.CString(string(r.Limit))
		limitLens[i] = C.size_t(len(r.Limit))
	}
	sizes := make([]uint64, len(ranges))
	numranges := C.int(len(ranges))
	startsPtr := &starts[0]
	limitsPtr := &limits[0]
	startLensPtr := &startLens[0]
	limitLensPtr := &limitLens[0]
	sizesPtr := (*C.uint64_t)(&sizes[0])
	C.rocksdb_rocksdb_approximate_sizes(
		db.Ldb, numranges, startsPtr, startLensPtr,
		limitsPtr, limitLensPtr, sizesPtr)
	for i := range ranges {
		C.free(unsafe.Pointer(starts[i]))
		C.free(unsafe.Pointer(limits[i]))
	}
	return sizes
}

// PropertyValue returns the value of a database property.
//
// Examples of properties include "rocksdb.stats", "rocksdb.sstables",
// and "rocksdb.num-files-at-level0".
func (db *DB) PropertyValue(propName string) string {
	cname := C.CString(propName)
	value := C.GoString(C.rocksdb_property_value(db.Ldb, cname))
	C.free(unsafe.Pointer(cname))
	return value
}

// NewSnapshot creates a new snapshot of the database.
//
// The snapshot, when used in a ReadOptions, provides a consistent view of
// state of the database at the the snapshot was created.
//
// To prevent memory leaks and resource strain in the database, the snapshot
// returned must be released with DB.ReleaseSnapshot method on the DB that
// created it.
//
// See the LevelDB documentation for details.
func (db *DB) NewSnapshot() *Snapshot {
	return &Snapshot{C.rocksdb_create_snapshot(db.Ldb)}
}

// ReleaseSnapshot removes the snapshot from the database's list of snapshots,
// and deallocates it.
func (db *DB) ReleaseSnapshot(snap *Snapshot) {
	C.rocksdb_release_snapshot(db.Ldb, snap.snap)
}

// CompactRange runs a manual compaction on the Range of keys given. This is
// not likely to be needed for typical usage.
func (db *DB) CompactRange(r Range) {
	var start, limit *C.char
	if len(r.Start) != 0 {
		start = (*C.char)(unsafe.Pointer(&r.Start[0]))
	}
	if len(r.Limit) != 0 {
		limit = (*C.char)(unsafe.Pointer(&r.Limit[0]))
	}
	C.rocksdb_compact_range(
		db.Ldb, start, C.size_t(len(r.Start)), limit, C.size_t(len(r.Limit)))
}

// Close closes the database, rendering it unusable for I/O, by deallocating
// the underlying handle.
//
// Any attempts to use the DB after Close is called will panic.
func (db *DB) Close() {
	C.rocksdb_close(db.Ldb)
}
