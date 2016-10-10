#!/bin/bash

# This file starts the Dgraph server, runs a simple mutation, does a query and checks the response.

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
        BUILD=$SRC/build
fi

ROCKSDBDIR=$BUILD/rocksdb-4.9
ICUDIR=$BUILD/icu/build

set -e

pushd $BUILD &> /dev/null
benchmark=$(pwd)/benchmarks/data
popd &> /dev/null

# build flags needed for rocksdb

export CGO_CPPFLAGS="-I${ROCKSDBDIR}/include -I${ICUDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR} -L${ICUDIR}/lib"
export LD_LIBRARY_PATH="${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

pushd cmd/dgraph &> /dev/null
go build .
./dgraph --p ~/dgraph/p0 --m ~/dgraph/m0 --cluster "1:localhost:4567" &

# Wait for server to start in the background.
until nc -z 127.0.0.1 8080;
do
        sleep 1
done

go test -v ../../contrib/freebase/simple_test.go

killall dgraph
popd &> /dev/null
