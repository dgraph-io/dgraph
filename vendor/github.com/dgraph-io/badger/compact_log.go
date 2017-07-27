/*
 * Copyright 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package badger

// Might consider moving this into a separate package.

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"sync/atomic"

	"github.com/dgraph-io/badger/table"
	"github.com/dgraph-io/badger/y"
	"github.com/pkg/errors"
)

type compactLog struct {
	fd *os.File
}

// compaction is our compaction in a easily serializable form.
type compaction struct {
	compactID uint64
	done      byte
	toDelete  []uint64
	toInsert  []uint64
}

func (s *compactLog) init(filename string) error {
	fd, err := y.OpenSyncedFile(filename, true)
	s.fd = fd
	return errors.Wrapf(err, "Opening file: %q", filename)
}

func (s *compactLog) close() error {
	return s.fd.Close()
}

func (s *compactLog) add(c *compaction) error {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, c.compactID); err != nil {
		return err
	}
	buf.WriteByte(c.done)
	if c.done == 0 {
		if err := binary.Write(&buf, binary.BigEndian, uint32(len(c.toDelete))); err != nil {
			return err
		}
		for _, id := range c.toDelete {
			if err := binary.Write(&buf, binary.BigEndian, id); err != nil {
				return err
			}
		}
		if err := binary.Write(&buf, binary.BigEndian, uint32(len(c.toInsert))); err != nil {
			return err
		}
		for _, id := range c.toInsert {
			if err := binary.Write(&buf, binary.BigEndian, id); err != nil {
				return err
			}
		}
	}
	b := buf.Bytes()
	_, err := s.fd.Write(b) // Write in one sync.
	return err
}

func compactLogIterate(filename string, f func(c *compaction)) error {
	fd, err := os.Open(filename) // Read only.
	if err != nil {
		return err
	}
	defer fd.Close()

	var buf [5]byte // Temp buffer.
	var size uint32
	for {
		var c compaction
		err := binary.Read(fd, binary.BigEndian, &c.compactID)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if _, err = fd.Read(buf[:1]); err != nil {
			return err
		}
		c.done = buf[0]
		if c.done == 0 {
			if err := binary.Read(fd, binary.BigEndian, &size); err != nil {
				return err
			}
			n := int(size)
			c.toDelete = make([]uint64, n)
			for i := 0; i < n; i++ {
				if err := binary.Read(fd, binary.BigEndian, &c.toDelete[i]); err != nil {
					return err
				}
			}
			if err := binary.Read(fd, binary.BigEndian, &size); err != nil {
				return err
			}
			n = int(size)
			c.toInsert = make([]uint64, n)
			for i := 0; i < n; i++ {
				if err := binary.Read(fd, binary.BigEndian, &c.toInsert[i]); err != nil {
					return err
				}
			}
		}
		f(&c)
	}
	return nil
}

// replay uses the compactLog to clean up the directory. Two cases.
// 1) Compaction is done: We will delete files that are in compaction.toDelete.
//    Some files may linger around because of iterators holding references.
// 2) Compaction is not done: We need to undo the compaction.

func deleteIfPresent(id uint64, dir string) {
	fn := table.NewFilename(id, dir)
	_, err := os.Stat(fn)
	if err == nil {
		os.Remove(fn)
	}
}

func compactLogReplay(filename, dir string, idMap map[uint64]struct{}) {
	cMap := make(map[uint64]*compaction)
	y.Check(compactLogIterate(filename, func(c *compaction) {
		if c.done == 0 {
			cMap[c.compactID] = c
			return
		}
		cRef, found := cMap[c.compactID]
		y.AssertTruef(found,
			"Trying to end compaction that is never present: %d", c.compactID)
		// A compaction is done. Check the files that are supposed to be deleted.
		for _, id := range cRef.toDelete {
			deleteIfPresent(id, dir)
		}
		// Files inserted by compaction may be deleted. We don't track this.
		delete(cMap, c.compactID)
	}))

	if len(cMap) == 0 {
		return
	}

	// Anything left in cMap are unterminated compactions. We want to undo these
	// compactions. Inserted files should be deleted. Deleted files are expected
	// to be present.
	for _, c := range cMap {
		for _, id := range c.toInsert {
			deleteIfPresent(id, dir)
		}
		for _, id := range c.toDelete {
			_, err := os.Stat(table.NewFilename(id, dir))
			y.Check(err)
		}
	}
}

func (s *levelsController) buildCompactionLogEntry(def *compactDef) *compaction {
	var newIDMin, newIDMax uint64
	c := new(compaction)
	c.compactID = atomic.AddUint64(&s.maxCompactID, 1)

	var estSize int64
	for _, t := range def.top {
		c.toDelete = append(c.toDelete, t.ID())
		estSize += t.Size()
	}
	for _, t := range def.bot {
		c.toDelete = append(c.toDelete, t.ID())
		estSize += t.Size()
	}
	estNumTables := 1 + (estSize+s.kv.opt.MaxTableSize-1)/s.kv.opt.MaxTableSize
	newIDMin, newIDMax = s.reserveFileIDs(int(estNumTables))
	// TODO: Consider storing just two numbers for toInsert.
	for i := newIDMin; i < newIDMax; i++ {
		c.toInsert = append(c.toInsert, uint64(i))
	}
	return c
}
