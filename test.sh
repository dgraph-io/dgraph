#!/bin/bash
#
# usage: test.sh [pkg_regex]

# Notes for testing under macOS (Sierra and up)
# Required Homebrew (https://brew.sh/) packages:
#   - bash
#   - curl
#   - coreutils
#   - gnu-getopt
#   - findutils
#
# Your $PATH must have all required packages in .bashrc:
#   PATH="/usr/local/opt/gnu-getopt/bin:$PATH"
#   PATH="/usr/local/opt/curl/bin:$PATH"
#   PATH="/usr/local/opt/coreutils/libexec/gnubin:$PATH"
#   PATH="/usr/local/opt/findutils/libexec/gnubin:$PATH"
#   export PATH
#
# After brew packages and PATHs are set, run tests with:
#   /usr/local/bin/bash test.sh
#
# Keep in mind that the test build will overwrite the "dgraph"
# binary in your $GOPATH/bin with the Linux-ELF binary for Docker.

set -e
readonly ME=${0##*/}
readonly DGRAPH_ROOT=$(dirname $0)

source $DGRAPH_ROOT/contrib/scripts/functions.sh

PATH+=:$DGRAPH_ROOT/contrib/scripts/
GO_TEST_OPTS=( )
TEST_FAILED=0
TEST_SET="unit"
BUILD_TAGS=

#
# Functions
#

function Usage {
    echo "usage: $ME [opts] [pkg_regex]

options:

    -h --help         output this help message
    -u --unit         run unit tests only
    -c --cluster      run unit tests and custom cluster test
    -C --cluster-only run custom cluster tests only
    -f --full         run all tests (unit, custom cluster, and systest tests)
    -F --systest-only run systest tests only
       --oss          run tests with 'oss' tagging
    -v --verbose      run tests in verbose mode
    -n --no-cache     re-run test even if previous result is in cache
       --short        run tests with -short=true

notes:

    Specifying pkg_regex implies -c."
}

function Info {
    echo -e "\e[1;36mINFO: $*\e[0m"
}

function FmtTime {
    local secs=$(($1 % 60)) min=$(($1 / 60 % 60)) hrs=$(($1 / 60 / 60))

    [[ $hrs -gt 0 ]]               && printf "%dh " $hrs
    [[ $hrs -gt 0 || $min -gt 0 ]] && printf "%dm " $min
                                      printf "%ds" $secs
}

function IsCi {
    [[ ! -z "$TEAMCITY_VERSION" ]]
}

function TestFailed {
    TEST_FAILED=1
    [[ $CURRENT_TEST ]] && echo $CURRENT_TEST >> $FAILED_TESTS
}

function ListFailedTests {
    echo -en "\e[1;31m"
    sed 's/^/  /' $FAILED_TESTS
    echo -en "\e[0m"
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

function FindDefaultClusterTests {
    touch $DEFAULT_CLUSTER_TESTS
    for PKG in $(grep -v -f $CUSTOM_CLUSTER_TESTS $MATCHING_TESTS); do
        echo $PKG >> $DEFAULT_CLUSTER_TESTS
    done
}

function Run {
    set -o pipefail
    echo -en "...\r"
    if IsCi; then
        go test -json -v ${GO_TEST_OPTS[*]} $@
        return
    fi
    go test ${GO_TEST_OPTS[*]} $@ \
    | GREP_COLORS='ne:mt=01;32' egrep --line-buffered --color=always '^ok\ .*|$' \
    | GREP_COLORS='ne:mt=00;38;5;226' egrep --line-buffered --color=always '^\?\ .*|$' \
    | GREP_COLORS='ne:mt=01;31' egrep --line-buffered --color=always '.*FAIL.*|$'
}

function RunCmd {
    CURRENT_TEST=$1
    IsCi && echo "##teamcity[testStarted name='$1' captureStandardOutput='true']"
    if eval "$@"; then
        echo -e "\e[1;32mok $1\e[0m"
        IsCi && echo "##teamcity[testFinished name='$1']"
        return 0
    else
        echo -e "\e[1;31mfail $1\e[0m"
        IsCi && echo "##teamcity[testFailed name='$1']"
        return 1
    fi
}

function RunDefaultClusterTests {
    while read -r PKG; do
        Info "Running test for $PKG"
        CURRENT_TEST=$PKG
        Run $PKG || TestFailed
    done < $DEFAULT_CLUSTER_TESTS
    CURRENT_TEST=
    return $TEST_FAILED
}

function RunCustomClusterTests {
    while read -r LINE; do
        DIR="${LINE:1:-1}"
        CFG="$DIR/docker-compose.yml"
        Info "Running tests in directory $DIR"
        restartCluster $DIR/docker-compose.yml
        pushd $DIR >/dev/null
        CURRENT_TEST=$DIR
        Run || TestFailed
        popd >/dev/null
    done < $CUSTOM_CLUSTER_TESTS
    CURRENT_TEST=
    return $TEST_FAILED
}

#
# MAIN
#

ARGS=$(getopt -n$ME -o"hucCfFvn" \
              -l"help,unit,cluster,cluster-only,full,systest-only,oss,verbose,no-cache,short" -- "$@") \
    || exit 1
eval set -- "$ARGS"
while true; do
    case "$1" in
        -h|--help)         Usage; exit 0                   ;;
        -u|--unit)         TEST_SET="unit"                 ;;
        -c|--cluster)      TEST_SET="unit:cluster"         ;;
        -C|--cluster-only) TEST_SET="cluster"              ;;
        -f|--full)         TEST_SET="unit:cluster:systest" ;;
        -F|--systest-only) TEST_SET="systest"              ;;
        -v|--verbose)      GO_TEST_OPTS+=( "-v" )          ;;
        -n|--no-cache)     GO_TEST_OPTS+=( "-count=1" )    ;;
           --oss)          GO_TEST_OPTS+=( "-tags=oss" )   ;;
           --short)        GO_TEST_OPTS+=( "-short=true" ) ;;
        --)                shift; break                    ;;
    esac
    shift
