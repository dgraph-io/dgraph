#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
  BUILD=$SRC/build
fi

set -e

echo "Running transaction tests."

source ./contrib/scripts/functions.sh

startZero

start

contrib=$GOPATH/src/github.com/dgraph-io/dgraph/contrib

echo "bank tests"
go run $contrib/bank/main.go

echo "account upsert tests"
# go run $GOPATH/src/github.com/dgraph-io/dgraph/contrib/acctupsert/main.go

echo "Running sentence swap tests"
go run $contrib/sentenceswap/main.go $contrib/sentenceswap/words.go
