#!/bin/bash

source contrib/scripts/functions.sh
function run {
  go test -short=true $@ |\
		GREP_COLORS='mt=01;32' egrep --line-buffered --color=always '^ok\ .*|$' |\
		GREP_COLORS='mt=00;38;5;226' egrep --line-buffered --color=always '^\?\ .*|$' |\
		GREP_COLORS='mt=01;31' egrep --line-buffered --color=always '.*FAIL.*|$'
}

function runAll {
  runCluster
  # pushd dgraph/cmd/server
  # go test -v .
  # popd

  for PKG in $(go list ./...|grep -v -E 'vendor|contrib|wiki|customtok'); do
    echo "Running test for $PKG"
    run $PKG
  done
}

# For piped commands return non-zero status if any command
# in the pipe returns a non-zero status
set -o pipefail
echo
echo "Running tests. Ignoring vendor folder."
runAll

echo
echo "Running load-test.sh"
./contrib/scripts/load-test.sh
