#!/bin/bash

# You might need to go get -v github.com/gogo/protobuf/...

protos=${GOPATH-$HOME/go}/src/github.com/dgraph-io/dgraph/protos
pushd $protos > /dev/null
protoc --gofast_out=plugins=grpc:api -I=. api.proto
protoc --gofast_out=plugins=grpc,Mapi.proto=github.com/dgraph-io/dgraph/protos/api:intern -I=. internal.proto
