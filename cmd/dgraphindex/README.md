# Dgraphindex

You run this to create Bleve indices based on a JSON config file.

Here are different ways of running it.
* Simplest way: stop writing to the posting store. Essentially, make Dgraph read-only for a while. Run `dgraphindex`, and let it finish before you allow writing to the posting store again.
* Assume `dgraph` runs all the time. Run `dgraphindex` only on a **snapshot** of the posting store. `dgraph` will keep a mutation log for a few days or so. Sequential access is fast, so no worries about efficiency. Once `dgraphindex` is done, ask `dgraph` to load in the indices. The indices contain timestamps and `dgraph` will automatically catch up with the indexing, using the mutation logs.

```
dgraphindex -postings p -indices i \
-config /home/jchiu/dgraph/sample_index_config.json
```

Comment from @mrjn:
I think dgraph would have to do backfill as well, as a PL snapshot gets applied to it. This binary is mostly for testing purposes, IMO. It won't be useful in live Dgraph as much, because we shouldn't stop Dgraph from applying writes to PLs, just to run indexer.

TODO(jchiu): Fix this in a subsequent PR.