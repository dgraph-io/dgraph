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
	"strconv"

	"github.com/dgraph-io/dgraph/rdb"
	"github.com/dgraph-io/dgraph/x"
)

var log = x.Log("store")

// Store contains some handles to RocksDB.
type Store struct {
	db       *rdb.DB
	opt      *rdb.Options // Contains blockopt.
	blockopt *rdb.BlockBasedTableOptions
	ropt     *rdb.ReadOptions
	wopt     *rdb.WriteOptions
}

func (s *Store) setOpts() {
	s.opt = rdb.NewDefaultOptions()
	s.blockopt = rdb.NewDefaultBlockBasedTableOptions()
	s.opt.SetBlockBasedTableFactory(s.blockopt)

	// If you want to access blockopt.blockCache, you need to grab handles to them
	// as well. Otherwise, they will be nil. However, for now, we do not really need
	// to do this.
	// s.blockopt.SetBlockCache(rocksdb.NewLRUCache(blockCacheSize))
	// s.blockopt.SetBlockCacheCompressed(rocksdb.NewLRUCache(blockCacheSize))

	s.opt.SetCreateIfMissing(true)
	fp := rdb.NewBloomFilter(16)
	s.blockopt.SetFilterPolicy(fp)

	s.ropt = rdb.NewDefaultReadOptions()
	s.wopt = rdb.NewDefaultWriteOptions()
	s.wopt.SetSync(false) // We don't need to do synchronous writes.
}

// NewStore constructs a Store object at filepath, given some options.
func NewStore(filepath string) (*Store, error) {
	s := &Store{}
	s.setOpts()
	var err error
	s.db, err = rdb.OpenDb(s.opt, filepath)
	if err != nil {
		return nil, x.Wrap(err)
	}
	return s, nil
}

// NewReadOnlyStore constructs a readonly Store object at filepath, given options.
func NewReadOnlyStore(filepath string) (*Store, error) {
	s := &Store{}
	s.setOpts()
	var err error
	s.db, err = rdb.OpenDbForReadOnly(s.opt, filepath, false)
	if err != nil {
		return nil, x.Wrap(err)
	}
	return s, nil
}

// Get returns the value given a key for RocksDB.
func (s *Store) Get(key []byte) ([]byte, error) {
	valSlice, err := s.db.Get(s.ropt, key)
	if err != nil {
		return nil, err
	}

	if valSlice == nil {
		return nil, nil
	}
	return valSlice.Data(), nil
}

// SetOne adds a key-value to data store.
func (s *Store) SetOne(k []byte, val []byte) error { return s.db.Put(s.wopt, k, val) }

// Delete deletes a key from data store.
func (s *Store) Delete(k []byte) error { return s.db.Delete(s.wopt, k) }

// NewIterator initializes a new iterator and returns it.
func (s *Store) NewIterator() *rdb.Iterator {
	ro := rdb.NewDefaultReadOptions()
	// SetFillCache should be set to false for bulk reads to avoid caching data
	// while doing bulk scans.
	ro.SetFillCache(false)
	return s.db.NewIterator(ro)
}

// Close closes our data store.
func (s *Store) Close() { s.db.Close() }

// Memtable returns the memtable size.
func (s *Store) MemtableSize() uint64 {
	memTableSize, _ := strconv.ParseUint(s.db.GetProperty("rocksdb.cur-size-all-mem-tables"), 10, 64)
	return memTableSize
}

// IndexFilterblockSize returns the filter block size.
func (s *Store) IndexFilterblockSize() uint64 {
	blockSize, _ := strconv.ParseUint(s.db.GetProperty("rocksdb.estimate-table-readers-mem"), 10, 64)
	return blockSize
}

// NewWriteBatch creates a new WriteBatch object and returns a pointer to it.
func (s *Store) NewWriteBatch() *rdb.WriteBatch { return rdb.NewWriteBatch() }

// WriteBatch does a batch write to RocksDB from the data in WriteBatch object.
func (s *Store) WriteBatch(wb *rdb.WriteBatch) error {
	if err := s.db.Write(s.wopt, wb); err != nil {
		return x.Wrap(err)
	}
	return nil
}

// NewCheckpoint creates new checkpoint from current store.
func (s *Store) NewCheckpoint() (*rdb.Checkpoint, error) { return s.db.NewCheckpoint() }
