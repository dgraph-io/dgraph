#!/bin/bash

# We run the assigner and the loader only when a commit is made on master/release
# branches.
if [[ $TRAVIS_BRANCH =~ master|release\/ ]] ; then
  bash contrib/assign.sh $1
  bash contrib/loader.sh $1
  bash contrib/queries.sh $1
fi
