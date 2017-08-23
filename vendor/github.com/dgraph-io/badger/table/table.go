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

package table

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/AndreasBriese/bbloom"
	"github.com/dgraph-io/badger/y"
	"github.com/pkg/errors"
)

const fileSuffix = ".sst"

const (
	Nothing = iota
	MemoryMap
	LoadToRAM
)

type keyOffset struct {
	key    []byte
	offset int
	len    int
}

type Table struct {
	sync.Mutex

	fd        *os.File // Own fd.
	tableSize int      // Initialized in OpenTable, using fd.Stat().

	blockIndex []keyOffset
	ref        int32 // For file garbage collection.  Atomic.

	mapTableTo int
	mmap       []byte // Memory mapped.

	// The following are initialized once and const.
	smallest, biggest []byte // Smallest and largest keys.
	id                uint64 // file id, part of filename

	bf bbloom.Bloom
}

func (t *Table) Ref() int32 { return atomic.LoadInt32(&t.ref) }

func (t *Table) IncrRef() {
	atomic.AddInt32(&t.ref, 1)
}

func (t *Table) DecrRef() error {
	newRef := atomic.AddInt32(&t.ref, -1)
	if newRef == 0 {
		// We can safely delete this file, because for all the current files, we always have
		// at least one reference pointing to them.

		// It's necessary to delete windows files
		if t.mapTableTo == MemoryMap {
			munmap(t.mmap)
		}
		if err := t.fd.Truncate(0); err != nil {
			// This is very important to let the FS know that the file is deleted.
			return err
		}
		filename := t.fd.Name()
		if err := t.fd.Close(); err != nil {
			return err
		}
		if err := os.Remove(filename); err != nil {
			return err
		}
	}
	return nil
}

type Block struct {
	offset int
	data   []byte
}

func (b Block) NewIterator() *BlockIterator {
	return &BlockIterator{data: b.data}
}

type byKey []keyOffset

func (b byKey) Len() int               { return len(b) }
func (b byKey) Swap(i int, j int)      { b[i], b[j] = b[j], b[i] }
func (b byKey) Less(i int, j int) bool { return bytes.Compare(b[i].key, b[j].key) < 0 }

// OpenTable assumes file has only one table and opens it.  Takes ownership of fd upon function
// entry.  Returns a table with one reference count on it (decrementing which may delete the file!
// -- consider t.Close() instead).  The fd has to writeable because we call Truncate on it before
// deleting.
func OpenTable(fd *os.File, mapTableTo int) (*Table, error) {
	fileInfo, err := fd.Stat()
	if err != nil {
		// It's OK to ignore fd.Close() errs in this function because we have only read
		// from the file.
		_ = fd.Close()
		return nil, y.Wrap(err)
	}

	filename := fileInfo.Name()
	id, ok := ParseFileID(filename)
	if !ok {
		_ = fd.Close()
		return nil, errors.Errorf("Invalid filename: %s", filename)
	}
	t := &Table{
		fd:         fd,
		ref:        1, // Caller is given one reference.
		id:         id,
		mapTableTo: mapTableTo,
	}

	t.tableSize = int(fileInfo.Size())

	if mapTableTo == MemoryMap {
		t.mmap, err = mmap(fd, fileInfo.Size())
		if err != nil {
			_ = fd.Close()
			return nil, y.Wrapf(err, "Unable to map file")
		}
	} else if mapTableTo == LoadToRAM {
		err = t.LoadToRAM()
		if err != nil {
			_ = fd.Close()
			return nil, y.Wrap(err)
		}
	}

	if err := t.readIndex(); err != nil {
		return nil, y.Wrap(err)
	}

	it := t.NewIterator(false)
	defer it.Close()
	it.Rewind()
	if it.Valid() {
		t.smallest = it.Key()
	}

	it2 := t.NewIterator(true)
	defer it2.Close()
	it2.Rewind()
	if it2.Valid() {
		t.biggest = it2.Key()
	}
	return t, nil
}

func (t *Table) Close() error {
	if t.mapTableTo == MemoryMap {
		munmap(t.mmap)
	}
	if err := t.fd.Close(); err != nil {
		return err
	}
	return nil
}

var EOF = errors.New("End of mapped region")

func (t *Table) read(off int, sz int) ([]byte, error) {
	if len(t.mmap) > 0 {
		if len(t.mmap[off:]) < sz {
			return nil, EOF
		}
		return t.mmap[off : off+sz], nil
	}

	res := make([]byte, sz)
	nbr, err := t.fd.ReadAt(res, int64(off))
	y.NumReads.Add(1)
	y.NumBytesRead.Add(int64(nbr))
	return res, err
}

