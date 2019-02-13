#!/bin/bash -e

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph

source $DGRAPH_ROOT/contrib/scripts/functions.sh

COMPOSE_FILE=$(dirname $0)/docker-compose.yml
SCHEMA_URL='https://github.com/dgraph-io/benchmarks/blob/master/data/21million.schema?raw=true'
DATA_URL='https://github.com/dgraph-io/benchmarks/blob/master/data/21million.rdf.gz?raw=true'

function Info {
    echo -e "INFO: $*"
}

function DockerCompose {
    docker-compose -p dgraph -f $COMPOSE_FILE "$@"
}

# TODO check for >24GB of RAM

Info "removing old data (if any)"
DockerCompose down -v

Info "bringing up zero container"
DockerCompose up -d zero1

Info "waiting for it to become leader"
DockerCompose logs -f | grep -q -m1 'became leader'

Info "bulk loading 21million data set"
DockerCompose run dg1 \
    bash -s <<EOF
        /gobin/dgraph bulk --schema=<(curl -LSs $SCHEMA_URL) --files=<(curl -LSs $DATA_URL) \
                       --format=rdf --zero=zero1:5080 --out=/data/dg1/bulk
        mv /data/dg1/bulk/0/p /data/dg1
EOF

Info "bringing up alpha container"
DockerCompose up -d dg1

