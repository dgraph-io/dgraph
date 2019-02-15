#!/bin/bash -e

readonly ME=${0##*/}
readonly SRCDIR=$(dirname $0)

COMPOSE_FILE=$SRCDIR/docker-compose.yml
QUERY_DIR=$SRCDIR/queries
BENCHMARKS_REPO="https://github.com/dgraph-io/benchmarks"
SCHEMA_URL="$BENCHMARKS_REPO/blob/master/data/21million.schema?raw=true"
DATA_URL="$BENCHMARKS_REPO/blob/master/data/21million.rdf.gz?raw=true"

# these may be used for testing the test
#DATA_URL="$BENCHMARKS_REPO/blob/master/data/goldendata.rdf.gz?raw=true"
DGRAPH_LOADER=${DGRAPH_LOADER:=bulk}
DGRAPH_RELOAD=${DGRAPH_RELOAD:=yes}
DGRAPH_LOAD_ONLY=${DGRAPH_LOAD_ONLY:=}

function Info {
    echo -e "INFO: $*"
}

function DockerCompose {
    docker-compose -p dgraph -f $COMPOSE_FILE "$@"
}

if [[ $DGRAPH_LOADER != bulk && $DGRAPH_LOADER != live ]]; then
    echo >&2 "$ME: loader must be 'bulk' or 'live' -- $DGRAPH_LOADER"
    exit 1
fi

if [[ ${DGRAPH_RELOAD,,} != yes ]]; then
    unset DGRAPH_RELOAD
fi

Info "entering directory $SRCDIR"
cd $SRCDIR

if [[ $DGRAPH_RELOAD ]]; then
    Info "removing old data (if any)"
    DockerCompose down -v
else
    Info "using previously loaded data"
fi

Info "bringing up zero container"
DockerCompose up -d zero1

Info "waiting for zero to become leader"
DockerCompose logs -f zero1 | grep -q -m1 "I've become the leader"

if [[ $DGRAPH_RELOAD && $DGRAPH_LOADER == bulk ]]; then
    Info "bulk loading 21million data set"
    DockerCompose run --rm dg1 \
        bash -s <<EOF
            /gobin/dgraph bulk --schema=<(curl -LSs $SCHEMA_URL) --files=<(curl -LSs $DATA_URL) \
                               --format=rdf --zero=zero1:5080 --out=/data/dg1/bulk
            mv /data/dg1/bulk/0/p /data/dg1
EOF
fi

Info "bringing up alpha container"
DockerCompose up -d dg1

Info "waiting for alpha to be ready"
DockerCompose logs -f dg1 | grep -q -m1 "Server is ready"

if [[ $DGRAPH_RELOAD && $DGRAPH_LOADER == live ]]; then
    Info "live loading 21million data set"
    dgraph live --schema=<(curl -LSs $SCHEMA_URL) --files=<(curl -LSs $DATA_URL) \
                --format=rdf --zero=:5080 --dgraph=:9180 --logtostderr
fi

# if env var is set, exit after loading data
[[ $DGRAPH_LOAD_ONLY ]] && exit 0

Info "running benchmarks/regression queries"
go test -v -tags standalone

Info "bringing down zero and alpha containers"
if [[ $DEGRAPH_RELOAD ]]; then
    DockerCompose down -v
else
    DockerCompose down
fi

if [[ $FOUND_DIFFS -eq 0 ]]; then
    Info "no diffs found in query results"
else
    Info "found some diffs in query results"
fi

exit $FOUND_DIFFS
