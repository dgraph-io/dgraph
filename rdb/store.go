package rdb

import (
	"strconv"

	"github.com/dgraph-io/dgraph/dgs"
	"github.com/dgraph-io/dgraph/x"
)

// Store contains some handles to RocksDB.
type Store struct {
	db       *DB
	opt      *Options // Contains blockopt.
	blockopt *BlockBasedTableOptions
	ropt     *ReadOptions
	wopt     *WriteOptions
}

func (s *Store) setOpts() {
	s.opt = NewDefaultOptions()
	s.blockopt = NewDefaultBlockBasedTableOptions()
	s.opt.SetBlockBasedTableFactory(s.blockopt)

	// If you want to access blockopt.blockCache, you need to grab handles to them
	// as well. Otherwise, they will be nil. However, for now, we do not really need
	// to do this.
	// s.blockopt.SetBlockCache(rocksdb.NewLRUCache(blockCacheSize))
	// s.blockopt.SetBlockCacheCompressed(rocksdb.NewLRUCache(blockCacheSize))

	s.opt.SetCreateIfMissing(true)
	fp := NewBloomFilter(16)
	s.blockopt.SetFilterPolicy(fp)

	s.ropt = NewDefaultReadOptions()
	s.wopt = NewDefaultWriteOptions()
	s.wopt.SetSync(false) // We don't need to do synchronous writes.
}

// NewStore constructs a Store object at filepath, given some options.
func NewStore(filepath string) (*Store, error) {
	s := &Store{}
	s.setOpts()
	var err error
	s.db, err = OpenDb(s.opt, filepath)
	return s, x.Wrap(err)
}

func NewSyncStore(filepath string) (*Store, error) {
	s := &Store{}
	s.setOpts()
	s.wopt.SetSync(true) // Do synchronous writes.
	var err error
	s.db, err = OpenDb(s.opt, filepath)
	return s, x.Wrap(err)
}

// NewReadOnlyStore constructs a readonly Store object at filepath, given options.
func NewReadOnlyStore(filepath string) (*Store, error) {
	s := &Store{}
	s.setOpts()
	var err error
	s.db, err = OpenDbForReadOnly(s.opt, filepath, false)
	return s, x.Wrap(err)
}

// Get returns the value given a key for RocksDB.
func (s *Store) Get(key []byte) ([]byte, func(), error) {
	valSlice, err := s.db.Get(s.ropt, key)
	if err != nil {
		return nil, func() {}, x.Wrapf(err, "Key: %v", key)
	}
	return valSlice.Data(), func() { valSlice.Free() }, nil
}

// SetOne adds a key-value to data store.
func (s *Store) SetOne(k []byte, val []byte) error { return s.db.Put(s.wopt, k, val) }

// Delete deletes a key from data store.
func (s *Store) Delete(k []byte) error { return s.db.Delete(s.wopt, k) }

// NewIterator initializes a new iterator and returns it.
func (s *Store) NewIterator(reversed bool) dgs.Iterator {
	ro := NewDefaultReadOptions()
	// SetFillCache should be set to false for bulk reads to avoid caching data
	// while doing bulk scans.
	ro.SetFillCache(false)
	return s.db.NewIterator(ro, reversed)
}

// Close closes our data store.
func (s *Store) Close() { s.db.Close() }

// Memtable returns the memtable size.
func (s *Store) MemtableSize() uint64 {
	memTableSize, _ := strconv.ParseUint(s.db.GetProperty("rocksdb.cur-size-all-mem-tables"), 10, 64)
	return memTableSize
}

// IndexFilterblockSize returns the filter block size.
func (s *Store) IndexFilterblockSize() uint64 {
	blockSize, _ := strconv.ParseUint(s.db.GetProperty("rocksdb.estimate-table-readers-mem"), 10, 64)
	return blockSize
}

// NewWriteBatch creates a new WriteBatch object and returns a pointer to it.
func (s *Store) NewWriteBatch() dgs.WriteBatch {
	return NewWriteBatch()
}

// WriteBatch does a batch write to RocksDB from the data in WriteBatch object.
func (s *Store) WriteBatch(wb dgs.WriteBatch) error {
	return x.Wrap(s.db.Write(s.wopt, wb.(*WriteBatch)))
}

// NewCheckpoint creates new checkpoint from current store.
func (s *Store) NewCheckpoint() (*Checkpoint, error) { return s.db.NewCheckpoint() }

// NewSnapshot creates new snapshot from current store.
func (s *Store) NewSnapshot() *Snapshot { return s.db.NewSnapshot() }

// SetSnapshot updates default read options to use the given snapshot.
func (s *Store) SetSnapshot(snapshot *Snapshot) { s.ropt.SetSnapshot(snapshot) }

// GetStats returns stats of our data store.
func (s *Store) GetStats() string { return s.db.GetStats() }
