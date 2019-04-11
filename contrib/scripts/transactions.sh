#!/bin/bash

basedir=$GOPATH/src/github.com/dgraph-io/dgraph
contrib=$basedir/contrib
set -e

# go test -v $contrib/integration/testtxn/main_test.go

source $contrib/scripts/functions.sh
restartCluster

echo "*  Running transaction tests."

echo "*  Running bank tests"
go run $contrib/integration/bank/main.go --alpha=localhost:9180,localhost:9182,localhost:9183 --verbose=false

echo "*  Running account upsert tests"
go run $contrib/integration/acctupsert/main.go --alpha=localhost:9180

echo "*  Running sentence swap tests"
pushd $contrib/integration/swap
go build . && ./swap --alpha=localhost:9180
popd

echo "*  Running mutate from #1750."
pushd $contrib/integration/mutates
go build . && ./mutates --add --alpha=localhost:9180
./mutates --alpha=localhost:9180
popd
