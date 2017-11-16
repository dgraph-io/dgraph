#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
  BUILD=$SRC/build
fi
mkdir -p $BUILD

set -e

echo "Running transaction tests."

contrib=$GOPATH/src/github.com/dgraph-io/dgraph/contrib

go test -v $contrib/integration/testtxn/main_test.go

source $contrib/scripts/functions.sh

rm -rf $BUILD/p* $BUILD/w*
startZero

start

echo "\n\nRunning bank tests"
go run $contrib/integration/bank/main.go

echo "\n\nRunning account upsert tests"
go run $GOPATH/src/github.com/dgraph-io/dgraph/contrib/integration/acctupsert/main.go

echo "\n\n Running sentence swap tests"
pushd $contrib/integration/swap
go build . && ./swap
popd

echo "\n\n Running mutate from #1750."
pushd $contrib/integration/mutates
go build . && ./mutates --add
./mutates
popd

quit 0
