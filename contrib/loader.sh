#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
  BUILD=$SRC/build
fi

ROCKSDBDIR=$BUILD/rocksdb-4.9

set -e

pushd $BUILD &> /dev/null
benchmark=$(pwd)/benchmarks/data
popd &> /dev/null

# build flags needed for rocksdb
export CGO_CPPFLAGS="-I${ROCKSDBDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR}"
export LD_LIBRARY_PATH="${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

pushd cmd/dgraphloader &> /dev/null
go build .
./dgraphloader --num 1 --idx 0 --rdfgzips $benchmark/actor-director.gz --u ~/dgraph/u --p ~/dgraph/p --stw_ram_mb 3000 --cores 1
popd &> /dev/null
