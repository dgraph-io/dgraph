#!/bin/bash -e

readonly ME=${0##*/}
readonly DGRAPH_ROOT=${GOPATH:-$HOME}/src/github.com/dgraph-io/dgraph

source $DGRAPH_ROOT/contrib/scripts/functions.sh

COMPOSE_FILE=$(dirname $0)/docker-compose.yml
GOLDEN_DIR=$(dirname $0)/goldendata
BENCHMARKS_REPO="https://github.com/dgraph-io/benchmarks"
QUERIES_BASE_URL="https://raw.githubusercontent.com/dgraph-io/benchmarks/master/regression/queries"
SCHEMA_URL="$BENCHMARKS_REPO/blob/master/data/21million.schema?raw=true"
DATA_URL="$BENCHMARKS_REPO/blob/master/data/21million.rdf.gz?raw=true"

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
DockerCompose run --rm dg1 \
    bash -s <<EOF
        /gobin/dgraph bulk --schema=<(curl -LSs $SCHEMA_URL) --files=<(curl -LSs $DATA_URL) \
                       --format=rdf --zero=zero1:5080 --out=/data/dg1/bulk
        mv /data/dg1/bulk/0/p /data/dg1
EOF

Info "bringing up alpha container"
DockerCompose up -d dg1

Info "running benchmarks/regression queries"
TEMP_DIR=$(mktemp -d --tmpdir $ME.tmp-XXXXXX)
FOUND_DIFFS=0
trap "rm -rf $TEMPDIR" EXIT
for IDX in 01 03 {05..70}; do
    echo >&2 -n "."
    OUT_FILE=$TEMP_DIR/query-$IDX.txt
    GOLDEN_FILE=$GOLDEN_DIR/query-$IDX.txt
    curl -LSs "$QUERIES_BASE_URL/query-$IDX.txt?raw=true"         > $OUT_FILE
    curl -Ss http://localhost:8180/query -d@$OUT_FILE | jq .data >> $OUT_FILE
    if ! diff -q $GOLDEN_FILE $OUT_FILE; then
        FOUND_DIFFS=1
        diff $GOLDEN_FILE $OUT_FILE
    fi
done
echo

if [[ $FOUND_DIFFS -eq 0 ]]; then
    Info "no diffs found in query results"
else
    Info "found some diffs in query results"
fi

exit $FOUND_DIFFS
