package store

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
	// Seek to >= key if !reversed. Seek to <= key if reversed. This makes merge iterators simpler.
	Seek(key []byte)

	// SeekToFirst if !reversed. SeekToLast if reversed.
	Rewind()
	Close()

	// Next if !reversed. Prev if reversed.
	Next()

	Valid() bool
	ValidForPrefix(prefix []byte) bool
	Key() []byte
	Value() []byte
	Err() error
}
