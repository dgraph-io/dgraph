#!/bin/bash -e

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph

source $DGRAPH_ROOT/contrib/scripts/functions.sh

COMPOSE_FILE=$(dirname $0)/docker-compose.yml
SCHEMA_URL='https://github.com/dgraph-io/benchmarks/blob/master/data/21million.schema?raw=true'
DATA_URL='https://github.com/dgraph-io/benchmarks/blob/master/data/21million.rdf.gz?raw=true'
WGET_CMD="wget -q -O-"

export ALPHA_DATA_DIR=/var/tmp/dgraph-21million

function Info {
    echo -e "INFO: $*"
}

Info "using data directory $ALPHA_DATA_DIR"
mkdir -p $ALPHA_DATA_DIR

Info "bringing up a zero server"
docker-compose -f $COMPOSE_FILE up -d zero1

#Info "bulk loading 21million data set"
#dgraph bulk --out=$ALPHA_DATA_DIR                                \
#            --schema=$HOME/work/benchmarks/data/21million.schema \
#            --files=$HOME/work/benchmarks/data/21million.rdf.gz
#mv $ALPHA_DATA_DIR/0/p $ALPHA_DATA_DIR/p
#rmdir $ALPHA_DATA_DIR/0

Info "bringing up alpha server"
docker-compose -f $COMPOSE_FILE up -d dg1
