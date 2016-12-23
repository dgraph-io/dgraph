#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
	  BUILD=$SRC/build
fi

ROCKSDBDIR=$BUILD/rocksdb-4.11.2
ICUDIR=$BUILD/icu/build

set -e

# build flags needed for rocksdb
export CGO_CPPFLAGS="-I${ROCKSDBDIR}/include -I${ICUDIR}/include"
export CGO_LDFLAGS="-L${ROCKSDBDIR} -L${ICUDIR}/lib"
export LD_LIBRARY_PATH="${ICUDIR}/lib:${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

pushd cmd/dgraph &> /dev/null
go build .
# Start dgraph in the background.
./dgraph --w ~/dgraph/w2 --mem dmem-"$TRAVIS_COMMIT".prof --cpu dcpu-"$TRAVIS_COMMIT".prof &

# Wait for server to start in the background.
until nc -z 127.0.0.1 8080;
do
        sleep 1
done

go test -v ../../contrib/freebase

# shutdown Dgraph server.
if curl 127.0.0.1:8080/admin/shutdown
then
	echo "done running throughput test"
else
	return 0
fi

popd &> /dev/null

# Write top x from memory and cpu profile to a file.
go tool pprof -text dgraph dmem-"$TRAVIS_COMMIT".prof | head -25 > topmem.txt
go tool pprof -svg dgraph dmem-"$TRAVIS_COMMIT".prof > topmem.svg

go tool pprof -text --alloc_space dgraph dmem-"$TRAVIS_COMMIT".prof | head -25 > topmem-alloc.txt
go tool pprof -svg --alloc_space dgraph dmem-"$TRAVIS_COMMIT".prof > topmem-alloc.svg

go tool pprof -text dgraph dcpu-"$TRAVIS_COMMIT".prof | head -25 > topcpu.txt
go tool pprof -svg dgraph dcpu-"$TRAVIS_COMMIT".prof > topcpu.svg

