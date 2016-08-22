#!/bin/bash

# We run the assigner and the loader only when a commit or PR is made against
# master/release branches.
if [[ $TRAVIS_BRANCH =~ master|release\/ ]] ; then
  bash contrib/assign.sh $1
  bash contrib/loader.sh $1
  bash contrib/queries.sh $1
fi
