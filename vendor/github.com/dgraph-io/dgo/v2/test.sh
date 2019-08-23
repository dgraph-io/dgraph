#!/usr/bin/env bash

if [ -z $GOPATH ]; then
    echo "Error: the GOPATH environment variable is not set"; exit 1
fi

source $GOPATH/src/github.com/dgraph-io/dgraph/contrib/scripts/functions.sh
function run {
  go test -short=true $@ |\
		GREP_COLORS='mt=01;32' egrep --line-buffered --color=always '^ok\ .*|$' |\
		GREP_COLORS='mt=00;38;5;226' egrep --line-buffered --color=always '^\?\ .*|$' |\
		GREP_COLORS='mt=01;31' egrep --line-buffered --color=always '.*FAIL.*|$'
}

function runAll {
  # pushd dgraph/cmd/server
  # go test -v .
  # popd

  local testsFailed=0
  for PKG in $(go list ./...); do
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
echo "Running tests. Ignoring vendor folder."
runAll || exit $?
stopCluster



