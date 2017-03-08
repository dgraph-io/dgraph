#!/bin/bash

SERVER=$1
CLIENT=$2
EXPECTED=$3

ROCKSDBDIR=$HOME/build/rocksdb-5.1.4
ICUDIR=$HOME/build/icu/build
export CGO_LDFLAGS="-L${ROCKSDBDIR}"
export LD_LIBRARY_PATH="${ICUDIR}/lib:${ROCKSDBDIR}:${LD_LIBRARY_PATH}"

$SERVER > /dev/null 2>&1 &
P=$!
timeout 30s $CLIENT > /dev/null 2>&1
RESULT=$?
pkill dgraph > /dev/null 2>&1
rm -rf p w

echo "$SERVER <-> $CLIENT: $RESULT (expected: $EXPECTED)"

if [ $RESULT == $EXPECTED ]; then
	exit 0
else
	exit 1
fi
