#!/bin/bash

# This file starts the Dgraph server, runs a simple mutation, does a query and checks the response.

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
        BUILD=$SRC/build
fi

set -e

pushd $BUILD &> /dev/null
benchmark=$(pwd)/benchmarks/data
popd &> /dev/null

pushd cmd/dgraphzero &> /dev/null
echo -e "\nBuilding and running Dgraph Zero."
go build .

./dgraphzero -w $BUILD/wz0 -port 12340 -idx 3 &
popd &> /dev/null

pushd cmd/dgraph &> /dev/null
go build .
./dgraph --p $BUILD/p0 --w $BUILD/w0 --memory_mb 4000 --zero "localhost:12340" &

# Wait for server to start in the background.
until nc -z 127.0.0.1 8080;
do
        sleep 1
done
sleep 5

go test ../../contrib/freebase/share_test.go
go test ../../contrib/freebase/simple_test.go

killall -9 dgraph dgraphzero
popd &> /dev/null
