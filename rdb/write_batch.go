package rdb

// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"

// WriteBatch is a batching of Puts, Merges and Deletes.
type WriteBatch struct {
	c *C.rdb_writebatch_t
}

// NewWriteBatch create a WriteBatch object.
func NewWriteBatch() *WriteBatch {
	return NewNativeWriteBatch(C.rdb_writebatch_create())
}

// NewNativeWriteBatch create a WriteBatch object.
func NewNativeWriteBatch(c *C.rdb_writebatch_t) *WriteBatch {
	return &WriteBatch{c}
}

// WriteBatchFrom creates a write batch from a serialized WriteBatch.
func WriteBatchFrom(data []byte) *WriteBatch {
	return NewNativeWriteBatch(C.rdb_writebatch_create_from(byteToChar(data), C.size_t(len(data))))
}

// Put queues a key-value pair.
func (wb *WriteBatch) Put(key, value []byte) {
	cKey := byteToChar(key)
	cValue := byteToChar(value)
	C.rdb_writebatch_put(wb.c, cKey, C.size_t(len(key)), cValue, C.size_t(len(value)))
}

// Delete queues a deletion of the data at key.
func (wb *WriteBatch) Delete(key []byte) {
	cKey := byteToChar(key)
	C.rdb_writebatch_delete(wb.c, cKey, C.size_t(len(key)))
}

// Count returns the number of updates in the batch.
func (wb *WriteBatch) Count() int {
	return int(C.rdb_writebatch_count(wb.c))
}

// Clear removes all the enqueued Put and Deletes.
func (wb *WriteBatch) Clear() {
	C.rdb_writebatch_clear(wb.c)
}

// Destroy deallocates the WriteBatch object.
func (wb *WriteBatch) Destroy() {
	C.rdb_writebatch_destroy(wb.c)
	wb.c = nil
}
