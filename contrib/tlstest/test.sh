#!/bin/bash

killall -9 dgraph

DGRAPH_ROOT=$GOPATH/src/github.com/dgraph-io/dgraph/dgraph
function build {
  pushd $DGRAPH_ROOT > /dev/null
  go build .
  popd > /dev/null
}

SERVER=$1
CLIENT=$2
EXPECTED=$3

build "dgraph"

$DGRAPH_ROOT/dgraph zero -w zw -o 1 > zero.log 2>&1 &
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
