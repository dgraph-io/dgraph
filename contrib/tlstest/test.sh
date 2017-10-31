#!/bin/bash


killall dgraph dgraphzero > /dev/null 2>&1

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

$DGRAPH_ROOT/dgraphzero/dgraphzero -w zw > /dev/null 2>&1 &
sleep 5


$SERVER > /dev/null 2>&1 &
timeout 30s $CLIENT > client.log 2>&1
RESULT=$?
# echo -e "Result $RESULT"

echo "$SERVER <-> $CLIENT: $RESULT (expected: $EXPECTED)"

if [ $RESULT == $EXPECTED ]; then
	exit 0
else
	exit 1
fi
