#!/bin/bash -e

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph

source $DGRAPH_ROOT/contrib/scripts/functions.sh

COMPOSE_FILE=$(dirname $0)/docker-compose.yml
SCHEMA_URL='https://github.com/dgraph-io/benchmarks/blob/master/data/21million.schema?raw=true'
DATA_URL='https://github.com/dgraph-io/benchmarks/blob/master/data/21million.rdf.gz?raw=true'
WGET_CMD="wget -q -O-"
SRCDIR=$(dirname $0)

function Info {
    echo -e "INFO: $*"
}


Info "entering directory $SRCDIR/"
cd $SRCDIR

Info "bringing up a zero container"
docker-compose up -d --force-recreate zero1

Info "creating alpha container"
docker-compose up -d --force-recreate dg1

Info "bulk loading 21million data set"
docker exec -it bank-dg1 \
    dgraph bulk --schema=<($WGET_CMD $SCHEMA_URL) --files=<($WGET_CMD $DATA_URL) \
                --format=rdf --out=/data/dg1/                                    \
    \&\& mv /data/dg1/bulk/0/p /data/dg1

Info "starting alpha service"
docker-compose start -d dg1
