#!/bin/bash

# run from directory containing this script
cd ${BASH_SOURCE[0]%/*}

source ./contrib/scripts/functions.sh
function run {
  go test -short=true $@ |\
		GREP_COLORS='mt=01;32' egrep --line-buffered --color=always '^ok\ .*|$' |\
		GREP_COLORS='mt=00;38;5;226' egrep --line-buffered --color=always '^\?\ .*|$' |\
		GREP_COLORS='mt=01;31' egrep --line-buffered --color=always '.*FAIL.*|$'
}

function runDir {
  pushd $1
  run
  popd
}

function runAll {
  local testsFailed=0
  for PKG in $(go list ./...|grep -v -E 'vendor|wiki|customtok|/_'); do
    echo "Running test for $PKG"
    run $PKG || {
        testsFailed=$((testsFailed+1))
    }
  done
  return $testsFailed
}

# For piped commands return non-zero status if any command
# in the pipe returns a non-zero status
set -o pipefail
restartCluster

echo
echo "Running tests. Ignoring vendor folder."
runAll || exit $?

echo
echo "Running load-test.sh"
./contrib/scripts/load-test.sh

# Run non-go tests.
./contrib/scripts/test-bulk-schema.sh

# Run tests requiring different cluster setup.
restartCluster "edgraph/_mutations_mode/docker-compose.yml"
runDir edgraph/_mutations_mode

stopCluster
