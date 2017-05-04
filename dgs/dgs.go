/*

dgs is dgraph store.
*/
package dgs

type Store interface {
	// Returns the value as a byte array.
	// Also returns the deallocate function. This is for freeing RocksDB slice.
	Get(key []byte) ([]byte, func(), error)

	WriteBatch(wb WriteBatch) error
	SetOne(k []byte, val []byte) error
	Delete(k []byte) error
	Close()
	NewWriteBatch() WriteBatch
	NewIterator(reversed bool) Iterator
	GetStats() string
}

type WriteBatch interface {
	SetOne(key, value []byte)
	Delete(key []byte)
	Count() int
	Clear()
	Destroy()
}

type Iterator interface {
	Rewind()
	Seek(key []byte)
	SeekForPrev(key []byte)
	Close()
	Next()
	Valid() bool
	ValidForPrefix(prefix []byte) bool
	Key() []byte
	Value() []byte
	Err() error
}
