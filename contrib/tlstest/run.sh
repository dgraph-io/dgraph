#!/bin/bash

pushd $GOPATH/src/github.com/dgraph-io/dgraph/contrib/tlstest
set -e
make test
popd
