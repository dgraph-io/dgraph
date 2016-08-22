#!/bin/bash

# if [[ $TRAVIS_BRANCH =~ master|release\/ ]] ; then
  bash contrib/assign.sh $1
  bash contrib/loader.sh $1
  bash contrib/queries.sh $1
# fi
