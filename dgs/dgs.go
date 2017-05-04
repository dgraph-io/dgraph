/*

dgs is dgraph store.
*/
package dgs

type Store interface {
	// Returns the value as a byte array.
	// Also returns the deallocate function. This is for freeing RocksDB slice.
	Get(key []byte) ([]byte, func(), error)

	// SetOne adds a key-value to data store.
	SetOne(k []byte, val []byte) error

	// Delete deletes a key from data store.
	Delete(k []byte) error

	// Execute a WriteBatch.
	WriteBatch(wb WriteBatch) error

	Close()
}

type WriteBatch interface {
	Put(key, value []byte)
	Delete(key []byte)
	Count() int
	Clear()
	Destroy()
}
