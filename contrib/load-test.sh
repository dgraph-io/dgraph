#!/bin/bash

set -e

# Simple end to end test run for all commits.
go run ./contrib/freebase/simple_test.go

# We run the assigner and the loader only when a commit is made on master/release
# branches.
# if [[ $TRAVIS_BRANCH =~ master|release\/ ]] && [ $TRAVIS_EVENT_TYPE = "push" ] ; then
  bash contrib/loader.sh $1
  bash contrib/queries.sh $1
# fi

