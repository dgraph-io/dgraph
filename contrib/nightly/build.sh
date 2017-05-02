#!/bin/bash

set -e

go get -u golang.org/x/net/context
go get -u golang.org/x/text/unicode/norm
go get -u google.golang.org/grpc

pushd contrib/releases &> /dev/null

# Building embedded binaries.
./build.sh

ls
popd &> /dev/null
