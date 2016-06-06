# gorocksdb, a Go wrapper for RocksDB

[![Build Status](https://travis-ci.org/tecbot/gorocksdb.png)](https://travis-ci.org/tecbot/gorocksdb) [![GoDoc](https://godoc.org/github.com/tecbot/gorocksdb?status.png)](http://godoc.org/github.com/tecbot/gorocksdb)

## Install

There exist two options to install gorocksdb.
You can use either a own shared library or you use the embedded RocksDB version from [CockroachDB](https://github.com/cockroachdb/c-rocksdb).

To install the embedded version (it might take a while):

    go get -tags=embed github.com/tecbot/gorocksdb

If you want to go the way with the shared library you'll need to build
[RocksDB](https://github.com/facebook/rocksdb) before on your machine.
If you built RocksDB you can install gorocksdb now:

    CGO_CFLAGS="-I/path/to/rocksdb/include" \
    CGO_LDFLAGS="-L/path/to/rocksdb -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy" \
      go get github.com/tecbot/gorocksdb