#!/bin/bash

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
	  BUILD=$SRC/build
fi

if [[ "${TRAVIS_OS_NAME}" == "osx" ]]; then
  export LIBRARY_PATH=$LIBRARY_PATH:/usr/local/lib;
  export C_INCLUDE_PATH=$C_INCLUDE_PATH:/usr/local/include;
else
  ROCKSDBDIR=$BUILD/rocksdb-5.1.4

  # build flags needed for rocksdb
  export CGO_CPPFLAGS="-I${ROCKSDBDIR}/include"
  export CGO_LDFLAGS="-L${ROCKSDBDIR}"
  export LD_LIBRARY_PATH="${ROCKSDBDIR}:${LD_LIBRARY_PATH}"
fi

set -e

pushd cmd/dgraph &> /dev/null
go build .
# Start dgraph in the background.
./dgraph -w ~/dgraph/w2 -p $BUILD/p &

# Wait for server to start in the background.
until nc -z 127.0.0.1 8080;
do
        sleep 1
done

go test -v ../../contrib/freebase

killall dgraph

popd &> /dev/null
