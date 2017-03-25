/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

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
// CAUTION: checkpointDir should not already exist. If so, nothing will happen.
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
