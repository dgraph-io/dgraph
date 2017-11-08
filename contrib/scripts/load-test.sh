#!/bin/bash

set -e

function finish {
	killall dgraph dgraphzero
	rm -rf $1
}

trap finish EXIT

# Simple end to end test run for all commits.
bash contrib/scripts/simple-e2e.sh $1

bash contrib/scripts/loader.sh $1
bash contrib/scripts/queries.sh $1

bash contrib/scripts/transaction.sh $1
