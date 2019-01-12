#!/bin/bash

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph

source $DGRAPH_ROOT/contrib/scripts/functions.sh

PATH+=:$DGRAPH_ROOT/contrib/scripts/
TEST_FAILED=0

CUSTOM_CLUSTER_TESTS=$(mktemp --tmpdir $ME.tmp-XXXXXX)
trap "rm -rf $CUSTOM_CLUSTER_TESTS" EXIT

function Info {
    echo -e "\e[1;36mINFO: $*\e[0m"
}

function FindCustomClusterTests {
    # look for directories containing a docker compose and *_test.go files
    for FILE in $(find -type f -name docker-compose.yml); do
        DIR=$(dirname $FILE)
        if [[ $(ls $DIR/*_test.go 2>/dev/null | wc -l) -gt 0 ]]; then
            echo "${DIR:1}\$" >> $CUSTOM_CLUSTER_TESTS
        fi
    done
}

function Run {
    set -o pipefail
    go test -v -short=true $@ \
    | GREP_COLORS='mt=01;32' egrep --line-buffered --color=always '^ok\ .*|$' \
    | GREP_COLORS='mt=00;38;5;226' egrep --line-buffered --color=always '^\?\ .*|$' \
    | GREP_COLORS='mt=01;31' egrep --line-buffered --color=always '.*FAIL.*|$'
}

function RunDefaultClusterTests {
    for PKG in $(go list ./... | grep -v -f $CUSTOM_CLUSTER_TESTS); do
        Info "Running test for $PKG"
        Run $PKG || TEST_FAILED=1
    done
    return $TEST_FAILED
}

function RunCustomClusterTests {
    while read -r LINE; do
        DIR="${LINE:1:-1}"
        CFG="$DIR/docker-compose.yml"
        Info "Running tests in directory $DIR"
        restartCluster $DIR/docker-compose.yml
        pushd $DIR >/dev/null
        Run || TEST_FAILED=1
        popd >/dev/null
    done < $CUSTOM_CLUSTER_TESTS
    return $TEST_FAILED
}

#
# MAIN
#

cd $DGRAPH_ROOT
FindCustomClusterTests

Info "Running tests using the default cluster"
restartCluster
RunDefaultClusterTests || TEST_FAILED=1

Info "Running load-test.sh"
./contrib/scripts/load-test.sh

Info "Running tests using custom clusters"
RunCustomClusterTests || TEST_FAILED=1

Info "Running custom test scripts"
./contrib/scripts/test-backup-restore.sh
./dgraph/cmd/bulk/systest/test-bulk-schema.sh

Info "Stopping cluster"
stopCluster

if [[ $TEST_FAILED -eq 0 ]]; then
    Info "\e[1;32mAll tests passed!"
else
    Info "\e[1;31m*** One or more tests failed! ***"
fi

exit $TEST_FAILED
