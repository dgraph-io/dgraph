#!/bin/bash
set -xe

## Build fuzzing targets
## go-fuzz doesn't support modules for now, so ensure we do everything
## in the old style GOPATH way
## Uncommenting line 8 has issues with go-fuzz-build below. Commenting for now.
# export GO111MODULE="off"
export GO111MODULE="on"

## Install go-fuzz
go get -u github.com/dvyukov/go-fuzz/go-fuzz github.com/dvyukov/go-fuzz/go-fuzz-build

# download dependencies into ${GOPATH}
# -d : only download (don't install)f
# -v : verbose
# -u : use the latest version
# will be different if you use vendoring or a dependency manager
# like godep
# go get -d -v -u ./...

go-fuzz-build -libfuzzer -o parser-fuzz-target.a $GOPATH/src/github.com/dgraph-io/dgraph/gql/
docker run --rm -v $(pwd):/tmp rsmmr/clang clang -fsanitize=fuzzer /tmp/parser-fuzz-target.a -o /tmp/parser-fuzz-target

## Install fuzzit latest version:
wget -O fuzzit https://github.com/fuzzitdev/fuzzit/releases/latest/download/fuzzit_Linux_x86_64
chmod a+x fuzzit

## upload fuzz target for long fuzz testing on fuzzit.dev server or run locally for regression
./fuzzit create target --skip-if-exists --seed ./gql/fuzz-data/corpus.tar.gz parser-fuzz-target
./fuzzit create job --type ${1} dgraph-io-gh/parser-fuzz-target parser-fuzz-target

rm -f parser-fuzz-target parser-fuzz-target.a fuzzit
