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
	"bytes"
	"encoding/binary"

	"github.com/manishrjain/gocrud/x"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var log = x.Log("store")

type Store struct {
	db  *leveldb.DB
	opt *opt.Options
}

func (s *Store) Init(filepath string) {
	var err error
	s.db, err = leveldb.OpenFile(filepath, s.opt)
	if err != nil {
		x.LogErr(log, err).WithField("filepath", filepath).
			Fatal("While opening store")
		return
	}
}

func (s *Store) IsNew(id uint64) bool {
	return false
}

// key = (attribute, entity id)
func Key(attr string, eid uint64) []byte {
	buf := new(bytes.Buffer)
	buf.WriteString(attr)
	if err := binary.Write(buf, binary.LittleEndian, eid); err != nil {
		log.Fatalf("Error while creating key with attr: %v eid: %v\n", attr, eid)
	}
	return buf.Bytes()
}

func (s *Store) Get(k []byte) (val []byte, rerr error) {
	return s.db.Get(k, nil)
}

func (s *Store) SetOne(k []byte, val []byte) error {
	wb := new(leveldb.Batch)
	wb.Put(k, val)
	return s.db.Write(wb, nil)
}

func (s *Store) Delete(k []byte) error {
	return s.db.Delete(k, nil)
}

func (s *Store) Close() error {
	return s.db.Close()
}
