#!/bin/bash

basedir=$GOPATH/src/github.com/dgraph-io/dgraph
contrib=$basedir/contrib
set -e

# go test -v $contrib/integration/testtxn/main_test.go

source $contrib/scripts/functions.sh
runCluster

echo "*  Running transaction tests."

echo "*  Running bank tests"
go run $contrib/integration/bank/main.go --addr=localhost:9180

echo "*  Running account upsert tests"
go run $contrib/integration/acctupsert/main.go --addr=localhost:9180

echo "*  Running sentence swap tests"
pushd $contrib/integration/swap
go build . && ./swap --addr=localhost:9180
popd

echo "*  Running mutate from #1750."
pushd $contrib/integration/mutates
go build . && ./mutates --add --addr=localhost:9180
./mutates --addr=localhost:9180
popd
