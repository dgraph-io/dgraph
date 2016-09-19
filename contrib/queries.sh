#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
	  BUILD=$SRC/build
fi

ROCKSDBDIR=$BUILD/rocksdb-4.9

set -e

# build flags needed for rocksdb
export CGO_CPPFLAGS="-I${ROCKSDBDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR}"
export LD_LIBRARY_PATH="${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

pushd cmd/dgraph &> /dev/null
go build .
# Start dgraph in the background.
./dgraph --m ~/dgraph/m --p ~/dgraph/p --u ~/dgraph/u --mem dmem-"$TRAVIS_COMMIT".prof --cpu dcpu-"$TRAVIS_COMMIT".prof --shutdown true &

pushd $BUILD/benchmarks/throughputtest &> /dev/null
go build . && ./throughputtest --numsec 30 --ip "http://127.0.0.1:8080/query"  --numuser 1000
# shutdown Dgraph server.
curl 127.0.0.1:8080/query -XPOST -d 'SHUTDOWN'
echo "done running throughput test"
