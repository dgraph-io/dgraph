#!/bin/bash
#
# usage: test.sh [-v] [pkg_regex]

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph

source $DGRAPH_ROOT/contrib/scripts/functions.sh

PATH+=:$DGRAPH_ROOT/contrib/scripts/
GO_TEST_OPTS=( "-short=true" )
TEST_FAILED=0
RUN_ALL=yes

#
# Functions
#

function Usage {
    echo "usage: $ME [opts] [pkg_regex]

options:

    -h --help   output this help message
    -c          run code tests only and skip integration tests
    -v          run tests in verbose mode

notes:

    Specifying pkg_regex implies -c.

    Tests are always run with -short=true."
}


function Info {
    echo -e "\e[1;36mINFO: $*\e[0m"
}

function FindCustomClusterTests {
    # look for directories containing a docker compose and *_test.go files
    touch $CUSTOM_CLUSTER_TESTS
    for FILE in $(find -type f -name docker-compose.yml); do
        DIR=$(dirname $FILE)
        if grep -q $DIR $MATCHING_TESTS && ls $DIR | grep -q "_test.go$"; then
            echo "${DIR:1}\$" >> $CUSTOM_CLUSTER_TESTS
        fi
    done
}

function Run {
    set -o pipefail
    go test ${GO_TEST_OPTS[*]} $@ \
    | GREP_COLORS='mt=01;32' egrep --line-buffered --color=always '^ok\ .*|$' \
    | GREP_COLORS='mt=00;38;5;226' egrep --line-buffered --color=always '^\?\ .*|$' \
    | GREP_COLORS='mt=01;31' egrep --line-buffered --color=always '.*FAIL.*|$'
}

function RunDefaultClusterTests {
    for PKG in $(grep -v -f $CUSTOM_CLUSTER_TESTS $MATCHING_TESTS); do
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


ARGS=$(/usr/bin/getopt -n$ME -o"vhc" -l"help,code-tests" -- "$@") || exit 1
eval set -- "$ARGS"
while true; do
    case "$1" in
        -v)         GO_TEST_OPTS+=( "-v" )                   ;;
        -c)         RUN_ALL=; Info "Running only code tests" ;;
        -h|--help)  Usage; exit 0                            ;;
        --)         shift; break                             ;;
    esac
    shift
done

cd $DGRAPH_ROOT

TMP_DIR=$(mktemp --tmpdir --directory $ME.tmp-XXXXXX)
MATCHING_TESTS=$TMP_DIR/tests
CUSTOM_CLUSTER_TESTS=$TMP_DIR/custom
trap "rm -rf $TMP_DIR" EXIT

if [[ $# -eq 0 ]]; then
    go list ./... > $MATCHING_TESTS
elif [[ $# -eq 1 ]]; then
    go list ./... | grep $1 > $MATCHING_TESTS
    Info "Running only tests matching '$1'"
    RUN_ALL=
else
    echo >&2 "usage: $ME [regex]"
    exit 1
fi

FindCustomClusterTests

Info "Running tests using the default cluster"
restartCluster
RunDefaultClusterTests || TEST_FAILED=1

Info "Running tests using custom clusters"
RunCustomClusterTests || TEST_FAILED=1

if [[ $RUN_ALL ]]; then
    Info "Running load-test.sh"
    ./contrib/scripts/load-test.sh

    Info "Running custom test scripts"
    ./contrib/scripts/test-backup-restore.sh
    ./dgraph/cmd/bulk/systest/test-bulk-schema.sh
fi

Info "Stopping cluster"
stopCluster

if [[ $TEST_FAILED -eq 0 ]]; then
    Info "\e[1;32mAll tests passed!"
else
    Info "\e[1;31m*** One or more tests failed! ***"
fi

exit $TEST_FAILED
