#!/bin/bash

# Used to install initial set of packages on Travis CI server.

set -ex

# Lets install the dependencies that are not vendored in anymore.
go get -d golang.org/x/net/context
go get -d google.golang.org/grpc

pushd $GOPATH/src/google.golang.org/grpc
  git checkout v1.8.2
popd

