package gorocksdb

// #include "rocksdb/c.h"
import "C"

// Snapshot provides a consistent view of read operations in a DB.
type Snapshot struct {
	c   *C.rocksdb_snapshot_t
	cDb *C.rocksdb_t
}

// NewNativeSnapshot creates a Snapshot object.
func NewNativeSnapshot(c *C.rocksdb_snapshot_t, cDb *C.rocksdb_t) *Snapshot {
	return &Snapshot{c, cDb}
}

// Release removes the snapshot from the database's list of snapshots.
func (s *Snapshot) Release() {
	C.rocksdb_release_snapshot(s.cDb, s.c)
	s.c, s.cDb = nil, nil
}
