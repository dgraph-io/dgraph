/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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

	"github.com/dgraph-io/dgraph/store/rocksdb"
	"github.com/dgraph-io/dgraph/x"
)

var log = x.Log("store")

type Store struct {
	db   *rocksdb.DB
	opt  *rocksdb.Options
	ropt *rocksdb.ReadOptions
	wopt *rocksdb.WriteOptions
}

func (s *Store) Init(filepath string) {
	s.opt = rocksdb.NewOptions()
	s.opt.SetCreateIfMissing(true)
	fp := rocksdb.NewBloomFilter(16)
	s.opt.SetFilterPolicy(fp)

	s.ropt = rocksdb.NewReadOptions()
	s.wopt = rocksdb.NewWriteOptions()
	s.wopt.SetSync(true)

	var err error
	s.db, err = rocksdb.Open(filepath, s.opt)
	if err != nil {
		x.Err(log, err).WithField("filepath", filepath).
			Fatal("While opening store")
		return
	}
}

func (s *Store) Get(key []byte) (val []byte, rerr error) {
	val, rerr = s.db.Get(s.ropt, key)
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

func (s *Store) Close() {
	s.db.Close()
}
