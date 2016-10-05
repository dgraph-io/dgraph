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
./dgraph --m ~/dgraph/m --p ~/dgraph/p --cluster "1:localhost:4567" --mem dmem-"$TRAVIS_COMMIT".prof --cpu dcpu-"$TRAVIS_COMMIT".prof --shutdown true &

# Wait for server to start in the background.
until nc -z 127.0.0.1 8080;
do
        sleep 1
done

go test -v ../../contrib/freebase

pushd $BUILD/benchmarks/throughputtest &> /dev/null
go build . && ./throughputtest --numsec 30 --ip "http://127.0.0.1:8080/query"  --numuser 100
# shutdown Dgraph server.
curl 127.0.0.1:8080/query -XPOST -d 'SHUTDOWN'
echo "done running throughput test"
popd &> /dev/null

# Write top x from memory and cpu profile to a file.
go tool pprof -text dgraph dmem-"$TRAVIS_COMMIT".prof | head -25 > topmem.txt
go tool pprof -svg dgraph dmem-"$TRAVIS_COMMIT".prof > topmem.svg

go tool pprof -text --alloc_space dgraph dmem-"$TRAVIS_COMMIT".prof | head -25 > topmem-alloc.txt
go tool pprof -svg --alloc_space dgraph dmem-"$TRAVIS_COMMIT".prof > topmem-alloc.svg

go tool pprof -text dgraph dcpu-"$TRAVIS_COMMIT".prof | head -25 > topcpu.txt
go tool pprof -svg dgraph dcpu-"$TRAVIS_COMMIT".prof > topcpu.svg

