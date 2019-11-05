#!/bin/bash

pushd $(dirname "${BASH_SOURCE[0]}")/queries &> /dev/null

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
    N=`curl -s -H 'Content-Type: application/graphql+-' localhost:8180/query -XPOST -d @${X}.in`
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
    exit 1
  else
    echo -e "Index test passed: ${X}\n"
  fi
}

echo -e "Running some queries and checking count of results returned."
run_index_test basic name 138677
run_index_test allof_the name 25432
run_index_test allof_the_a name 368
run_index_test allof_the_first name 4384
run_index_test releasedate release_date 137859
run_index_test releasedate_sort release_date 137859
run_index_test releasedate_sort_first_offset release_date 2316
run_index_test releasedate_geq release_date 60992
run_index_test gen_anyof_good_bad name 1104

popd &> /dev/null

