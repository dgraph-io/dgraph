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
	snap     *rocksdb.Snapshot
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

func (s *Store) GetIterator() *rocksdb.Iterator {
	return s.db.NewIterator(s.ropt)
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

// SetSnapshot creates a snapshot and stores it in store so that it can be
// released later.
func (s *Store) SetSnapshot() {
	snap := s.db.NewSnapshot()
	// SetFillCache should be set to false for bulk reads to avoid caching data
	// while doing bulk scans.
	s.ropt.SetFillCache(false)
	s.ropt.SetSnapshot(snap)
	s.snap = snap
}

// ReleaseSnapshot releases a snapshot.
func (s *Store) ReleaseSnapshot() {
	s.ropt.SetFillCache(true)
	s.snap.Release()
}

const (
	MB = 1 << 20
)

// WriteBatch performs a batch write of key value pairs to RocksDB.
func (s *Store) WriteBatch(kv chan x.KV, che chan error) {
	wb := rocksdb.NewWriteBatch()
	for i := range kv {
		wb.Put(i.Key, i.Val)
		if len(wb.Data()) > 32*MB {
			if err := s.db.Write(s.wopt, wb); err != nil {
				che <- err
				return
			}
		}
	}
	// After channel is closed the above loop would exit, we write the data in
	// write batch here.
	if len(wb.Data()) > 0 {
		if err := s.db.Write(s.wopt, wb); err != nil {
			che <- err
			return
		}
	}
	che <- nil
}
