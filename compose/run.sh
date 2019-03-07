#!/bin/bash

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph
readonly COMPOSE_FILE="./docker-compose.yml"

if [[ $1 == "-h" || $1 == "--help" ]]; then
    cat <<EOF
usage: ./run.sh [./compose args ...]

description:

    Without arguments, rebuild dgraph and bring up the docker-compose.yml
    config found here.

    With arguments, pass them all to ./compose to create a docker-compose.yml
    file first, then rebuild dgraph and bring up the config.
EOF
    exit 0
fi

function Info {
    echo -e "INFO: $*"
}

#
# MAIN
#

set -e

if [[ $# -gt 0 ]]; then
    Info "creating compose file ..."
    go build compose.go
    ./compose "$@"
fi

if [[ ! -e $COMPOSE_FILE ]]; then
    echo >&2 "$ME: no $COMPOSE_FILE found"
    exit 1
fi

Info "rebuilding dgraph ..."
( cd $DGRAPH_ROOT/dgraph && make install )

# No need to down existings containers, if any.
# The up command handles that automatically

Info "bringing up containers"
docker-compose -p dgraph up --force-recreate --remove-orphans
