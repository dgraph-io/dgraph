#!/bin/bash

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph

PATH+=:$DGRAPH_ROOT/contrib/scripts/
CUSTOM_CLUSTER_TEST=()

function Info {
    echo -e "INFO: $*"
}

function Run {
    go test -short=true $@ |\
        GREP_COLORS='mt=01;32' egrep --line-buffered --color=always '^ok\ .*|$' |\
        GREP_COLORS='mt=00;38;5;226' egrep --line-buffered --color=always '^\?\ .*|$' |\
        GREP_COLORS='mt=01;31' egrep --line-buffered --color=always '.*FAIL.*|$'
}

function RunDefaultTests {
    for PKG in $(go list ./...); do
        DIR=$(echo $PKG | cut -d/ -f4-)
        Info "PKG=$PKG  DIR=$DIR"
        if [[ -e $DIR/docker-compose.yml ]]; then
            Info "Skipping $PKG because it requires custom cluster"
        else
            Info "Running test for $PKG"
            Run $PKG
        fi
    done
}

cd $DGRAPH_ROOT

Info "Running tests using the default cluster"
#cluster.sh -b restart
#sleep 5
RunDefaultTests
