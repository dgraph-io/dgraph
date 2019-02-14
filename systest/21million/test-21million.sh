#!/bin/bash -e

readonly ME=${0##*/}

COMPOSE_FILE=$(dirname $0)/docker-compose.yml
QUERY_DIR=$(dirname $0)/queries
BENCHMARKS_REPO="https://github.com/dgraph-io/benchmarks"
SCHEMA_URL="$BENCHMARKS_REPO/blob/javier/update_21million/data/21million.schema?raw=true"
DATA_URL="$BENCHMARKS_REPO/blob/master/data/21million.rdf.gz?raw=true"

# these may be changed for testing the test
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
TEMP_DIR=$(mktemp -d --tmpdir $ME.tmp-XXXXXX)
FOUND_DIFFS=0
trap "rm -rf $TEMP_DIR" EXIT
for IN_FILE in $QUERY_DIR/*-query; do
    echo >&2 -n "."
    REF_FILE=${IN_FILE/query/result}
    OUT_FILE=$TEMP_DIR/$(basename $REF_FILE)

    # sorting the JSON destroys its structure but allows it to be diff'd
    curl -Ss http://localhost:8180/query -d@$IN_FILE | jq .data | sort >> $OUT_FILE
    if ! diff -q $REF_FILE $OUT_FILE &>/dev/null; then
        echo -e "\n$IN_FILE results differ"
        FOUND_DIFFS=1
    fi
done
echo

Info "bringing down zero and alpha containers"
DockerCompose down -v

if [[ $FOUND_DIFFS -eq 0 ]]; then
    Info "no diffs found in query results"
else
    Info "found some diffs in query results"
fi

exit $FOUND_DIFFS
