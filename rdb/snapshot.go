package rdb

// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"

// Snapshot provides a consistent view of read operations in a DB.
type Snapshot struct {
	c   *C.rdb_snapshot_t
	cDb *C.rdb_t
}

// NewNativeSnapshot creates a Snapshot object.
func NewNativeSnapshot(c *C.rdb_snapshot_t, cDb *C.rdb_t) *Snapshot {
	return &Snapshot{c, cDb}
}

// Release removes the snapshot from the database's list of snapshots.
func (s *Snapshot) Release() {
	C.rdb_release_snapshot(s.cDb, s.c)
	s.c, s.cDb = nil, nil
}

// NewSnapshot creates a new snapshot of the database.
func (db *DB) NewSnapshot() *Snapshot {
	cSnap := C.rdb_create_snapshot(db.c)
	return NewNativeSnapshot(cSnap, db.c)
}