done

cd $DGRAPH_ROOT

# tests should put temp files under this directory for easier cleanup
export TMPDIR=$(mktemp --tmpdir --directory $ME.tmp-XXXXXX)
trap "rm -rf $TMPDIR" EXIT

# docker-compose files may use this to run as user instead of as root
export UID

MATCHING_TESTS=$TMPDIR/tests
CUSTOM_CLUSTER_TESTS=$TMPDIR/custom
DEFAULT_CLUSTER_TESTS=$TMPDIR/default
FAILED_TESTS=$TMPDIR/failures

if [[ $# -eq 0 ]]; then
    go list ./... > $MATCHING_TESTS
    if [[ $TEST_SET == unit ]]; then
        Info "Running only unit tests"
    fi
elif [[ $# -eq 1 ]]; then
    REGEX=${1%/}
    go list ./... | grep $REGEX > $MATCHING_TESTS
    Info "Running only tests matching '$REGEX'"
    RUN_ALL=
else
    echo >&2 "usage: $ME [pkg_regex]"
    exit 1
fi

# assemble list of tests before executing any
FindCustomClusterTests
FindDefaultClusterTests

# abort all tests on Ctrl-C, not just the current one
trap "echo >&2 SIGINT ; exit 2" SIGINT

START_TIME=$(date +%s)

if [[ :${TEST_SET}: == *:unit:* ]]; then
    if [[ -s $DEFAULT_CLUSTER_TESTS ]]; then
        Info "Running tests using the default cluster"
        restartCluster
        RunDefaultClusterTests || TestFailed
    else
        Info "Skipping default cluster tests because none match"
    fi
fi

if [[ :${TEST_SET}: == *:cluster:* ]]; then
    if [[ -s $CUSTOM_CLUSTER_TESTS ]]; then
        Info "Running tests using custom clusters"
        RunCustomClusterTests || TestFailed
    else
        Info "Skipping custom cluster tests because none match"
    fi
fi

if [[ :${TEST_SET}: == *:systest:* ]]; then
    Info "Running posting size calculation"
    cd posting
    RunCmd ./size_test.sh || TestFailed
    cd ..

    Info "Running small load test"
    RunCmd ./contrib/scripts/load-test.sh || TestFailed

    Info "Running custom test scripts"
    RunCmd ./dgraph/cmd/bulk/systest/test-bulk-schema.sh || TestFailed

    Info "Running large load test"
    RunCmd ./systest/21million/test-21million.sh || TestFailed

    Info "Running rebuilding index test"
    RunCmd ./systest/1million/test-reindex.sh || TestFailed

    Info "Running background index test"
    RunCmd ./systest/bgindex/test-bgindex.sh || TestFailed
fi

Info "Stopping cluster"
stopCluster

END_TIME=$(date +%s)
Info "Tests completed in" $( FmtTime $((END_TIME - START_TIME)) )

if [[ $TEST_FAILED -eq 0 ]]; then
    Info "\e[1;32mAll tests passed!"
else
    Info "\e[1;31m*** One or more tests failed! ***"
    ListFailedTests
fi

exit $TEST_FAILED
