// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package polkadb

// PutItem wraps the database write operation supported by regular database.
type PutItem interface {
	Put(key []byte, value []byte) error
}

// Database wraps all database operations. All methods are safe for concurrent use.
type Database interface {
	PutItem
	Iteratee
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
	Del(key []byte) error
	NewBatch() Batch
	Close() error
	Path() string
}

// Batch is a write-only operation
type Batch interface {
	PutItem
	ValueSize() int
	Write() error
	Reset()
	Delete(key []byte) error
}

// Iterator iterates over BadgerDBs key/value pairs in ascending key order
// must be released after use
type Iterator interface {
	Next() bool
	Key() []byte
	Value() []byte
	Release()
}

// Iteratee wraps the NewIterator methods of BadgerService
type Iteratee interface {
	NewIterator() Iterable
}

type Reader interface {
	Get(key []byte) ([]byte, error)
	Has(key []byte) (bool, error)
}

type Writer interface {
	PutItem
	Del(key []byte) error
}
