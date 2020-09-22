#!/usr/bin/env bash

set -e
readonly ME=${0##*/}
readonly DGRAPH_ROOT=$(dirname $0)/..

DOCKER_COMPOSE_FILE=${1:-$DGRAPH_ROOT/dgraph/docker-compose.yml}

source $DGRAPH_ROOT/contrib/scripts/functions.sh

restartCluster $DOCKER_COMPOSE_FILE

tailClusterLogs $DOCKER_COMPOSE_FILE

stopCluster
