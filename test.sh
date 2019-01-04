#!/bin/bash

# run from directory containing this script
cd ${BASH_SOURCE[0]%/*}

source ./contrib/scripts/functions.sh
function run {
  # check to see if the package has a customized docker-compose file
  dir="$GOPATH/src/$@"
  pushd $dir > /dev/null && \
  if [ -f docker-compose.yml ]; then
    if ls *_test.go 1>/dev/null 2>&1 ; then
      echo "Restarting the cluster using the package local docker-compose.yml"
      restartCluster $dir $(cat testZeroPort.txt) $(cat testAlphaPort.txt) /tmp/dg-acl
    fi
  fi && \
  go test -short=true |\
		GREP_COLORS='mt=01;32' egrep --line-buffered --color=always '^ok\ .*|$' |\
		GREP_COLORS='mt=00;38;5;226' egrep --line-buffered --color=always '^\?\ .*|$' |\
		GREP_COLORS='mt=01;31' egrep --line-buffered --color=always '.*FAIL.*|$' && \
  if [ -f docker-compose.yml ]; then
    if ls *_test.go 1>/dev/null 2>&1 ; then
      stopCluster $dir
    fi
  fi && \
  popd > /dev/null
}

function runAll {
  # pushd dgraph/cmd/server
  # go test -v .
  # popd

  local testsFailed=0
  for PKG in $(go list ./...|grep -v -E 'vendor|wiki|customtok'); do
  #dir=(github.com/dgraph-io/dgraph/ee/acl)
  #for PKG in $dir; do
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
restartCluster $GOPATH/src/github.com/dgraph-io/dgraph/dgraph 6080 9180 /tmp/dg
#echo
echo "Running tests. Ignoring vendor folder."
runAll || exit $?

# Run non-go tests.
./contrib/scripts/test-bulk-schema.sh

echo
echo "Running load-test.sh"
./contrib/scripts/load-test.sh

stopCluster $GOPATH/src/github.com/dgraph-io/dgraph/dgraph
