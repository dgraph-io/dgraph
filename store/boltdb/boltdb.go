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

package boltdb

import (
	"fmt"
	"path/filepath"

	"github.com/boltdb/bolt"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

const (
	Name            = `boltdb`
	defaultFilename = `dgraph.db`
)

var (
	log           = x.Log("store")
	defaultBucket = []byte(`dgraph`)
)

func init() {
	store.Registry.Add(Name, func() store.Store { return &Store{} })
}

type Store struct {
	db *bolt.DB
}

func (s *Store) Init(dir string) {
	var err error
	path := filepath.Join(dir, defaultFilename)
	s.db, err = bolt.Open(path, 0600, nil)
	if err != nil {
		log.Fatalf("Error while opening filepath: %v", path)
		return
	}

	err = s.db.Batch(func(tx *bolt.Tx) error {
		_, err = tx.CreateBucketIfNotExists(defaultBucket)
		return err
	})
	if err != nil {
		log.Fatalf("Error creating default bucket: %v", path)
		return
	}
}

func (s *Store) InitReadOnly(dir string) {
	var err error
	path := filepath.Join(dir, defaultFilename)
	s.db, err = bolt.Open(path, 0600, &bolt.Options{ReadOnly: true})
	// TODO(Ashwin):When will it be true
	if err != nil {
		log.Fatalf("Error while opening filepath: %v", path)
		return
	}
}

func (s *Store) Get(key []byte) (val []byte, rerr error) {
	rerr = s.db.View(func(tx *bolt.Tx) error {
		val = tx.Bucket(defaultBucket).Get(key)
		if val == nil {
			return fmt.Errorf("E_KEY_NOT_FOUND")
		}
		return nil
	})
	if rerr != nil {
		return nil, rerr
	}
	return val, nil
}

func (s *Store) SetOne(k []byte, val []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(defaultBucket).Put(k, val)
	})
}

func (s *Store) Delete(k []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(defaultBucket).Delete(k)
	})
}

func (s *Store) GetIterator() store.Iterator {
	ready := make(chan struct{})
	i := &Iterator{done: make(chan struct{})}

	go s.db.View(func(tx *bolt.Tx) error {
		i.Cursor = tx.Bucket(defaultBucket).Cursor()
		close(ready)
		<-i.done
		return nil
	})
	<-ready

	return i
}

func (s *Store) Close() {
	s.db.Close()
}

type Iterator struct {
	key  []byte
	done chan struct{}
	*bolt.Cursor
}

func (i *Iterator) First() (k []byte, val []byte) {
	i.key, val = i.Cursor.First()
	return i.key, val
}

func (i *Iterator) Valid() bool {
	return i.key != nil
}

func (i *Iterator) Next() (k []byte, val []byte) {
	i.key, val = i.Cursor.Next()
	return i.key, val
}

func (i *Iterator) Close() {
	close(i.done)
}
