package rdb

// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"
import (
	"bytes"
	"errors"
	"unsafe"
)

// Iterator provides a way to seek to specific keys and iterate through
// the keyspace from that point, as well as access the values of those keys.
//
// For example:
//
//  it := rdb.NewIterator(readOpts)
//  defer it.Close()
//
//  it.Seek([]byte("foo"))
//		for ; it.Valid(); it.Next() {
//    fmt.Printf("Key: %v Value: %v\n", it.Key().Data(), it.Value().Data())
//  }
//
//  if err := it.Err(); err != nil {
//    return err
//  }
type Iterator struct {
	c        *C.rdb_iterator_t
	reversed bool
}

// NewNativeIterator creates a Iterator object.
func NewNativeIterator(c unsafe.Pointer, reversed bool) *Iterator {
	return &Iterator{
		c:        (*C.rdb_iterator_t)(c),
		reversed: reversed,
	}
}

// Valid returns false only when an Iterator has iterated past either the
// first or the last key in the database.
func (iter *Iterator) Valid() bool {
	return C.rdb_iter_valid(iter.c) != 0
}

// ValidForPrefix returns false only when an Iterator has iterated past the
// first or the last key in the database or the specified prefix.
func (iter *Iterator) ValidForPrefix(prefix []byte) bool {
	return C.rdb_iter_valid(iter.c) != 0 && bytes.HasPrefix(iter.Key(), prefix)
}

// Key returns the key the iterator currently holds.
func (iter *Iterator) Key() []byte {
	var cLen C.size_t
	cKey := C.rdb_iter_key(iter.c, &cLen)
	if cKey == nil {
		return nil
	}
	slice := &Slice{cKey, cLen, true}
	return slice.Data()
}

// Value returns the value in the database the iterator currently holds.
func (iter *Iterator) Value() []byte {
	var cLen C.size_t
	cVal := C.rdb_iter_value(iter.c, &cLen)
	if cVal == nil {
		return nil
	}
	slice := &Slice{cVal, cLen, true}
	return slice.Data()
}

// Next moves the iterator to the next sequential key in the database.
func (iter *Iterator) Next() {
	if !iter.reversed {
		C.rdb_iter_next(iter.c)
	} else {
		C.rdb_iter_prev(iter.c)
	}
}

// Rewind either seeks to first or last.
func (iter *Iterator) Rewind() {
	if !iter.reversed {
		C.rdb_iter_seek_to_first(iter.c)
	} else {
		C.rdb_iter_seek_to_last(iter.c)
	}
}

// Seek moves the iterator to the position >= key.
func (iter *Iterator) Seek(key []byte) {
	cKey := byteToChar(key)
	C.rdb_iter_seek(iter.c, cKey, C.size_t(len(key)))
}

// SeekForPrev moves the iterator to the position <= key.
func (iter *Iterator) SeekForPrev(key []byte) {
	cKey := byteToChar(key)
	C.rdb_iter_seek_for_prev(iter.c, cKey, C.size_t(len(key)))
}

// Err returns nil if no errors happened during iteration, or the actual
// error otherwise.
func (iter *Iterator) Err() error {
	var cErr *C.char
	C.rdb_iter_get_error(iter.c, &cErr)
	if cErr != nil {
		defer C.free(unsafe.Pointer(cErr))
		return errors.New(C.GoString(cErr))
	}
	return nil
}

// Close closes the iterator.
func (iter *Iterator) Close() {
	C.rdb_iter_destroy(iter.c)
	iter.c = nil
}
