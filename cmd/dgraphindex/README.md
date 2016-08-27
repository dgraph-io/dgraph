# Dgraphindex

You run this to create Bleve indices based on a JSON config file.

Here are different ways of running it.
* Simplest way: stop writing to the posting store. Essentially, make Dgraph read-only for a while. Run `dgraphindex`, and let it finish before you allow writing to the posting store again.
* Assume `dgraph` runs all the time. Run `dgraphindex` only on a **snapshot** of the posting store. `dgraph` will keep a mutation log for a few days or so. Sequential access is fast, so no worries about efficiency. Once `dgraphindex` is done, ask `dgraph` to load in the indices. The indices contain timestamps and `dgraph` will automatically catch up with the indexing, using the mutation logs.


./dgraphindex -postings /home/jchiu/dgraph/p \
-config /home/jchiu/dgraph/sample_index_config.json \
-indices /home/jchiu/dgraph/i