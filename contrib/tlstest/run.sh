#!/bin/bash

pushd $GOPATH/src/github.com/dgraph-io/dgraph/contrib/tlstest
make test
popd
