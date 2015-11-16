/*

Package rocksdb is a fork of the levigo package with the identifiers changed to target rocksdb and
the package name changed to rocksdb.

This was accomplished by running a sed script over the source code.
Many thanks to Jeff Hodges for creating levigo without which this package would not exist.

Original package documentation follows.

Package rocksdb provides the ability to create and access LevelDB databases.

rocksdb.Open opens and creates databases.

	opts := rocksdb.NewOptions()
	opts.SetCache(rocksdb.NewLRUCache(3<<30))
	opts.SetCreateIfMissing(true)
	db, err := rocksdb.Open("/path/to/db", opts)

The DB struct returned by Open provides DB.Get, DB.Put and DB.Delete to modify
and query the database.

	ro := rocksdb.NewReadOptions()
	wo := rocksdb.NewWriteOptions()
	// if ro and wo are not used again, be sure to Close them.
	data, err := db.Get(ro, []byte("key"))
	...
	err = db.Put(wo, []byte("anotherkey"), data)
	...
	err = db.Delete(wo, []byte("key"))

For bulk reads, use an Iterator. If you want to avoid disturbing your live
traffic while doing the bulk read, be sure to call SetFillCache(false) on the
ReadOptions you use when creating the Iterator.

	ro := rocksdb.NewReadOptions()
	ro.SetFillCache(false)
	it := db.NewIterator(ro)
	defer it.Close()
	it.Seek(mykey)
	for it = it; it.Valid(); it.Next() {
		munge(it.Key(), it.Value())
	}
	if err := it.GetError(); err != nil {
		...
	}

Batched, atomic writes can be performed with a WriteBatch and
DB.Write.

	wb := rocksdb.NewWriteBatch()
	// defer wb.Close or use wb.Clear and reuse.
	wb.Delete([]byte("removed"))
	wb.Put([]byte("added"), []byte("data"))
	wb.Put([]byte("anotheradded"), []byte("more"))
	err := db.Write(wo, wb)

If your working dataset does not fit in memory, you'll want to add a bloom
filter to your database. NewBloomFilter and Options.SetFilterPolicy is what
you want. NewBloomFilter is amount of bits in the filter to use per key in
your database.

	filter := rocksdb.NewBloomFilter(10)
	opts.SetFilterPolicy(filter)
	db, err := rocksdb.Open("/path/to/db", opts)

If you're using a custom comparator in your code, be aware you may have to
make your own filter policy object.

This documentation is not a complete discussion of LevelDB. Please read the
LevelDB documentation <http://code.google.com/p/rocksdb> for information on
its operation. You'll find lots of goodies there.
*/
package rocksdb
