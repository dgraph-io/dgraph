#!/bin/bash
set -xe

export GO111MODULE="on"
## Step 1:  Build fuzzing targets

## Install go-fuzz
go get -u github.com/dvyukov/go-fuzz/go-fuzz github.com/dvyukov/go-fuzz/go-fuzz-build

## Build a fuzz target which is later used for fuzzitdev for Continuous Fuzzing.
go-fuzz-build -libfuzzer -o parser-fuzz-target.a $GOPATH/src/github.com/dgraph-io/dgraph/gql/
docker run --rm -v $(pwd):/tmp rsmmr/clang clang -fsanitize=fuzzer /tmp/parser-fuzz-target.a -o /tmp/parser-fuzz-target

## Step 2: Perform Fuzzing and local regression on the fuzz target using fuzzit CLI

## Install fuzzit latest version:
wget -O fuzzit https://github.com/fuzzitdev/fuzzit/releases/latest/download/fuzzit_Linux_x86_64
chmod a+x fuzzit

## Create a target on fuzzit servers
./fuzzit create target --skip-if-exists --seed ./gql/fuzz-data/corpus.tar.gz parser-fuzz-target
## Start a job (${1} = [fuzzing][local-regression]). 
./fuzzit create job --type ${1} dgraph-io-gh/parser-fuzz-target parser-fuzz-target

rm -f parser-fuzz-target parser-fuzz-target.a fuzzit
