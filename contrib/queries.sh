#!/bin/bash
set -e

source ./contrib/functions.sh

BUILD=$1

pushd cmd/dgraphzero &> /dev/null
echo -e "\nBuilding and running Dgraph Zero."
go build .

startZero
popd &> /dev/null

pushd cmd/dgraph &> /dev/null
go build .
# Start dgraph in the background.
start
sleep 5

# Wait for server to start in the background.
until nc -z 127.0.0.1 8080;
do
        sleep 1
done

go test -v ../../contrib/freebase

quit 0

popd &> /dev/null
