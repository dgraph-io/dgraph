#!/bin/bash

# "stable" mode tests assume data is static
# "live" mode tests assume data dynamic

SCRIPT=$(basename ${BASH_SOURCE[0]})
TEST=""
QTD=1
SLEEP_TIMEOUT=5
TEST_QTD=3

PORT=7000
RPC_PORT=8540
HOSTNAME="0.0.0.0"
MODE="stable"

declare -a keys=("alice" "bob" "charlie" "dave" "eve" "ferdie" "george" "heather" "ian")

usage() {
  echo "Usage: $SCRIPT"
  echo "Optional command line arguments"
  echo "-t <string>  -- Test to run. eg: rpc"
  echo "-q <number>  -- Quantity of nodes to run. eg: 3"
  echo "-z <number>  -- Quantity of nodes to run tests against eg: 3"
  echo "-s <number>  -- Sleep between operations in secs. eg: 5"
  exit 1
}

while getopts "h?t:q:z:s:" args; do
case $args in
    h|\?)
      usage;
      exit;;
    t ) TEST=${OPTARG};;
    q ) QTD=${OPTARG};;
    z ) TEST_QTD=${OPTARG};;
    s ) SLEEP_TIMEOUT=${OPTARG};;
  esac
done

set -euxo pipefail

BASE_PATH=$(mktemp -d -t gossamer-basepath.XXXXX)

if [[ ! "$BASE_PATH" ]]; then
  echo "Could not create $BASE_PATH"
  exit 1
fi

# Compile gossamer
echo "compiling gossamer"
make build

# PID array declaration
arr=()

start_func() {
  echo "starting gossamer node $i in background ..."
  "$PWD"/bin/gossamer --port=$(($PORT + $i)) --key=${keys[$i-1]} --basepath="$BASE_PATH$i" \
    --rpc --rpchost=$HOSTNAME --rpcport=$(($RPC_PORT + $i)) --roles=1 --rpcmods=system,author,chain >"$BASE_PATH"/node"$i".log 2>&1 & disown

  GOSSAMER_PID=$!
  echo "started gossamer node, pid=$GOSSAMER_PID"
  # add PID to array
  arr+=("$GOSSAMER_PID")
}

# Run node with static blockchain database
# For loop N times
for i in $(seq 1 "$QTD"); do
  start_func "$i"
  echo "sleeping $SLEEP_TIMEOUT seconds for startup"
  sleep "$SLEEP_TIMEOUT"
  echo "done sleeping"
done

echo "sleeping $SLEEP_TIMEOUT seconds before running tests ... "
sleep "$SLEEP_TIMEOUT"
echo "done sleeping"

set +e

if [[ -z $TEST || $TEST == "rpc" ]]; then

  for i in $(seq 1 "$TEST_QTD"); do
    echo "going to test gossamer node $(($RPC_PORT + $i))..."
    MODE=$MODE NETWORK_SIZE=$QTD HOSTNAME=$HOSTNAME PORT=$(($RPC_PORT + $i)) go test ./tests/rpc/... -timeout=60s -v -count=1

    RPC_FAIL=$?
  done

fi

stop_func() {
  GOSSAMER_PID=$i
  echo "shutting down gossamer node, pid=$GOSSAMER_PID ..."

  # Shutdown gossamer node
  kill -9 "$GOSSAMER_PID"
  wait "$GOSSAMER_PID"
}


for i in "${arr[@]}"; do
  stop_func "$i"
done

if [[ (-z $TEST || $TEST == "rpc") && $RPC_FAIL -ne 0 ]]; then
  exit $RPC_FAIL
else
  exit 0
fi
