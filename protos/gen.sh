#!/bin/bash

# You might need to go get -v github.com/gogo/protobuf/...

dgraph_io=${GOPATH-$HOME/go}/src/github.com/dgraph-io
protos=$dgraph_io/dgraph/protos
pushd $protos > /dev/null
protoc -I=$dgraph_io/dgo/protos -I=. --gogofaster_out=plugins=grpc,Mapi.proto=github.com/dgraph-io/dgo/protos/api:pb pb.proto
