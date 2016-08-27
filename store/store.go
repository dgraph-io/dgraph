/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package store

import (
	"fmt"
	"strconv"

	rocksdb "github.com/tecbot/gorocksdb"

	"github.com/dgraph-io/dgraph/x"
)

var log = x.Log("store")

type Store struct {
	db       *rocksdb.DB
	opt      *rocksdb.Options
	blockopt *rocksdb.BlockBasedTableOptions
	ropt     *rocksdb.ReadOptions
	wopt     *rocksdb.WriteOptions
}

func (s *Store) setOpts() {
	s.opt = rocksdb.NewDefaultOptions()
	s.blockopt = rocksdb.NewDefaultBlockBasedTableOptions()
	s.opt.SetCreateIfMissing(true)
	fp := rocksdb.NewBloomFilter(16)
	s.blockopt.SetFilterPolicy(fp)

	s.ropt = rocksdb.NewDefaultReadOptions()
	s.wopt = rocksdb.NewDefaultWriteOptions()
	s.wopt.SetSync(false) // We don't need to do synchronous writes.
}

func (s *Store) Init(filepath string) (err error) {
	s.setOpts()
	s.db, err = rocksdb.OpenDb(s.opt, filepath)
	return
}

func (s *Store) InitReadOnly(filepath string) (err error) {
	s.setOpts()
	s.db, err = rocksdb.OpenDbForReadOnly(s.opt, filepath, false)
	return
}

func (s *Store) Get(key []byte) (val []byte, rerr error) {
	valSlice, rerr := s.db.Get(s.ropt, key)
	if rerr != nil {
		return []byte(""), rerr
	}

	if valSlice == nil {
		return []byte(""), fmt.Errorf("E_KEY_NOT_FOUND")
	}

	val = valSlice.Data()
	if val == nil {
		return []byte(""), fmt.Errorf("E_KEY_NOT_FOUND")
	}
	return val, nil
}

func (s *Store) SetOne(k []byte, val []byte) error {
	return s.db.Put(s.wopt, k, val)
}

func (s *Store) Delete(k []byte) error {
	return s.db.Delete(s.wopt, k)
}

// GetIterator initializes a new iterator and returns it.
func (s *Store) GetIterator() *rocksdb.Iterator {
	ro := rocksdb.NewDefaultReadOptions()
	// SetFillCache should be set to false for bulk reads to avoid caching data
	// while doing bulk scans.
	ro.SetFillCache(false)
	return s.db.NewIterator(ro)
}

func (s *Store) Close() {
	s.db.Close()
}

func (s *Store) MemtableSize() uint64 {
	memTableSize, _ := strconv.ParseUint(s.db.GetProperty("rocksdb.cur-size-all-mem-tables"), 10, 64)
	return memTableSize
}

func (s *Store) IndexFilterblockSize() uint64 {
	blockSize, _ := strconv.ParseUint(s.db.GetProperty("rocksdb.estimate-table-readers-mem"), 10, 64)
	return blockSize
}

// NewWriteBatch creates a new WriteBatch object and returns a pointer to it.
func (s *Store) NewWriteBatch() *rocksdb.WriteBatch {
	return rocksdb.NewWriteBatch()
}

// WriteBatch does a batch write to RocksDB from the data in WriteBatch object.
func (s *Store) WriteBatch(wb *rocksdb.WriteBatch) error {
	if err := s.db.Write(s.wopt, wb); err != nil {
		return err
	}
	return nil
}
