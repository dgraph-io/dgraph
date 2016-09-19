package rdb

// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"
import (
	"errors"
	"unsafe"
)

// Checkpoint can be used to create openable snapshots.
type Checkpoint struct {
	c   *C.rdb_checkpoint_t
	cDb *C.rdb_t
}

// Destroy removes the snapshot from the database's list of snapshots.
func (s *Checkpoint) Destroy() {
	C.rdb_destroy_checkpoint(s.c)
	s.c, s.cDb = nil, nil
}

// NewCheckpoint creates a new snapshot of the database.
func (db *DB) NewCheckpoint() (*Checkpoint, error) {
	var cErr *C.char
	cCheck := C.rdb_create_checkpoint(db.c, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return nil, errors.New(C.GoString(cErr))
	}
	return &Checkpoint{
		c:   cCheck,
		cDb: db.c,
	}, nil
}

// Save builds openable snapshot of RocksDB on disk.
func (s *Checkpoint) Save(checkpointDir string) error {
	var (
		cErr *C.char
		cDir = C.CString(checkpointDir)
	)
	defer C.free(unsafe.Pointer(cDir))
	C.rdb_open_checkpoint(s.c, cDir, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return errors.New(C.GoString(cErr))
	}
	return nil
}
