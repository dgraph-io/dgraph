#!/bin/bash

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph

source $DGRAPH_ROOT/contrib/scripts/functions.sh

PATH+=:$DGRAPH_ROOT/contrib/scripts/
CUSTOM_CLUSTER_TEST=()

function Info {
    echo -e "INFO: $*"
}

function Run {
    go test -short=true $@ \
    | GREP_COLORS='mt=01;32' egrep --line-buffered --color=always '^ok\ .*|$' \
    | GREP_COLORS='mt=00;38;5;226' egrep --line-buffered --color=always '^\?\ .*|$' \
    | GREP_COLORS='mt=01;31' egrep --line-buffered --color=always '.*FAIL.*|$'
}

function RunPackageTests {
    for PKG in $(go list ./...); do
        Info "Running test for $PKG"
        Run $PKG
    done
}

cd $DGRAPH_ROOT

Info "Running tests using the default cluster"
restartCluster
RunPackageTests
