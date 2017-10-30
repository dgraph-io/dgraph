#!/bin/bash

set -e

DGRAPH_ROOT=$GOPATH/src/github.com/dgraph-io/dgraph/cmd
function build {
  pushd $DGRAPH_ROOT/$1 > /dev/null
  go build .
  popd > /dev/null
}

SERVER=$1
CLIENT=$2
EXPECTED=$3

build "dgraphzero"
build "dgraph"
build "dgraph-live-loader"

$DGRAPH_ROOT/dgraphzero/dgraphzero -w zw &
sleep 5


$SERVER &
echo $CLIENT
timeout 7s $CLIENT &> client.log
RESULT=$?
pkill dgraph > /dev/null 2>&1
rm -rf p w

pkill dgraphzero > /dev/null 2>&1
rm -rf zw

echo "$SERVER <-> $CLIENT: $RESULT (expected: $EXPECTED)"

if [ $RESULT == $EXPECTED ]; then
	exit 0
else
	exit 1
fi
