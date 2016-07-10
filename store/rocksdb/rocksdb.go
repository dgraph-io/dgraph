// +build rocksdb,cgo

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

package rocksdb

import (
	"fmt"

	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
	"github.com/tecbot/gorocksdb"
)

const Name = `rocksdb`

var log = x.Log("store")

func init() {
	store.Registry.Add(Name, func() store.Store { return &Store{} })
}

type Store struct {
	db       *gorocksdb.DB
	opt      *gorocksdb.Options
	blockopt *gorocksdb.BlockBasedTableOptions
	ropt     *gorocksdb.ReadOptions
	wopt     *gorocksdb.WriteOptions
}

func (s *Store) setOpts() {
	s.opt = gorocksdb.NewDefaultOptions()
	s.blockopt = gorocksdb.NewDefaultBlockBasedTableOptions()
	s.opt.SetCreateIfMissing(true)
	fp := gorocksdb.NewBloomFilter(16)
	s.blockopt.SetFilterPolicy(fp)

	s.ropt = gorocksdb.NewDefaultReadOptions()
	s.wopt = gorocksdb.NewDefaultWriteOptions()
	s.wopt.SetSync(false) // We don't need to do synchronous writes.
}

func (s *Store) Init(filepath string) {
	s.setOpts()
	var err error
	s.db, err = gorocksdb.OpenDb(s.opt, filepath)
	if err != nil {
		log.Fatalf("Error while opening filepath: %v", filepath)
		return
	}
}

func (s *Store) InitReadOnly(filepath string) {
	s.setOpts()
	var err error
	s.db, err = gorocksdb.OpenDbForReadOnly(s.opt, filepath, false)
	// TODO(Ashwin):When will it be true
	if err != nil {
		log.Fatalf("Error while opening filepath: %v", filepath)
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

func (s *Store) GetIterator() store.Iterator {
	return &Iterator{s.db.NewIterator(s.ropt)}
}

func (s *Store) Close() {
	s.db.Close()
}

type Iterator struct {
	*gorocksdb.Iterator
}

func (i *Iterator) First() (key, value []byte) {
	i.Iterator.SeekToFirst()
	if !i.Valid() {
		return nil, nil
	}
	return i.Iterator.Key().Data(), i.Iterator.Value().Data()
}

func (i *Iterator) Valid() bool {
	return i.Iterator.Valid()
}

func (i *Iterator) Next() (key, value []byte) {
	i.Iterator.Next()
	if !i.Valid() {
		return nil, nil
	}
	return i.Iterator.Key().Data(), i.Iterator.Value().Data()
}

func (i *Iterator) Close() {
}
