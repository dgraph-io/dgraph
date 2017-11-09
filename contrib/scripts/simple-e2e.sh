#!/bin/bash

# This file starts the Dgraph server, runs a simple mutation, does a query and checks the response.

SRC="$( cd -P "$( dirname "${BASH_SOURCE[0]}" )" && pwd )/.."

BUILD=$1
# If build variable is empty then we set it.
if [ -z "$1" ]; then
        BUILD=$SRC/build
fi

set -e

source ./contrib/scripts/functions.sh

startZero
start

go test ./contrib/freebase/share_test.go

quit 0
# This is so that count for export which runs later on goldendata matches.
rm -rf $BUILD/w $BUILD/w2 $BUILD/p $BUILD/p2 $BUILD/zw
