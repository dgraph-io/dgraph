#!/bin/bash
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
