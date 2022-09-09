#!/bin/bash
for i in {1..10}
do
  echo Iteration: $i
  go build .
  ./t -r
  ./t --pkg=$1
  if [ $? -eq 1 ]; then
    echo "FAIL"
    exit 1
  else
    echo "SUCCESS"
  fi
  go clean -testcache
  echo Finish iteration: $i
done