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
	"flag"
	"fmt"
	"strconv"

	rocksdb "github.com/tecbot/gorocksdb"

	"github.com/dgraph-io/dgraph/x"
)

var (
	log = x.Log("store")

	// It is not advisable to put flags not in mains, but in this case, it reduces
	// quite a bit of typing and it should be clear (and remain clear) that all
	// RocksDB flags are found here and here only.
	enableBlockCache = flag.Bool("enableBlockCache", false, "use block cache for RocksDB")
	blockCacheSize   = flag.Int("blockCacheSize", 8<<20, "block cache size for RocksDB")
)

// Store contains some handles to RocksDB.
type Store struct {
	db       *rocksdb.DB
	opt      *rocksdb.Options // Contains blockopt.
	blockopt *rocksdb.BlockBasedTableOptions
	ropt     *rocksdb.ReadOptions
	wopt     *rocksdb.WriteOptions
}

// Options is a set of options to configure RocksDB usage in Dgraph only.
type Options struct {
	EnableBlockCache bool
	BlockCacheSize   int // In bytes. A good number is 8 << 20 or 8M.
}

// NewDefaultOptions creates default options for RocksDB in Dgraph.
func NewDefaultOptions() *Options {
	return &Options{
		EnableBlockCache: *enableBlockCache,
		BlockCacheSize:   *blockCacheSize,
	}
}

func (s *Store) setOpts(opt *Options) {
	s.opt = rocksdb.NewDefaultOptions()

	// Initialize BlockBasedTableOptions.
	s.blockopt = rocksdb.NewDefaultBlockBasedTableOptions()
	if opt.EnableBlockCache {
		// This gives us handles to the cache so that we can query its usage.
		s.blockopt.SetBlockCache(rocksdb.NewLRUCache(opt.BlockCacheSize))
		s.blockopt.SetBlockCacheCompressed(rocksdb.NewLRUCache(opt.BlockCacheSize))
		s.blockopt.SetNoBlockCache(false)
	} else {
		s.blockopt.SetNoBlockCache(true)
	}
	// It is crucial to link s.blockopt to s.opt.
	s.opt.SetBlockBasedTableFactory(s.blockopt)

	s.opt.SetCreateIfMissing(true)
	fp := rocksdb.NewBloomFilter(16)
	s.blockopt.SetFilterPolicy(fp)

	s.ropt = rocksdb.NewDefaultReadOptions()
	s.wopt = rocksdb.NewDefaultWriteOptions()
	s.wopt.SetSync(false) // We don't need to do synchronous writes.
}

func newStore(opt *Options) *Store {
	s := &Store{}
	s.setOpts(opt)
	return s
}

// NewStore constructs a Store object at filepath, given some options.
func NewStore(filepath string, opt *Options) (*Store, error) {
	s := newStore(opt)
	var err error
	s.db, err = rocksdb.OpenDb(s.opt, filepath)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// NewReadOnlyStore constructs a readonly Store object at filepath, given options.
func NewReadOnlyStore(filepath string, opt *Options) (*Store, error) {
	s := newStore(opt)
	var err error
	s.db, err = rocksdb.OpenDbForReadOnly(s.opt, filepath, false)
	if err != nil {
		return nil, err
	}
	return s, nil
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
