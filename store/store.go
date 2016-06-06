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

	"github.com/dgraph-io/dgraph/x"
	rocksdb "github.com/tecbot/gorocksdb"
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

func (s *Store) Init(filepath string) {
	s.setOpts()
	var err error
	s.db, err = rocksdb.OpenDb(s.opt, filepath)
	if err != nil {
		x.Err(log, err).WithField("filepath", filepath).
			Fatal("While opening store")
		return
	}
}

func (s *Store) InitReadOnly(filepath string) {
	s.setOpts()
	var err error
	s.db, err = rocksdb.OpenDbForReadOnly(s.opt, filepath, false)
	// TODO(Ashwin):When will it be true
	if err != nil {
		x.Err(log, err).WithField("filepath", filepath).
			Fatal("While opening store")
		return
	}
}

func (s *Store) Get(key []byte) (val []byte, rerr error) {
	valS, rerr := s.db.Get(s.ropt, key)
	val = valS.Data()
	if rerr == nil && val == nil {
		return []byte(""), fmt.Errorf("E_KEY_NOT_FOUND")
	}
	return val, rerr
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
