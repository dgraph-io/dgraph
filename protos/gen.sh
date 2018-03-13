#!/bin/bash

# You might need to go get -v github.com/gogo/protobuf/...

dgraph_io=${GOPATH-$HOME/go}/src/github.com/dgraph-io
protos=$dgraph_io/dgraph/protos
pushd $protos > /dev/null
protoc -I=/home/pawan/go/src/github.com/dgraph-io/dgo/protos -I=. --gofast_out=plugins=grpc:intern internal.proto
