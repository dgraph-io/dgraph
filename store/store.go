/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

// Package store is an interface with KV stores.
package store

// Store is our KV store.
type Store interface {
	// Returns the value as a byte array.
	// Also returns the deallocate function. This is for freeing RocksDB slice.
	Get(key []byte) ([]byte, func(), error)

	// WriteBatch executes the given WriteBatch.
	WriteBatch(wb WriteBatch) error

	// SetOne sets a kv pair.
	SetOne(k []byte, val []byte) error

	// Delete deletes a key.
	Delete(k []byte) error

	// Close closes the KV store.
	Close()

	// NewWriteBatch creates a new WriteBatch for store.
	NewWriteBatch() WriteBatch

	// NewIterator creates a new Iterator for store.
	NewIterator(reversed bool) Iterator

	// GetStats returns some stats about the store.
	GetStats() string
}

// WriteBatch is used for batching mutations to Store.
type WriteBatch interface {
	// SetOne adds a Set operation to the WriteBatch.
	SetOne(key, value []byte)

	// Delete adds a Del operation to the WriteBatch.
	Delete(key []byte)

	// Count returns the size of the WriteBatch.
	Count() int

	// Clear clears the WriteBatch. Good to reuse WriteBatch.
	Clear()

	// Destroy destroys our WriteBatch. It is important to call this especially for RocksDB.
	Destroy()
}

// Iterator is an iterator for Store.
type Iterator interface {
	// Seek to >= key if !reversed. Seek to <= key if reversed. This makes merge iterators simpler.
	Seek(key []byte)

	// SeekToFirst if !reversed. SeekToLast if reversed.
	Rewind()

	// Closes closes an iterator and possibly decreases a reference. It is important to close iterators.
	Close()

	// Next if !reversed. Prev if reversed.
	Next()

	// Valid returns whether the Iterator is still valid.
	Valid() bool

	// ValidForPrefix returns whether the Iterator is valid and its current key has the right prefix.
	ValidForPrefix(prefix []byte) bool

	// Key returns the current key.
	Key() []byte

	// Value returns the current value.
	Value() []byte

	// Err returns the error for the last operation if any.
	Err() error
}
