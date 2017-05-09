#!/bin/bash

protos=$GOPATH/src/github.com/dgraph-io/dgraph/protos
pushd $protos > /dev/null
protoc --gofast_out=plugins=grpc:. -I=. *.proto
