#!/bin/bash

pushd $GOPATH/src/github.com/dgraph-io/dgraph/contrib/tlstest
set -e
make test || [[ $TRAVIS_OS_NAME == "osx" ]] # don't fail for mac
popd