func (t *Table) readNoFail(off int, sz int) []byte {
	res, err := t.read(off, sz)
	y.Check(err)
	return res
}

func (t *Table) readIndex() error {
	readPos := t.tableSize

	// Read bloom filter.
	readPos -= 4
	buf := t.readNoFail(readPos, 4)
	bloomLen := int(binary.BigEndian.Uint32(buf))
	readPos -= bloomLen
	data := t.readNoFail(readPos, bloomLen)
	t.bf = bbloom.JSONUnmarshal(data)

	readPos -= 4
	buf = t.readNoFail(readPos, 4)
	restartsLen := int(binary.BigEndian.Uint32(buf))

	readPos -= 4 * restartsLen
	buf = t.readNoFail(readPos, 4*restartsLen)

	offsets := make([]int, restartsLen)
	for i := 0; i < restartsLen; i++ {
		offsets[i] = int(binary.BigEndian.Uint32(buf[:4]))
		buf = buf[4:]
	}

	// The last offset stores the end of the last block.
	for i := 0; i < len(offsets); i++ {
		var o int
		if i == 0 {
			o = 0
		} else {
			o = offsets[i-1]
		}

		ko := keyOffset{
			offset: o,
			len:    offsets[i] - o,
		}
		t.blockIndex = append(t.blockIndex, ko)
	}

	if len(t.blockIndex) == 1 {
		return nil
	}

	che := make(chan error, len(t.blockIndex))
	for i := 0; i < len(t.blockIndex); i++ {

		bo := &t.blockIndex[i]
		go func(ko *keyOffset) {
			var h header

			offset := ko.offset
			buf, err := t.read(offset, h.Size())
			if err != nil {
				che <- errors.Wrap(err, "While reading first header in block")
				return
			}

			h.Decode(buf)
			y.AssertTruef(h.plen == 0, "Key offset: %+v, h.plen = %d", *ko, h.plen)

			offset += h.Size()
			buf = make([]byte, h.klen)
			var out []byte
			if out, err = t.read(offset, int(h.klen)); err != nil {
				che <- errors.Wrap(err, "While reading first key in block")
				return
			}
			y.AssertTrue(len(buf) == copy(buf, out))

			ko.key = buf
			che <- nil
		}(bo)
	}

	for range t.blockIndex {
		err := <-che
		if err != nil {
			return err
		}
	}
	sort.Sort(byKey(t.blockIndex))
	return nil
}

func (t *Table) block(idx int) (Block, error) {
	y.AssertTruef(idx >= 0, "idx=%d", idx)
	if idx >= len(t.blockIndex) {
		return Block{}, errors.New("Block out of index.")
	}

	ko := t.blockIndex[idx]
	block := Block{
		offset: ko.offset,
	}
	var err error
	block.data, err = t.read(block.offset, ko.len)
	return block, err
}

func (t *Table) Size() int64                 { return int64(t.tableSize) }
func (t *Table) Smallest() []byte            { return t.smallest }
func (t *Table) Biggest() []byte             { return t.biggest }
func (t *Table) Filename() string            { return t.fd.Name() }
func (t *Table) ID() uint64                  { return t.id }
func (t *Table) DoesNotHave(key []byte) bool { return !t.bf.Has(key) }

func ParseFileID(name string) (uint64, bool) {
	name = path.Base(name)
	if !strings.HasSuffix(name, fileSuffix) {
		return 0, false
	}
	//	suffix := name[len(fileSuffix):]
	name = strings.TrimSuffix(name, fileSuffix)
	id, err := strconv.Atoi(name)
	if err != nil {
		return 0, false
	}
	y.AssertTrue(id >= 0)
	return uint64(id), true
}

func TableFilename(id uint64) string {
	return fmt.Sprintf("%06d", id) + fileSuffix
}

func NewFilename(id uint64, dir string) string {
	return filepath.Join(dir, TableFilename(id))
}

func (t *Table) LoadToRAM() error {
	t.mmap = make([]byte, t.tableSize)
	read, err := t.fd.ReadAt(t.mmap, 0)
	if err != nil || read != t.tableSize {
		return y.Wrapf(err, "Unable to load file in memory. Table file: %s", t.Filename())
	}
	y.NumReads.Add(1)
	y.NumBytesRead.Add(int64(read))
	return nil
}
