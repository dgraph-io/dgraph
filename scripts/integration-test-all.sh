#!/bin/bash

# "stable" mode tests assume data is static
# "live" mode tests assume data dynamic


SCRIPT=`basename ${BASH_SOURCE[0]}`
TEST=""

PORT="7001"
RPC_PORT="8545"
IP_ADDR="0.0.0.0"
HOST_RPC="http://$IP_ADDR:$RPC_PORT"
MODE="stable"

KEY="alice"

usage () {
  echo "Usage: $SCRIPT"
  echo "Optional command line arguments"
  echo "-t <string>  -- Test to run. eg: rpc"
  exit 1
}

while getopts "h?t" args; do
case $args in
    h|\?)
        usage;
        exit;;
    t ) TEST=${OPTARG};;
  esac
done

set -euxo pipefail

DATA_DIR=$(mktemp -d -t gossamer-datadir.XXXXX)

if [[ ! "$DATA_DIR" ]]; then
  echo "Could not create $DATA_DIR"
  exit 1
fi


# Compile gossamer
echo "compiling gossamer"
make build


# Run node with static blockchain database
echo "starting gossamer node in background with http listener on $HOST_RPC"

$PWD/bin/gossamer --port=$PORT --key=$KEY --datadir=$DATA_DIR \
                  --rpc --rpchost=$IP_ADDR --rpcport=$RPC_PORT --rpcmods=system,author &

GOSSAMER_PID=$!

echo "gossamer node pid=$GOSSAMER_PID"

echo "sleeping for startup"
sleep 5
echo "done sleeping"

set +e


if [[ -z $TEST || $TEST = "rpc" ]]; then

GOSSAMER_INTEGRATION_TEST_MODE=$MODE GOSSAMER_NODE_HOST=$HOST_RPC go test ./tests/rpc/... -timeout=60s -v

RPC_FAIL=$?

fi


echo "shutting down gossamer node"

# Shutdown gossamer node
kill -9 $GOSSAMER_PID
wait $GOSSAMER_PID


if [[ (-z $TEST || $TEST = "rpc") && $RPC_FAIL -ne 0 ]]; then
  exit $RPC_FAIL
else
  exit 0
fi
