#!/bin/bash -e

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph

source $DGRAPH_ROOT/contrib/scripts/functions.sh

COMPOSE_FILE=$(dirname $0)/docker-compose.yml
QUERY_DIR=$(dirname $0)/queries
BENCHMARKS_REPO="https://github.com/dgraph-io/benchmarks"
SCHEMA_URL="$BENCHMARKS_REPO/blob/javier/update_21million/data/21million.schema?raw=true"
SCHEMA_FILE=$QUERY_DIR/schema.txt
DATA_URL="$BENCHMARKS_REPO/blob/master/data/21million.rdf.gz?raw=true"

# these may be changed for testing the test
DGRAPH_LOADER=${DGRAPH_LOADER:=bulk}
DGRAPH_RELOAD=${DGRAPH_RELOAD:=yes}

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

Info "running benchmarks/regression queries"
TEMP_DIR=$(mktemp -d --tmpdir $ME.tmp-XXXXXX)
FOUND_DIFFS=0
trap "rm -rf $TEMPDIR" EXIT
for FILE in $QUERY_DIR/query-*.txt; do
#    echo >&2 -n "."
#    OUT_FILE=$TEMP_DIR/query-$IDX.txt
#    GOLDEN_FILE=$QUERY_DIR/query-$IDX.txt
#    curl -LSs "$QUERIES_BASE_URL/query-$IDX.txt?raw=true"         > $OUT_FILE
#    curl -Ss http://localhost:8180/query -d@$OUT_FILE | jq .data >> $OUT_FILE
#    if ! diff -q $GOLDEN_FILE $OUT_FILE; then
#        FOUND_DIFFS=1
#        diff $GOLDEN_FILE $OUT_FILE
#    fi
    echo $FILE
    curl -Ss http://localhost:8180/query -d@$FILE | jq -S .data >> $FILE
done
echo

#Info "bringing down zero and alpha containers"
#DockerCompose down
#
#if [[ $FOUND_DIFFS -eq 0 ]]; then
#    Info "no diffs found in query results"
#else
#    Info "found some diffs in query results"
#fi

exit $FOUND_DIFFS
