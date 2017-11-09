#!/bin/bash

source $GOPATH/src/github.com/dgraph-io/dgraph/contrib/scripts/functions.sh

pushd $GOPATH/src/github.com/dgraph-io/dgraph/contrib/indextest &> /dev/null

function run_index_test {
  local max_attempts=${ATTEMPTS-5}
  local timeout=${TIMEOUT-1}
  local attempt=0
  local exitCode=0

  X=$1
  GREPFOR=$2
  ANS=$3
  echo "Running test: ${X}"
  while (( $attempt < $max_attempts ))
  do
    set +e
    N=`curl -s localhost:8080/query -XPOST -d @${X}.in`
    exitCode=$?

    set -e

    if [[ $exitCode == 0 ]]
    then
      break
    fi

    echo "Failure! Retrying in $timeout.." 1>&2
    sleep $timeout
    attempt=$(( attempt + 1 ))
    timeout=$(( timeout * 2 ))
  done

  NUM=$(echo $N | python -m json.tool | grep $GREPFOR | wc -l)
  if [[ ! "$NUM" -eq "$ANS" ]]; then
    echo "Index test failed: ${X}  Expected: $ANS  Got: $NUM"
    quit 1
  else
    echo -e "Index test passed: ${X}\n"
  fi
}

echo -e "Running some queries and checking count of results returned."
run_index_test basic name 138676
run_index_test allof_the name 25431
run_index_test allof_the_a name 367
run_index_test allof_the_first name 4383
run_index_test releasedate release_date 137858
run_index_test releasedate_sort release_date 137858
run_index_test releasedate_sort_first_offset release_date 2315
run_index_test releasedate_geq release_date 60991
run_index_test gen_anyof_good_bad name 1103

popd &> /dev/null

