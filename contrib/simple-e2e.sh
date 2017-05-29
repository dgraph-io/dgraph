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

pushd cmd/dgraph &> /dev/null
go build .
./dgraph --p ~/dgraph/p0 --w ~/dgraph/w0 &

# Wait for server to start in the background.
until nc -z 127.0.0.1 8080;
do
        sleep 1
done

go test ../../contrib/freebase/simple_test.go

killall dgraph
popd &> /dev/null
