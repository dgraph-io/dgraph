#!/bin/bash

# Simple end to end test run for all commits.
bash contrib/simple-e2e.sh $1

# We run the assigner and the loader only when a commit is made on master/release
# branches.
# if [[ $TRAVIS_BRANCH =~ master|release\/ ]] && [ $TRAVIS_EVENT_TYPE = "push" ] ; then
if [ $TRAVIS_EVENT_TYPE = "push" ] ; then
  bash contrib/assign.sh $1
  bash contrib/loader.sh $1
  bash contrib/queries.sh $1
fi

