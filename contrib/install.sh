#!/bin/bash

set -e

if [[ $TRAVIS_OS_NAME == "osx" ]]; then
  brew update
  brew install jq
fi

# Lets install the dependencies that are not vendored in anymore.
go get -d golang.org/x/net/context
go get -d google.golang.org/grpc/...


