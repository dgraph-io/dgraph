#!/bin/bash

# Lets install the dependencies that are not vendored in anymore.
go get -d golang.org/x/net/context
go get -d google.golang.org/grpc/...
go get -u github.com/dgraph-io/badger/...


